// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package awscloudwatchmetricsreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"

	"github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver/internal/metadata"
)

func TestNewFactory(t *testing.T) {
	f := NewFactory()
	assert.Equal(t, metadata.Type, f.Type())
}

func TestCreateDefaultConfig(t *testing.T) {
	f := NewFactory()
	cfg := f.CreateDefaultConfig()
	require.NotNil(t, cfg)
	require.NoError(t, componenttest.CheckConfigStruct(cfg))

	c := cfg.(*Config)
	assert.Equal(t, 60*time.Second, c.CollectionInterval)
	assert.Empty(t, c.Metrics)
	assert.Empty(t, c.Region)
}

func TestCreateMetricsReceiver(t *testing.T) {
	f := NewFactory()
	cfg := f.CreateDefaultConfig().(*Config)
	cfg.Region = "us-east-1"
	cfg.Metrics = []MetricConfig{
		{Namespace: "AWS/EC2", MetricName: "CPUUtilization"},
	}

	r, err := f.CreateMetrics(
		context.Background(),
		receivertest.NewNopSettings(metadata.Type),
		cfg,
		consumertest.NewNop(),
	)
	require.NoError(t, err)
	require.NotNil(t, r)
}

func TestCreateMetricsReceiver_ValidatesConfig(t *testing.T) {
	f := NewFactory()
	cfg := f.CreateDefaultConfig().(*Config)
	require.Error(t, cfg.Validate(), "empty config should fail validation")
}

func TestComponentFactoryType(t *testing.T) {
	require.Equal(t, component.MustNewType("awscloudwatchmetrics"), NewFactory().Type())
}
