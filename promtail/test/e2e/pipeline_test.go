package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// filePipelineConfig returns a config tailing the files matching pattern into the Loki push API at pushURL.
func filePipelineConfig(pattern, pushURL string) string {
	return fmt.Sprintf(`
local.file_match "logs" {
	path_targets = [{"__path__" = %q, "job" = "e2e-file"}]
	sync_period  = "200ms"
}

loki.source.file "logs" {
	targets    = local.file_match.logs.targets
	forward_to = [loki.process.logs.receiver]
}

loki.process "logs" {
	stage.static_labels {
		values = {environment = "e2e"}
	}
	forward_to = [loki.write.loki.receiver]
}

loki.write "loki" {
	endpoint {
		url        = %q
		batch_wait = "100ms"
	}
}
`, pattern, pushURL)
}

// apiPipelineConfig returns a config forwarding the logs pushed to 127.0.0.1:port into the Loki push API at pushURL.
func apiPipelineConfig(port int, pushURL string) string {
	return fmt.Sprintf(`
loki.source.api "push" {
	http {
		listen_address = "127.0.0.1"
		listen_port    = %d
	}
	labels     = {job = "e2e-api"}
	forward_to = [loki.write.loki.receiver]
}

loki.write "loki" {
	endpoint {
		url        = %q
		batch_wait = "100ms"
	}
}
`, port, pushURL)
}

func TestFilePipeline(t *testing.T) {
	skipWithoutBinary(t)
	t.Parallel()
	loki := newFakeLoki(t)
	dir := t.TempDir()
	first := filepath.Join(dir, "first.log")
	second := filepath.Join(dir, "second.log")

	appendLines(t, first, "first 1", "first 2")
	p := startAlloy(t, filePipelineConfig(filepath.Join(dir, "*.log"), loki.pushURL()))
	p.eventually(t, loki.hasLines("first 1", "first 2"))

	appendLines(t, first, "first 3")
	appendLines(t, second, "second 1")
	p.eventually(t, loki.hasLines("first 3", "second 1"))

	labels := func(filename string) map[string]string {
		return map[string]string{"job": "e2e-file", "environment": "e2e", "filename": filename}
	}
	require.ElementsMatch(t, []entry{
		{line: "first 1", labels: labels(first)},
		{line: "first 2", labels: labels(first)},
		{line: "first 3", labels: labels(first)},
		{line: "second 1", labels: labels(second)},
	}, loki.received())
}

func TestAPIPipeline(t *testing.T) {
	skipWithoutBinary(t)
	t.Parallel()
	loki := newFakeLoki(t)
	port := freePort(t)
	apiURL := "http://127.0.0.1:" + strconv.Itoa(port)

	p := startAlloy(t, apiPipelineConfig(port, loki.pushURL()))
	p.eventually(t, statusIs(apiURL+"/ready", http.StatusOK))

	body, err := json.Marshal(map[string]any{
		"streams": []any{map[string]any{
			"stream": map[string]string{"source": "e2e"},
			"values": [][]string{{strconv.FormatInt(time.Now().UnixNano(), 10), "pushed line"}},
		}},
	})
	require.NoError(t, err)
	resp, err := httpClient.Post(apiURL+"/loki/api/v1/push", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusNoContent, resp.StatusCode)

	p.eventually(t, loki.hasLines("pushed line"))
	require.ElementsMatch(t, []entry{
		{line: "pushed line", labels: map[string]string{"job": "e2e-api", "source": "e2e"}},
	}, loki.received())
}

// appendLines appends lines to the file at path, creating it when needed.
func appendLines(t *testing.T, path string, lines ...string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	require.NoError(t, err)
	_, err = f.WriteString(strings.Join(lines, "\n") + "\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())
}
