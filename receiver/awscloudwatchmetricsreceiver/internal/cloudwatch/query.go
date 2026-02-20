// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package cloudwatch // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver/internal/cloudwatch"

import (
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
)

// MetricDefinition describes a CloudWatch metric to query.
type MetricDefinition struct {
	Namespace  string
	MetricName string
	Period     time.Duration
	Statistics []string
	Dimensions []string
}

// BuildGetMetricDataInput creates a GetMetricDataInput from the discovered metrics
// and their definitions. Each unique metric+stat combination gets a distinct query ID.
//
// The GetMetricData API accepts up to 500 MetricDataQueries per call. Callers
// should split into batches if the total exceeds this limit.
func BuildGetMetricDataInput(
	metrics []cwtypes.Metric,
	defs map[string]*MetricDefinition,
	startTime, endTime time.Time,
) *cloudwatch.GetMetricDataInput {
	var queries []cwtypes.MetricDataQuery

	for i, m := range metrics {
		ns := aws.ToString(m.Namespace)
		mn := aws.ToString(m.MetricName)

		defKey := ns + "/" + mn
		def, ok := defs[defKey]
		if !ok {
			continue
		}

		period := int32(def.Period.Seconds())

		for _, stat := range def.Statistics {
			queryID := fmt.Sprintf("q_%d_%s", i, strings.ToLower(stat))

			queries = append(queries, cwtypes.MetricDataQuery{
				Id: aws.String(queryID),
				MetricStat: &cwtypes.MetricStat{
					Metric: &cwtypes.Metric{
						Namespace:  m.Namespace,
						MetricName: m.MetricName,
						Dimensions: m.Dimensions,
					},
					Period: aws.Int32(period),
					Stat:   aws.String(stat),
				},
			})
		}
	}

	return &cloudwatch.GetMetricDataInput{
		StartTime:         aws.Time(startTime),
		EndTime:           aws.Time(endTime),
		MetricDataQueries: queries,
	}
}

// BuildListMetricsInput creates a ListMetricsInput for a given namespace and
// optional metric name. When dimension names are specified, they are used as
// DimensionFilters (name-only, no value filter) to restrict results.
func BuildListMetricsInput(namespace, metricName string, dimensionNames []string) *cloudwatch.ListMetricsInput {
	input := &cloudwatch.ListMetricsInput{
		Namespace: aws.String(namespace),
	}

	if metricName != "" {
		input.MetricName = aws.String(metricName)
	}

	for _, dn := range dimensionNames {
		input.Dimensions = append(input.Dimensions, cwtypes.DimensionFilter{
			Name: aws.String(dn),
		})
	}

	return input
}

// MaxMetricDataQueries is the AWS API limit per GetMetricData call.
const MaxMetricDataQueries = 500

// BatchQueries splits a list of MetricDataQueries into batches of at most
// MaxMetricDataQueries. This ensures compliance with AWS API limits.
func BatchQueries(queries []cwtypes.MetricDataQuery) [][]cwtypes.MetricDataQuery {
	var batches [][]cwtypes.MetricDataQuery
	for i := 0; i < len(queries); i += MaxMetricDataQueries {
		end := i + MaxMetricDataQueries
		if end > len(queries) {
			end = len(queries)
		}
		batches = append(batches, queries[i:end])
	}
	return batches
}
