package internal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	defaultTestConnTimeout  = time.Second
	defaultTestPollInterval = time.Second
)

func TestConfig_validate(t *testing.T) {
	var cfg *Config
	require.EqualError(t, cfg.Validate(), "empty config")

	cfg = new(Config)
	require.ErrorContains(t, cfg.Validate(), "wrong connection timeout")

	cfg.ConnTimeout = defaultTestConnTimeout
	require.ErrorContains(t, cfg.Validate(), "wrong poll interval")

	cfg.PollInterval = defaultTestPollInterval
	require.ErrorContains(t, cfg.Validate(), "empty endpoints")

	cfg.Endpoints = make([]Endpoint, 1)
	require.ErrorContains(t, cfg.Validate(), "empty #0 endpoint host")

	cfg.Endpoints[0].Host = "localhost"
	require.ErrorContains(t, cfg.Validate(), "empty #0 endpoint apiPort")

	cfg.Endpoints[0].API = 8080
	require.ErrorContains(t, cfg.Validate(), "empty #0 endpoint webPort")

	cfg.Endpoints[0].WEB = 8080
	require.NoError(t, cfg.Validate())
}
