//go:build promtail_cloud

package main

func init() {
	expectedComponents = append(expectedComponents,
		"loki.source.awsfirehose",
		"loki.source.azure_event_hubs",
		"loki.source.cloudflare",
		"loki.source.gcplog",
		"loki.source.heroku",
		"loki.source.kafka",
		"remote.s3",
		"remote.vault",
	)
}
