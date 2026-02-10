package traefik_provider

import (
	"errors"
	"fmt"
	"time"

	"github.com/im-kulikov/traefik-provider/internal"
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
	ConnTimeout  string     `json:"connTimeout"  yaml:"connTimeout"  toml:"connTimeout"  mapstructure:"connTimeout"`
	PollInterval string     `json:"pollInterval" yaml:"pollInterval" toml:"pollInterval" mapstructure:"pollInterval"`
	Endpoints    []Endpoint `json:"endpoints"    yaml:"endpoints"    toml:"endpoints"    mapstructure:"endpoints"`
	TLSResolver  *string    `json:"tlsResolver"  yaml:"tlsResolver"  toml:"tlsResolver"  mapstructure:"tlsResolver"`

	*internal.Config `mapstructure:"-"`
}

func CreateConfig() *Config {
	return &Config{
		PollInterval: "5s",
		ConnTimeout:  "15s",
	}
}

func (c *Config) validate() error {
	if c == nil {
		return errors.New("empty config")
	}

	c.Config = new(internal.Config)

	var err error
	if c.Config.ConnTimeout, err = time.ParseDuration(c.ConnTimeout); err != nil {
		return fmt.Errorf("wrong connection timeout(%q): %w", c.ConnTimeout, err)
	}

	if c.Config.PollInterval, err = time.ParseDuration(c.PollInterval); err != nil {
		return fmt.Errorf("wrong poll interval(%q): %w", c.PollInterval, err)
	}

	if len(c.Endpoints) == 0 {
		return fmt.Errorf("empty endpoints: %d", len(c.Endpoints))
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

		var tlsConfig *internal.TLS
		if endpoint.TLS != nil {
			tlsConfig = &internal.TLS{
				IgnoreInsecure: endpoint.TLS.IgnoreInsecure,
			}
		}

		c.Config.Endpoints = append(c.Config.Endpoints, internal.Endpoint{
			Host: endpoint.Host,
			API:  endpoint.API,
			WEB:  endpoint.WEB,
			TLS:  tlsConfig,
		})
	}

	if c.TLSResolver != nil {
		c.Config.TLSResolver = c.TLSResolver
	}

	return c.Validate()
}
