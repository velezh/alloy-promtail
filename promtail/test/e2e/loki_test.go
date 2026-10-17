package e2e

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/golang/snappy"
	"github.com/grafana/loki/pkg/push"
)

// entry is a log line received by fakeLoki.
type entry struct {
	line   string
	labels map[string]string
}

// fakeLoki is a Loki push API server recording the entries sent by loki.write.
type fakeLoki struct {
	t   *testing.T
	srv *httptest.Server

	mu      sync.Mutex
	entries []entry
}

// newFakeLoki starts a fakeLoki that is stopped when the test ends.
func newFakeLoki(t *testing.T) *fakeLoki {
	t.Helper()
	l := &fakeLoki{t: t}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /loki/api/v1/push", l.handlePush)
	l.srv = httptest.NewServer(mux)
	t.Cleanup(l.srv.Close)
	return l
}

// pushURL returns the URL of the push endpoint.
func (l *fakeLoki) pushURL() string {
	return l.srv.URL + "/loki/api/v1/push"
}

// received returns a copy of the received entries.
func (l *fakeLoki) received() []entry {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.entries)
}

// hasLines returns a condition that fails while any of lines is not received.
func (l *fakeLoki) hasLines(lines ...string) func() error {
	return func() error {
		received := l.received()
		var missing []string
		for _, line := range lines {
			if !slices.ContainsFunc(received, func(e entry) bool { return e.line == line }) {
				missing = append(missing, line)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("fake Loki has not received lines %q", missing)
		}
		return nil
	}
}

func (l *fakeLoki) handlePush(w http.ResponseWriter, r *http.Request) {
	entries, err := decodePush(r)
	if err != nil {
		l.t.Errorf("fake Loki: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	l.mu.Lock()
	l.entries = append(l.entries, entries...)
	l.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// decodePush decodes the snappy compressed protobuf push request of r.
func decodePush(r *http.Request) ([]entry, error) {
	if ct := r.Header.Get("Content-Type"); ct != "application/x-protobuf" {
		return nil, fmt.Errorf("unexpected Content-Type %q", ct)
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	data, err := snappy.Decode(nil, body)
	if err != nil {
		return nil, fmt.Errorf("decoding snappy: %w", err)
	}
	var req push.PushRequest
	if err := req.Unmarshal(data); err != nil {
		return nil, fmt.Errorf("decoding protobuf: %w", err)
	}

	var entries []entry
	for _, s := range req.Streams {
		labels, err := parseLabels(s.Labels)
		if err != nil {
			return nil, err
		}
		for _, e := range s.Entries {
			entries = append(entries, entry{line: e.Line, labels: labels})
		}
	}
	return entries, nil
}

// parseLabels parses a stream label set formatted as {name="value", ...}.
func parseLabels(s string) (map[string]string, error) {
	rest, ok := strings.CutPrefix(s, "{")
	if ok {
		rest, ok = strings.CutSuffix(rest, "}")
	}
	if !ok {
		return nil, fmt.Errorf("malformed labels %q", s)
	}

	labels := make(map[string]string)
	for rest != "" {
		name, value, found := strings.Cut(rest, "=")
		if !found {
			return nil, fmt.Errorf("malformed labels %q", s)
		}
		quoted, err := strconv.QuotedPrefix(value)
		if err != nil {
			return nil, fmt.Errorf("malformed labels %q: %w", s, err)
		}
		labels[name], _ = strconv.Unquote(quoted)
		rest = strings.TrimPrefix(value[len(quoted):], ", ")
	}
	return labels, nil
}
