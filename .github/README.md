# alloy-promtail

A fork of [Grafana Alloy](https://github.com/grafana/alloy) built for log
collection only: the promtail successor on the Alloy engine.

- ~42 MiB binary instead of 390 MiB upstream (linux/amd64): no OpenTelemetry
  Collector, Prometheus, profiling, cloud SDKs, web UI or config converters.
- Secure defaults: no usage reporting to Grafana Labs, no pprof, no support bundle.

Build, components, overlay design, upstream sync and releases are described in
[promtail/README.md](../promtail/README.md). The original Alloy README is
[README.md](../README.md).

```sh
make -C promtail build      # build/promtail/alloy-promtail
make -C promtail check      # build, dependency checks, tests, e2e
```

Licensed under [Apache-2.0](../LICENSE); exceptions are listed in
[LICENSING.md](../LICENSING.md).
