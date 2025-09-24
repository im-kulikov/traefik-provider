package traefik_provider

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/genconf/dynamic"

	"github.com/im-kulikov/traefik-provider/internal"
)

const (
	whoamiService = "whoami"
	testService   = "test"
)

func catchError(args ...any) error {
	if ln := len(args); ln < 0 {
		return nil
	} else if err, ok := args[ln-1].(error); ok {
		return err
	}

	return nil
}

func TestProvider(t *testing.T) {
	dataOne, err := os.ReadFile("fixtures/jaeger-api-rawdata.json")
	require.NoError(t, err)

	dataTwo := bytes.ReplaceAll(dataOne, []byte(whoamiService), []byte(testService))
	dataTwo = bytes.ReplaceAll(dataTwo, []byte("192.168.97.2"), []byte("192.168.98.2"))

	srvOne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write(dataOne)))
	}))

	srvTwo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write(dataTwo)))
	}))

	addrOne, ok := srvOne.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	addrTwo, ok := srvTwo.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
	defer cancel()

	resolver := "letsencrypt"

	cfg := Config{
		ConnTimeout:  "15s",
		PollInterval: "5s",
		TLSResolver:  &resolver,
		Endpoints: []Endpoint{{
			Host: addrOne.IP.String(),
			API:  addrOne.Port,
			WEB:  addrOne.Port,
		}, {
			Host: addrTwo.IP.String(),
			API:  addrTwo.Port,
			WEB:  addrTwo.Port,
		}},
	}

	p, err := New(ctx, &cfg, "test")
	require.NoError(t, err)
	require.NoError(t, p.Init())

	out := make(chan json.Marshaler, 100)
	require.NoError(t, p.Provide(out))

	select {
	case <-ctx.Done():
		t.Fatal("no response")
	case result := <-out:
		require.ErrorIs(t, p.Stop(), context.Canceled)

		spew.Dump(result)

		require.Equal(t, dynamic.JSONPayload{
			Configuration: &dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						internal.PrepareName(whoamiService, addrOne.String()): {
							Middlewares: []string{"http2https"},
							Service:     internal.PrepareName(whoamiService, addrOne.String()),
							Rule:        "Host(`whoami.example.com`)",
						},
						internal.PrepareName(whoamiService, addrOne.String()) + "-secure": {
							Service: internal.PrepareName(whoamiService, addrOne.String()),
							Rule:    "Host(`whoami.example.com`)",
							TLS:     &dynamic.RouterTLSConfig{CertResolver: resolver},
						},
						internal.PrepareName(testService, addrTwo.String()): {
							Middlewares: []string{"http2https"},
							Service:     internal.PrepareName(testService, addrTwo.String()),
							Rule:        "Host(`test.example.com`)",
						},
						internal.PrepareName(testService, addrTwo.String()) + "-secure": {
							Service: internal.PrepareName(testService, addrTwo.String()),
							Rule:    "Host(`test.example.com`)",
							TLS:     &dynamic.RouterTLSConfig{CertResolver: resolver},
						},
					},
					Services: map[string]*dynamic.Service{
						internal.PrepareName(whoamiService, addrOne.String()): {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								Servers: []dynamic.Server{{URL: (&url.URL{
									Scheme: "http",
									Host:   addrOne.String(),
									Path:   "/",
								}).String()}},
							},
						},
						internal.PrepareName(testService, addrTwo.String()): {
							LoadBalancer: &dynamic.ServersLoadBalancer{
								Servers: []dynamic.Server{{URL: (&url.URL{
									Scheme: "http",
									Host:   addrTwo.String(),
									Path:   "/",
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
			},
		}, result)
	}
}

func TestProvider_failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))

	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
	defer cancel()

	resolver := "letsencrypt"

	cfg := Config{
		ConnTimeout:  "15s",
		PollInterval: "5s",
		TLSResolver:  &resolver,
		Endpoints: []Endpoint{{
			Host: addr.IP.String(),
			API:  addr.Port,
			WEB:  addr.Port,
		}},
	}

	p, err := New(ctx, &cfg, "test")
	require.NoError(t, err)
	require.NoError(t, p.Init())

	out := make(chan json.Marshaler, 100)
	require.NoError(t, p.Provide(out))

	result, err := (<-out).MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(result))
}

func TestProvider_empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write([]byte(`{}`))))
	}))

	addr, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond*100)
	defer cancel()

	resolver := "letsencrypt"

	cfg := Config{
		ConnTimeout:  "15s",
		PollInterval: "5s",
		TLSResolver:  &resolver,
		Endpoints: []Endpoint{{
			Host: addr.IP.String(),
			API:  addr.Port,
			WEB:  addr.Port,
		}},
	}

	p, err := New(ctx, &cfg, "test")
	require.NoError(t, err)
	require.NoError(t, p.Init())

	out := make(chan json.Marshaler, 100)
	require.NoError(t, p.Provide(out))

	result, err := (<-out).MarshalJSON()
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(result))
}
