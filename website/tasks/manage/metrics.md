---
title: Monitoring Weave Net with Prometheus
menu_order: 90
search_type: Documentation
---

Two endpoints are exposed: one for the Weave Net router, and, when deployed as
a [Kubernetes Addon]({{ '/kubernetes/kube-addon' | relative_url }}), one for the [network policy
controller]({{ '/kubernetes/kube-addon#npc' | relative_url }}).

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

# Weave Net Monitoring Setup in Kubernetes using kube-prometheus

Weave Net monitoring can be setup in Kubernetes using the kube-prometheus [library](https://github.com/coreos/kube-prometheus/blob/master/jsonnet/kube-prometheus/kube-prometheus-weave-net.libsonnet) for Weave Net. You can read about the example document [here](https://github.com/coreos/kube-prometheus/blob/master/docs/weave-net-support.md).

Let's setup weave monitoring using [kube-prometheus](https://github.com/coreos/kube-prometheus).

#### Install golang
Follow this [document](https://golang.org/doc/install/source)

#### Install jssonet builder
```
go get github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb
```

#### Install jsonnet
Follow this [document](https://github.com/google/jsonnet#building-jsonnet)

#### Install gojsonyaml
```
go get github.com/brancz/gojsontoyaml
```

#### Update dependencies
```
jb update
```

#### Create weave-net.jsonnet
**Note:** Some alert configurations are environment specific and may require modifications of alert thresholds.
```
cat << EOF > weave-net.jsonnet
local kp =  (import 'kube-prometheus/kube-prometheus.libsonnet') +
            (import 'kube-prometheus/kube-prometheus-weave-net.libsonnet') + {
  _config+:: {
    namespace: 'monitoring',
  },
  prometheusAlerts+:: {
    groups: std.map(
      function(group)
        if group.name == 'weave-net' then
          group {
            rules: std.map(function(rule)
              if rule.alert == "WeaveNetFastDPFlowsLow" then
                rule {
                  expr: "sum(weave_flows) < 20000"
                }
              else if rule.alert == "WeaveNetIPAMUnreachable" then
                rule {
                  expr: "weave_ipam_unreachable_percentage > 25"
                }
              else
                rule
              ,
              group.rules
            )
          }
        else
          group,
        super.groups
      ),
  },
};

{ ['00namespace-' + name]: kp.kubePrometheus[name] for name in std.objectFields(kp.kubePrometheus) } +
{ ['0prometheus-operator-' + name]: kp.prometheusOperator[name] for name in std.objectFields(kp.prometheusOperator) } +
{ ['node-exporter-' + name]: kp.nodeExporter[name] for name in std.objectFields(kp.nodeExporter) } +
{ ['kube-state-metrics-' + name]: kp.kubeStateMetrics[name] for name in std.objectFields(kp.kubeStateMetrics) } +
{ ['prometheus-' + name]: kp.prometheus[name] for name in std.objectFields(kp.prometheus) } +
{ ['prometheus-adapter-' + name]: kp.prometheusAdapter[name] for name in std.objectFields(kp.prometheusAdapter) } +
{ ['grafana-' + name]: kp.grafana[name] for name in std.objectFields(kp.grafana) }
EOF
```

#### Create manifests
```
jsonnet -J vendor -m manifests weave-net.jsonnet | xargs -I{} sh -c 'cat $1 | gojsontoyaml > $1.yaml; rm -f $1' -- {}
```

#### Apply manifests
Applying the created manifests will install the following components in your Kubernetes cluster:
- [Namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) "monitoring"
- [Prometheus](https://github.com/prometheus/prometheus)
- [Kube State Metrics](https://github.com/kubernetes/kube-state-metrics)
- [Prometheus Operator](https://github.com/coreos/prometheus-operator) Operator to manage prometheus
- [Grafana](https://grafana.com) Dashboards on top of Prometheus metrics.
- [Weave Net ServiceMonitor](https://github.com/coreos/kube-prometheus/blob/7a85d7d8a6a81eda1db090846759fd6ca6532884/jsonnet/kube-prometheus/kube-prometheus-weave-net.libsonnet#L12-L43) ServiceMonitor brings the weave net metrics to prometheus.
- [Weave Net Service](https://github.com/coreos/kube-prometheus/blob/7a85d7d8a6a81eda1db090846759fd6ca6532884/jsonnet/kube-prometheus/kube-prometheus-weave-net.libsonnet#L7-L11) Weave Net Service which the ServiceMonitor scrapes.
- [Weave Net Grafana Dashboard](https://grafana.com/grafana/dashboards/11789)
- [Weave Net (Cluster) Grafana Dashboard](https://grafana.com/grafana/dashboards/11804)
```
cd manifests
ls *.yaml | grep -e ^prometheus -e ^node -e ^grafana -e ^kube | xargs -I {} kubectl create -f {} -n monitoring
kubectl create -f prometheus-serviceWeaveNet.yaml -n kube-system
kubectl create -f prometheus-serviceMonitorWeaveNet.yaml -n monitoring
```
**Note:** If you want to make changes to the created yamls, please modify the weave-net.jsonnet and recreate the manifests before applying.
