//go:build promtail_k8s

package components

import (
	_ "github.com/grafana/alloy/internal/component/discovery/kubelet"             // Import discovery.kubelet
	_ "github.com/grafana/alloy/internal/component/discovery/kubernetes"          // Import discovery.kubernetes
	_ "github.com/grafana/alloy/internal/component/loki/source/kubernetes"        // Import loki.source.kubernetes
	_ "github.com/grafana/alloy/internal/component/loki/source/kubernetes_events" // Import loki.source.kubernetes_events
	_ "github.com/grafana/alloy/internal/component/loki/source/podlogs"           // Import loki.source.podlogs
	_ "github.com/grafana/alloy/internal/component/remote/kubernetes/configmap"   // Import remote.kubernetes.configmap
	_ "github.com/grafana/alloy/internal/component/remote/kubernetes/secret"      // Import remote.kubernetes.secret
)
