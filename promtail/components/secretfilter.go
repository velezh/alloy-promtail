//go:build promtail_secretfilter

package components

import (
	_ "github.com/grafana/alloy/internal/component/loki/secretfilter" // Import loki.secretfilter
)
