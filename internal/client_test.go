package internal

import (
	"context"
	"io"
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

func TestClient_Endpoint(t *testing.T) {
	var client *Client
	require.NotPanics(t, func() {
		require.Equal(t, client.Endpoint(), "empty")
	})
}

func TestClient_FetchErrors(t *testing.T) {
	cli := new(Client)
	cli.Client = new(http.Client)
	{
		out := make(chan *dynamic.Configuration, 2)
		require.ErrorContains(t, cli.FetchRaw(nil, out), "nil Context") // nolint:staticcheck
		close(out)
		require.Len(t, out, 1)
		require.Nil(t, <-out)
	}

	{
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		out := make(chan *dynamic.Configuration, 2)
		require.ErrorIs(t, cli.FetchRaw(ctx, out), context.Canceled)
		close(out)
		require.Len(t, out, 1)
		require.Nil(t, <-out)
	}
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

	require.Equal(t, cli[0].Endpoint(), addr.IP.String())

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

func TestClient_empty(t *testing.T) {
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
	require.ErrorIs(t, cli[0].FetchRaw(t.Context(), out), ErrEmptyResponse)

	select {
	case <-ctx.Done():
		t.Fatal("expect result")
	case msg := <-out:
		require.Empty(t, msg)
	}
}

const noServiceResponse = `{"services": {"two@docker": {}}, "routers": {"one@docker": {}}}}`

func TestClient_noService(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write([]byte(noServiceResponse))))
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
	require.NoError(t, cli[0].FetchRaw(t.Context(), out))

	select {
	case <-ctx.Done():
		t.Fatal("expect result")
	case msg := <-out:
		require.Empty(t, msg)
	}
}

func TestClient_failure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
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
	require.ErrorIs(t, cli[0].FetchRaw(t.Context(), out), io.EOF)

	select {
	case <-ctx.Done():
		t.Fatal("expect result")
	case msg := <-out:
		require.Empty(t, msg)
	}
}
