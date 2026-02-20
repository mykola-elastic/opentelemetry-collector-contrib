// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package cloudwatch // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver/internal/cloudwatch"

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// MetricsClient abstracts the CloudWatch API calls needed by the receiver.
// Using an interface allows for easy mocking in tests.
type MetricsClient interface {
	GetMetricData(ctx context.Context, input *cloudwatch.GetMetricDataInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricDataOutput, error)
	ListMetrics(ctx context.Context, input *cloudwatch.ListMetricsInput, opts ...func(*cloudwatch.Options)) (*cloudwatch.ListMetricsOutput, error)
}

// ClientConfig holds the parameters needed to construct a CloudWatch client.
type ClientConfig struct {
	Region       string
	Profile      string
	IMDSEndpoint string
}

// NewClient creates a new CloudWatch SDK client from the provided config.
func NewClient(ctx context.Context, cfg ClientConfig) (MetricsClient, error) {
	optFns := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}

	if cfg.Profile != "" {
		optFns = append(optFns, awsconfig.WithSharedConfigProfile(cfg.Profile))
	}

	if cfg.IMDSEndpoint != "" {
		optFns = append(optFns, awsconfig.WithEC2IMDSEndpoint(cfg.IMDSEndpoint))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return cloudwatch.NewFromConfig(awsCfg), nil
}

// GetMetricDataPaginated calls GetMetricData handling pagination and collecting
// all result pages into a single slice. It respects context cancellation between pages.
func GetMetricDataPaginated(ctx context.Context, client MetricsClient, input *cloudwatch.GetMetricDataInput) (*cloudwatch.GetMetricDataOutput, error) {
	var allResults []cwtypes.MetricDataResult
	var allMessages []cwtypes.MessageData

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := client.GetMetricData(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("GetMetricData API call failed: %w", err)
		}

		allResults = append(allResults, resp.MetricDataResults...)
		allMessages = append(allMessages, resp.Messages...)

		if resp.NextToken == nil || *resp.NextToken == "" {
			break
		}
		input.NextToken = resp.NextToken
	}

	return &cloudwatch.GetMetricDataOutput{
		MetricDataResults: allResults,
		Messages:          allMessages,
	}, nil
}

// ListMetricsPaginated calls ListMetrics handling pagination. Collects all metrics
// matching the input filter. Respects context cancellation.
func ListMetricsPaginated(ctx context.Context, client MetricsClient, input *cloudwatch.ListMetricsInput) ([]cwtypes.Metric, error) {
	var allMetrics []cwtypes.Metric

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		resp, err := client.ListMetrics(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("ListMetrics API call failed: %w", err)
		}

		allMetrics = append(allMetrics, resp.Metrics...)

		if resp.NextToken == nil || *resp.NextToken == "" {
			break
		}
		input.NextToken = resp.NextToken
	}

	return allMetrics, nil
}

// MetricKey identifies a unique combination of namespace+metric+dimensions
// for deduplication purposes.
type MetricKey struct {
	Namespace  string
	MetricName string
	// Sorted comma-separated dimension key=value pairs
	DimensionKey string
}

// BuildMetricKey creates a MetricKey from a CloudWatch Metric.
func BuildMetricKey(m cwtypes.Metric) MetricKey {
	key := MetricKey{
		Namespace:  aws.ToString(m.Namespace),
		MetricName: aws.ToString(m.MetricName),
	}
	dimParts := ""
	for i, d := range m.Dimensions {
		if i > 0 {
			dimParts += ","
		}
		dimParts += aws.ToString(d.Name) + "=" + aws.ToString(d.Value)
	}
	key.DimensionKey = dimParts
	return key
}
