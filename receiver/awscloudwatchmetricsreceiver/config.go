// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package awscloudwatchmetricsreceiver // import "github.com/open-telemetry/opentelemetry-collector-contrib/receiver/awscloudwatchmetricsreceiver"

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/scraper/scraperhelper"
)

var _ component.Config = (*Config)(nil)

var (
	defaultPollInterval = 60 * time.Second
	defaultPeriod       = 60 * time.Second
	defaultStatistics   = []string{"Average"}
)

var (
	errNoRegion       = errors.New("no region was specified")
	errNoMetrics      = errors.New("at least one metric must be configured")
	errNoNamespace    = errors.New("metric namespace is required")
	errNoMetricName   = errors.New("metric_name is required")
	errInvalidPeriod  = errors.New("period must be a positive duration and a multiple of 60s")
	errInvalidStat    = errors.New("invalid statistic; must be one of: Average, Sum, Minimum, Maximum, SampleCount")
	errInvalidIMDSURI = errors.New("unable to parse URI for imds_endpoint")
)

// Config defines the configuration for the AWS CloudWatch Metrics receiver.
type Config struct {
	scraperhelper.ControllerConfig `mapstructure:",squash"`

	// Region is the AWS region to collect metrics from.
	Region string `mapstructure:"region"`

	// Profile is the named AWS profile to use for credentials.
	Profile string `mapstructure:"profile"`

	// IMDSEndpoint is the custom endpoint for the EC2 Instance Metadata Service.
	IMDSEndpoint string `mapstructure:"imds_endpoint"`

	// Metrics defines the CloudWatch metrics to collect.
	Metrics []MetricConfig `mapstructure:"metrics"`
}

// MetricConfig represents a single CloudWatch metric to collect.
type MetricConfig struct {
	// Namespace is the CloudWatch namespace (e.g. "AWS/EC2", "AWS/ECS").
	Namespace string `mapstructure:"namespace"`

	// MetricName is the CloudWatch metric name (e.g. "CPUUtilization").
	MetricName string `mapstructure:"metric_name"`

	// Period is the aggregation period. Must be a multiple of 60s. Defaults to 60s.
	Period time.Duration `mapstructure:"period"`

	// Statistics to retrieve. Defaults to ["Average"].
	// Valid values: Average, Sum, Minimum, Maximum, SampleCount.
	Statistics []string `mapstructure:"statistics"`

	// Dimensions to filter metrics by (e.g. ["InstanceId"]).
	// When specified, only metrics matching these dimension names are returned.
	Dimensions []string `mapstructure:"dimensions"`
}

var validStatistics = map[string]bool{
	"Average":     true,
	"Sum":         true,
	"Minimum":     true,
	"Maximum":     true,
	"SampleCount": true,
}

func (c *Config) Validate() error {
	if c.Region == "" {
		return errNoRegion
	}

	if c.IMDSEndpoint != "" {
		if _, err := url.ParseRequestURI(c.IMDSEndpoint); err != nil {
			return fmt.Errorf("%w: %w", errInvalidIMDSURI, err)
		}
	}

	if len(c.Metrics) == 0 {
		return errNoMetrics
	}

	var errs error
	for i, m := range c.Metrics {
		if m.Namespace == "" {
			errs = errors.Join(errs, fmt.Errorf("metrics[%d]: %w", i, errNoNamespace))
		}
		if m.MetricName == "" {
			errs = errors.Join(errs, fmt.Errorf("metrics[%d]: %w", i, errNoMetricName))
		}
		if m.Period != 0 {
			if m.Period < 0 || m.Period.Seconds() < 60 || int(m.Period.Seconds())%60 != 0 {
				errs = errors.Join(errs, fmt.Errorf("metrics[%d]: %w", i, errInvalidPeriod))
			}
		}
		for _, s := range m.Statistics {
			if !validStatistics[s] {
				errs = errors.Join(errs, fmt.Errorf("metrics[%d]: %w: %q", i, errInvalidStat, s))
			}
		}
	}
	return errs
}

// withDefaults returns a copy of the MetricConfig with defaults applied.
func (m MetricConfig) withDefaults() MetricConfig {
	if m.Period == 0 {
		m.Period = defaultPeriod
	}
	if len(m.Statistics) == 0 {
		m.Statistics = defaultStatistics
	}
	return m
}
