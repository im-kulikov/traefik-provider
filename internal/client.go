package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
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

func PrepareName(name, endpoint string) string {
	return fmt.Sprintf("%s-%s", name, strings.NewReplacer(".", "_", ":", "__").Replace(endpoint))
}

func (c *Client) Endpoint() string {
	if c == nil {
		return "empty"
	}

	return net.JoinHostPort(c.endpoint.Host, strconv.Itoa(c.endpoint.API))
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
	endpoint := c.Endpoint()

	var output dynamic.Configuration
	for key, item := range res.HTTP.Routers {
		if strings.HasSuffix(key, "@internal") {
			continue
		}

		name := PrepareName(strings.Split(key, "@")[0], endpoint)

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

func (c *Client) FetchRaw(ctx context.Context, out chan<- *dynamic.Configuration) (err error) {
	var res *dynamic.Configuration
	if res, err = c.httpCall(ctx); err != nil {
		err = fmt.Errorf("(client:%q): %w", c.Endpoint(), err)
	} else if len(res.HTTP.Routers) > 0 && len(res.HTTP.Services) > 0 {
		out <- c.prepareResponse(res)

		return nil
	}

	out <- nil
	if err == nil {
		err = fmt.Errorf("(client:%q): %w", c.Endpoint(), ErrEmptyResponse)
	}

	return err
}
