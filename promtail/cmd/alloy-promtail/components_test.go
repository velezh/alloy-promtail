package main

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/alloy/internal/component"
)

// expectedComponents lists the components registered by the base group;
// group test files append their components.
var expectedComponents = []string{
	"discovery.consul",
	"discovery.consulagent",
	"discovery.dns",
	"discovery.docker",
	"discovery.dockerswarm",
	"discovery.file",
	"discovery.http",
	"discovery.relabel",
	"local.file",
	"local.file_match",
	"loki.echo",
	"loki.enrich",
	"loki.process",
	"loki.relabel",
	"loki.source.api",
	"loki.source.docker",
	"loki.source.file",
	"loki.source.gelf",
	"loki.source.journal",
	"loki.source.syslog",
	"loki.write",
	"remote.http",
}

func TestRegisteredComponents(t *testing.T) {
	if _, found := component.Get("prometheus.scrape"); found {
		t.Fatal("upstream components are registered: run the tests with the overlay, `make -C promtail test`")
	}

	want := slices.Sorted(slices.Values(expectedComponents))
	got := slices.Sorted(slices.Values(component.AllNames()))
	require.Equal(t, want, got)
}
