# alloy-promtail

[Grafana Alloy](https://github.com/grafana/alloy) built for log collection
only. It covers what promtail did — tail files and the journal, receive
syslog/GELF/push API, discover targets, process and ship to Loki — using the
Alloy engine and configuration syntax.

The upstream binary carries the OpenTelemetry Collector, Prometheus, profiling,
eBPF, cloud SDKs and a web UI. alloy-promtail links none of that:

| Build (linux/amd64, stripped) | Size |
|---|---|
| Grafana Alloy upstream | 389.5 MiB |
| alloy-promtail `base` | 41.7 MiB |
| `base` + `secretfilter` | 50.2 MiB |
| `base` + `cloud` | 58.6 MiB |
| `base` + `k8s` | 74.2 MiB |
| all groups | 99.0 MiB |

Upstream code is never edited; the build applies an overlay at compile time,
so syncing with `grafana/alloy` is a plain `git merge`.

## Quick start

```sh
make -C promtail build
LOKI_URL=http://loki:3100/loki/api/v1/push \
  build/promtail/alloy-promtail run promtail/example-config.alloy --storage.path=./data
```

```sh
make -C promtail image IMAGE=alloy-promtail:dev
docker run -d -v /var/log:/var/log:ro -v alloy-promtail-data:/var/lib/alloy/data \
  -e LOKI_URL=http://loki:3100/loki/api/v1/push alloy-promtail:dev
```

The example config tails `/var/log/*.log`. Configuration syntax and component
reference: https://grafana.com/docs/alloy/latest/.

## Components

`base` is always built. Other groups are opt-in build tags, selected with
`COMPONENT_GROUPS`:

| Group | Tag | Components |
|---|---|---|
| `base` | — | `local.file`, `local.file_match`, `loki.source.file`, `loki.source.journal`, `loki.source.syslog`, `loki.source.api`, `loki.source.docker`, `loki.source.gelf`, `loki.process`, `loki.relabel`, `loki.write`, `loki.echo`, `loki.enrich`, `discovery.consul`, `discovery.consulagent`, `discovery.dns`, `discovery.docker`, `discovery.dockerswarm`, `discovery.file`, `discovery.http`, `discovery.relabel`, `remote.http` |
| `k8s` | `promtail_k8s` | `discovery.kubernetes`, `discovery.kubelet`, `loki.source.kubernetes`, `loki.source.kubernetes_events`, `loki.source.podlogs`, `remote.kubernetes.configmap`, `remote.kubernetes.secret` |
| `cloud` | `promtail_cloud` | `loki.source.awsfirehose`, `loki.source.azure_event_hubs`, `loki.source.cloudflare`, `loki.source.gcplog`, `loki.source.heroku`, `loki.source.kafka`, `remote.s3`, `remote.vault` |
| `secretfilter` | `promtail_secretfilter` | `loki.secretfilter` |

Unchanged from upstream: `import.file`, `import.http`, `import.string`,
`declare`, `foreach`, the `logging`, `http` and `remotecfg` blocks, `/-/reload`,
and clustering with `--cluster.join-addresses`. `loki.enrich` is experimental
and needs `--stability.level=experimental`.

### Removed

| Removed | Why | Instead |
|---|---|---|
| `otelcol.*`, `alloy otel`, `prometheus.*`, `pyroscope.*`, `beyla.*`, `mimir.*`, `faro.*`, `database_observability.*`, `loki.rules.kubernetes`, other `discovery.*` | not log collection | — |
| `alloy convert`, `--config.format` | converters link the OTel Collector and every Prometheus SD (+17.7 MiB for the promtail converter alone) | convert once with upstream Alloy: `alloy convert --source-format=promtail` |
| `--cluster.discover-peers` | go-discover links AWS, Azure, GCE, vSphere and other SDKs (−24.6 MiB) | `--cluster.join-addresses` with DNS A/SRV, e.g. a headless Service |
| `import.git` | go-git (−3.3 MiB) | `import.http`, `import.file` |
| Web UI, `/api/v0/web`, GraphQL, live debugging | exposed endpoints (−1.8 MiB) | `/metrics`, `/-/ready`, `/-/healthy`; `--server.http.enable-graphql` fails the start |
| `alloy tools`, `alloy gql` | `prometheus.remote_write` tooling, GraphQL client | — |
| `tracing.write_to` | accepts `otelcol.*` components only | sampling and `send_traceparent` still work |

### Secure defaults

| `run` flag | alloy-promtail | upstream | Restore upstream behavior |
|---|---|---|---|
| `--disable-reporting` | `true`: no usage stats to Grafana Labs | `false` | `--disable-reporting=false` |
| `--server.http.enable-pprof` | `false`: no `/debug/pprof` | `true` | `--server.http.enable-pprof` |
| `--server.http.disable-support-bundle` | `true`: `/-/support` returns 403 | `false` | `--server.http.disable-support-bundle=false` |

The 403 message comes from upstream and suggests removing the flag; pass
`=false` instead. The HTTP server listens on `127.0.0.1:12345`, as upstream.
The agent connects only where the configuration points it: no usage stats,
update checks or telemetry.

## Build

Requires the Go version from `go.mod`. `loki.source.journal` also needs CGO and
libsystemd headers (`apt install libsystemd-dev`); it loads `libsystemd.so.0`
at runtime. Run make from the repository root.

```sh
make -C promtail build                          # build/promtail/alloy-promtail
make -C promtail build COMPONENT_GROUPS="k8s"   # base + k8s
make -C promtail build GOOS=linux GOARCH=arm64 CGO_ENABLED=0
make -C promtail check                          # go-version, build, deps, test, e2e, size
make -C promtail image IMAGE=alloy-promtail:dev # host platform image
make -C promtail help                           # all targets and variables
```

`CGO_ENABLED=0` gives a static binary, but `loki.source.journal` becomes a stub
that starts, logs an info message and reads nothing.

`promtail/Dockerfile` cross-compiles amd64/arm64 with CGO (no emulation) into an
Ubuntu 24.04 chisel rootfs with libc, libsystemd, ca-certificates and tzdata —
no shell, no package manager. Linux binaries need glibc 2.34 or newer. The
image runs as root because host logs are usually root-readable; use
`--user 473:473` otherwise. Mount `/var/lib/alloy/data`, it keeps the file read
positions; without it every start resends all lines.

## How the build works

The repository is a regular fork with the full upstream history. Upstream files
are never modified: the fork adds `promtail/`, `.github/README.md` and
`.github/workflows/promtail-*.yml` and removes the upstream workflows. A merge
from upstream therefore never conflicts in code.

`make build` does two things:

1. `tools/overlay` reads `overlay/manifest.txt` and writes
   `build/promtail/overlay/overlay.json`.
2. `go build -overlay=overlay.json` compiles `cmd/alloy-promtail` with the tags of
   the selected groups. The compiler sees the overlaid files instead of the
   upstream ones; the working tree stays untouched.

`make check` adds the dependency check (`tools/depcheck`), unit tests and the
end-to-end test.

| Path | Purpose |
|---|---|
| `cmd/alloy-promtail/` | entry point; reuses the upstream CLI, removes `convert`, `gql`, `tools`, hides dead flags, sets secure defaults |
| `components/` | component registry; `base.go` plus one file per group behind its build tag |
| `overlay/manifest.txt` | upstream file substitutions, each with rationale and measured saving |
| `overlay/_files/` | replacement files; the underscore keeps them out of `go build ./...` |
| `overlay/patches/` | `git diff` patches to upstream files |
| `denylist.txt` | packages that must not be linked, optionally per group |
| `modules.txt` | allowlist of Go modules that may be linked, optionally per group |
| `tools/overlay`, `tools/depcheck` | stdlib-only build tools |
| `scripts/sync-upstream.sh`, `scripts/patch.sh` | upstream sync, patch editing |
| `test/e2e/` | end-to-end test of the built binary |

Size is cut at two levels. The registry imports only the selected components,
and `internal/component/all` is replaced by an empty package. The overlay then
removes import chains that pull heavy dependencies into the core even with no
component linked — for example `--cluster.discover-peers` alone brings in the
AWS, Azure and GCE SDKs.

Manifest operations:

| Operation | Effect |
|---|---|
| `delete <file>` | drops the upstream file |
| `replace <file> [blob=<sha1>]` | uses `_files/<file>`; `blob` pins the upstream revision it was derived from |
| `patch <file>` | applies `patches/<file>.patch`; every hunk must match exactly one location, line shifts are tolerated |
| `keep <dir> <file>...` | drops every other non-test Go file of the package |

```
# --cluster.discover-peers links hashicorp/go-discover with AWS, Azure, GCE,
# vSphere, DigitalOcean and other cloud SDKs (-24.6 MiB).
# --cluster.join-addresses keeps working.
patch internal/service/cluster/discovery/go_discovery.go
patch internal/service/cluster/discovery/peer_discovery.go
```

An upstream change that breaks an assumption fails the build instead of
silently growing the binary or changing behavior:

| Check | Fails when |
|---|---|
| `make overlay` | a patch no longer applies or matches ambiguously, a `blob`-pinned file changed, a deleted or kept file is gone |
| compilation | a replacement or patch no longer fits the upstream API |
| `make deps` | a package from `denylist.txt` or a module missing from `modules.txt` gets linked; prints the import chain |
| `make go-version` | `go.mod` requires a newer Go than the `golang` image in `promtail/Dockerfile` |
| `TestRegisteredComponents` | the registered component set differs from the expected one |
| `TestNewCommand*` | upstream renamed a command or flag the entry point adjusts |
| `make e2e` | the binary stops collecting or shipping logs, or a removed endpoint responds |

## Upstream sync

The fork follows upstream `main`; the `upstream` remote has push disabled.

```sh
promtail/scripts/sync-upstream.sh            # fetch and merge upstream/main, no commit
make -C promtail check COMPONENT_GROUPS="k8s cloud secretfilter"
git commit
```

The script merges with `--no-commit`, removes upstream workflows (only
`promtail-*.yml` run in the fork), verifies the overlay and the Go version and
prints which manifest paths changed upstream. After a manual `git merge`, run
`promtail/scripts/sync-upstream.sh --prune-workflows`.

`promtail-sync.yml` runs the same weekly and opens a pull request with the
results. The merge and all checks run in a job without write permissions or
secrets; a second job only verifies the merge, pushes the branch and opens the
pull request. Required setup:

- `SYNC_TOKEN` secret: fine-grained PAT with contents, pull-requests and
  workflows write access. `GITHUB_TOKEN` cannot push merges that touch
  `.github/workflows`, and pull requests it opens do not trigger CI.
- Merge sync pull requests with a merge commit. Squash or rebase drops the
  upstream ancestry the next sync relies on.

Fixing a failed sync:

| Failure | Fix |
|---|---|
| patch does not apply | `make -C promtail patch-start FILE=<path>` applies it, falling back to a 3-way merge with conflict markers; edit, then `make -C promtail patch-save FILE=<path>`. Works in the middle of a merge. For an ambiguous hunk add `CONTEXT=8` to `patch-save`. |
| pinned `replace` changed | the error prints `git diff <old> <new>`; port the change to `_files/<path>` and update `blob=` |
| denied package or new module | follow the printed import chain and cut it with a manifest entry; if the dependency is wanted, add the module to `modules.txt` (`if <group>` for a group-only module) or drop the denylist rule |
| `go-version` | update the `golang` tag and digest in `promtail/Dockerfile` (`docker buildx imagetools inspect golang:<version>-bookworm`) |
| `dropped by keep: <file>` in the summary | add the file to the `keep` line if the package needs it, e.g. for its `init()` |
| component set changed | update `promtail/components/*.go` and `cmd/alloy-promtail/components*_test.go` |

## Adding a component

Import it in the matching `promtail/components/*.go`, add its name to
`cmd/alloy-promtail/components*_test.go` and run `make -C promtail check` with
the group enabled. `make deps` reports any denied package or new module it
brings in.

A new group needs `promtail/components/<group>.go` with the
`//go:build promtail_<group>` tag, `unless <group>` rules in `denylist.txt` and
`if <group>` entries in `modules.txt`. `COMPONENT_GROUPS` accepts only groups
that have a registry file.

## Releases

Release tags are `v<upstream>-promtail.<n>`: the latest upstream release merged
into `main`, plus a fork build number.

```sh
git fetch upstream --tags
git describe --tags --abbrev=0 --match 'v[0-9]*' --exclude '*-promtail.*'   # v1.20.0
git tag v1.20.0-promtail.1
git push origin v1.20.0-promtail.1
```

`promtail-release.yml` rejects a tag whose upstream part differs from
`git describe`. An upstream release candidate gives a prerelease, e.g.
`v1.21.0-rc.0-promtail.1`.

Published artifacts:

| Artifact | Details |
|---|---|
| `ghcr.io/velezh/alloy-promtail:<version>` | linux/amd64 + linux/arm64, SBOM and provenance attestations; `<version>` is the tag without `v` |
| `ghcr.io/velezh/alloy-promtail:latest` | moved only by the newest non-prerelease tag |
| `alloy-promtail-<version>-linux-{amd64,arm64}.tar.gz` | binary and `LICENSE`; reproducible archives |
| `SHA256SUMS` | checksums of the archives |

Releases contain the `base` group. Build other variants with
`make -C promtail image COMPONENT_GROUPS="k8s"` or
`docker buildx build --build-arg COMPONENT_GROUPS=k8s -f promtail/Dockerfile .`.

Re-running the workflow for an existing tag (`workflow_dispatch`) rebuilds and
replaces the release assets. The first publish creates a private GHCR package;
make it public in the package settings if needed.

Verify a download:

```sh
sha256sum -c SHA256SUMS --ignore-missing
docker buildx imagetools inspect ghcr.io/velezh/alloy-promtail:<version> --format '{{json .Provenance}}'
```
