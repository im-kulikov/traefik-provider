package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/traefik/genconf/dynamic"
)

type Client struct {
	*http.Client

	endpoint Endpoint
	resolver *string
}

const defaultRawPath = "/api/rawdata"

func (c *Client) FetchRaw(ctx context.Context, out chan<- *dynamic.Configuration) error {
	uri := url.URL{
		Scheme: "http",
		Path:   defaultRawPath,
		Host:   fmt.Sprintf("%s:%d", c.endpoint.Host, c.endpoint.API),
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uri.String(), nil)
	if err != nil {
		return fmt.Errorf("could not prepare request for %s: %w", uri.String(), err)
	}

	var res *http.Response
	if res, err = c.Do(req); err != nil {
		return fmt.Errorf("could not make request for %s: %w", uri.String(), err)
	}

	var result dynamic.Configuration
	if err = json.NewDecoder(res.Body).Decode(&result.HTTP); err != nil {
		return fmt.Errorf("could not decode response for %s: %w", uri.String(), err)
	}

	var output dynamic.Configuration
	for key, item := range result.HTTP.Routers {
		if strings.HasSuffix(key, "@internal") {
			continue
		}

		name := strings.Split(key, "@")[0]
		name = fmt.Sprintf("%s-%s", name, c.endpoint.Host)

		if output.HTTP == nil {
			output.HTTP = &dynamic.HTTPConfiguration{
				Routers:     make(map[string]*dynamic.Router),
				Services:    make(map[string]*dynamic.Service),
				Middlewares: make(map[string]*dynamic.Middleware),
			}
		}

		service, ok := result.HTTP.Services[key]
		if !ok {
			continue
		}

		output.HTTP.Routers[name] = &dynamic.Router{
			Service: name,
			Rule:    item.Rule,
		}

		var servers []dynamic.Server
		for range service.LoadBalancer.Servers {
			servers = append(servers, dynamic.Server{URL: (&url.URL{
				Scheme: "http",
				Host:   fmt.Sprintf("%s:%d", c.endpoint.Host, c.endpoint.WEB),
			}).String()})
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

	out <- &output

	return res.Body.Close()
}
