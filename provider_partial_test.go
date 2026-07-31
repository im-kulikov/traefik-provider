package traefik_provider

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/genconf/dynamic"
)

// deadAddr binds a port and releases it immediately, so connections to it are
// refused instantly -- the way a powered-off host behaves.
func deadAddr(t *testing.T) *net.TCPAddr {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)
	require.NoError(t, ln.Close())

	return addr
}

// TestProvider_constructsWithUnreachableEndpoint asserts that the provider can
// still be constructed when an endpoint is down at startup.
//
// Regression test: PrepareClients probed both ports of every endpoint and
// returned on the first failure, so New() failed outright. Traefik does not
// retry a provider that fails to start, so a single host being powered off at
// the moment Traefik booted left every endpoint unrouted until Traefik was
// restarted -- and bringing the host back changed nothing.
func TestProvider_constructsWithUnreachableEndpoint(t *testing.T) {
	data, err := os.ReadFile("fixtures/jaeger-api-rawdata.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write(data)))
	}))
	defer srv.Close()

	good, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	dead := deadAddr(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	resolver := "letsencrypt"

	cfg := Config{
		ConnTimeout:  "15s",
		PollInterval: "5s",
		TLSResolver:  &resolver,
		Endpoints: []Endpoint{
			{Host: dead.IP.String(), API: dead.Port, WEB: dead.Port},
			{Host: good.IP.String(), API: good.Port, WEB: good.Port},
		},
	}

	p, err := New(ctx, &cfg, "test")
	require.NoError(t, err, "an endpoint being down at startup must not prevent construction")
	require.NotNil(t, p)
	require.NoError(t, p.Init())
}

// TestProvider_partialEndpointFailure asserts that a single unreachable endpoint
// does not discard configuration that was fetched successfully from the healthy
// endpoints.
//
// Regression test: previously a failing client returned its error from the
// runner goroutine, which cancelled the context shared by every fetch in the
// poll. The aggregator selects on that same context, so it could break out of
// its collect loop before draining the healthy responses and publish an empty
// configuration. Traefik then applied that empty config and removed every
// router the provider owned, so one powered-off host took down routing for all
// of them.
//
// The failing endpoint is a closed port so it is refused instantly, while the
// healthy endpoint is delayed. That ordering makes the cancellation land first
// and the race deterministic.
func TestProvider_partialEndpointFailure(t *testing.T) {
	data, err := os.ReadFile("fixtures/jaeger-api-rawdata.json")
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(75 * time.Millisecond)
		w.WriteHeader(http.StatusOK)

		assert.NoError(t, catchError(w.Write(data)))
	}))
	defer srv.Close()

	good, ok := srv.Listener.Addr().(*net.TCPAddr)
	require.True(t, ok)

	dead := deadAddr(t)

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	resolver := "letsencrypt"

	cfg := Config{
		ConnTimeout:  "15s",
		PollInterval: "5s",
		TLSResolver:  &resolver,
		Endpoints: []Endpoint{
			{Host: dead.IP.String(), API: dead.Port, WEB: dead.Port},
			{Host: good.IP.String(), API: good.Port, WEB: good.Port},
		},
	}

	p, err := New(ctx, &cfg, "test")
	require.NoError(t, err)
	require.NoError(t, p.Init())

	out := make(chan json.Marshaler, 100)
	require.NoError(t, p.Provide(out))

	select {
	case <-ctx.Done():
		t.Fatal("provider published no configuration at all")
	case result := <-out:
		payload, okPayload := result.(dynamic.JSONPayload)
		require.True(t, okPayload)
		require.NotNil(t, payload.Configuration)

		require.NotNil(t, payload.HTTP,
			"healthy endpoint's configuration was discarded because another endpoint failed")

		name := "whoami-" + good.IP.String()
		require.Contains(t, payload.HTTP.Routers, name,
			"router from the healthy endpoint is missing")
		require.Contains(t, payload.HTTP.Services, name,
			"service from the healthy endpoint is missing")
	}
}
