// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package awscloudwatchmetricsreceiver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/scraper/scraperhelper"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name: "valid minimal config",
			cfg: Config{
				ControllerConfig: scraperhelper.NewDefaultControllerConfig(),
				Region:           "us-east-1",
				Metrics: []MetricConfig{
					{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
				},
			},
		},
		{
			name: "valid full config",
			cfg: Config{
				ControllerConfig: scraperhelper.NewDefaultControllerConfig(),
				Region:           "eu-west-1",
				Profile:          "prod",
				Metrics: []MetricConfig{
					{
						Namespace:  "AWS/EC2",
						MetricName: "CPUUtilization",
						Period:     120 * time.Second,
						Statistics: []string{"Average", "Maximum"},
						Dimensions: []string{"InstanceId"},
					},
					{
						Namespace:  "AWS/ECS",
						MetricName: "CPUUtilization",
						Statistics: []string{"Sum"},
					},
				},
			},
		},
		{
			name: "missing region",
			cfg: Config{
				Metrics: []MetricConfig{
					{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
				},
			},
			wantErr: "no region was specified",
		},
		{
			name: "no metrics",
			cfg: Config{
				Region:  "us-east-1",
				Metrics: []MetricConfig{},
			},
			wantErr: "at least one metric must be configured",
		},
		{
			name: "missing namespace",
			cfg: Config{
				Region: "us-east-1",
				Metrics: []MetricConfig{
					{MetricName: "CPUUtilization"},
				},
			},
			wantErr: "metric namespace is required",
		},
		{
			name: "missing metric_name",
			cfg: Config{
				Region: "us-east-1",
				Metrics: []MetricConfig{
					{Namespace: "AWS/EC2"},
				},
			},
			wantErr: "metric_name is required",
		},
		{
			name: "invalid period - not multiple of 60",
			cfg: Config{
				Region: "us-east-1",
				Metrics: []MetricConfig{
					{
						Namespace:  "AWS/EC2",
						MetricName: "CPUUtilization",
						Period:     45 * time.Second,
					},
				},
			},
			wantErr: "period must be a positive duration and a multiple of 60s",
		},
		{
			name: "invalid statistic",
			cfg: Config{
				Region: "us-east-1",
				Metrics: []MetricConfig{
					{
						Namespace:  "AWS/EC2",
						MetricName: "CPUUtilization",
						Statistics: []string{"p99"},
					},
				},
			},
			wantErr: "invalid statistic",
		},
		{
			name: "invalid IMDS endpoint",
			cfg: Config{
				Region:       "us-east-1",
				IMDSEndpoint: "not-a-url",
				Metrics: []MetricConfig{
					{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
				},
			},
			wantErr: "unable to parse URI for imds_endpoint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestMetricConfig_WithDefaults(t *testing.T) {
	t.Run("applies default period and statistics", func(t *testing.T) {
		mc := MetricConfig{
			Namespace:  "AWS/EC2",
			MetricName: "CPUUtilization",
		}
		resolved := mc.withDefaults()
		assert.Equal(t, 60*time.Second, resolved.Period)
		assert.Equal(t, []string{"Average"}, resolved.Statistics)
	})

	t.Run("preserves explicit values", func(t *testing.T) {
		mc := MetricConfig{
			Namespace:  "AWS/EC2",
			MetricName: "CPUUtilization",
			Period:     300 * time.Second,
			Statistics: []string{"Sum", "Maximum"},
		}
		resolved := mc.withDefaults()
		assert.Equal(t, 300*time.Second, resolved.Period)
		assert.Equal(t, []string{"Sum", "Maximum"}, resolved.Statistics)
	})
}
