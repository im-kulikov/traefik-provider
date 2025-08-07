package internal

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/genconf/dynamic"
)

func catchError(args ...any) error {
	if ln := len(args); ln < 0 {
		return nil
	} else if err, ok := args[ln-1].(error); ok {
		return err
	}

	return nil
}

func TestClient(t *testing.T) {
	data, err := os.ReadFile("../fixtures/jaeger-api-rawdata.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write(data)))
	}))

	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
	defer cancel()

	resolver := "letsencrypt"

	cfg := Config{
		ConnTimeout:  defaultTestConnTimeout,
		PollInterval: defaultTestPollInterval,
		TLSResolver:  &resolver,
		Endpoints: []Endpoint{{
			Host: addr.IP.String(),
			API:  addr.Port,
			WEB:  addr.Port,
		}},
	}

	cli, err := cfg.PrepareClients(ctx)
	require.NoError(t, err)

	out := make(chan *dynamic.Configuration, 1)
	if err = cli[0].FetchRaw(t.Context(), out); err != nil {
		t.Fatal(err)
	}

	var result *dynamic.Configuration
	select {
	case <-ctx.Done():
		t.Fatal("no response")
	case result = <-out:
		require.NotEmpty(t, result)
		require.Equal(t, &dynamic.Configuration{
			HTTP: &dynamic.HTTPConfiguration{
				Routers: map[string]*dynamic.Router{
					"whoami-" + addr.IP.String(): {
						Middlewares: []string{"http2https"},
						Service:     "whoami-" + addr.IP.String(),
						Rule:        "Host(`whoami.example.com`)",
					},
					"whoami-" + addr.IP.String() + "-secure": {
						Service: "whoami-" + addr.IP.String(),
						Rule:    "Host(`whoami.example.com`)",
						TLS:     &dynamic.RouterTLSConfig{CertResolver: resolver},
					},
				},
				Services: map[string]*dynamic.Service{
					"whoami-" + addr.IP.String(): {
						LoadBalancer: &dynamic.ServersLoadBalancer{
							Servers: []dynamic.Server{{URL: (&url.URL{
								Scheme: "http",
								Host:   addr.String(),
							}).String()}},
						},
					},
				},
				Middlewares: map[string]*dynamic.Middleware{
					"http2https": {RedirectScheme: &dynamic.RedirectScheme{
						Scheme:    "https",
						Permanent: true,
					}},
				},
			},
		}, result)
	}
}
