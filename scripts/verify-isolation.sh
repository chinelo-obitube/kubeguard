
#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8081}"
TIMEOUT="${TIMEOUT:-3}"

ISOLATION_REQUEST='{
  "groupA": {
    "namespace": "frontend",
    "selector": {"run": "client"}
  },
  "groupB": {
    "namespace": "backend",
    "selector": {"app": "test-server"}
  }
}'

cleanup() {
  echo
  echo "Cleaning up Kubeguard isolation policies..."
  curl --silent --show-error \
    --max-time 15 \
    -X POST \
    -H "Content-Type: application/json" \
    -d "$ISOLATION_REQUEST" \
    "$API_URL/isolation/disable" >/dev/null || true
}

trap cleanup EXIT

check_http() {
  local source_namespace="$1"
  local source_pod="$2"
  local destination="$3"
  local expected="$4"
  local actual

  actual="$(
    kubectl exec -n "$source_namespace" "$source_pod" -- \
      curl --silent --output /dev/null \
      --max-time "$TIMEOUT" \
      --write-out '%{http_code}' \
      "http://$destination:5678" 2>/dev/null
  )" || true

  if [[ "$expected" == "allowed" && "$actual" == "200" ]]; then
    echo "PASS: $source_namespace/$source_pod -> $destination allowed"
  elif [[ "$expected" == "blocked" && "$actual" != "200" ]]; then
    echo "PASS: $source_namespace/$source_pod -> $destination blocked"
  else
    echo "FAIL: $source_namespace/$source_pod -> $destination"
    echo "      expected=$expected actual=${actual:-no response}"
    exit 1
  fi
}

echo "Checking Kubeguard API..."
curl --fail --silent --show-error "$API_URL/readyz" >/dev/null

echo "Waiting for sample Pods..."
for target in \
  "frontend/client" \
  "frontend/frontend-server" \
  "backend/backend-client" \
  "backend/test-server" \
  "database/database-server"
do
  namespace="${target%%/*}"
  pod="${target#*/}"

  kubectl wait -n "$namespace" \
    --for=condition=Ready \
    "pod/$pod" \
    --timeout=180s
done

echo
echo "1. Baseline connectivity"
check_http frontend client test-server.backend.svc.cluster.local allowed
check_http backend backend-client frontend-server.frontend.svc.cluster.local allowed
check_http frontend client database-server.database.svc.cluster.local allowed

echo
echo "2. Enabling bidirectional isolation"
curl --fail --silent --show-error \
  -X POST \
  -H "Content-Type: application/json" \
  -d "$ISOLATION_REQUEST" \
  "$API_URL/isolation/enable" >/dev/null

echo
echo "3. Connectivity during isolation"
check_http frontend client test-server.backend.svc.cluster.local blocked
check_http backend backend-client frontend-server.frontend.svc.cluster.local blocked
check_http frontend client database-server.database.svc.cluster.local allowed

echo
echo "4. Disabling isolation"
curl --fail --silent --show-error \
  -X POST \
  -H "Content-Type: application/json" \
  -d "$ISOLATION_REQUEST" \
  "$API_URL/isolation/disable" >/dev/null

echo
echo "5. Restored connectivity"
check_http frontend client test-server.backend.svc.cluster.local allowed
check_http backend backend-client frontend-server.frontend.svc.cluster.local allowed
check_http frontend client database-server.database.svc.cluster.local allowed

trap - EXIT

echo
echo "PASS: All network isolation checks completed successfully."
