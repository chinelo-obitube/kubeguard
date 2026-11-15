
# Kubeguard

Kubeguard is a Go-based Kubernetes SRE tool that provides on-demand network isolation and workload security hardening.

## Features

- **Bidirectional network isolation:** Isolate two groups of Pods identified by namespace and label selector, while preserving unrelated traffic.
- **Workload hardening:** Preview and apply missing resource requests, limits, and security settings across one or more namespaces.
- **Kubernetes-native:** Uses NetworkPolicies and workload updates through the Kubernetes API.
- **On-demand:** Changes occur when an API endpoint is called; no periodic scanning or reconciliation loop.

## Prerequisites

- Go 1.27.4
- Docker
- kind
- kubectl
- make
- bash
- curl
- jq

## Quick Start

Create a kind cluster:

```bash
make cluster-up
```

Install Calico CNI v3.33.0:

```bash
make install-calico
```

Build and load Kubeguard:

```bash
make build-image
make load-image
```

Deploy Kubeguard and the sample workloads:

```bash
make deploy
make deploy-isolation-samples
make deploy-hardening-samples
```

In a separate terminal, forward the API port:

```bash
make port-forward
```

Run the verification suite:

```bash
make verify
```

Run Go unit tests:

```bash
make test
```

## Development Time

Approximately **5 hours**.



## API Usage

Kubeguard exposes an HTTP API on port `8080` inside the cluster.

After running `make port-forward`, the API is available at
`http://localhost:8081`.

### Health Checks

```bash
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz
```

### Network Isolation

Kubeguard isolates two groups of Pods, each defined by a namespace
and a label selector.

Enable bidirectional isolation:

```bash
curl -i -X POST http://localhost:8081/isolation/enable \
  -H "Content-Type: application/json" \
  -d '{
    "groupA": {
      "namespace": "frontend",
      "selector": {"run": "client"}
    },
    "groupB": {
      "namespace": "backend",
      "selector": {"app": "test-server"}
    }
  }'
```

The service creates a NetworkPolicy in each namespace. Matching
Pods cannot communicate across the two groups in either direction,
while unrelated traffic remains allowed.

Inspect the policies:

```bash
kubectl get networkpolicies -A
kubectl -n frontend describe networkpolicy kubeguard-isolation
kubectl -n backend describe networkpolicy kubeguard-isolation
```

Disable isolation:

```bash
curl -i -X POST http://localhost:8081/isolation/disable \
  -H "Content-Type: application/json" \
  -d '{
    "groupA": {
      "namespace": "frontend",
      "selector": {"run": "client"}
    },
    "groupB": {
      "namespace": "backend",
      "selector": {"app": "test-server"}
    }
  }'
```

Disabling removes Kubeguard's managed policies and restores
connectivity, provided no other NetworkPolicies restrict it.

NetworkPolicies also apply to newly created Pods matching their
selectors. Kubeguard does not need to poll for new Pods.

### Workload Hardening

Kubeguard examines Deployments, StatefulSets, and DaemonSets,
including their init containers.

Preview changes across multiple namespaces:

```bash
curl -sS -X POST http://localhost:8081/hardening/preview \
  -H "Content-Type: application/json" \
  -d '{
    "namespaces": [
      "hardening-demo-a",
      "hardening-demo-b"
    ]
  }' | jq .
```

Preview reports missing settings and proposed changes without
modifying workloads.

Apply hardening:

```bash
curl -sS -X POST http://localhost:8081/hardening/apply \
  -H "Content-Type: application/json" \
  -d '{
    "namespaces": [
      "hardening-demo-a",
      "hardening-demo-b"
    ]
  }' | jq .
```

A single namespace is also supported:

```bash
curl -sS -X POST http://localhost:8081/hardening/preview \
  -H "Content-Type: application/json" \
  -d '{"namespace":"hardening-demo-a"}' | jq .
```

Kubeguard applies the following defaults only when they are missing:

| Setting | Default |
| --- | --- |
| CPU request | `100m` |
| CPU limit | `500m` |
| Memory request | `128Mi` |
| Memory limit | `256Mi` |
| `allowPrivilegeEscalation` | `false` |
| Dropped Linux capabilities | `ALL` |
| Seccomp profile | `RuntimeDefault` |

Kubeguard recommends `runAsNonRoot: true` but does not force it,
because some container images require root to start.

Existing resource values and security settings are preserved.
Applying hardening again is idempotent.

Updating a workload's Pod template triggers a Kubernetes rollout.
Inspect the results:

```bash
kubectl -n hardening-demo-a get deployment demo-app -o yaml
kubectl -n hardening-demo-b get deployment demo-app -o yaml
```

### Logs

```bash
kubectl -n kubeguard-system logs deployment/kubeguard
```



## Architecture and Design Decisions

Kubeguard is a Go HTTP service deployed inside Kubernetes.
It uses a Kubernetes ServiceAccount and RBAC permissions to
manage NetworkPolicies and supported workload resources.

### Network Isolation

- Each group is identified by one namespace and a Pod label selector.
- Enabling isolation creates one NetworkPolicy per namespace.
- Policies restrict communication between the two selected groups
  in both directions while allowing unrelated traffic.
- Kubernetes automatically applies these policies to newly created
  Pods that match the selectors.
- Disabling isolation deletes only policies identified as managed
  by Kubeguard.
- The service validates the request and checks that matching Pods
  exist before enabling isolation.
- If creating the second policy fails, Kubeguard attempts to roll
  back the first policy.

### Workload Hardening

- Hardening is triggered explicitly through the HTTP API.
- Preview inspects workloads without modifying them.
- Apply fills in missing resource and security settings while
  preserving existing values.
- Deployments, StatefulSets, DaemonSets, and init containers
  are supported.
- Multiple namespaces can be processed in one request.
- Namespace validation occurs before applying changes, preventing
  partial changes caused by invalid or protected namespace inputs.
- Repeated application is idempotent.
- Workload updates use the Kubernetes API and trigger normal
  Kubernetes rollouts.

### Safety Decisions

Kubeguard rejects hardening requests targeting these namespaces:

- `kube-system`
- `kube-public`
- `kube-node-lease`
- `kubeguard-system`
- `calico-system`
- `tigera-operator`

Security defaults are intentionally conservative. For example,
`runAsNonRoot: true` is recommended but not automatically applied,
because enforcing it can prevent some container images from starting.

## Known Limitations

### Network Isolation

- Only one namespace per group is currently supported.
- Kubeguard uses a fixed NetworkPolicy name, so it supports only
  one active isolation configuration per namespace.
- Isolation enablement is rejected if any NetworkPolicy already
  exists in the cluster. This is deliberately conservative because
  Kubernetes NetworkPolicies are additive and an existing allow
  policy could undermine the intended isolation.
- Network isolation requires a CNI that enforces NetworkPolicies.
  The verification environment uses Calico v3.33.0.
- Kubeguard does not run a reconciliation loop or persist a
  desired-state configuration.
- Rollback after partial policy creation is best-effort.
- The service is designed for a single replica; it does not
  coordinate isolation operations across multiple replicas.

### Workload Hardening

- Hardening changes may cause workload rollouts or application
  failures if an image depends on capabilities that are removed.
- There is no automatic undo operation. Restore a previous
  workload manifest or use Kubernetes rollout history where
  available.
- Applying changes across namespaces is not transactional.
  An API failure partway through an apply request can leave
  earlier workloads updated.
- Standalone Pods, Jobs, and CronJobs are not hardened.
- The resource defaults are demonstration values, not
  production workload-sizing recommendations.

### Operational Limitations

- The HTTP API does not implement authentication or authorization
  for callers. The sample setup uses local port-forwarding;
  do not expose the API publicly.
- Network isolation and hardening run on demand. Kubeguard does
  not continuously scan workloads or automatically harden new ones.
- Kubernetes API failures and rollout failures require operator
  inspection and possible manual recovery.

## Verification

Run all end-to-end checks against the deployed kind cluster:

```bash
make verify
```

The verification suite checks:

**Network isolation**
- Connectivity succeeds before isolation.
- Traffic between the selected groups is blocked in both directions.
- Traffic to an unrelated namespace remains allowed.
- Connectivity is restored after disabling isolation.

**Workload hardening**
- Sample workloads initially lack hardening defaults.
- Preview reports changes without modifying workloads.
- Apply hardens workloads across two namespaces.
- Updated Deployments successfully roll out.
- Applying the same changes again is idempotent.
- Protected namespaces are rejected without partial mutation.

Both end-to-end verification scripts passed on the development
kind cluster with Calico v3.33.0.

Run Go unit tests:

```bash
make test
```

The unit tests cover policy construction, enable/disable behavior,
rollback handling, hardening inspection and application, and
multi-namespace API validation.

## Assignment Feedback

The exercise combines Kubernetes networking, workload security,
API design, and operational verification. The most challenging
part was validating real NetworkPolicy behavior in a local cluster,
rather than relying only on unit tests.

A useful extension would be persistent isolation configurations
with reconciliation, more precise handling of pre-existing
NetworkPolicies, and a reversible hardening workflow.
