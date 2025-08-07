package traefik_provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/traefik/genconf/dynamic"

	"github.com/im-kulikov/traefik-provider/internal"
)

type PluginProvider interface {
	Init() error
	Provide(cfgChan chan<- json.Marshaler) error
	Stop() error
}

type Provider struct {
	config  *internal.Config
	routine Routine
	clients []*internal.Client
}

func New(ctx context.Context, cfg *Config, name string) (*Provider, error) {
	var cli []*internal.Client
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("could not validate config %q: %w", name, err)
	} else if cli, err = cfg.PrepareClients(ctx); err != nil {
		return nil, fmt.Errorf("could not prepare clients for %q: %w", name, err)
	}

	return &Provider{config: cfg.Config, clients: cli, routine: newRunner(ctx)}, nil
}

func (p *Provider) Init() error {
	if err := p.config.Validate(); err != nil {
		return err
	}

	if p.routine == nil {
		return errors.New("method New not called")
	}

	return nil
}

func fetchConfig(top context.Context, out chan<- json.Marshaler, clients []*internal.Client) error {
	merge := make(chan *dynamic.Configuration, 2)
	defer close(merge)

	run := newRunner(top)
	for _, client := range clients {
		run.Go(func(ctx context.Context) error { return client.FetchRaw(ctx, merge) })
	}

	run.Go(func(ctx context.Context) error {
		var (
			cnt int
			val dynamic.Configuration
		)

	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case msg := <-merge:
				cnt++

				if val.HTTP == nil {
					val.HTTP = &dynamic.HTTPConfiguration{
						Routers:     make(map[string]*dynamic.Router),
						Services:    make(map[string]*dynamic.Service),
						Middlewares: make(map[string]*dynamic.Middleware),
					}
				}

				for key, item := range msg.HTTP.Routers {
					val.HTTP.Routers[key] = item
				}

				for key, item := range msg.HTTP.Services {
					val.HTTP.Services[key] = item
				}

				for key, item := range msg.HTTP.Middlewares {
					val.HTTP.Middlewares[key] = item
				}

				if cnt == len(clients) {
					break loop
				}
			}
		}

		out <- dynamic.JSONPayload{Configuration: &val}

		return nil
	})

	if err := run.Wait(); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}

	return nil
}

func (p *Provider) Provide(out chan<- json.Marshaler) error {
	if p.routine == nil {
		return errors.New("method New not called")
	}

	p.routine.Go(func(top context.Context) error {
		tick := time.NewTimer(time.Microsecond)
		defer tick.Stop()

		for {
			select {
			case <-top.Done():
				return nil
			case <-tick.C:
				ctx, cancel := context.WithTimeout(top, p.config.PollInterval)
				if err := fetchConfig(ctx, out, p.clients); err != nil {
					log.Print(err)
				}
				cancel()

				tick.Reset(p.config.PollInterval)
			}
		}
	})

	return nil
}

func (p *Provider) Stop() error {
	p.routine.Cancel(nil)

	return p.routine.Wait()
}
