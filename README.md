# reflector

A lightweight **Go** reimplementation of
[emberstack/kubernetes-reflector](https://github.com/emberstack/kubernetes-reflector).

It watches `Secrets`, `ConfigMaps` and `Namespaces` cluster-wide and mirrors
resources across namespaces based on the exact same
`reflector.v1.k8s.emberstack.com/*` annotation contract — so it is a **drop-in
replacement** for the original image, with a fraction of the memory footprint.

| | original (.NET) | this (Go) |
|---|---|---|
| Idle RSS | ~300 MB | < 20 MB |
| Base image | `mcr.microsoft.com/dotnet/aspnet` | `distroless/static:nonroot` |

This is a clean-room implementation: the behaviour (annotation contract, matching
semantics, reflection lifecycle, health endpoints and the environment variables the
Helm chart injects) was reproduced so existing source/mirror objects and existing
deployments keep working without any migration.

## Compatibility

Fully compatible with the original:

* **Annotations** — identical prefix and names (see below).
* **Matching** — `*-namespaces` are comma-separated **regular expressions** that
  must match the whole namespace name; `*-namespaces-selector` are standard
  Kubernetes label selectors. Name pattern OR label selector; both empty = all.
* **Resources** — `Secret` (mirrors `data`) and `ConfigMap` (mirrors `data` +
  `binaryData`). Helm release secrets (`type=helm.sh/release.v1`) are ignored.
* **Health** — `GET /health/live` and `GET /health/ready` on container port `8080`.
* **Config** — reads the same `ES_*` environment variables the chart sets.

### Annotations

Set on the **source** Secret/ConfigMap:

| Annotation | Meaning |
|---|---|
| `reflector.v1.k8s.emberstack.com/reflection-allowed` | `"true"` to permit reflection |
| `reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces` | regex list of namespaces allowed to mirror |
| `reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces-selector` | label selector of allowed namespaces |
| `reflector.v1.k8s.emberstack.com/reflection-auto-enabled` | `"true"` to auto-create mirrors |
| `reflector.v1.k8s.emberstack.com/reflection-auto-namespaces` | regex list of namespaces to auto-mirror to |
| `reflector.v1.k8s.emberstack.com/reflection-auto-namespaces-selector` | label selector for auto-mirror targets |

Set on a **mirror** (or written by reflector):

| Annotation | Meaning |
|---|---|
| `reflector.v1.k8s.emberstack.com/reflects` | `"namespace/name"` of the source (manual mirrors) |
| `reflector.v1.k8s.emberstack.com/auto-reflects` | `True`/`False` — written by reflector |
| `reflector.v1.k8s.emberstack.com/reflected-version` | source resourceVersion at last sync — written by reflector |
| `reflector.v1.k8s.emberstack.com/reflected-at` | timestamp of last sync — written by reflector |

#### Example — automatic reflection

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tls-wildcard
  namespace: cert-manager
  annotations:
    reflector.v1.k8s.emberstack.com/reflection-allowed: "true"
    reflector.v1.k8s.emberstack.com/reflection-allowed-namespaces: "team-.*"
    reflector.v1.k8s.emberstack.com/reflection-auto-enabled: "true"
    reflector.v1.k8s.emberstack.com/reflection-auto-namespaces: "team-.*"
type: kubernetes.io/tls
data: { ... }
```

#### Example — manual reflection

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: tls-wildcard
  namespace: team-blue
  annotations:
    reflector.v1.k8s.emberstack.com/reflects: "cert-manager/tls-wildcard"
type: kubernetes.io/tls
```

## Configuration

| Environment variable | Default | Description |
|---|---|---|
| `ES_Serilog__MinimumLevel__Default` | `Information` | Log level (`Verbose`/`Debug`/`Information`/`Warning`/`Error`) |
| `ES_Reflector__Watcher__Timeout` | `3600` | Watch/resync period in seconds |
| `ES_Reflector__Watcher__ExcludedNamespaces` | _(empty)_ | Comma-separated glob list (`*`, `?`) of namespaces to ignore |
| `ES_Ignite__KubernetesClient__SkipTlsVerify` | `false` | Skip API server TLS verification |

## Install

### Helm (chart published to DockerHub as an OCI artifact)

```bash
helm install reflector oci://registry-1.docker.io/pharndt/reflector \
  --namespace reflector --create-namespace
```

### Use the original emberstack chart, just swap the image

Because the annotation contract, env vars, health paths and port are identical, you
can keep your existing release and only override the image:

```yaml
image:
  repository: docker.io/pharndt/reflector
  tag: "v0.1.0"
```

## Architecture

* `config` — environment-variable configuration (drop-in `ES_*` names).
* `reflector` — the controller:
  * `annotations.go` / `properties.go` — the annotation contract.
  * `match.go` / `glob.go` — namespace regex, label selectors, exclusion globs.
  * `secret.go` / `configmap.go` — typed backends (`Backend` interface).
  * `controller.go` — informer + workqueue reconciliation (source → mirrors,
    mirror → restore/delete, namespace events → re-evaluate sources).
  * `health.go` / `kube.go` — health endpoints and the in-cluster client.

It uses `client-go` shared informers with `managedFields` stripping and a field
selector that keeps helm release secrets out of the cache, which keeps the
in-memory footprint small.

## Development

```bash
cd app
make build      # build the binary
make test       # run the unit + fake-cluster tests
make run        # run against your current kubeconfig (out-of-cluster)
make docker     # build the container image
```

Releases are cut with GoReleaser (multi-arch images + the Helm chart pushed to
DockerHub) via the `Build release` workflow.
