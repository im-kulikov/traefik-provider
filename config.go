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
}

type Config struct {
	ConnTimeout  string     `json:"connTimeout"  yaml:"connTimeout"  toml:"connTimeout"  mapstructure:"connTimeout"`
	PollInterval string     `json:"pollInterval" yaml:"pollInterval" toml:"pollInterval" mapstructure:"pollInterval"`
	Endpoints    []Endpoint `json:"endpoints"    yaml:"endpoints"    toml:"endpoints"    mapstructure:"endpoints"`
	TLSResolver  *string    `json:"tlsResolver"  yaml:"tlsResolver"  toml:"tlsResolver"  mapstructure:"tlsResolver"`

	*internal.Config `mapstructure:"-"`
}

func CreateConfig() *Config { return new(Config) }

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

		c.Config.Endpoints = append(c.Config.Endpoints, internal.Endpoint{
			Host: endpoint.Host,
			API:  endpoint.API,
			WEB:  endpoint.WEB,
		})
	}

	if c.TLSResolver != nil {
		c.Config.TLSResolver = c.TLSResolver
	}

	return c.Validate()
}
