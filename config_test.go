package traefik_provider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_validate(t *testing.T) {
	var cfg *Config
	require.EqualError(t, cfg.validate(), "empty config")

	cfg = CreateConfig()
	require.ErrorContains(t, cfg.validate(), "time: invalid duration", "empty connTimeout")

	cfg.ConnTimeout = "5s"
	require.ErrorContains(t, cfg.validate(), "time: invalid duration", "empty pollTimeout")

	cfg.PollInterval = "5s"
	require.ErrorContains(t, cfg.validate(), "empty endpoints", "empty endpoints")

	cfg.Endpoints = make([]Endpoint, 1)
	require.ErrorContains(t, cfg.validate(), "empty #0 endpoint host")

	cfg.Endpoints[0].Host = "localhost"
	require.ErrorContains(t, cfg.validate(), "empty #0 endpoint apiPort")

	cfg.Endpoints[0].API = 8080
	require.ErrorContains(t, cfg.validate(), "empty #0 endpoint webPort")
}
