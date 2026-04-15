#!/usr/bin/env bash
# Run chmod +x scripts/*.sh before using
set -euo pipefail

# ---------------------------------------------------------------------------
# deploy-cluster-b.sh — Deploy OTel Collector Contrib and BankFlow services
# to Cluster B: warach-otel-sdk-otelcontrib
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
ENV_FILE="${PROJECT_ROOT}/.env"

if [[ ! -f "${ENV_FILE}" ]]; then
  echo "ERROR: .env file not found at ${ENV_FILE}"
  echo "       Copy .env.example to .env and fill in your credentials."
  exit 1
fi

# shellcheck source=../.env
source "${ENV_FILE}"

: "${DD_API_KEY:?ERROR: DD_API_KEY is not set in .env}"

CONTEXT="gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-otelcontrib"

echo "==> Switching to Cluster B context: ${CONTEXT}"
kubectl config use-context "${CONTEXT}"

# ---------------------------------------------------------------------------
# Helm — OpenTelemetry Collector
# ---------------------------------------------------------------------------
echo "==> Adding OpenTelemetry Helm repository..."
helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts
helm repo update

# ---------------------------------------------------------------------------
# Namespaces
# ---------------------------------------------------------------------------
echo "==> Applying namespace manifests..."
kubectl apply -f "${PROJECT_ROOT}/cluster-b-otelcontrib/namespace.yaml"

# ---------------------------------------------------------------------------
# OTel Collector DaemonSet (node-level: OTLP recv, logs, kubelet, host metrics)
# ---------------------------------------------------------------------------
echo "==> Deploying OTel Collector DaemonSet..."
helm upgrade --install otel-collector-daemonset open-telemetry/opentelemetry-collector \
  -n otel-system \
  -f "${PROJECT_ROOT}/cluster-b-otelcontrib/daemonset-values.yaml" \
  --wait --timeout 5m

# ---------------------------------------------------------------------------
# OTel Collector Deployment (cluster-level: k8s_cluster + k8sobjects)
# ---------------------------------------------------------------------------
echo "==> Deploying OTel Collector Deployment (cluster-level)..."
helm upgrade --install otel-collector-deployment open-telemetry/opentelemetry-collector \
  -n otel-system \
  -f "${PROJECT_ROOT}/cluster-b-otelcontrib/deployment-values.yaml" \
  --wait --timeout 5m

# ---------------------------------------------------------------------------
# BankFlow application manifests
# ---------------------------------------------------------------------------
echo "==> Deploying BankFlow application manifests..."
kubectl apply -f "${PROJECT_ROOT}/cluster-b-otelcontrib/account-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-b-otelcontrib/transaction-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-b-otelcontrib/fraud-detection-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-b-otelcontrib/notification-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-b-otelcontrib/k6-deployment.yaml"

# ---------------------------------------------------------------------------
# Wait for rollouts
# ---------------------------------------------------------------------------
echo "==> Waiting for deployments to roll out..."
kubectl rollout status deployment/account-service -n bankflow --timeout=5m
kubectl rollout status deployment/transaction-service -n bankflow --timeout=5m
kubectl rollout status deployment/fraud-detection-service -n bankflow --timeout=5m
kubectl rollout status deployment/notification-service -n bankflow --timeout=5m
kubectl rollout status deployment/k6-loadgen -n bankflow --timeout=5m

echo ""
echo "SUCCESS: Cluster B (OTel Contrib) is fully deployed."
echo ""
echo "Next steps:"
echo "  1. Check Datadog APM → Service Map (filter env:otel-demo)"
echo "  2. Look for service names ending in '-oss'"
echo "  3. Verify traces, metrics, and logs are flowing into Datadog"
echo "  4. Compare side-by-side with Cluster A in the APM Service Map"
