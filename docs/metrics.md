# Prometheus Metrics

The Azure Lustre CSI driver exposes Prometheus metrics for monitoring operation
latency and error rates.

## Exposed Metrics

The driver registers metrics via the
[cloud-provider-azure](https://github.com/kubernetes-sigs/cloud-provider-azure)
metrics package in the Prometheus legacy registry. The following metrics are
available at the `/metrics` HTTP endpoint:

| Metric | Type | Description |
| ------ | ---- | ----------- |
| `cloudprovider_azure_op_duration_seconds` | Histogram | Latency of CSI operations (create volume, delete volume, publish, unpublish) |
| `cloudprovider_azure_op_failure_count` | Counter | Number of failed CSI operations |
| `cloudprovider_azure_api_request_duration_seconds` | Histogram | Latency of Azure API calls |
| `cloudprovider_azure_api_request_errors` | Counter | Number of Azure API errors |
| `cloudprovider_azure_api_request_throttled_count` | Counter | Number of throttled Azure API calls |

Labels include `request` (operation name), `resource_group`, `subscription_id`,
and `source`.

## Ports

| Component | Health Port | Metrics Port |
| --------- | ----------- | ------------ |
| Controller | 29762 | 29764 |
| Node | 29763 | 29765 |

> [!NOTE]
> The node DaemonSet runs with `hostNetwork: true`, so its metrics endpoint (port
> 29765) is reachable on each node's host IP and is unauthenticated; the metric
> labels include `subscription_id` and `resource_group`. Restrict access with a
> `NetworkPolicy` or your cluster's NSG/firewall so only your Prometheus scraper
> can reach it. The controller endpoint (29764) is on the pod network and is not
> host-exposed.

## Enabling Metrics Collection

### Option 1: Annotation-based discovery (simplest)

The driver pods include `prometheus.io/scrape` and `prometheus.io/port`
annotations by default. If your Prometheus is configured to scrape pods based
on these annotations, metrics will be collected automatically.

### Option 2: Helm — Service + ServiceMonitor (recommended for Prometheus Operator)

> [!NOTE]
> Enable both `metrics.service.enabled` and `metrics.serviceMonitor.enabled`. The
> ServiceMonitor selects the metrics Service, so enabling it without the Service
> produces a ServiceMonitor that scrapes nothing.

If your cluster uses the
[Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator),
enable the Service and ServiceMonitor via Helm values:

```shell
helm install azurelustre azurelustre-csi-driver/azurelustre-csi-driver \
  --namespace kube-system \
  --set metrics.service.enabled=true \
  --set metrics.serviceMonitor.enabled=true
```

This creates:

- A `ClusterIP` Service exposing the metrics port for both controller and node
- A `ServiceMonitor` that tells Prometheus Operator to scrape the service

To add labels for Prometheus Operator service discovery (e.g., if your
Prometheus is configured to only scrape ServiceMonitors with specific labels):

```shell
helm install azurelustre azurelustre-csi-driver/azurelustre-csi-driver \
  --namespace kube-system \
  --set metrics.service.enabled=true \
  --set metrics.serviceMonitor.enabled=true \
  --set metrics.serviceMonitor.labels.release=prometheus
```

To change the scrape interval (default 15s):

```shell
--set metrics.serviceMonitor.interval=30s
```

### Option 3: Manual kubectl apply (non-Helm users)

Apply the example YAMLs from the repository:

```shell
kubectl apply -f docs/examples/metrics/
```

This creates the same Service and ServiceMonitor resources as Option 2, but
with hardcoded values. Edit the YAMLs to customize ports or labels.

### Option 4: Azure Monitor managed Prometheus (AKS)

If using Azure Monitor managed Prometheus on AKS, configure the
`ama-metrics-settings-configmap` to enable pod annotation-based scraping:

```yaml
monitor_kubernetes_pods = true
```

See [Azure Monitor documentation](https://learn.microsoft.com/en-us/azure/azure-monitor/containers/prometheus-metrics-scrape-configuration)
for details.

## Verifying Metrics

Port-forward to the controller pod and curl the metrics endpoint:

```shell
kubectl port-forward -n kube-system deploy/csi-azurelustre-controller 29764:29764
curl http://localhost:29764/metrics
```

You should see Prometheus exposition format output including
`cloudprovider_azure_op_duration_seconds` histograms.

## Helm Values Reference

| Parameter | Description | Default |
| --------- | ----------- | ------- |
| `controller.metricsPort` | Metrics port for controller | `29764` |
| `node.metricsPort` | Metrics port for node | `29765` |
| `metrics.service.enabled` | Create ClusterIP Service for metrics | `false` |
| `metrics.serviceMonitor.enabled` | Create ServiceMonitor for Prometheus Operator | `false` |
| `metrics.serviceMonitor.interval` | Scrape interval | `15s` |
| `metrics.serviceMonitor.labels` | Extra labels on ServiceMonitor resources | `{}` |
