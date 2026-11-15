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

Approximately ****5 hours****.

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

curl -i -X POST http://localhost:8081/isolation/enable \\

  -H "Content-Type: application/json" \\

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

curl -i -X POST http://localhost:8081/isolation/disable \\

  -H "Content-Type: application/json" \\

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

curl -sS -X POST http://localhost:8081/hardening/preview \\

  -H "Content-Type: application/json" \\

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

curl -sS -X POST http://localhost:8081/hardening/apply \\

  -H "Content-Type: application/json" \\

  -d '{

    "namespaces": [

      "hardening-demo-a",

      "hardening-demo-b"

    ]

  }' | jq .

```

A single namespace is also supported:

```bash

curl -sS -X POST http://localhost:8081/hardening/preview \\

  -H "Content-Type: application/json" \\

  -d '{"namespace":"hardening-demo-a"}' | jq .

```

Kubeguard applies the following defaults only when they are missing:

\| Setting | Default |

\| --- | --- |

\| CPU request | `100m` |

\| CPU limit | `500m` |

\| Memory request | `128Mi` |

\| Memory limit | `256Mi` |

\| `allowPrivilegeEscalation` | `false` |

\| Dropped Linux capabilities | `ALL` |

\| Seccomp profile | `RuntimeDefault` |

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

This is an assignment-scale, on-demand operator API rather than a production-ready Kubernetes controller. The constraints below are intentional or documented so operators can understand the safety boundaries.

### Network isolation

- **Existing NetworkPolicies:** Enabling isolation is rejected if *any* NetworkPolicy already exists in the cluster. Kubernetes NetworkPolicies are additive: an existing allow policy can make a new restrictive policy ineffective. Rejecting the request is safer than claiming isolation that cannot be guaranteed, but this makes the current implementation unsuitable for many production clusters.
- **One active configuration per namespace:** The managed NetworkPolicy has a fixed name (`kubeguard-isolation`), and each group belongs to one namespace. Overlapping or concurrent isolation requests are not supported.
- **No reconciliation:** The service does not persist desired isolation state or continuously repair policies that are edited or deleted outside Kubeguard. Kubernetes itself automatically applies existing NetworkPolicy selectors to newly created matching Pods; Kubeguard does not need to watch Pods for that behavior.
- **Best-effort rollback:** If the second policy creation fails, Kubeguard attempts to remove the first. If rollback also fails, operator intervention may be required.
- **CNI dependency:** Isolation only works when the cluster's CNI enforces NetworkPolicies. The reproducible test environment uses Calico v3.33.0.

### Workload hardening

- **Broad targeting:** Hardening processes supported controllers in the requested namespaces rather than selecting individual workloads. Supported resources are Deployments, StatefulSets, and DaemonSets, including init containers; standalone Pods, Jobs, and CronJobs are excluded.
- **No built-in undo:** Hardening changes Pod templates and triggers rollouts. Operators can restore manifests from version control or use rollout history where available. A future undo feature would record the original Pod template and resource version, then use optimistic concurrency checks to avoid overwriting later changes.
- **Not transactional:** Namespace validation happens before updates, but an API error partway through a multi-workload or multi-namespace apply can leave earlier workloads modified.
- **Compatibility risks:** Dropping capabilities, enforcing seccomp, or setting resource limits can break applications. The tool preserves existing settings and does not force `runAsNonRoot`, but defaults must still be reviewed against each workload.
- **Demo sizing:** CPU and memory defaults demonstrate the mechanism; they are not production sizing recommendations. Hardening applies only when explicitly requested, so newly deployed workloads are not automatically hardened.

### Operations and availability

- **Unauthenticated HTTP API:** The example relies on local port-forwarding. Do not expose the service publicly. Production use requires authenticated callers, authorization, and audit logging.
- **Single replica:** The current design does not coordinate simultaneous operations across replicas. It has no PodDisruptionBudget (PDB), so availability during voluntary node maintenance is not protected by a PDB.
- **Manual recovery:** Kubernetes API failures, unexpected policy state, and failed workload rollouts may require inspection and intervention. There is no persistent operation journal or automated recovery controller.

## What I'd Do With More Time

In priority order:

1. **Make isolation compatible with real clusters.** Evaluate existing NetworkPolicies and their effective allow rules, detect conflicting policies, and either prove the requested isolation is enforceable or return an actionable conflict. Support multiple independent isolation operations with unique policy names and durable operation IDs.
2. **Add reconciliation and recovery.** Persist desired isolation state, reconcile drift after restarts or manual changes, and record operation status. Use concurrency control so simultaneous requests cannot silently overwrite each other. `controller-runtime` or Kubebuilder would be useful if this evolves into a Kubernetes controller; for this bounded on-demand API, direct `client-go` calls kept the implementation smaller.
3. **Make hardening reversible and safer.** Save original Pod templates, add `/hardening/undo` with resource-version checks, support workload selectors, validate admission and policy constraints, and optionally apply changes in stages with rollout-health checks.
4. **Improve API security and observability.** Add authentication, least-privilege authorization, structured audit events, metrics, request tracing, and clearer operation status for partial failures.
5. **Improve availability.** Add a PDB to protect against voluntary Pod evictions. For higher availability, move toward multiple replicas with `minAvailable: 1`, but first implement operation coordination or leader election. A PDB does not prevent involuntary node failures or guarantee uptime.
6. **Expand testing and packaging.** Add fault-injection tests for API conflicts and partial multi-namespace updates, policy drift and restarts, and end-to-end tests for newly created matching Pods and more workload controller types. Add CI to run automated checks and a Helm chart for configurable deployment.

## Verification and Why These Tests

The tests combine fast, deterministic Go tests with real Kubernetes integration tests. Unit tests validate request handling and failure paths; end-to-end tests confirm behavior that cannot be proven by inspecting generated manifests alone.

### End-to-end verification

After deploying the sample workloads and starting `make port-forward` in another terminal, run:

```bash
make verify
```

The network verification script (`scripts/verify-isolation.sh`) checks **nine traffic outcomes** across three stages:

| Stage | Frontend → backend | Backend → frontend | Frontend → unrelated database |
| --- | --- | --- | --- |
| Before isolation | Allowed | Allowed | Allowed |
| During isolation | Blocked | Blocked | Allowed |
| After disabling | Allowed | Allowed | Allowed |

**Why:** A NetworkPolicy can be syntactically valid yet fail to enforce the intended behavior because of CNI support or rule semantics. Testing actual Pod-to-Service traffic in both directions proves the isolation effect; testing the unrelated database guards against overly broad denial; testing after disable verifies restoration.

The hardening verification script (`scripts/verify-hardening.sh`) checks that sample workloads initially lack the defaults, preview proposes changes **without mutation**, apply updates workloads in **two namespaces**, the resulting Deployments roll out successfully, a second apply is **idempotent**, and a protected-namespace request is rejected **without partially mutating another namespace**.

**Why:** Hardening is operationally risky: a preview must not change running workloads, retries must not continually rewrite Pod templates, and invalid multi-namespace requests must fail before changing anything.

The full `make verify` suite passed in a newly created kind cluster using Calico v3.33.0. This verifies the core behavior independently of the development cluster.

### Automated Go tests

```bash
make test
go vet ./...
```

The Go tests exercise NetworkPolicy construction, enable/disable behavior, failure handling, hardening inspection and application, and multi-namespace API validation.

### Tricky case: partial network isolation failure

The test **`TestEnableRollsBackWhenSecondPolicyFails`** simulates the first NetworkPolicy being created successfully while creation of the second fails. Bidirectional isolation requires *both* policies; leaving just one can cause asymmetric connectivity and surprise operators. The test checks that Kubeguard attempts to delete the first policy rather than leave a half-applied operation.

The companion test **`TestEnableReportsRollbackFailure`** simulates cleanup failing as well and checks that the failure is surfaced rather than hidden.

**Why I chose these tests:** Kubernetes API operations across namespaces are not atomic. Partial failure is realistic, easy to miss on a happy-path-only implementation, and especially consequential for a tool that changes network access during an incident. The tests focus on what happens when recovery itself goes wrong, not just when everything succeeds.

## Assignment Feedback

The exercise provided a useful balance of Kubernetes networking, workload security, Go development, and operational reasoning. The most challenging part was verifying actual NetworkPolicy enforcement in a local cluster rather than relying only on unit tests.

One ambiguity is how isolation should interact with pre-existing NetworkPolicies. Since Kubernetes NetworkPolicies are additive, a new restrictive policy cannot revoke traffic already allowed by another policy. I chose to reject requests when any policy already exists rather than claim isolation that may not be effective. A production implementation would need more nuanced conflict detection and policy compatibility.

Another design choice is the scope of hardening. I interpreted the requirement as applying missing settings to supported workload controllers in one or more specified namespaces, with a non-mutating preview. More granular workload targeting and undo would be useful extensions.

I prioritized a reproducible, tested core over optional features within the time available.
