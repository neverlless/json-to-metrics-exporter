# JSON to Metrics Exporter

[![Go](https://github.com/neverlless/json-to-metrics-exporter/actions/workflows/go.yml/badge.svg)](https://github.com/neverlless/json-to-metrics-exporter/actions/workflows/go.yml)
[![Docker Image Publish](https://github.com/neverlless/json-to-metrics-exporter/actions/workflows/docker.yml/badge.svg)](https://github.com/neverlless/json-to-metrics-exporter/actions/workflows/docker.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

A Prometheus exporter that probes JSON endpoints, such as health checks, and turns their responses into metrics. Like the Blackbox exporter, it takes the target from each scrape, so one instance can serve many endpoints.

## Features

- Probes any JSON endpoint passed in the `target` query parameter
- Converts numbers, booleans, strings, nested objects and arrays into metrics
- Decides what counts as healthy with a configurable regex
- Reports HTTP probe metrics: duration, status code and success
- Restricts which URLs can be probed with an allowlist
- Ships as a small image that runs as a non-root user
- Includes a [Grafana dashboard](grafana/grafana_dashboard.json)

## Quick start

```bash
docker run -p 9908:9908 ghcr.io/neverlless/json-to-metrics-exporter:latest
curl "http://localhost:9908/metrics?target=https://your-json-endpoint"
```

Binaries for Linux and macOS are on the [Releases](https://github.com/neverlless/json-to-metrics-exporter/releases) page.

To build from source, you need Go 1.27 or newer:

```bash
git clone https://github.com/neverlless/json-to-metrics-exporter.git
cd json-to-metrics-exporter
go build -o json-to-metrics ./cmd
./json-to-metrics
```

## Configuration

All settings are environment variables.

| Variable | Default | Description |
| --- | --- | --- |
| `HOST` | `0.0.0.0` | Address to listen on |
| `PORT` | `9908` | Port to listen on |
| `HEALTHY_REGEX` | `OK\|success` | Strings that match are healthy |
| `ALLOWED_TARGETS` | not set | Regex that must match the whole target URL. When not set, any URL is allowed |
| `TLS_INSECURE` | `false` | Set to `true` to skip TLS certificate checks |

The exporter refuses to start if a regex is invalid.

### Security

The exporter requests whatever URL the `target` parameter names, so anyone who can reach its port can make it call your internal network. Set `ALLOWED_TARGETS` unless the port is reachable only by Prometheus:

```bash
ALLOWED_TARGETS='https://([a-z0-9-]+\.)?example\.com/.*'
```

Other targets get `403 Forbidden`, and redirects to them are not followed.

## Prometheus configuration

```yaml
scrape_configs:
  - job_name: json
    metrics_path: /metrics
    static_configs:
      - targets:
          - https://service-a.example.com/health
          - https://service-b.example.com/health
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: json-to-metrics:9908
```

## Metrics

### Probe metrics

| Metric | Description |
| --- | --- |
| `probe_success` | `1` if the target returned HTTP 200 and valid JSON, otherwise `0` |
| `probe_http_duration_seconds` | Duration of the HTTP request |
| `probe_http_status_code` | HTTP status code returned by the target. Absent if no response was received |

Responses larger than 10 MiB count as failed probes.

### JSON metrics

For example, this response:

```json
{
  "status": "OK",
  "version": "1.2.3",
  "connections": 42,
  "ready": true,
  "checks": { "database": "OK", "cache": "down", "queue": true, "latency_ms": 12 },
  "nodes": [{ "load": 0.5 }]
}
```

becomes:

```text
jsonmetric_status{text="OK",url="..."} 1
jsonmetric_version{url="..."} 0
jsonmetric_connections{url="..."} 42
jsonmetric_ready{url="..."} 1
jsonmetric_services{category="checks",service="database",url="..."} 1
jsonmetric_services{category="checks",service="cache",url="..."} 0
jsonmetric_services{category="checks",service="queue",url="..."} 1
jsonmetric_checks_latency_ms{url="..."} 12
jsonmetric_nodes_0_load{url="..."} 0.5
```

- Numbers keep their value, and booleans become `1` or `0`.
- Strings become `1` if they match `HEALTHY_REGEX`, otherwise `0`.
- Strings and booleans inside an object become `jsonmetric_services`, with the object's path in the `category` label.
- Only strings that match `HEALTHY_REGEX` get a `text` label, so free text such as a timestamp does not create a new time series on every scrape.
- Characters outside `a-zA-Z0-9` in a name become `_`. If two names end up the same, the first one found is kept.

## Upgrading from earlier versions

- Images moved from Docker Hub to `ghcr.io/neverlless/json-to-metrics-exporter`.
- JSON metric names lost their trailing underscore: `jsonmetric_status_` is now `jsonmetric_status`.
- The `text` label is now set only on values that match `HEALTHY_REGEX`.
- Booleans inside objects now report their real value. Numbers and nested objects there are no longer reported as failed services.
- `probe_success` is now `0` for invalid JSON, and for a non-200 response the scrape no longer fails with HTTP 500.
- TLS certificates are now verified. Set `TLS_INSECURE=true` for targets with self-signed certificates.

## Development

```bash
go test -race ./...
go vet ./...
```

## License

This project is licensed under the [MIT License](LICENSE).
