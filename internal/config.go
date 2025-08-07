package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Endpoint struct {
	Host string `json:"host"    yaml:"host"    toml:"host"    mapstructure:"host"`
	API  int    `json:"apiPort" yaml:"apiPort" toml:"apiPort" mapstructure:"apiPort"`
	WEB  int    `json:"webPort" yaml:"webPort" toml:"webPort" mapstructure:"webPort"`
}

type Config struct {
	ConnTimeout  time.Duration `json:"connTimeout"  yaml:"connTimeout"  toml:"connTimeout"  mapstructure:"connTimeout"`
	PollInterval time.Duration `json:"pollInterval" yaml:"pollInterval" toml:"pollInterval" mapstructure:"pollInterval"`
	Endpoints    []Endpoint    `json:"endpoints"    yaml:"endpoints"    toml:"endpoints"    mapstructure:"endpoints"`
	TLSResolver  *string       `json:"tlsResolver"  yaml:"tlsResolver"  toml:"tlsResolver"  mapstructure:"tlsResolver"`
}

func (c *Config) Validate() error {
	if c == nil {
		return errors.New("empty config")
	}

	if c.ConnTimeout <= 0 {
		return fmt.Errorf("wrong connection timeout: %s", c.ConnTimeout)
	}

	if c.PollInterval <= 0 {
		return fmt.Errorf("wrong poll interval: %s", c.PollInterval)
	}

	if len(c.Endpoints) == 0 {
		return errors.New("empty endpoints")
	}

	for i, endpoint := range c.Endpoints {
		if endpoint.Host == "" {
			return fmt.Errorf("empty #%d endpoint host", i)
		}

		if endpoint.API <= 0 {
			return fmt.Errorf("empty #%d endpoint apiPort: %d", i, endpoint.API)
		}

		if endpoint.WEB <= 0 {
			return fmt.Errorf("empty #%d endpoint webPort: %d", i, endpoint.WEB)
		}
	}

	return nil
}

func (c *Config) PrepareClients(top context.Context) ([]*Client, error) {
	ctx, cancel := context.WithTimeout(top, c.ConnTimeout)
	defer cancel()

	cli := new(http.Client)
	out := make([]*Client, 0, len(c.Endpoints))
	for _, endpoint := range c.Endpoints {
		var err error
		for _, port := range []int{endpoint.API, endpoint.WEB} {
			uri := url.URL{
				Host:   fmt.Sprintf("%s:%d", endpoint.Host, port),
				Scheme: "http",
				Path:   "/",
			}

			var req *http.Request
			if req, err = http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil); err != nil {
				return nil, fmt.Errorf("could not prepare request(%s): %w", uri.String(), err)
			}

			var res *http.Response
			if res, err = cli.Do(req); err != nil {
				return nil, fmt.Errorf("could not call request(%s): %w", uri.String(), err)
			}

			if err = res.Body.Close(); err != nil {
				return nil, fmt.Errorf("could not close response body: %w", err)
			}
		}

		out = append(out, &Client{
			Client:   cli,
			endpoint: endpoint,
			resolver: c.TLSResolver,
		})
	}

	return out, nil
}
