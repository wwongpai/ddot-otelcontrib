#!/usr/bin/env bash
# Run chmod +x scripts/*.sh before using
set -euo pipefail

# ---------------------------------------------------------------------------
# deploy-cluster-a.sh — Deploy Datadog Agent (DDOT) and BankFlow services
# to Cluster A: warach-otel-sdk-ddot
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

CONTEXT="gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-ddot"

echo "==> Switching to Cluster A context: ${CONTEXT}"
kubectl config use-context "${CONTEXT}"

# ---------------------------------------------------------------------------
# Helm — Datadog Agent with DDOT
# ---------------------------------------------------------------------------
echo "==> Adding Datadog Helm repository..."
helm repo add datadog https://helm.datadoghq.com
helm repo update

echo "==> Deploying Datadog Agent (DDOT) via Helm..."
helm upgrade --install datadog-agent datadog/datadog \
  -n datadog \
  -f "${PROJECT_ROOT}/cluster-a-ddot/datadog-values.yaml" \
  --wait --timeout 5m

# ---------------------------------------------------------------------------
# BankFlow application manifests
# ---------------------------------------------------------------------------
echo "==> Deploying BankFlow application manifests..."
kubectl apply -f "${PROJECT_ROOT}/cluster-a-ddot/namespace.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-a-ddot/account-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-a-ddot/transaction-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-a-ddot/fraud-detection-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-a-ddot/notification-service.yaml"
kubectl apply -f "${PROJECT_ROOT}/cluster-a-ddot/k6-deployment.yaml"

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
echo "SUCCESS: Cluster A (DDOT) is fully deployed."
echo ""
echo "Next steps:"
echo "  1. Check Datadog APM → Service Map (filter env:otel-demo)"
echo "  2. Look for service names ending in '-ddot'"
echo "  3. Explore Infrastructure → Containers for live container data"
echo "  4. Check Network Performance Monitoring for east-west traffic"
echo "  5. Open K8s Explorer to browse cluster resources"
