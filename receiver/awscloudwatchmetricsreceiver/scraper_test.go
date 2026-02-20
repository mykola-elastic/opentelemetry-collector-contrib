// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package awscloudwatchmetricsreceiver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver/internal/cloudwatch"
	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver/internal/metadata"
)

type mockCloudWatchClient struct {
	getMetricDataFunc func(ctx context.Context, input *cwsdk.GetMetricDataInput, opts ...func(*cwsdk.Options)) (*cwsdk.GetMetricDataOutput, error)
	listMetricsFunc   func(ctx context.Context, input *cwsdk.ListMetricsInput, opts ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error)
}

func (m *mockCloudWatchClient) GetMetricData(ctx context.Context, input *cwsdk.GetMetricDataInput, opts ...func(*cwsdk.Options)) (*cwsdk.GetMetricDataOutput, error) {
	if m.getMetricDataFunc != nil {
		return m.getMetricDataFunc(ctx, input, opts...)
	}
	return &cwsdk.GetMetricDataOutput{}, nil
}

func (m *mockCloudWatchClient) ListMetrics(ctx context.Context, input *cwsdk.ListMetricsInput, opts ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
	if m.listMetricsFunc != nil {
		return m.listMetricsFunc(ctx, input, opts...)
	}
	return &cwsdk.ListMetricsOutput{}, nil
}

var _ cloudwatch.MetricsClient = (*mockCloudWatchClient)(nil)

func TestScraper_Scrape(t *testing.T) {
	now := time.Now()

	mock := &mockCloudWatchClient{
		listMetricsFunc: func(_ context.Context, input *cwsdk.ListMetricsInput, _ ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
			return &cwsdk.ListMetricsOutput{
				Metrics: []cwtypes.Metric{
					{
						Namespace:  input.Namespace,
						MetricName: input.MetricName,
						Dimensions: []cwtypes.Dimension{
							{Name: aws.String("InstanceId"), Value: aws.String("i-1234567890abcdef0")},
						},
					},
				},
			}, nil
		},
		getMetricDataFunc: func(_ context.Context, _ *cwsdk.GetMetricDataInput, _ ...func(*cwsdk.Options)) (*cwsdk.GetMetricDataOutput, error) {
			return &cwsdk.GetMetricDataOutput{
				MetricDataResults: []cwtypes.MetricDataResult{
					{
						Id:         aws.String("q_0_average"),
						StatusCode: cwtypes.StatusCodeComplete,
						Values:     []float64{42.5},
						Timestamps: []time.Time{now},
					},
				},
			}, nil
		},
	}

	cfg := &Config{
		Region: "us-east-1",
		Metrics: []MetricConfig{
			{
				Namespace:  "AWS/EC2",
				MetricName: "CPUUtilization",
				Period:     60 * time.Second,
				Statistics: []string{"Average"},
				Dimensions: []string{"InstanceId"},
			},
		},
	}

	set := receivertest.NewNopSettings(metadata.Type)
	s := newCloudWatchScraperStruct(set, cfg)
	s.client = mock

	md, err := s.scrape(context.Background())
	require.NoError(t, err)

	require.Equal(t, 1, md.ResourceMetrics().Len())
	rm := md.ResourceMetrics().At(0)

	attrs := rm.Resource().Attributes()
	v, ok := attrs.Get("cloud.provider")
	assert.True(t, ok)
	assert.Equal(t, "aws", v.Str())

	v, ok = attrs.Get("aws.cloudwatch.namespace")
	assert.True(t, ok)
	assert.Equal(t, "AWS/EC2", v.Str())

	v, ok = attrs.Get("aws.cloudwatch.dimension.InstanceId")
	assert.True(t, ok)
	assert.Equal(t, "i-1234567890abcdef0", v.Str())

	require.Equal(t, 1, rm.ScopeMetrics().Len())
	sm := rm.ScopeMetrics().At(0)
	require.Equal(t, 1, sm.Metrics().Len())

	m := sm.Metrics().At(0)
	assert.Equal(t, "aws.cloudwatch.aws.ec2.cpuutilization.average", m.Name())
	assert.Equal(t, "%", m.Unit())
	require.Equal(t, 1, m.Gauge().DataPoints().Len())
	assert.InDelta(t, 42.5, m.Gauge().DataPoints().At(0).DoubleValue(), 0.01)
}

func TestScraper_Scrape_MultipleMetrics(t *testing.T) {
	now := time.Now()

	mock := &mockCloudWatchClient{
		listMetricsFunc: func(_ context.Context, input *cwsdk.ListMetricsInput, _ ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
			return &cwsdk.ListMetricsOutput{
				Metrics: []cwtypes.Metric{
					{Namespace: input.Namespace, MetricName: input.MetricName},
				},
			}, nil
		},
		getMetricDataFunc: func(_ context.Context, input *cwsdk.GetMetricDataInput, _ ...func(*cwsdk.Options)) (*cwsdk.GetMetricDataOutput, error) {
			var results []cwtypes.MetricDataResult
			for _, q := range input.MetricDataQueries {
				results = append(results, cwtypes.MetricDataResult{
					Id:         q.Id,
					StatusCode: cwtypes.StatusCodeComplete,
					Values:     []float64{10.0},
					Timestamps: []time.Time{now},
				})
			}
			return &cwsdk.GetMetricDataOutput{MetricDataResults: results}, nil
		},
	}

	cfg := &Config{
		Region: "us-east-1",
		Metrics: []MetricConfig{
			{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
			{Namespace: "AWS/ECS", MetricName: "CPUUtilization"},
		},
	}

	set := receivertest.NewNopSettings(metadata.Type)
	s := newCloudWatchScraperStruct(set, cfg)
	s.client = mock

	md, err := s.scrape(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, md.ResourceMetrics().Len())
}

func TestScraper_Scrape_NoMetricsDiscovered(t *testing.T) {
	mock := &mockCloudWatchClient{
		listMetricsFunc: func(_ context.Context, _ *cwsdk.ListMetricsInput, _ ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
			return &cwsdk.ListMetricsOutput{Metrics: []cwtypes.Metric{}}, nil
		},
	}

	cfg := &Config{
		Region: "us-east-1",
		Metrics: []MetricConfig{
			{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
		},
	}

	set := receivertest.NewNopSettings(metadata.Type)
	s := newCloudWatchScraperStruct(set, cfg)
	s.client = mock

	md, err := s.scrape(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, md.ResourceMetrics().Len())
}

func TestScraper_Scrape_IncompleteStatus(t *testing.T) {
	mock := &mockCloudWatchClient{
		listMetricsFunc: func(_ context.Context, input *cwsdk.ListMetricsInput, _ ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
			return &cwsdk.ListMetricsOutput{
				Metrics: []cwtypes.Metric{
					{Namespace: input.Namespace, MetricName: input.MetricName},
				},
			}, nil
		},
		getMetricDataFunc: func(_ context.Context, _ *cwsdk.GetMetricDataInput, _ ...func(*cwsdk.Options)) (*cwsdk.GetMetricDataOutput, error) {
			return &cwsdk.GetMetricDataOutput{
				MetricDataResults: []cwtypes.MetricDataResult{
					{
						Id:         aws.String("q_0_average"),
						StatusCode: cwtypes.StatusCodeInternalError,
						Values:     []float64{42.5},
					},
				},
			}, nil
		},
	}

	cfg := &Config{
		Region: "us-east-1",
		Metrics: []MetricConfig{
			{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
		},
	}

	set := receivertest.NewNopSettings(metadata.Type)
	s := newCloudWatchScraperStruct(set, cfg)
	s.client = mock

	md, err := s.scrape(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, md.ResourceMetrics().Len())
}

func TestScraper_ScrapeWithoutClient(t *testing.T) {
	cfg := &Config{
		Region: "us-east-1",
		Metrics: []MetricConfig{
			{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
		},
	}

	set := receivertest.NewNopSettings(metadata.Type)
	s := newCloudWatchScraperStruct(set, cfg)

	_, err := s.scrape(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client not initialized")
}

func TestScraper_Scrape_ListMetricsError(t *testing.T) {
	mock := &mockCloudWatchClient{
		listMetricsFunc: func(_ context.Context, _ *cwsdk.ListMetricsInput, _ ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
			return nil, fmt.Errorf("access denied")
		},
	}

	cfg := &Config{
		Region: "us-east-1",
		Metrics: []MetricConfig{
			{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
		},
	}

	set := receivertest.NewNopSettings(metadata.Type)
	s := newCloudWatchScraperStruct(set, cfg)
	s.client = mock

	md, err := s.scrape(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, md.ResourceMetrics().Len())
}

func TestScraper_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mock := &mockCloudWatchClient{
		listMetricsFunc: func(ctx context.Context, _ *cwsdk.ListMetricsInput, _ ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
			return nil, ctx.Err()
		},
	}

	cfg := &Config{
		Region: "us-east-1",
		Metrics: []MetricConfig{
			{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
		},
	}

	set := receivertest.NewNopSettings(metadata.Type)
	s := newCloudWatchScraperStruct(set, cfg)
	s.client = mock

	md, err := s.scrape(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, md.ResourceMetrics().Len())
}

func TestBuildOTelMetricName(t *testing.T) {
	tests := []struct {
		namespace  string
		metricName string
		stat       string
		expected   string
	}{
		{"AWS/EC2", "CPUUtilization", "Average", "aws.cloudwatch.aws.ec2.cpuutilization.average"},
		{"AWS/ECS", "MemoryUtilization", "Maximum", "aws.cloudwatch.aws.ecs.memoryutilization.maximum"},
		{"Custom/MyApp", "RequestCount", "Sum", "aws.cloudwatch.custom.myapp.requestcount.sum"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, buildOTelMetricName(tt.namespace, tt.metricName, tt.stat))
		})
	}
}

func TestGuessUnit(t *testing.T) {
	tests := []struct {
		metricName string
		expected   string
	}{
		{"NetworkBytesIn", "By"},
		{"CPUUtilization", "%"},
		{"RequestCount", "{count}"},
		{"Latency", "s"},
		{"StatusCheckFailed", "1"},
	}
	for _, tt := range tests {
		t.Run(tt.metricName, func(t *testing.T) {
			assert.Equal(t, tt.expected, guessUnit(tt.metricName))
		})
	}
}
