---
title: Monitoring with Prometheus
menu_order: 90
search_type: Documentation
---

Two endpoints are exposed: one for the Weave Net router, and, when deployed as
a [Kubernetes Addon](/site/kubernetes/kube-addon.md), one for the [network policy
controller](/site/kubernetes/kube-addon.md#npc).

### Router Metrics

* `weave_connections` - Number of peer-to-peer connections.
* `weave_connection_terminations_total` - Number of peer-to-peer
  connections terminated.
* `weave_ips` - Number of IP addresses.
* `weave_max_ips` - Size of IP address space used by allocator.
* `weave_dns_entries` - Number of DNS entries.
* `weave_flows` - Number of FastDP flows.
* `weave_ipam_unreachable_count` - Number of unreachable peers that own IPAM addresses.
* `weave_ipam_unreachable_percentage` - Percentage of all IP addresses owned by unreachable peers.
* `weave_ipam_pending_allocates` - Number of pending allocates.
* `weave_ipam_pending_claims` - Number of pending claims.

### Kubernetes Network Policy Controller Metrics

The following metric is
exposed:

* `weavenpc_blocked_connections_total` - Connection attempts blocked
  by policy controller.

### Metrics Endpoint Addresses

When installed as a Kubernetes Addon, the router listens for metrics
requests on 0.0.0.0:6782 and the Network Policy Controller listens on
0.0.0.0:6781. No other requests are served on these endpoints.

>Note: If your Kubernetes hosts are exposed to the public internet
then these metrics endpoints will also be exposed.

When started via `weave launch`, by default weave listens on its local
interface to serve metrics and other read-only status requests. To
publish your metrics throughout your cluster, you can set
`WEAVE_STATUS_ADDR`:

`WEAVE_STATUS_ADDR=X.X.X.X:PORT`

Set it to an empty string to disable.

You can also pass the parameter `--metrics-addr=X.X.X.X:PORT` to
`weave launch` to specify an address to listen for metrics only.

# Static Configuration for Weave Net

The following YAML fragment can be used in your Prometheus configuration to
scrape the router metrics endpoint:

```
scrape_configs:
- job_name: 'weave'
  scrape_interval: 15s
  static_configs:
  - targets: ['localhost:6782']
```

# Discovery Configuration for Kubernetes

If you're running Weave in conjunction with Kubernetes, it is possible
to discover the endpoints for all nodes automatically. The following
Kubernetes config will install and configure Prometheus 1.3 on your
cluster and configure it to discover and scrape the Weave endpoints:

```
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus
data:
  prometheus.yml: |-
    global:
      scrape_interval: 15s
    scrape_configs:
    - job_name: 'weave'
      kubernetes_sd_configs:
      - api_server:
        role: pod
      relabel_configs:
      - source_labels: [__meta_kubernetes_namespace,__meta_kubernetes_pod_label_name]
        action: keep
        regex: ^kube-system;weave-net$
      - source_labels: [__meta_kubernetes_pod_container_name,__address__]
        action: replace
        target_label: __address__
        regex: ^weave;(.+?)(?::\d+)?$
        replacement: $1:6782
      - source_labels: [__meta_kubernetes_pod_container_name,__address__]
        action: replace
        target_label: __address__
        regex: ^weave-npc;(.+?)(?::\d+)?$
        replacement: $1:6781
      - source_labels: [__meta_kubernetes_pod_container_name]
        action: replace
        target_label: job
---
apiVersion: v1
kind: Service
metadata:
  labels:
    name: prometheus
  name: prometheus
spec:
  selector:
    app: prometheus
  type: NodePort
  ports:
  - name: prometheus
    protocol: TCP
    port: 9090
    nodePort: 30900
---
apiVersion: extensions/v1beta1
kind: Deployment
metadata:
  name: prometheus
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prometheus
  template:
    metadata:
      name: prometheus
      labels:
        app: prometheus
    spec:
      containers:
      - name: prometheus
        image: prom/prometheus:v1.3.0
        args:
          - '-config.file=/etc/prometheus/prometheus.yml'
        ports:
        - name: web
          containerPort: 9090
        volumeMounts:
        - name: config-volume
          mountPath: /etc/prometheus
      volumes:
      - name: config-volume
        configMap:
          name: prometheus
```

# Weave Cloud Integration

Finally, you can configure your local Prometheus instance to push
metrics to Weave Cloud by including the following fragment in the
`prometheus.yml` above (after replacing `<WEAVE_CLOUD_TOKEN>` with
your own token):

```
remote_write:
  url: http://frontend.dev.weave.works/api/prom/push
  basic_auth:
    password: <WEAVE_CLOUD_TOKEN>
```

For information on getting started with Weave Cloud metrics, including
on how to obtain your Weave Cloud token, please go
[here](https://github.com/weaveworks/cortex).

# Weave Net Monitoring Setup using kube-prometheus

Weave Net monitoring can be setup using the [weave-net kube-prometheus library](https://github.com/coreos/kube-prometheus/blob/master/jsonnet/kube-prometheus/kube-prometheus-weave-net.libsonnet). Please follow this [document](https://github.com/coreos/kube-prometheus/blob/master/docs/weave-net-support.md) for setting up the monitoring.
