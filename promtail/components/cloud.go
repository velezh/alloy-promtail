//go:build promtail_cloud

package components

import (
	_ "github.com/grafana/alloy/internal/component/loki/source/aws_firehose"     // Import loki.source.awsfirehose
	_ "github.com/grafana/alloy/internal/component/loki/source/azure_event_hubs" // Import loki.source.azure_event_hubs
	_ "github.com/grafana/alloy/internal/component/loki/source/cloudflare"       // Import loki.source.cloudflare
	_ "github.com/grafana/alloy/internal/component/loki/source/gcplog"           // Import loki.source.gcplog
	_ "github.com/grafana/alloy/internal/component/loki/source/heroku"           // Import loki.source.heroku
	_ "github.com/grafana/alloy/internal/component/loki/source/kafka"            // Import loki.source.kafka
	_ "github.com/grafana/alloy/internal/component/remote/s3"                    // Import remote.s3
	_ "github.com/grafana/alloy/internal/component/remote/vault"                 // Import remote.vault
)
