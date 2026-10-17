// Package converter keeps the config converter API of Grafana Alloy.
// alloy-promtail ships without converters.
package converter

import (
	"fmt"

	"github.com/grafana/alloy/internal/converter/diag"
)

// Input represents the type of config file being fed into the converter.
type Input string

// SupportedFormats lists the source formats accepted by Convert.
var SupportedFormats []string

// Convert rejects every input format.
//
// Returns a critical diagnostic.
func Convert(_ []byte, kind Input, _ []string) ([]byte, diag.Diagnostics) {
	var diags diag.Diagnostics
	diags.Add(diag.SeverityLevelCritical, fmt.Sprintf("converting %q configs is not supported by alloy-promtail, use `alloy convert` from Grafana Alloy", kind))
	return nil, diags
}
