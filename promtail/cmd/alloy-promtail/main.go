// Command alloy-promtail is a log collection distribution of Grafana Alloy.
package main

import (
	"fmt"
	"os"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/alloy"
	"github.com/grafana/alloy/internal/build"

	_ "github.com/grafana/alloy/promtail/components"

	// Embed a set of fallback X.509 trusted roots
	// Allows the app to work correctly even when the OS does not provide a verifier or systems roots pool
	_ "golang.org/x/crypto/x509roots/fallback"
)

func init() {
	// If the build version wasn't set by the build process, we'll set it based
	// on the version in .release-please-manifest.json.
	if build.Version == "" || build.Version == "v0.0.0" {
		build.Version = alloy.FallbackVersion()
	}

	prometheus.MustRegister(build.NewCollector("alloy"))
}

func main() {
	cmd, err := newCommand()
	if err != nil {
		fmt.Fprintf(os.Stderr, "alloy-promtail: %v\n", err)
		os.Exit(1)
	}
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
