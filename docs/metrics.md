# Prometheus Metrics

The Azure Lustre CSI driver exposes Prometheus metrics for monitoring
operation latency and error rates.

## Exposed Metrics

The driver registers metrics via the
[cloud-provider-azure](https://github.com/kubernetes-sigs/cloud-provider-azure)
metrics package in the Prometheus legacy registry. The following metrics
are available at the `/metrics` HTTP endpoint:

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `cloudprovider_azure_op_duration_seconds` | Histogram | CSI operation latency |
| `cloudprovider_azure_op_failure_count` | Counter | Failed CSI operations |
| `cloudprovider_azure_api_request_duration_seconds` | Histogram | Azure API call latency |
| `cloudprovider_azure_api_request_errors` | Counter | Azure API errors |
| `cloudprovider_azure_api_request_throttled_count` | Counter | Throttled Azure API calls |

Labels include `request` (operation name), `resource_group`,
`subscription_id`, and `source`.

## Ports

| Component | Health Port | Metrics Port |
| --------- | ----------- | ------------ |
| Controller | 29762 | 29764 |
| Node | 29763 | 29765 |

## Enabling Metrics Collection

### Option 1: Annotation-based discovery (simplest)

The driver pods include `prometheus.io/scrape` and `prometheus.io/port`
annotations by default. If your Prometheus is configured to scrape pods
based on these annotations, metrics will be collected automatically.

### Option 2: Azure Monitor managed Prometheus (AKS)

If using Azure Monitor managed Prometheus on AKS, configure the
`ama-metrics-settings-configmap` to enable pod annotation-based
scraping:

```yaml
monitor_kubernetes_pods = true
```

See
[Azure Monitor documentation](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/prometheus-metrics-scrape-configuration)
for details.

## Verifying Metrics

Port-forward to the controller pod and curl the metrics endpoint:

```shell
kubectl port-forward -n kube-system \
  deploy/csi-azurelustre-controller 29764:29764
curl http://localhost:29764/metrics
```

You should see Prometheus exposition format output including
`cloudprovider_azure_op_duration_seconds` histograms.

## Helm Values Reference

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `controller.metricsPort` | Metrics port for controller | `29764` |
| `node.metricsPort` | Metrics port for node | `29765` |
