# JSON to Metrics Exporter

This is a JSON to Prometheus metrics exporter written in Go. It converts JSON data from specified endpoints into metrics formatted for Prometheus.

## Features

- Dynamic JSON endpoint fetching
- Configurable health status with regex
- Compatible with Prometheus scraping
- HTTP probe metrics (duration, status code, success)
- Lightweight and efficient

## Metrics

The exporter provides the following metrics:

### Standard Probe Metrics

- **`probe_http_duration_seconds`** - Duration of HTTP request in seconds
- **`probe_http_status_code`** - HTTP status code returned by the target
- **`probe_success`** - Indicates whether the probe succeeded (1 = success, 0 = failure)

### JSON-based Metrics

- **`jsonmetric_services`** - Service health status parsed from JSON (with labels: service, category, url)
- **`jsonmetric_*`** - Dynamic metrics parsed from JSON structure based on the response content

## Installation

### From Source

1. Clone the repository:

   ```bash
   git clone https://github.com/yourrepo/json-to-metrics-exporter.git
   cd json-to-metrics-exporter
   ```

2. Build and run:

   ```bash
   go build -o json-to-metrics cmd/main.go
   ./json-to-metrics
   ```

### Using Docker

1. Build the Docker image:

   ```bash
   docker build -t json-to-metrics-exporter .
   ```

2. Run the Docker container:

   ```bash
   docker run -p 9908:9908 json-to-metrics-exporter
   ```

## Usage

### Environment Variables

- `PORT`: Port on which the exporter will listen (default `9908`).
- `HOST`: IP address on which the server will run (default `0.0.0.0`).
- `HEALTHY_REGEX`: Regex pattern for determining healthy metrics (default `OK|success`).

### Fetching Metrics

Send a request to the `/metrics` endpoint, e.g.,

```bash
curl "http://localhost:9908/metrics?target=https://your-json-endpoint"
```

## License

This project is licensed under the MIT License.
