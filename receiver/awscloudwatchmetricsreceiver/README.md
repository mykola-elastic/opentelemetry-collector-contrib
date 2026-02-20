# AWS CloudWatch Metrics Receiver

| Status        |           |
|---------------|-----------|
| Stability     | [development]: metrics   |
| Distributions | [] |
| Issues        | [![Open issues](https://img.shields.io/github/issues-search/open-telemetry/opentelemetry-collector-contrib?query=is%3Aissue%20is%3Aopen%20label%3Areceiver%2Fawscloudwatchmetrics%20&label=open&color=orange&logo=opentelemetry)](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues?q=is%3Aopen+is%3Aissue+label%3Areceiver%2Fawscloudwatchmetrics) [![Closed issues](https://img.shields.io/github/issues-search/open-telemetry/opentelemetry-collector-contrib?query=is%3Aissue%20is%3Aclosed%20label%3Areceiver%2Fawscloudwatchmetrics%20&label=closed&color=blue&logo=opentelemetry)](https://github.com/open-telemetry/opentelemetry-collector-contrib/issues?q=is%3Aclosed+is%3Aissue+label%3Areceiver%2Fawscloudwatchmetrics) |

[development]: https://github.com/open-telemetry/opentelemetry-collector/blob/main/docs/component-stability.md#development

## Overview

The AWS CloudWatch Metrics Receiver collects metrics from [AWS CloudWatch](https://aws.amazon.com/cloudwatch/) using the
[GetMetricData](https://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/API_GetMetricData.html) API
and converts them into OpenTelemetry metrics.

This is a **metrics-only** receiver. It does not support logs or metric streams.

## Configuration

```yaml
receivers:
  awscloudwatchmetrics:
    region: us-east-1
    collection_interval: 60s
    metrics:
      - namespace: AWS/EC2
        metric_name: CPUUtilization
        dimensions:
          - InstanceId
      - namespace: AWS/ECS
        metric_name: CPUUtilization
      - namespace: AWS/RDS
        metric_name: DatabaseConnections
        period: 300s
        statistics:
          - Sum
          - Average
```

### Top-level settings

| Setting               | Required | Default | Description                                               |
|-----------------------|----------|---------|-----------------------------------------------------------|
| `region`              | Yes      |         | AWS region to collect metrics from                        |
| `profile`             | No       |         | Named AWS profile for credentials                         |
| `imds_endpoint`       | No       |         | Custom EC2 IMDS endpoint                                  |
| `collection_interval` | No       | `60s`   | How often to poll CloudWatch                              |
| `metrics`             | Yes      |         | List of CloudWatch metrics to collect (at least one)      |

### Metric settings

| Setting      | Required | Default       | Description                                                       |
|--------------|----------|---------------|-------------------------------------------------------------------|
| `namespace`  | Yes      |               | CloudWatch namespace (e.g. `AWS/EC2`, `AWS/ECS`, `Custom/MyApp`)  |
| `metric_name`| Yes      |               | CloudWatch metric name (e.g. `CPUUtilization`)                    |
| `period`     | No       | `60s`         | Aggregation period (must be a multiple of 60s)                    |
| `statistics` | No       | `["Average"]` | Statistics to retrieve: `Average`, `Sum`, `Minimum`, `Maximum`, `SampleCount` |
| `dimensions` | No       |               | Dimension names to filter by (e.g. `["InstanceId"]`)              |

## How it works

1. On each collection interval, the receiver calls [ListMetrics](https://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/API_ListMetrics.html) to discover all metric/dimension combinations matching the configured namespaces.
2. It then builds [GetMetricData](https://docs.aws.amazon.com/AmazonCloudWatch/latest/APIReference/API_GetMetricData.html) queries for each discovered metric, batching to stay within the 500-query API limit.
3. Results are converted to OpenTelemetry gauge metrics with resource attributes for the CloudWatch namespace and dimensions.

### Metric naming

CloudWatch metrics are mapped to OpenTelemetry metric names as:

```
aws.cloudwatch.<namespace>.<metric_name>.<statistic>
```

For example, `AWS/EC2` `CPUUtilization` with `Average` becomes:

```
aws.cloudwatch.aws.ec2.cpuutilization.average
```

### Resource attributes

Each metric includes the following resource attributes:

| Attribute                                | Description                              |
|------------------------------------------|------------------------------------------|
| `cloud.provider`                         | Always `aws`                             |
| `aws.cloudwatch.namespace`               | CloudWatch namespace (e.g. `AWS/EC2`)    |
| `aws.cloudwatch.dimension.<DimensionName>` | Dimension value for each dimension     |

## Authentication

The receiver uses the standard AWS SDK credential chain. You can configure credentials through:

- Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
- Shared credentials file (`~/.aws/credentials`)
- IAM role for EC2/ECS
- Named profile via the `profile` config setting

### Required IAM permissions

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "cloudwatch:ListMetrics",
        "cloudwatch:GetMetricData"
      ],
      "Resource": "*"
    }
  ]
}
```
