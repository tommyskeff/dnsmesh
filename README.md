# DNSMesh

Simple DNS-based service discovery for multi-cluster Kubernetes

## Overview

DNSMesh watches pods and services across multiple Kubernetes clusters and serves DNS A records for resources with specific annotations. It enables seamless cross-cluster service discovery using a single DNS query.

## Installation

### Helm (Recommended)

```bash
# x-release-please-start-version
helm install dnsmesh oci://ghcr.io/tommyskeff/charts/dnsmesh --version 0.1.0 -n dnsmesh --create-namespace
# x-release-please-end
```

### Docker Image

```bash
# x-release-please-start-version
docker pull ghcr.io/tommyskeff/dnsmesh:0.1.0
# x-release-please-end
```

## DNS Format

```
<service>.<realm>.<domain>
```

- **service** - The service name (from `dnsmesh.tommyjs.dev/service` annotation)
- **realm** - The realm/environment (from `dnsmesh.tommyjs.dev/realm` annotation)
- **domain** - Base domain (from Helm `dns.domain`, default: `clusterset.local`)

Example: `nginx-test.prod-na.clusterset.local`

## Features

- **Multi-cluster support** - Watch resources across multiple Kubernetes clusters
- **Annotation-based discovery** - Opt-in via `dnsmesh.tommyjs.dev/expose: "true"`
- **Random load balancing** - Multiple IPs for the same name are returned randomly
- **Configurable TTL** - Control DNS caching behavior
- **Health endpoints** - `/healthz` and `/readyz` for Kubernetes probes
- **Exponential backoff** - Graceful handling of cluster connectivity issues

## Configuration

### Global Config Secret

DNSMesh reads cluster configuration from a Kubernetes secret. The secret must contain a `config` key with JSON in this format:

```json
{
  "clusters": [
    {
      "name": "na-prod",
      "apiServer": "https://10.20.1.1:6443",
      "caCert": "<CA_CERTIFICATE_PEM>",
      "token": "<SERVICE_ACCOUNT_TOKEN>"
    }
  ]
}
```

| Field | Description |
|-------|-------------|
| `clusters[].name` | Unique identifier for the cluster |
| `clusters[].apiServer` | Kubernetes API server URL |
| `clusters[].caCert` | CA certificate for TLS verification |
| `clusters[].token` | ServiceAccount bearer token |

### Pod/Service Annotations

Add these annotations to resources you want to expose via DNS:

```yaml
annotations:
  dnsmesh.tommyjs.dev/expose: "true"
  dnsmesh.tommyjs.dev/service: "my-app"
  dnsmesh.tommyjs.dev/realm: "prod-na"
```

This creates DNS records like `my-app.prod-na.clusterset.local`.

### CoreDNS Configuration

To forward DNS queries for your domain to DNSMesh, add a server block to your CoreDNS ConfigMap:

```
clusterset.local:53 {
    errors
    cache 30
    forward . <DNSMESH_SERVICE_IP>
}
```

## Helm Chart Values

See `charts/dnsmesh/values.yaml` for all options. Key settings:

| Value | Default | Description |
|-------|---------|-------------|
| `dns.domain` | `clusterset.local` | Base domain for all DNS records |
| `dns.ttl` | `30` | DNS record TTL in seconds |
| `logLevel` | `info` | Log level: debug, info, warn, error |
| `service.clusterIP` | `""` | Static ClusterIP for stable CoreDNS config |

## Development

```bash
go build -o dnsmesh ./cmd/dnsmesh
go test ./...
docker build -t dnsmesh:local .
helm upgrade --install dnsmesh ./charts/dnsmesh -n dnsmesh \
--set image.repository=dnsmesh --set image.tag=local
```

## Releases

This project uses [release-please](https://github.com/googleapis/release-please) to manage releases.

- **App releases**: Tagged as `v<version>` (e.g. `v0.1.0`)
- **Chart releases**: Tagged as `dnsmesh-chart-v<version>` (e.g. `dnsmesh-chart-v0.1.0`)

## License

Apache License 2.0
