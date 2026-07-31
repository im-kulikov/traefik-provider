package traefik_provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/genconf/dynamic"
)

// rawDataFor returns a minimal /api/rawdata payload advertising a single router
// and matching service under the given name.
func rawDataFor(name string) []byte {
	payload := fmt.Sprintf(`{
      "routers": {
        %[1]q: {"entryPoints":["web"],"service":%[1]q,"rule":"Host(`+"`"+`%[1]s.example.com`+"`"+`)","status":"enabled"}
      },
      "services": {
        %[1]q: {"status":"enabled","loadBalancer":{"servers":[{"url":"http://127.0.0.1:1/"}]}}
      },
      "middlewares": {}
    }`, name+"@docker")

	return []byte(payload)
}

func serveRawData(t *testing.T, name string) *net.TCPAddr {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write(rawDataFor(name))))
	}))
	t.Cleanup(srv.Close)

	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	return addr
}

// TestProvider_collectsFromEveryEndpoint asserts that a poll returns the routers
// from ALL configured endpoints, not just one.
//
// Regression test for a loop-variable capture: `for _, client := range clients`
// followed by a closure referencing `client` is per-iteration under Go 1.22+,
// but Yaegi -- which is how Traefik actually executes this plugin -- shares the
// variable across iterations. Every fetch then queries whichever endpoint the
// loop finished on, so the provider silently serves one endpoint's routers for
// all of them. It compiles and passes `go test` regardless, which is why it can
// go unnoticed.
//
// The two endpoints advertise distinct router names so the merged configuration
// shows exactly which ones were actually contacted.
func TestProvider_collectsFromEveryEndpoint(t *testing.T) {
	alpha := serveRawData(t, "alpha")
	beta := serveRawData(t, "beta")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cfg := Config{
		ConnTimeout:  "15s",
		PollInterval: "5s",
		Endpoints: []Endpoint{
			{Host: alpha.IP.String(), API: alpha.Port, WEB: alpha.Port},
			{Host: beta.IP.String(), API: beta.Port, WEB: beta.Port},
		},
	}

	p, err := New(ctx, &cfg, "test")
	require.NoError(t, err)
	require.NoError(t, p.Init())

	out := make(chan json.Marshaler, 100)
	require.NoError(t, p.Provide(out))

	select {
	case <-ctx.Done():
		t.Fatal("provider published no configuration")
	case result := <-out:
		payload, ok := result.(dynamic.JSONPayload)
		require.True(t, ok)
		require.NotNil(t, payload.Configuration)
		require.NotNil(t, payload.HTTP)

		names := make([]string, 0, len(payload.HTTP.Routers))
		for name := range payload.HTTP.Routers {
			names = append(names, name)
		}

		require.Contains(t, names, "alpha-"+alpha.IP.String(),
			"routers from the first endpoint are missing; got %v", names)
		require.Contains(t, names, "beta-"+beta.IP.String(),
			"routers from the second endpoint are missing; got %v", names)
	}
}
