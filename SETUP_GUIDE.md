# OTel Showcase — Setup Guide

## Purpose

This showcase demonstrates how **Datadog supports OpenTelemetry** across two distinct telemetry pipelines, running side by side on separate GKE clusters:

| | Cluster A — DDOT | Cluster B — OTel Collector Contrib |
|---|---|---|
| **Pipeline** | OTel SDK &rarr; DDOT (inside Datadog Agent) &rarr; Datadog | OTel SDK &rarr; OTel Collector Contrib &rarr; Datadog |
| **Service suffix** | `-ddot` | `-oss` |
| **Collector namespace** | `datadog` | `otel-system` |

Both clusters run **identical application code** instrumented exclusively with the OpenTelemetry SDK — no Datadog SDK or tracer anywhere. The only difference is the telemetry pipeline. Both report to the **same Datadog organization** under `env:otel-demo`.

### What This Proves

- **Traces, Metrics, Logs** &rarr; full parity across both pipelines
- **CNM, Live Containers, Live Processes, USM, K8s Explorer** &rarr; available only on the DDOT cluster (requires the Datadog Agent's privileged system access)
- **Fleet Automation** &rarr; both pipelines (DDOT natively; OTel Contrib via the `datadog` extension)

This gap is **architectural, not a limitation of the application**.

---

## The Application — BankFlow

A polyglot banking backend simulating retail banking operations.

### Services

| Service | Language | Port | Description |
|---|---|---|---|
| **account-service** | Java (Spring Boot 3 + OTel Java Agent) | 8080 | Manages customer accounts and balances. Pre-seeded with 5 accounts (ACC001–ACC005). Entry point for k6 traffic. |
| **transaction-service** | Go (OTel SDK, manual init) | 8080 | Processes fund transfers. Injects 15% random failures and 300–900ms latency. Calls fraud-detection and notification services. |
| **fraud-detection-service** | Python (Flask + OTel SDK) | 3000 | Real-time fraud scoring. 5% of requests return high-risk. Emits custom metrics (`fraud.score.total`, `fraud.scoring.latency.ms`). |
| **notification-service** | Java (Spring Boot 3 + OTel Java Agent) | 8080 | Sends simulated transaction confirmations and fraud alerts. |
| **k6-loadgen** | k6 (grafana/k6) | — | Continuous load generator. 60% transfer requests (full chain), 40% balance queries, 10% invalid accounts for error traces. |

### Request Flow

```
k6 loadgen
    |
    v
account-service ──> transaction-service ──> fraud-detection-service
                         |
                         └──> notification-service
```

### OTel Instrumentation Approach

| Service | Approach |
|---|---|
| account-service | **Zero-code** — OTel Java Agent (`-javaagent`) auto-instruments Spring Boot, RestTemplate, Logback |
| notification-service | **Zero-code** — Same OTel Java Agent approach |
| transaction-service | **Manual SDK init** — `otel.go` sets up TracerProvider + MeterProvider + LoggerProvider over OTLP gRPC |
| fraud-detection-service | **SDK + auto-instrumentation** — `FlaskInstrumentor` for HTTP, manual spans/metrics for scoring logic |

---

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| `kubectl` | 1.28+ | Kubernetes CLI |
| `helm` | 3.12+ | Helm chart deployment |
| `docker` with `buildx` | 24+ | Multi-platform image builds |
| `gcloud` | latest | GKE authentication |
| Java JDK | 21 | Only if building Java services locally (not needed — Maven runs inside Docker) |
| Go | 1.22 | Only if building Go service locally |
| Python | 3.12 | Only if building Python service locally |

---

## Cluster Details

Both clusters already exist in GKE. **Do not create or delete clusters.**

| | Cluster A — DDOT | Cluster B — OTel Contrib |
|---|---|---|
| **kubectl context** | `gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-ddot` | `gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-otelcontrib` |
| **App namespace** | `bankflow` | `bankflow` |
| **Collector namespace** | `datadog` | `otel-system` |
| **Cluster name** | `warach-otel-sdk-ddot` | `warach-otel-sdk-otelcontrib` |

---

## Step-by-Step Setup

### Step 0 — Authenticate to GKE

```bash
gcloud container clusters get-credentials warach-otel-sdk-ddot --zone asia-southeast1-a --project datadog-ese-sandbox
gcloud container clusters get-credentials warach-otel-sdk-otelcontrib --zone asia-southeast1-a --project datadog-ese-sandbox
```

### Step 1 — Configure Credentials

```bash
cp .env.example .env
```

Edit `.env` and fill in your Datadog API key and App key:

```
DD_API_KEY=<your_datadog_api_key>
DD_APP_KEY=<your_datadog_app_key>
```

> **Important:** `.env` is in `.gitignore` — never commit real credentials.

### Step 2 — Create Namespaces and Secrets

```bash
chmod +x scripts/*.sh
./scripts/setup-secrets.sh
```

This creates:
- Cluster A: `bankflow` + `datadog` namespaces, `datadog-secret` (api-key + app-key)
- Cluster B: `bankflow` + `otel-system` namespaces, `datadog-secret` (DD_API_KEY)

### Step 3 — Build and Push Docker Images

> **Critical:** All images must target `linux/amd64` because GKE nodes run x86_64, but this MacBook builds arm64 natively.

Ensure Docker buildx has a multi-platform builder:

```bash
docker buildx create --name multiarch-builder --use 2>/dev/null || true
```

Build and push all 4 services:

```bash
# account-service (Java — Maven runs inside Docker, no local JDK needed)
docker buildx build --platform linux/amd64 -t docker.io/wwongpai/account-service:latest --push services/account-service/

# transaction-service (Go)
docker buildx build --platform linux/amd64 -t docker.io/wwongpai/transaction-service:latest --push services/transaction-service/

# fraud-detection-service (Python)
docker buildx build --platform linux/amd64 -t docker.io/wwongpai/fraud-detection-service:v5 --push services/fraud-detection-service/

# notification-service (Java — Maven runs inside Docker)
docker buildx build --platform linux/amd64 -t docker.io/wwongpai/notification-service:latest --push services/notification-service/
```

Or use the script: `./scripts/build-push.sh`

### Step 4 — Deploy Cluster A (DDOT)

```bash
./scripts/deploy-cluster-a.sh
```

What this does:
1. Switches to the DDOT cluster context
2. Adds the `datadog` Helm repo
3. Installs the Datadog Agent via Helm with:
   - **DDOT collector** enabled (OTLP gRPC on hostPort 4317, HTTP on 4318)
   - **Datadog Cluster Agent** for K8s metadata, orchestrator explorer
   - **System probe** configured for GKE COS (`providers.gke.cos: true`, `systemProbe.enableDefaultKernelHeadersPaths: false`)
   - **Operator disabled** (`datadog.operator.enabled: false` — we use plain Helm, not CRDs)
   - Full feature set: APM, logs, process monitoring, network monitoring, USM, orchestrator explorer
4. Deploys all 4 app services + k6 load generator to `bankflow` namespace
5. Waits for all rollouts to complete

**Key Helm values** (`cluster-a-ddot/datadog-values.yaml`):
```yaml
datadog:
  operator:
    enabled: false              # Plain Helm chart, not operator
  providers:
    gke:
      cos: true                 # GKE Container-Optimized OS support
  systemProbe:
    enableDefaultKernelHeadersPaths: false  # Required for COS
  otelCollector:
    enabled: true               # DDOT collector
```

### Step 5 — Deploy Cluster B (OTel Collector Contrib)

```bash
./scripts/deploy-cluster-b.sh
```

What this does:
1. Switches to the OTel Contrib cluster context
2. Adds the `open-telemetry` Helm repo
3. Installs **two** OTel Collector Helm releases:

   **DaemonSet** (`otel-collector-daemonset`) — one per node:
   - OTLP receiver on hostPort 4317/4318
   - `k8sattributes` processor (pod/node metadata enrichment)
   - `kubeletstats` receiver (node + pod metrics)
   - `filelog` receiver (container logs from `/var/log/pods`)
   - `hostmetrics` receiver (CPU, memory, disk, network)
   - `datadog` extension for Fleet Automation (`deployment_type: daemonset`)
   - Exports traces, metrics, logs to Datadog

   **Deployment** (`otel-collector-deployment`) — single replica:
   - `k8s_cluster` receiver (cluster-level metrics: pod counts, node status)
   - `k8sobjects` receiver (Kubernetes events)
   - `datadog` extension for Fleet Automation (`deployment_type: gateway`)
   - Exports metrics + logs to Datadog
   - Does NOT collect node-level data (avoids duplication with DaemonSet)

4. Deploys all 4 app services + k6 load generator to `bankflow` namespace
5. Waits for all rollouts to complete

### Step 6 — Validate Deployment

Check all pods are running on both clusters:

```bash
# Cluster A
kubectl config use-context gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-ddot
kubectl get pods -n bankflow
kubectl get pods -n datadog

# Cluster B
kubectl config use-context gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-otelcontrib
kubectl get pods -n bankflow
kubectl get pods -n otel-system
```

**Expected pod counts:**

| Namespace | Cluster A | Cluster B |
|---|---|---|
| `bankflow` | 9 pods (2 account + 2 transaction + 2 fraud + 1 notification + 1 k6 + 1 spare) | Same |
| `datadog` | 2 agent DaemonSet + 1 cluster-agent | — |
| `otel-system` | — | 2 DaemonSet + 1 Deployment |

Verify traces are exporting (no gRPC errors):

```bash
# On Cluster B — Go service should show clean startup, no export errors
kubectl logs -n bankflow -l app=transaction-service --tail=5
```

---

## Verifying in Datadog

### APM Traces

Navigate to [APM > Services](https://app.datadoghq.com/apm/services) and filter by `env:otel-demo`.

You should see 8 services:
- `account-service-ddot`, `transaction-ddot`, `fraud-detection-ddot`, `notification-ddot`
- `account-service-oss`, `transaction-oss`, `fraud-detection-oss`, `notification-oss`

### Service Map

[APM > Service Map](https://app.datadoghq.com/apm/map) filtered by `env:otel-demo` — shows the full request chain for both clusters.

### Metrics

Custom metrics from transaction-service and fraud-detection-service:
- `transfer.total` (counter, by status/currency)
- `transfer.latency.ms` (histogram)
- `fraud.score.total` (counter, by risk_level)
- `fraud.scoring.latency.ms` (histogram)

### Logs

[Logs Explorer](https://app.datadoghq.com/logs) filtered by `env:otel-demo` — container stdout collected by:
- Cluster A: Datadog Agent `containerCollectAll`
- Cluster B: OTel Collector `filelog` receiver

### Infrastructure (DDOT only)

These features require the Datadog Agent and are **not available** on Cluster B:

- [Infrastructure > Containers](https://app.datadoghq.com/containers) — Live Containers
- [Infrastructure > Processes](https://app.datadoghq.com/process) — Live Processes
- [Network Performance](https://app.datadoghq.com/network) — CNM
- [Universal Service Monitoring](https://app.datadoghq.com/services/usm) — USM
- [Kubernetes Explorer](https://app.datadoghq.com/orchestration/overview/pod) — K8s Explorer

### Fleet Automation

[Fleet Automation](https://app.datadoghq.com/fleet) — shows both:
- Datadog Agents from Cluster A
- OTel Collectors from Cluster B (via `datadog` extension)

---

## Key Design Decisions

### Why `HOST_IP` pattern instead of ClusterIP service?

The DDOT collector runs as a DaemonSet inside the Datadog Agent. Apps must send OTLP to the **node-local** agent via the host IP, not a ClusterIP service (which would load-balance across nodes and break the host-affinity model). We use the same pattern on Cluster B for consistency:

```yaml
env:
  - name: HOST_IP
    valueFrom:
      fieldRef:
        fieldPath: status.hostIP
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://$(HOST_IP):4317"
```

> **Critical:** `HOST_IP` must be defined **before** `OTEL_EXPORTER_OTLP_ENDPOINT` in the env list. Kubernetes interpolates `$(VAR)` in declaration order. If `HOST_IP` comes after, the literal string `$(HOST_IP)` is used and gRPC fails with `produced zero addresses`.

### Why two Helm releases for OTel Contrib?

- **DaemonSet**: Collects node-level telemetry (OTLP from apps, kubelet stats, host metrics, container logs). Must run on every node.
- **Deployment**: Collects cluster-level telemetry (k8s_cluster metrics, Kubernetes events). Only needs one replica. Keeping these separate avoids duplicating cluster-level data across nodes.

### Why disable the Datadog Operator?

The `datadog/datadog` Helm chart v3.x bundles the Datadog Operator as a sub-chart dependency. Since we use the plain Helm chart approach (not operator CRDs), it must be disabled with `datadog.operator.enabled: false`.

### GKE COS and system-probe

GKE uses Container-Optimized OS (COS) which requires:
- `providers.gke.cos: true` — enables proper eBPF mounts (debugfs, tracefs, bpffs) and capabilities
- `systemProbe.enableDefaultKernelHeadersPaths: false` — COS doesn't expose `/usr/src` kernel headers the standard way

Without these, the system-probe container fails with `mkdir /usr/src: read-only file system`.

---

## Teardown

```bash
./scripts/teardown.sh
```

This removes all Kubernetes resources (Helm releases, namespaces, deployments) from both clusters. **GKE clusters are NOT deleted.**

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| Pods crash with `exec format error` | arm64 image on amd64 node | Rebuild with `docker buildx build --platform linux/amd64` |
| Java pods killed by liveness probe | Spring Boot + OTel agent slow startup (~32s) | Use `startupProbe` with `failureThreshold: 30, periodSeconds: 5` |
| Go service: `produced zero addresses` | `HOST_IP` defined after `OTEL_EXPORTER_OTLP_ENDPOINT` | Move `HOST_IP` to first position in env list |
| `-oss` traces missing in Datadog | OTLP endpoint not resolving (same env ordering bug) | Check `kubectl exec deploy/transaction-service -- printenv OTEL_EXPORTER_OTLP_ENDPOINT` — should show an IP, not `$(HOST_IP)` |
| system-probe `CreateContainerError` | GKE COS missing eBPF config | Add `providers.gke.cos: true` and `systemProbe.enableDefaultKernelHeadersPaths: false` |
| Datadog Operator running unexpectedly | Chart v3.x bundles operator by default | Add `datadog.operator.enabled: false` in Helm values |
| OTel Collectors not in Fleet Automation | Missing `datadog` extension | Add `extensions.datadog` config with API key and `deployment_type` |
| Python `ModuleNotFoundError: pkg_resources` | OTel instrumentation 0.48b0 + Python 3.12-slim | Upgrade to `opentelemetry-instrumentation-flask==0.51b0` + add `setuptools` to requirements |
| Docker Hub CDN serving stale images | `latest` tag cached in regional CDN | Use a unique tag (e.g., `:v5`) instead of `:latest` |

---

## Repository Structure

```
otel-showcase/
├── .env.example                         # Credential template
├── .gitignore                           # Excludes .env
├── README.md                            # Project overview
├── SETUP_GUIDE.md                       # This file
├── services/
│   ├── account-service/                 # Java (Spring Boot 3 + OTel Java Agent)
│   │   ├── Dockerfile                   # Multi-stage: Maven build + JRE runtime
│   │   ├── pom.xml
│   │   └── src/main/java/...
│   ├── transaction-service/             # Go 1.22 (OTel SDK manual init)
│   │   ├── Dockerfile
│   │   ├── go.mod
│   │   ├── main.go
│   │   └── otel.go                      # TracerProvider + MeterProvider + LoggerProvider
│   ├── fraud-detection-service/         # Python 3.12 (Flask + OTel SDK)
│   │   ├── Dockerfile
│   │   ├── requirements.txt
│   │   └── app.py
│   └── notification-service/            # Java (Spring Boot 3 + OTel Java Agent)
│       ├── Dockerfile
│       ├── pom.xml
│       └── src/main/java/...
├── cluster-a-ddot/                      # Cluster A manifests
│   ├── datadog-values.yaml              # Helm values: DDOT + DCA + all features
│   ├── namespace.yaml
│   ├── account-service.yaml
│   ├── transaction-service.yaml
│   ├── fraud-detection-service.yaml
│   ├── notification-service.yaml
│   └── k6-deployment.yaml
├── cluster-b-otelcontrib/               # Cluster B manifests
│   ├── daemonset-values.yaml            # OTel Collector DaemonSet + datadog extension
│   ├── deployment-values.yaml           # OTel Collector Deployment + datadog extension
│   ├── namespace.yaml
│   ├── account-service.yaml
│   ├── transaction-service.yaml
│   ├── fraud-detection-service.yaml
│   ├── notification-service.yaml
│   └── k6-deployment.yaml
├── k6/
│   └── script.js                        # Standalone k6 script
└── scripts/
    ├── setup-secrets.sh
    ├── build-push.sh
    ├── deploy-cluster-a.sh
    ├── deploy-cluster-b.sh
    └── teardown.sh
```
