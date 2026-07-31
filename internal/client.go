package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/traefik/genconf/dynamic"
)

type Client struct {
	*http.Client

	endpoint Endpoint
	resolver *string
}

const defaultRawPath = "/api/rawdata"

var ErrEmptyResponse = errors.New("received empty response")

func (c *Client) Endpoint() string {
	if c == nil {
		return "empty"
	}

	return c.endpoint.Host
}

func (c *Client) httpCall(ctx context.Context) (*dynamic.Configuration, error) {
	uri := c.endpoint.buildURI(c.endpoint.API, defaultRawPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri, nil)
	if err != nil {
		return nil, fmt.Errorf("could not prepare request for %s: %w", uri, err)
	}

	var res *http.Response
	if res, err = c.Do(req); err != nil {
		return nil, fmt.Errorf("could not make request for %s: %w", uri, err)
	}

	buf := new(bytes.Buffer)
	tee := io.TeeReader(res.Body, buf)

	var result dynamic.Configuration
	if err = json.NewDecoder(tee).Decode(&result.HTTP); err != nil {
		return nil, fmt.Errorf("could not decode response for %s: %s: %w", uri, buf.String(), err)
	}

	return &result, res.Body.Close()
}

func (c *Client) prepareResponse(res *dynamic.Configuration) *dynamic.Configuration {
	var output dynamic.Configuration
	for key, item := range res.HTTP.Routers {
		if strings.HasSuffix(key, "@internal") {
			continue
		}

		name := strings.Split(key, "@")[0]
		name = fmt.Sprintf("%s-%s", name, c.endpoint.Host)

		service, ok := res.HTTP.Services[key]
		if !ok {
			continue
		}

		if output.HTTP == nil {
			output.HTTP = &dynamic.HTTPConfiguration{
				Routers:     make(map[string]*dynamic.Router),
				Services:    make(map[string]*dynamic.Service),
				Middlewares: make(map[string]*dynamic.Middleware),
			}
		}

		output.HTTP.Routers[name] = &dynamic.Router{
			Service: name,
			Rule:    item.Rule,
		}

		var servers []dynamic.Server
		for range service.LoadBalancer.Servers {
			servers = append(servers, dynamic.Server{
				URL: c.endpoint.buildURI(c.endpoint.WEB, defaultPath),
			})
		}

		output.HTTP.Services[name] = &dynamic.Service{
			LoadBalancer: &dynamic.ServersLoadBalancer{Servers: servers},
		}

		if c.resolver != nil {
			output.HTTP.Routers[name].Middlewares = append(
				output.HTTP.Routers[name].Middlewares,
				"http2https",
			)

			output.HTTP.Routers[name+"-secure"] = &dynamic.Router{
				Service: name,
				Rule:    item.Rule,
				TLS:     &dynamic.RouterTLSConfig{CertResolver: *c.resolver},
			}

			output.HTTP.Middlewares["http2https"] = &dynamic.Middleware{
				RedirectScheme: &dynamic.RedirectScheme{Scheme: "https", Permanent: true},
			}
		}
	}

	return &output
}

func (c *Client) FetchRaw(ctx context.Context, out chan<- *dynamic.Configuration) error {
	if res, err := c.httpCall(ctx); err != nil {
		c.emit(ctx, out, nil)

		return err
	} else if len(res.HTTP.Routers) > 0 && len(res.HTTP.Services) > 0 {
		c.emit(ctx, out, c.prepareResponse(res))

		return nil
	}

	c.emit(ctx, out, nil)

	return fmt.Errorf("%w (1client:%q)", ErrEmptyResponse, c.endpoint.Host)
}

// emit delivers cfg to out, abandoning the send once ctx is done.
//
// out is a small buffered channel drained by a single aggregator. If that
// aggregator has already stopped collecting -- for example because the poll
// deadline expired -- an unconditional send would block forever, leaking the
// goroutine and hanging the WaitGroup the poll loop waits on, which would stop
// the provider from ever polling again.
func (c *Client) emit(
	ctx context.Context,
	out chan<- *dynamic.Configuration,
	cfg *dynamic.Configuration,
) {
	// Fast path: while the aggregator still has buffer space, deliver
	// unconditionally. This keeps behaviour identical to a plain send even when
	// ctx is already cancelled or nil, which callers rely on.
	select {
	case out <- cfg:
		return
	default:
	}

	if ctx == nil {
		out <- cfg

		return
	}

	// Buffer is full: wait for the aggregator, but abandon the send if the poll
	// has already been given up on, so this goroutine cannot leak.
	select {
	case out <- cfg:
	case <-ctx.Done():
	}
}
