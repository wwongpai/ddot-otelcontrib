#!/usr/bin/env bash
# Run chmod +x scripts/*.sh before using
set -euo pipefail

# ---------------------------------------------------------------------------
# setup-secrets.sh — Create Kubernetes namespaces and Datadog API secrets
# for both GKE clusters.
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
: "${DD_APP_KEY:?ERROR: DD_APP_KEY is not set in .env}"

echo "==> Setting up secrets for Cluster A (DDOT)..."

kubectl config use-context gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-ddot

kubectl create namespace bankflow --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace datadog --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic datadog-secret \
  --from-literal=api-key="${DD_API_KEY}" \
  --from-literal=app-key="${DD_APP_KEY}" \
  -n datadog --dry-run=client -o yaml | kubectl apply -f -

echo "==> Setting up secrets for Cluster B (OTel Contrib)..."

kubectl config use-context gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-otelcontrib

kubectl create namespace bankflow --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace otel-system --dry-run=client -o yaml | kubectl apply -f -
kubectl create secret generic datadog-secret \
  --from-literal=DD_API_KEY="${DD_API_KEY}" \
  -n otel-system --dry-run=client -o yaml | kubectl apply -f -

echo ""
echo "SUCCESS: Namespaces and secrets created on both clusters."
echo "  Cluster A namespaces: bankflow, datadog"
echo "  Cluster B namespaces: bankflow, otel-system"
