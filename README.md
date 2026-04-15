# BankFlow OTel Demo — Datadog + OpenTelemetry Showcase

BankFlow is a polyglot banking microservices demo that showcases Datadog's first-class OpenTelemetry support across two deployment patterns. The same application code runs on two GKE clusters simultaneously — one using the Datadog Agent's built-in DDOT collector, and one using the upstream OpenTelemetry Collector Contrib — so you can compare the observability coverage each approach delivers side by side.

> Run `chmod +x scripts/*.sh` once after cloning before executing any scripts.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Cluster A — warach-otel-sdk-ddot                                           │
│                                                                             │
│  ┌───────────┐     ┌──────────────────┐     ┌──────────────────────┐       │
│  │  k6 Load  │────▶│  account-service │────▶│  transaction-service │       │
│  │ Generator │     │  (Java/OTel SDK) │     │    (Go/OTel SDK)     │       │
│  └───────────┘     └──────────────────┘     └──────────┬───────────┘       │
│                                                ┌───────┴────────┐          │
│                    ┌──────────────────────┐    │                │          │
│                    │ notification-service │◀───┤  fraud-detect  │          │
│                    │  (Java/OTel SDK)     │    │ (Python/OTel)  │          │
│                    └──────────────────────┘    └────────────────┘          │
│                                                                             │
│   All services ──OTLP gRPC──▶ hostIP:4317                                  │
│  ┌────────────────────────────────────────────────────────────────────┐     │
│  │  Datadog Agent DaemonSet  (DDOT Collector + system-probe + APM)   │     │
│  │  + Datadog Cluster Agent  (K8s metadata, orchestrator explorer)    │     │
│  └───────────────────────────────┬────────────────────────────────────┘     │
└──────────────────────────────────┼──────────────────────────────────────────┘
                                   │ HTTPS
                                   ▼
                          ┌─────────────────┐
                          │     Datadog     │
                          │ (datadoghq.com) │
                          └────────┬────────┘
                                   │ HTTPS
┌──────────────────────────────────┼──────────────────────────────────────────┐
│  Cluster B — warach-otel-sdk-otelcontrib                                    │
│                                  │                                          │
│  ┌───────────────────────────────┴──────────────────────────────────────┐   │
│  │  OTel Collector Contrib DaemonSet  (+datadog extension)             │   │
│  │  OTLP receiver ∙ filelog ∙ kubeletstats ∙ hostmetrics              │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │  OTel Collector Contrib Deployment  (+datadog extension)            │   │
│  │  k8s_cluster receiver ∙ k8sobjects receiver                        │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌───────────┐     ┌──────────────────┐     ┌──────────────────────┐       │
│  │  k6 Load  │────▶│  account-service │────▶│  transaction-service │       │
│  │ Generator │     │  (Java/OTel SDK) │     │    (Go/OTel SDK)     │       │
│  └───────────┘     └──────────────────┘     └──────────┬───────────┘       │
│                                                ┌───────┴────────┐          │
│                    ┌──────────────────────┐    │                │          │
│                    │ notification-service │◀───┤  fraud-detect  │          │
│                    │  (Java/OTel SDK)     │    │ (Python/OTel)  │          │
│                    └──────────────────────┘    └────────────────┘          │
│                                                                             │
│   All services ──OTLP gRPC──▶ hostIP:4317                                  │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Prerequisites

| Tool | Version | Notes |
|------|---------|-------|
| `kubectl` | any recent | configured with gcloud credentials |
| `helm` | v3+ | |
| `docker buildx` | any recent | needs `linux/amd64` builder |
| `gcloud` | any recent | for GKE auth |
| Java / Maven | 21 / 3.9+ | for account-service and notification-service |
| Go | 1.22+ | for transaction-service |
| Python | 3.12+ | for fraud-detection-service |

---

## Quick Start

**1. Clone the repository**

```bash
git clone <repo-url>
cd otel-showcase
```

**2. Configure credentials**

```bash
cp .env.example .env
# Edit .env and fill in your Datadog API key and App key
```

**3. Configure Docker buildx for linux/amd64**

```bash
docker buildx create --use
```

**4. Create namespaces and secrets on both clusters**

```bash
./scripts/setup-secrets.sh
```

**5. Build and push all service images**

```bash
./scripts/build-push.sh
```

**6. Deploy to Cluster A (Datadog Agent with DDOT)**

```bash
./scripts/deploy-cluster-a.sh
```

**7. Deploy to Cluster B (OTel Collector Contrib)**

```bash
./scripts/deploy-cluster-b.sh
```

---

## Feature Matrix

| Feature                   | Cluster A (DDOT) | Cluster B (OTel Contrib) |
|---------------------------|:----------------:|:------------------------:|
| Traces (APM)              | YES              | YES                      |
| Metrics                   | YES              | YES                      |
| Logs                      | YES              | YES                      |
| CNM (Network Monitoring)  | YES              | NO                       |
| Live Containers           | YES              | NO                       |
| Live Processes            | YES              | NO                       |
| USM (Service Monitoring)  | YES              | NO                       |
| K8s Explorer              | YES              | NO                       |
| Fleet Automation          | YES              | YES (via datadog ext)    |
| OTel Collector Contrib    | NO               | YES                      |

---

## Verifying in Datadog

**APM Service Map**
- Navigate to APM → Service Map
- Filter by `env:otel-demo`
- Services from Cluster A have names ending in `-ddot`; Cluster B services end in `-oss`
- The full transfer chain should appear: account-service → transaction-service → fraud-detection-service → notification-service

**Infrastructure — Containers (Cluster A only)**
- Navigate to Infrastructure → Containers
- Filter by cluster name `warach-otel-sdk-ddot`
- Live container metrics, resource usage, and logs are available because the Datadog Agent runs on the node

**Network Performance Monitoring (Cluster A only)**
- Navigate to NPM → Overview
- Filter by `cluster_name:warach-otel-sdk-ddot`
- East-west traffic flows between bankflow services are visible without any code changes

**K8s Explorer (Cluster A only)**
- Navigate to Infrastructure → Kubernetes
- Select cluster `warach-otel-sdk-ddot`
- Explore Pods, Deployments, Services, and Nodes with correlated metrics and events

---

## Demo Script

**Opening narrative**

"Both clusters are running the exact same four microservices — same Docker images, same OTel SDK instrumentation, same k6 load generator hammering the transfer endpoint. The only difference is the collector pipeline."

**Architectural difference**

- Cluster A uses the Datadog Agent, which has the DDOT (Datadog OpenTelemetry) collector built in. Applications ship OTLP over gRPC to the Agent's `hostIP:4317` endpoint. The Agent then enriches, batches, and forwards everything to Datadog. Because the full Agent runs on every node, you get CNM, live containers, live processes, USM, and the K8s Explorer for free.

- Cluster B uses the upstream OpenTelemetry Collector Contrib, deployed as both a DaemonSet (for node-level signals: OTLP, logs, kubelet, host metrics) and a Deployment (for cluster-level signals: k8s_cluster receiver, k8sobjects receiver). Traces, metrics, and logs all flow to Datadog via the Datadog exporter, but without a Datadog Agent on the node you lose the additional platform features.

**Side-by-side T/M/L parity**

1. Open APM → Service Map and show both `-ddot` and `-oss` service graphs — identical topology.
2. Open Metrics Explorer and plot `http.server.request.duration` for both environments — same shape, same cardinality.
3. Open Log Explorer, filter by `env:otel-demo` — structured logs flowing from both clusters.

**DDOT-only features**

Switch to the Cluster A perspective and walk through:
- NPM: point out the east-west flow map between account-service and transaction-service
- K8s Explorer: correlate pod restarts with APM error spikes on the same timeline
- Live Containers: show real-time CPU/memory without any sidecar or extra configuration

---

## Teardown

To remove all Kubernetes resources from both clusters (the GKE clusters themselves are **not** deleted):

```bash
./scripts/teardown.sh
```

---

## Troubleshooting

**Services crash-looping with `exec format error`**

Your images were built for the wrong architecture. Verify you used `--platform linux/amd64` in every `docker buildx build` call. The `build-push.sh` script enforces this, but if you built manually ensure the same flag was present. Re-run `./scripts/build-push.sh` to rebuild.

**Traces not appearing in Datadog (Cluster A)**

Check that the OTLP endpoint is being resolved via the node's `hostIP`. The recommended pattern is:

```yaml
env:
  - name: NODE_IP
    valueFrom:
      fieldRef:
        fieldPath: status.hostIP
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://$(NODE_IP):4317"
```

Confirm the Datadog Agent DaemonSet pod on the same node is running and that port 4317 is open in the Agent's `datadog-values.yaml` (`otlp.receiver.protocols.grpc.enabled: true`).

**Traces not appearing in Datadog (Cluster B)**

Ensure the OTel Collector DaemonSet pod is running on every node (`kubectl get pods -n otel-system -o wide`) and that the service applications are pointing their `OTEL_EXPORTER_OTLP_ENDPOINT` to `http://$(NODE_IP):4317` (same pattern as Cluster A, but targeting the OTel Collector DaemonSet instead).

**Missing Kubernetes attributes on spans (Cluster B)**

The `k8sattributesprocessor` in the OTel Collector requires RBAC permissions to read Pod and Namespace metadata. Check that the ServiceAccount for the DaemonSet has a ClusterRole binding that grants `get`, `watch`, and `list` on `pods`, `namespaces`, `nodes`, `replicationcontrollers`, `replicasets`, `statefulsets`, `daemonsets`, `deployments`, and `jobs`. A missing RBAC binding will cause the processor to silently skip attribute enrichment without crashing.

**Helm release stuck in `pending-install`**

Run `helm list -n datadog` (or `-n otel-system`) to check the release state. If a previous partial install left it stuck, run `helm uninstall <release-name> -n <namespace>` and re-run the deploy script.
