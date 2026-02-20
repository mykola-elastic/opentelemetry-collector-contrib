// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package awscloudwatchmetricsreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver"

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/scraper"
	"go.uber.org/zap"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver/internal/cloudwatch"
)

type cloudWatchScraper struct {
	logger *zap.Logger
	cfg    *Config
	client cloudwatch.MetricsClient

	// metricDefs maps "Namespace/MetricName" → resolved MetricDefinition
	metricDefs map[string]*cloudwatch.MetricDefinition
}

func newCloudWatchScraper(set receiver.Settings, cfg *Config) (scraper.Metrics, error) {
	s := newCloudWatchScraperStruct(set, cfg)

	return scraper.NewMetrics(
		s.scrape,
		scraper.WithStart(s.start),
		scraper.WithShutdown(s.shutdown),
	)
}

func newCloudWatchScraperStruct(set receiver.Settings, cfg *Config) *cloudWatchScraper {
	defs := make(map[string]*cloudwatch.MetricDefinition, len(cfg.Metrics))
	for _, m := range cfg.Metrics {
		resolved := m.withDefaults()
		key := resolved.Namespace + "/" + resolved.MetricName
		defs[key] = &cloudwatch.MetricDefinition{
			Namespace:  resolved.Namespace,
			MetricName: resolved.MetricName,
			Period:     resolved.Period,
			Statistics: resolved.Statistics,
			Dimensions: resolved.Dimensions,
		}
	}

	return &cloudWatchScraper{
		logger:     set.Logger,
		cfg:        cfg,
		metricDefs: defs,
	}
}

func (s *cloudWatchScraper) start(ctx context.Context, _ component.Host) error {
	client, err := cloudwatch.NewClient(ctx, cloudwatch.ClientConfig{
		Region:       s.cfg.Region,
		Profile:      s.cfg.Profile,
		IMDSEndpoint: s.cfg.IMDSEndpoint,
	})
	if err != nil {
		return fmt.Errorf("failed to create CloudWatch client: %w", err)
	}
	s.client = client
	return nil
}

func (s *cloudWatchScraper) shutdown(_ context.Context) error {
	s.client = nil
	return nil
}

func (s *cloudWatchScraper) scrape(ctx context.Context) (pmetric.Metrics, error) {
	if s.client == nil {
		return pmetric.Metrics{}, fmt.Errorf("cloudwatch client not initialized")
	}

	endTime := time.Now()

	var allDiscoveredMetrics []cwtypes.Metric

	for _, def := range s.metricDefs {
		input := cloudwatch.BuildListMetricsInput(def.Namespace, def.MetricName, def.Dimensions)
		discovered, err := cloudwatch.ListMetricsPaginated(ctx, s.client, input)
		if err != nil {
			s.logger.Error("failed to list metrics",
				zap.String("namespace", def.Namespace),
				zap.String("metric_name", def.MetricName),
				zap.Error(err))
			continue
		}
		s.logger.Debug("discovered metrics",
			zap.String("namespace", def.Namespace),
			zap.String("metric_name", def.MetricName),
			zap.Int("count", len(discovered)))

		allDiscoveredMetrics = append(allDiscoveredMetrics, discovered...)
	}

	if len(allDiscoveredMetrics) == 0 {
		s.logger.Debug("no CloudWatch metrics discovered")
		return pmetric.NewMetrics(), nil
	}

	// The lookback window is the maximum period across all definitions,
	// ensuring we capture at least one complete data point.
	maxPeriod := time.Duration(0)
	for _, def := range s.metricDefs {
		if def.Period > maxPeriod {
			maxPeriod = def.Period
		}
	}
	startTime := endTime.Add(-2 * maxPeriod)

	getInput := cloudwatch.BuildGetMetricDataInput(allDiscoveredMetrics, s.metricDefs, startTime, endTime)

	batches := cloudwatch.BatchQueries(getInput.MetricDataQueries)
	var allResults []cwtypes.MetricDataResult

	for _, batch := range batches {
		batchInput := &cwsdk.GetMetricDataInput{
			StartTime:         getInput.StartTime,
			EndTime:           getInput.EndTime,
			MetricDataQueries: batch,
		}

		output, err := cloudwatch.GetMetricDataPaginated(ctx, s.client, batchInput)
		if err != nil {
			s.logger.Error("GetMetricData failed", zap.Error(err))
			continue
		}

		for _, msg := range output.Messages {
			if msg.Value != nil {
				s.logger.Warn("CloudWatch API message", zap.String("message", *msg.Value))
			}
		}

		allResults = append(allResults, output.MetricDataResults...)
	}

	return s.convertToMetrics(allResults, allDiscoveredMetrics, getInput.MetricDataQueries), nil
}

// convertToMetrics transforms CloudWatch MetricDataResults into pmetric.Metrics.
// Each CloudWatch metric becomes a resource, each statistic becomes a gauge data point.
func (s *cloudWatchScraper) convertToMetrics(
	results []cwtypes.MetricDataResult,
	discoveredMetrics []cwtypes.Metric,
	queries []cwtypes.MetricDataQuery,
) pmetric.Metrics {
	md := pmetric.NewMetrics()

	queryMap := make(map[string]cwtypes.MetricDataQuery, len(queries))
	for _, q := range queries {
		queryMap[aws.ToString(q.Id)] = q
	}

	for _, result := range results {
		if result.StatusCode != cwtypes.StatusCodeComplete {
			s.logger.Warn("metric data result not complete",
				zap.String("id", aws.ToString(result.Id)),
				zap.String("status", string(result.StatusCode)))
			continue
		}

		if len(result.Values) == 0 {
			continue
		}

		query, ok := queryMap[aws.ToString(result.Id)]
		if !ok || query.MetricStat == nil {
			continue
		}

		stat := query.MetricStat
		namespace := aws.ToString(stat.Metric.Namespace)
		metricName := aws.ToString(stat.Metric.MetricName)
		statName := aws.ToString(stat.Stat)

		rm := md.ResourceMetrics().AppendEmpty()
		res := rm.Resource()
		res.Attributes().PutStr("cloud.provider", "aws")
		res.Attributes().PutStr("aws.cloudwatch.namespace", namespace)

		for _, dim := range stat.Metric.Dimensions {
			res.Attributes().PutStr(
				"aws.cloudwatch.dimension."+aws.ToString(dim.Name),
				aws.ToString(dim.Value),
			)
		}

		sm := rm.ScopeMetrics().AppendEmpty()
		sm.Scope().SetName("github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver")

		otelMetricName := buildOTelMetricName(namespace, metricName, statName)

		m := sm.Metrics().AppendEmpty()
		m.SetName(otelMetricName)
		m.SetUnit(guessUnit(metricName))

		gauge := m.SetEmptyGauge()

		for i, val := range result.Values {
			dp := gauge.DataPoints().AppendEmpty()
			dp.SetDoubleValue(val)
			if i < len(result.Timestamps) {
				dp.SetTimestamp(pcommon.NewTimestampFromTime(result.Timestamps[i]))
			}
		}
	}

	return md
}

// buildOTelMetricName creates a normalized OpenTelemetry metric name from
// CloudWatch namespace, metric name, and statistic.
func buildOTelMetricName(namespace, metricName, stat string) string {
	ns := strings.ToLower(strings.ReplaceAll(namespace, "/", "."))
	mn := strings.ToLower(metricName)
	st := strings.ToLower(stat)
	return fmt.Sprintf("aws.cloudwatch.%s.%s.%s", ns, mn, st)
}

// guessUnit provides a best-effort unit mapping for common CloudWatch metrics.
func guessUnit(metricName string) string {
	lower := strings.ToLower(metricName)
	switch {
	case strings.Contains(lower, "bytes"):
		return "By"
	case strings.Contains(lower, "percent"), strings.Contains(lower, "utilization"):
		return "%"
	case strings.Contains(lower, "count"):
		return "{count}"
	case strings.Contains(lower, "seconds"), strings.Contains(lower, "latency"), strings.Contains(lower, "duration"):
		return "s"
	default:
		return "1"
	}
}
