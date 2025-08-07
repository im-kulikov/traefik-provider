# Traefik Provider Plugin

![Traefik Provider Logo](.github/logo.png)

This is a custom provider plugin for [Traefik v3](https://doc.traefik.io/traefik/) that dynamically configures routing 
based on the state of another Traefik instance. It polls the `/api/rawdata` endpoint of the remote Traefik and translates 
the configuration into the current instance.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
   - [Endpoint Object](#endpoint-object)
- [Use Case](#use-case)
   - [🏡 Example: Distributed HomeLab with Multiple Docker Hosts](#-example-distributed-homelab-with-multiple-docker-hosts)
   - [✅ Perfect for:](#-perfect-for)
- [License](#license)

## Features

* Automatically synchronizes routes from a remote Traefik instance
* Supports multiple endpoints
* Customizable polling interval and timeout
* Integrates with existing TLS resolvers

## Installation

To use this provider plugin:

1. Enable experimental plugins in your Traefik configuration:
    ```yaml
    experimental:
      plugins:
        traefik-provider:
          moduleName: "github.com/im-kulikov/traefik-provider"
          version: "v0.1.0"
    ```

2. Configure the provider:
    ```yaml
    providers:
      plugin:
        traefik-provider:
          pollInterval: "5s"
          connTimeout: "15s"
          tlsResolver: "letsencrypt"
          endpoints:
            - host: localhost
              apiPort: 8080
              webPort: 5180
    ```

## Configuration

| Key            | Type   | Default | Description                            |
| -------------- | ------ | ------- | -------------------------------------- |
| `pollInterval` | string | "5s"    | Time between syncs with remote Traefik |
| `connTimeout`  | string | "15s"   | Connection timeout when polling remote |
| `tlsResolver`  | string |         | Optional name of the TLS cert resolver |
| `endpoints`    | list   |         | List of remote Traefik endpoints       |

### Endpoint Object

Each endpoint in `endpoints` should include:

```yaml
- host: 127.0.0.1
  apiPort: 8080
  webPort: 80
```

* `host`: IP or hostname of the remote Traefik
* `apiPort`: Port used to fetch `/api/rawdata`
* `webPort`: Port used for service routing

## Use Case

This provider is useful for:

* Multi-zone or distributed Traefik setups
* Scenarios where a central Traefik instance proxies traffic to internal instances

### 🏡 Example: Distributed HomeLab with Multiple Docker Hosts

Imagine a HomeLab environment consisting of **several Docker hosts**, each running its own stack of containers and services. On each of these hosts runs a **Traefik instance in "slave" mode**, responsible for watching local containers and exposing service metadata.

At the same time, there's a central **Traefik** — the **single point of entry** into your HomeLab. This primary instance:

- Connects to multiple Traefik workers via their API and WEB ports
- Aggregates and applies routing rules from all connected hosts
- Serves as the unified external access point into your local network
- Can use middlewares, TLS configurations, and plugins across the entire HomeLab

This plugin makes it possible to:

- Dynamically discover services from all Traefik worker nodes
- Filter them using labels (e.g. `traefik-internal=true`)
- Configure routing on the primary node without duplicating setup
- Apply unified access policies and security settings

### ✅ Perfect for:

- A **distributed self-hosted environment**
- Managing microservices across **multiple lightweight servers or clusters**
- Building a robust and modular **smart home backend**
- Running isolated workloads in different zones while maintaining a **centralized entry point**

With this setup, your reverse proxy becomes **truly decentralized**, while remaining **centrally manageable**.

## License

[MIT License](LICENSE)
