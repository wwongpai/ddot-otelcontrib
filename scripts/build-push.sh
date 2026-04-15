#!/usr/bin/env bash
# Run chmod +x scripts/*.sh before using
set -euo pipefail

# ---------------------------------------------------------------------------
# build-push.sh — Build and push all BankFlow service images for linux/amd64.
#
# NOTE: Requires docker buildx with linux/amd64 builder.
#       If you haven't already, run:  docker buildx create --use
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

REGISTRY="docker.io/wwongpai"

echo "==> Building and pushing BankFlow images to ${REGISTRY}"
echo "    Platform: linux/amd64"
echo ""

# ---------------------------------------------------------------------------
# account-service (Java — requires Maven build first)
# ---------------------------------------------------------------------------
echo "==> [1/4] account-service (Java)"
cd "${PROJECT_ROOT}/services/account-service"
mvn clean package -DskipTests -q
docker buildx build --platform linux/amd64 \
  -t "${REGISTRY}/account-service:latest" \
  --push .
cd "${PROJECT_ROOT}"

# ---------------------------------------------------------------------------
# transaction-service (Go)
# ---------------------------------------------------------------------------
echo "==> [2/4] transaction-service (Go)"
cd "${PROJECT_ROOT}/services/transaction-service"
docker buildx build --platform linux/amd64 \
  -t "${REGISTRY}/transaction-service:latest" \
  --push .
cd "${PROJECT_ROOT}"

# ---------------------------------------------------------------------------
# fraud-detection-service (Node.js)
# ---------------------------------------------------------------------------
echo "==> [3/4] fraud-detection-service (Node.js)"
cd "${PROJECT_ROOT}/services/fraud-detection-service"
docker buildx build --platform linux/amd64 \
  -t "${REGISTRY}/fraud-detection-service:latest" \
  --push .
cd "${PROJECT_ROOT}"

# ---------------------------------------------------------------------------
# notification-service (Java — requires Maven build first)
# ---------------------------------------------------------------------------
echo "==> [4/4] notification-service (Java)"
cd "${PROJECT_ROOT}/services/notification-service"
mvn clean package -DskipTests -q
docker buildx build --platform linux/amd64 \
  -t "${REGISTRY}/notification-service:latest" \
  --push .
cd "${PROJECT_ROOT}"

echo ""
echo "SUCCESS: All images pushed."
echo ""
echo "  ${REGISTRY}/account-service:latest"
echo "  ${REGISTRY}/transaction-service:latest"
echo "  ${REGISTRY}/fraud-detection-service:latest"
echo "  ${REGISTRY}/notification-service:latest"
