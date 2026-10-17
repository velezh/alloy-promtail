// Package e2e tests the alloy-promtail binary given by the ALLOY_PROMTAIL_BINARY environment variable.
package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	binaryEnv    = "ALLOY_PROMTAIL_BINARY"
	waitTimeout  = time.Minute
	pollInterval = 50 * time.Millisecond
)

var (
	binaryPath = os.Getenv(binaryEnv)
	httpClient = &http.Client{Timeout: 10 * time.Second}
)

// skipWithoutBinary skips the test when the tested binary is not given.
func skipWithoutBinary(t *testing.T) {
	t.Helper()
	if binaryPath == "" {
		t.Skipf("%s is not set", binaryEnv)
	}
}

// alloy is a running `alloy-promtail run` process.
type alloy struct {
	httpAddr string
	output   bytes.Buffer
	exited   chan struct{}
	err      error
}

// startAlloy runs the binary with config until the test ends.
//
// The output of the process is logged when the test fails.
func startAlloy(t *testing.T, config string, flags ...string) *alloy {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	p := &alloy{
		httpAddr: freeAddr(t),
		exited:   make(chan struct{}),
	}
	cmd := exec.CommandContext(ctx, binaryPath, runArgs(t, p.httpAddr, config, flags...)...)
	cmd.Stdout = &p.output
	cmd.Stderr = &p.output
	if err := cmd.Start(); err != nil {
		cancel()
		require.NoError(t, err)
	}
	go func() {
		p.err = cmd.Wait()
		close(p.exited)
	}()

	t.Cleanup(func() {
		cancel()
		<-p.exited
		if t.Failed() {
			t.Logf("alloy-promtail output:\n%s", p.output.String())
		}
	})
	return p
}

// url returns the URL of path on the HTTP server of the process.
func (p *alloy) url(path string) string {
	return "http://" + p.httpAddr + path
}

// eventually polls cond until it returns nil.
//
// Fails the test when the process exits or cond keeps failing for waitTimeout.
func (p *alloy) eventually(t *testing.T, cond func() error) {
	t.Helper()
	timeout := time.NewTimer(waitTimeout)
	defer timeout.Stop()
	tick := time.NewTicker(pollInterval)
	defer tick.Stop()

	for {
		err := cond()
		if err == nil {
			return
		}
		select {
		case <-p.exited:
			t.Fatalf("alloy-promtail exited (%v) while waiting: %v", p.err, err)
		case <-timeout.C:
			t.Fatalf("timed out after %s: %v", waitTimeout, err)
		case <-tick.C:
		}
	}
}

// statusIs returns a condition that fails while a GET request of url does not return status.
func statusIs(url string, status int) func() error {
	return func() error {
		got, _, err := get(url)
		if err != nil {
			return err
		}
		if got != status {
			return fmt.Errorf("GET %s returned %d, want %d", url, got, status)
		}
		return nil
	}
}

// runCommand runs the binary with args to completion.
//
// Returns the combined output and the exit code of the process.
func runCommand(t *testing.T, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	out, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "%q did not exit, output:\n%s", args, out)
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		require.NoError(t, err)
	}
	return string(out), cmd.ProcessState.ExitCode()
}

// runArgs returns the arguments of `alloy-promtail run` serving HTTP on httpAddr with config
// written to a temporary file and a temporary storage path.
func runArgs(t *testing.T, httpAddr, config string, flags ...string) []string {
	t.Helper()
	args := []string{
		"run",
		"--server.http.listen-addr=" + httpAddr,
		"--storage.path=" + t.TempDir(),
	}
	args = append(args, flags...)
	return append(args, writeConfig(t, config))
}

// writeConfig writes config to a temporary file and returns its path.
func writeConfig(t *testing.T, config string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.alloy")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	return path
}

// freePort returns a TCP port of 127.0.0.1 that is free at the time of the call.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// freeAddr returns a 127.0.0.1 address with a port that is free at the time of the call.
func freeAddr(t *testing.T) string {
	t.Helper()
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(freePort(t)))
}

// get performs a GET request of url.
//
// Returns the response status code and body.
func get(url string) (int, string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body), err
}
