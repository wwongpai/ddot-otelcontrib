#!/usr/bin/env bash
# Run chmod +x scripts/*.sh before using
set -euo pipefail

# ---------------------------------------------------------------------------
# teardown.sh — Remove all BankFlow Kubernetes resources from both clusters.
#
# NOTE: GKE clusters are NOT deleted. Only Kubernetes resources are removed.
# ---------------------------------------------------------------------------

echo "WARNING: This will remove all BankFlow resources from both clusters."
echo "         Press Ctrl+C to cancel."
sleep 5

# ---------------------------------------------------------------------------
# Cluster A — DDOT
# ---------------------------------------------------------------------------
echo "==> Tearing down Cluster A (DDOT)..."
kubectl config use-context gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-ddot

helm uninstall datadog-agent -n datadog || true

kubectl delete namespace bankflow --ignore-not-found
kubectl delete namespace datadog --ignore-not-found

# ---------------------------------------------------------------------------
# Cluster B — OTel Contrib
# ---------------------------------------------------------------------------
echo "==> Tearing down Cluster B (OTel Contrib)..."
kubectl config use-context gke_datadog-ese-sandbox_asia-southeast1-a_warach-otel-sdk-otelcontrib

helm uninstall otel-collector-daemonset -n otel-system || true
helm uninstall otel-collector-deployment -n otel-system || true

kubectl delete namespace bankflow --ignore-not-found
kubectl delete namespace otel-system --ignore-not-found

echo ""
echo "SUCCESS: All BankFlow Kubernetes resources have been removed."
echo ""
echo "NOTE: GKE clusters are NOT deleted. Only Kubernetes resources were removed."
echo "      To redeploy, run setup-secrets.sh followed by deploy-cluster-a.sh"
echo "      and deploy-cluster-b.sh."
