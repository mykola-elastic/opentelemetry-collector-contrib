// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package cloudwatch

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildGetMetricDataInput(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)

	metrics := []cwtypes.Metric{
		{
			Namespace:  aws.String("AWS/EC2"),
			MetricName: aws.String("CPUUtilization"),
			Dimensions: []cwtypes.Dimension{
				{Name: aws.String("InstanceId"), Value: aws.String("i-123")},
			},
		},
	}

	defs := map[string]*MetricDefinition{
		"AWS/EC2/CPUUtilization": {
			Namespace:  "AWS/EC2",
			MetricName: "CPUUtilization",
			Period:     60 * time.Second,
			Statistics: []string{"Average", "Maximum"},
		},
	}

	input := BuildGetMetricDataInput(metrics, defs, startTime, endTime)

	require.NotNil(t, input)
	assert.Equal(t, startTime, *input.StartTime)
	assert.Equal(t, endTime, *input.EndTime)
	assert.Len(t, input.MetricDataQueries, 2)

	q1 := input.MetricDataQueries[0]
	assert.Equal(t, "q_0_average", *q1.Id)
	assert.Equal(t, "Average", *q1.MetricStat.Stat)
	assert.Equal(t, int32(60), *q1.MetricStat.Period)

	q2 := input.MetricDataQueries[1]
	assert.Equal(t, "q_0_maximum", *q2.Id)
	assert.Equal(t, "Maximum", *q2.MetricStat.Stat)
}

func TestBuildGetMetricDataInput_NoMatchingDefs(t *testing.T) {
	metrics := []cwtypes.Metric{
		{Namespace: aws.String("AWS/S3"), MetricName: aws.String("BucketSize")},
	}

	defs := map[string]*MetricDefinition{
		"AWS/EC2/CPUUtilization": {
			Namespace:  "AWS/EC2",
			MetricName: "CPUUtilization",
			Period:     60 * time.Second,
			Statistics: []string{"Average"},
		},
	}

	input := BuildGetMetricDataInput(metrics, defs, time.Now(), time.Now())
	assert.Empty(t, input.MetricDataQueries)
}

func TestBuildListMetricsInput(t *testing.T) {
	t.Run("with dimensions", func(t *testing.T) {
		input := BuildListMetricsInput("AWS/EC2", "CPUUtilization", []string{"InstanceId"})
		assert.Equal(t, "AWS/EC2", *input.Namespace)
		assert.Equal(t, "CPUUtilization", *input.MetricName)
		require.Len(t, input.Dimensions, 1)
		assert.Equal(t, "InstanceId", *input.Dimensions[0].Name)
	})

	t.Run("without dimensions", func(t *testing.T) {
		input := BuildListMetricsInput("AWS/EC2", "CPUUtilization", nil)
		assert.Equal(t, "AWS/EC2", *input.Namespace)
		assert.Equal(t, "CPUUtilization", *input.MetricName)
		assert.Empty(t, input.Dimensions)
	})

	t.Run("without metric name", func(t *testing.T) {
		input := BuildListMetricsInput("AWS/EC2", "", nil)
		assert.Equal(t, "AWS/EC2", *input.Namespace)
		assert.Nil(t, input.MetricName)
	})
}

func TestBatchQueries(t *testing.T) {
	t.Run("under limit", func(t *testing.T) {
		queries := make([]cwtypes.MetricDataQuery, 10)
		batches := BatchQueries(queries)
		assert.Len(t, batches, 1)
		assert.Len(t, batches[0], 10)
	})

	t.Run("exact limit", func(t *testing.T) {
		queries := make([]cwtypes.MetricDataQuery, MaxMetricDataQueries)
		batches := BatchQueries(queries)
		assert.Len(t, batches, 1)
	})

	t.Run("over limit", func(t *testing.T) {
		queries := make([]cwtypes.MetricDataQuery, MaxMetricDataQueries+1)
		batches := BatchQueries(queries)
		assert.Len(t, batches, 2)
		assert.Len(t, batches[0], MaxMetricDataQueries)
		assert.Len(t, batches[1], 1)
	})

	t.Run("empty", func(t *testing.T) {
		batches := BatchQueries(nil)
		assert.Empty(t, batches)
	})
}

func TestBuildMetricKey(t *testing.T) {
	m := cwtypes.Metric{
		Namespace:  aws.String("AWS/EC2"),
		MetricName: aws.String("CPUUtilization"),
		Dimensions: []cwtypes.Dimension{
			{Name: aws.String("InstanceId"), Value: aws.String("i-123")},
			{Name: aws.String("AutoScalingGroupName"), Value: aws.String("asg-1")},
		},
	}

	key := BuildMetricKey(m)
	assert.Equal(t, "AWS/EC2", key.Namespace)
	assert.Equal(t, "CPUUtilization", key.MetricName)
	assert.Equal(t, "InstanceId=i-123,AutoScalingGroupName=asg-1", key.DimensionKey)
}
