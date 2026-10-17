package e2e

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHTTPServer(t *testing.T) {
	skipWithoutBinary(t)
	t.Parallel()

	t.Run("defaults", func(t *testing.T) {
		t.Parallel()
		p := startAlloy(t, "")
		p.eventually(t, statusIs(p.url("/-/ready"), http.StatusOK))

		status, body, err := get(p.url("/metrics"))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status)
		require.Contains(t, body, "alloy_build_info")

		for _, path := range []string{"/", "/api/v0/web/components", "/debug/pprof/"} {
			status, _, err := get(p.url(path))
			require.NoError(t, err)
			require.Equal(t, http.StatusNotFound, status, path)
		}

		status, _, err = get(p.url("/-/support"))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, status)
	})

	t.Run("pprof enabled", func(t *testing.T) {
		t.Parallel()
		p := startAlloy(t, "", "--server.http.enable-pprof")
		p.eventually(t, statusIs(p.url("/-/ready"), http.StatusOK))

		status, _, err := get(p.url("/debug/pprof/"))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, status)
	})
}
