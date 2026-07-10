# 08 — Deployment

Kubernetes manifests live in a **separate repo**. This repo produces a container image
and documents what the deploy repo must provide.

## Container

- Multi-stage `Dockerfile`. Build stage: `golang:1.26` (or matching), `CGO_ENABLED=0`,
  static binary, `-ldflags "-s -w"`, `-trimpath`. Import `_ "time/tzdata"` so timezone
  data is embedded (no OS tzdata in the runtime image).
- Runtime stage: `gcr.io/distroless/static:nonroot`. Runs as **non-root**, read-only
  root filesystem friendly (no disk writes — tokens go to the K8s Secret).
- Single binary with subcommands: default `CMD ["serve"]`; `mock` also available.
- Default env in the image: `TOKEN_STORE=kube`.
- Exposes the health port (`HEALTH_ADDR`, default `:8080`).

## Token persistence (stateless-friendly)

The deployment is stateless (no PVC). Tokens round-trip through a **Kubernetes Secret
the pod mutates**, so etcd is the source of truth and restarts avoid re-login (EU login
is itself rate-limited).

- `TokenStore` interface: `Load(ctx) (Tokens, bool, error)`, `Save(ctx, Tokens) error`.
- **`kubeSecretStore`** (prod): reads the named Secret on startup; patches it after
  each token refresh. In-cluster auth via the mounted ServiceAccount token + CA
  (`rest.InClusterConfig` / `client-go`). Namespace from
  `/var/run/secrets/kubernetes.io/serviceaccount/namespace`. Secret name from
  `TOKEN_SECRET_NAME`.
  - Optimistic concurrency: use the Secret's `resourceVersion` on update; retry **once**
    on a `409 Conflict`. Single replica ⇒ effectively single-writer.
  - The Secret is **created empty by the deploy repo**; the app is the writer.
- **`memoryStore`** (dev/godog): in-process only.
- Selected by `TOKEN_STORE`.

Stored token payload: `access_token`, `refresh_token`, `device_id`, `valid_until`
(RFC3339). Keys in the Secret's `data` map (base64 per K8s).

## RBAC (deploy repo provides)

A `Role` granting `get`, `update`, `patch` (and optionally `list`/`watch` not needed)
on the single named Secret, bound via a `RoleBinding` to the pod's `ServiceAccount`.
Least privilege: scoped to that one Secret's name, not all secrets.

```
verbs: [get, update, patch]  resources: [secrets]  resourceNames: [<TOKEN_SECRET_NAME>]
```

## Runtime expectations for the deploy repo

- `replicas: 1` (single writer for the token Secret).
- Liveness probe → `/healthz`; readiness probe → `/readyz` with generous
  `failureThreshold`/`periodSeconds` given the 30m poll cadence (see `06`).
- Env/Secret refs for `BLUELINK_*`, `MQTT_*`, `TOKEN_SECRET_NAME`.
- `terminationGracePeriodSeconds` long enough for the explicit `offline` publish +
  clean MQTT disconnect (a few seconds is ample).
- ServiceAccount with the Role above; automount the SA token (default).

## CI / verification

- `go build ./...`, `go vet ./...`, `go test ./...`, `go test ./features/...`.
- `/spec-reconcile` reports every `REQ-*` implemented with no untraceable code.
