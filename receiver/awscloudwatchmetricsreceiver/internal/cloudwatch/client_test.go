// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package cloudwatch

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwsdk "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockClient struct {
	getMetricDataPages []*cwsdk.GetMetricDataOutput
	getMetricDataErr   error
	listMetricsPages   []*cwsdk.ListMetricsOutput
	listMetricsErr     error
	callCount          int
	listCallCount      int
}

func (m *mockClient) GetMetricData(_ context.Context, _ *cwsdk.GetMetricDataInput, _ ...func(*cwsdk.Options)) (*cwsdk.GetMetricDataOutput, error) {
	if m.getMetricDataErr != nil {
		return nil, m.getMetricDataErr
	}
	if m.callCount >= len(m.getMetricDataPages) {
		return &cwsdk.GetMetricDataOutput{}, nil
	}
	page := m.getMetricDataPages[m.callCount]
	m.callCount++
	return page, nil
}

func (m *mockClient) ListMetrics(_ context.Context, _ *cwsdk.ListMetricsInput, _ ...func(*cwsdk.Options)) (*cwsdk.ListMetricsOutput, error) {
	if m.listMetricsErr != nil {
		return nil, m.listMetricsErr
	}
	if m.listCallCount >= len(m.listMetricsPages) {
		return &cwsdk.ListMetricsOutput{}, nil
	}
	page := m.listMetricsPages[m.listCallCount]
	m.listCallCount++
	return page, nil
}

func TestGetMetricDataPaginated_SinglePage(t *testing.T) {
	client := &mockClient{
		getMetricDataPages: []*cwsdk.GetMetricDataOutput{
			{
				MetricDataResults: []cwtypes.MetricDataResult{
					{Id: aws.String("q1"), Values: []float64{1.0}},
				},
			},
		},
	}

	input := &cwsdk.GetMetricDataInput{}
	output, err := GetMetricDataPaginated(context.Background(), client, input)
	require.NoError(t, err)
	assert.Len(t, output.MetricDataResults, 1)
}

func TestGetMetricDataPaginated_MultiplePages(t *testing.T) {
	client := &mockClient{
		getMetricDataPages: []*cwsdk.GetMetricDataOutput{
			{
				MetricDataResults: []cwtypes.MetricDataResult{
					{Id: aws.String("q1"), Values: []float64{1.0}},
				},
				NextToken: aws.String("token1"),
			},
			{
				MetricDataResults: []cwtypes.MetricDataResult{
					{Id: aws.String("q2"), Values: []float64{2.0}},
				},
			},
		},
	}

	input := &cwsdk.GetMetricDataInput{}
	output, err := GetMetricDataPaginated(context.Background(), client, input)
	require.NoError(t, err)
	assert.Len(t, output.MetricDataResults, 2)
	assert.Equal(t, 2, client.callCount)
}

func TestGetMetricDataPaginated_Error(t *testing.T) {
	client := &mockClient{
		getMetricDataErr: fmt.Errorf("throttled"),
	}

	input := &cwsdk.GetMetricDataInput{}
	_, err := GetMetricDataPaginated(context.Background(), client, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "throttled")
}

func TestGetMetricDataPaginated_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &mockClient{
		getMetricDataPages: []*cwsdk.GetMetricDataOutput{
			{MetricDataResults: []cwtypes.MetricDataResult{}},
		},
	}

	input := &cwsdk.GetMetricDataInput{}
	_, err := GetMetricDataPaginated(ctx, client, input)
	require.Error(t, err)
}

func TestListMetricsPaginated_SinglePage(t *testing.T) {
	client := &mockClient{
		listMetricsPages: []*cwsdk.ListMetricsOutput{
			{
				Metrics: []cwtypes.Metric{
					{Namespace: aws.String("AWS/EC2"), MetricName: aws.String("CPUUtilization")},
				},
			},
		},
	}

	input := &cwsdk.ListMetricsInput{}
	metrics, err := ListMetricsPaginated(context.Background(), client, input)
	require.NoError(t, err)
	assert.Len(t, metrics, 1)
}

func TestListMetricsPaginated_MultiplePages(t *testing.T) {
	client := &mockClient{
		listMetricsPages: []*cwsdk.ListMetricsOutput{
			{
				Metrics:   []cwtypes.Metric{{Namespace: aws.String("AWS/EC2")}},
				NextToken: aws.String("next"),
			},
			{
				Metrics: []cwtypes.Metric{{Namespace: aws.String("AWS/ECS")}},
			},
		},
	}

	input := &cwsdk.ListMetricsInput{}
	metrics, err := ListMetricsPaginated(context.Background(), client, input)
	require.NoError(t, err)
	assert.Len(t, metrics, 2)
}

func TestListMetricsPaginated_Error(t *testing.T) {
	client := &mockClient{
		listMetricsErr: fmt.Errorf("access denied"),
	}

	input := &cwsdk.ListMetricsInput{}
	_, err := ListMetricsPaginated(context.Background(), client, input)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
}
