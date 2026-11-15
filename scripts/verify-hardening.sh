
#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8081}"
NAMESPACES='{"namespaces":["hardening-demo-a","hardening-demo-b"]}'

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

api_post() {
  local endpoint="$1"
  local payload="$2"

  curl --fail --silent --show-error \
    --max-time 30 \
    -X POST \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "$API_URL$endpoint"
}

get_deployment() {
  kubectl -n "$1" get deployment demo-app -o json
}

# Extract the exact Pod template from the Deployment.
get_template() {
  get_deployment "$1" | jq -c '.spec.template'
}

echo "Checking prerequisites..."
command -v curl >/dev/null || fail "curl is required"
command -v kubectl >/dev/null || fail "kubectl is required"
command -v jq >/dev/null || fail "jq is required"

curl --fail --silent --show-error "$API_URL/readyz" >/dev/null

for namespace in hardening-demo-a hardening-demo-b; do
  kubectl -n "$namespace" get deployment demo-app >/dev/null
done

echo
echo "1. Checking initial workload configuration"

for namespace in hardening-demo-a hardening-demo-b; do
  get_deployment "$namespace" | jq -e '
    .spec.template.spec.containers[0] |
    ((.resources.requests // {}) | length == 0)
  ' >/dev/null || fail "$namespace already has resource requests"

  get_deployment "$namespace" | jq -e '
    .spec.template.spec.containers[0] |
    (.securityContext == null)
  ' >/dev/null || fail "$namespace already has a securityContext"

  echo "PASS: $namespace starts without hardening defaults"
done

echo
echo "2. Previewing hardening across both namespaces"

before_a="$(get_template hardening-demo-a)"
before_b="$(get_template hardening-demo-b)"

preview="$(api_post /hardening/preview "$NAMESPACES")"

echo "$preview" | jq -e '
  any(.[]; .Namespace == "hardening-demo-a") and
  any(.[]; .Namespace == "hardening-demo-b")
' >/dev/null || fail "Preview did not include both namespaces"

[[ "$(get_template hardening-demo-a)" == "$before_a" ]] ||
  fail "Preview modified hardening-demo-a"

[[ "$(get_template hardening-demo-b)" == "$before_b" ]] ||
  fail "Preview modified hardening-demo-b"

echo "PASS: Preview includes both namespaces without modifying workloads"

echo
echo "3. Applying hardening"

api_post /hardening/apply "$NAMESPACES" >/dev/null

for namespace in hardening-demo-a hardening-demo-b; do
  get_deployment "$namespace" | jq -e '
    .spec.template.spec.containers[0] |
    .resources.requests.cpu == "100m" and
    .resources.requests.memory == "128Mi" and
    .resources.limits.cpu == "500m" and
    .resources.limits.memory == "256Mi" and
    .securityContext.allowPrivilegeEscalation == false and
    (.securityContext.capabilities.drop | index("ALL") != null) and
    .securityContext.seccompProfile.type == "RuntimeDefault"
  ' >/dev/null || fail "$namespace is missing hardening defaults"

  kubectl -n "$namespace" rollout status deployment/demo-app --timeout=180s

  echo "PASS: $namespace hardened successfully"
done

echo
echo "4. Checking idempotency"

before_a="$(get_template hardening-demo-a)"
before_b="$(get_template hardening-demo-b)"

api_post /hardening/apply "$NAMESPACES" >/dev/null

[[ "$(get_template hardening-demo-a)" == "$before_a" ]] ||
  fail "Second apply changed hardening-demo-a"

[[ "$(get_template hardening-demo-b)" == "$before_b" ]] ||
  fail "Second apply changed hardening-demo-b"

echo "PASS: Second apply makes no Pod template changes"

echo
echo "5. Checking protected namespace rejection"

# First remove one default from a test workload so we can prove
# the rejected request does not partially harden it.
kubectl -n hardening-demo-a patch deployment demo-app \
  --type=json \
  -p='[{"op":"remove","path":"/spec/template/spec/containers/0/resources"}]' \
  >/dev/null

before_a="$(get_template hardening-demo-a)"

status="$(
  curl --silent --show-error \
    --output /dev/null \
    --write-out '%{http_code}' \
    --max-time 30 \
    -X POST \
    -H "Content-Type: application/json" \
    -d '{"namespaces":["hardening-demo-a","kube-system"]}' \
    "$API_URL/hardening/apply"
)"

[[ "$status" == "400" ]] ||
  fail "Expected HTTP 400 for protected namespace, got $status"

[[ "$(get_template hardening-demo-a)" == "$before_a" ]] ||
  fail "Protected namespace request partially modified hardening-demo-a"

echo "PASS: Protected namespace rejected without partial mutation"

echo
echo "PASS: All multiple-namespace hardening checks completed successfully."
