package internal

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"
)

type Endpoint struct {
	Host string `json:"host"    yaml:"host"    toml:"host"    mapstructure:"host"`
	API  int    `json:"apiPort" yaml:"apiPort" toml:"apiPort" mapstructure:"apiPort"`
	WEB  int    `json:"webPort" yaml:"webPort" toml:"webPort" mapstructure:"webPort"`
	TLS  *TLS   `json:"tls"     yaml:"tls"     toml:"tls"     mapstructure:"tls"`
}

type TLS struct {
	IgnoreInsecure bool `json:"ignoreInsecure" yaml:"ignoreInsecure" toml:"ignoreInsecure" mapstructure:"ignoreInsecure"`
}

type Config struct {
	ConnTimeout  time.Duration `json:"connTimeout"  yaml:"connTimeout"  toml:"connTimeout"  mapstructure:"connTimeout"`
	PollInterval time.Duration `json:"pollInterval" yaml:"pollInterval" toml:"pollInterval" mapstructure:"pollInterval"`
	Endpoints    []Endpoint    `json:"endpoints"    yaml:"endpoints"    toml:"endpoints"    mapstructure:"endpoints"`
	TLSResolver  *string       `json:"tlsResolver"  yaml:"tlsResolver"  toml:"tlsResolver"  mapstructure:"tlsResolver"`
}

const defaultPath = "/"

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

func (e Endpoint) buildURI(port int, path string) string {
	uri := url.URL{
		Host:   fmt.Sprintf("%s:%d", e.Host, port),
		Scheme: "http",
		Path:   path,
	}

	if e.TLS != nil {
		uri.Scheme = "https"
	}

	return uri.String()
}

func (c *Config) PrepareClients(top context.Context) ([]*Client, error) {
	ctx, cancel := context.WithTimeout(top, c.ConnTimeout)
	defer cancel()

	out := make([]*Client, 0, len(c.Endpoints))
	for _, endpoint := range c.Endpoints {
		cli := new(http.Client)
		if endpoint.TLS != nil && endpoint.TLS.IgnoreInsecure {
			cli.Transport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // #nosec G402
			}
		}

		// The reachability probe is diagnostic only. An endpoint that is down
		// when the provider is constructed must NOT abort construction: hosts
		// get rebooted, and a later poll picks them up again. Aborting here
		// leaves Traefik with no provider at all -- and because Traefik does not
		// retry a provider that failed to start, every endpoint stays unrouted
		// until Traefik itself is restarted.
		var err error
		for _, port := range []int{endpoint.API, endpoint.WEB} {
			uri := endpoint.buildURI(port, defaultPath)

			var req *http.Request
			if req, err = http.NewRequestWithContext(ctx, http.MethodGet, uri, nil); err != nil {
				return nil, fmt.Errorf("could not prepare request(%s): %w", uri, err)
			}

			var res *http.Response
			if res, err = cli.Do(req); err != nil {
				log.Printf(
					"endpoint unreachable at startup, will retry on next poll (%s): %s",
					uri,
					err,
				)

				continue
			}

			if err = res.Body.Close(); err != nil {
				log.Printf("could not close response body (%s): %s", uri, err)
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
