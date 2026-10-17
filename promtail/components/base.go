// Package components registers the Alloy components included in alloy-promtail.
package components

import (
	_ "github.com/grafana/alloy/internal/component/discovery/consul"      // Import discovery.consul
	_ "github.com/grafana/alloy/internal/component/discovery/consulagent" // Import discovery.consulagent
	_ "github.com/grafana/alloy/internal/component/discovery/dns"         // Import discovery.dns
	_ "github.com/grafana/alloy/internal/component/discovery/docker"      // Import discovery.docker
	_ "github.com/grafana/alloy/internal/component/discovery/dockerswarm" // Import discovery.dockerswarm
	_ "github.com/grafana/alloy/internal/component/discovery/file"        // Import discovery.file
	_ "github.com/grafana/alloy/internal/component/discovery/http"        // Import discovery.http
	_ "github.com/grafana/alloy/internal/component/discovery/relabel"     // Import discovery.relabel
	_ "github.com/grafana/alloy/internal/component/local/file"            // Import local.file
	_ "github.com/grafana/alloy/internal/component/local/file_match"      // Import local.file_match
	_ "github.com/grafana/alloy/internal/component/loki/echo"             // Import loki.echo
	_ "github.com/grafana/alloy/internal/component/loki/enrich"           // Import loki.enrich
	_ "github.com/grafana/alloy/internal/component/loki/process"          // Import loki.process
	_ "github.com/grafana/alloy/internal/component/loki/relabel"          // Import loki.relabel
	_ "github.com/grafana/alloy/internal/component/loki/source/api"       // Import loki.source.api
	_ "github.com/grafana/alloy/internal/component/loki/source/docker"    // Import loki.source.docker
	_ "github.com/grafana/alloy/internal/component/loki/source/file"      // Import loki.source.file
	_ "github.com/grafana/alloy/internal/component/loki/source/gelf"      // Import loki.source.gelf
	_ "github.com/grafana/alloy/internal/component/loki/source/journal"   // Import loki.source.journal
	_ "github.com/grafana/alloy/internal/component/loki/source/syslog"    // Import loki.source.syslog
	_ "github.com/grafana/alloy/internal/component/loki/write"            // Import loki.write
	_ "github.com/grafana/alloy/internal/component/remote/http"           // Import remote.http
)
