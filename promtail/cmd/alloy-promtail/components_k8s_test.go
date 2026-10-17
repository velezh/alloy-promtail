//go:build promtail_k8s

package main

func init() {
	expectedComponents = append(expectedComponents,
		"discovery.kubelet",
		"discovery.kubernetes",
		"loki.source.kubernetes",
		"loki.source.kubernetes_events",
		"loki.source.podlogs",
		"remote.kubernetes.configmap",
		"remote.kubernetes.secret",
	)
}
