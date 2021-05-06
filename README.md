# Prometheus lolMiner Exporter

[![GitHub release](https://img.shields.io/github/v/release/HON95/prometheus-lolminer-exporter?label=Version)](https://github.com/HON95/prometheus-lolminer-exporter/releases)
[![CI](https://github.com/HON95/prometheus-lolminer-exporter/workflows/CI/badge.svg?branch=master)](https://github.com/HON95/prometheus-lolminer-exporter/actions?query=workflow%3ACI)
[![FOSSA status](https://app.fossa.com/api/projects/git%2Bgithub.com%2FHON95%2Fprometheus-lolminer-exporter.svg?type=shield)](https://app.fossa.com/projects/git%2Bgithub.com%2FHON95%2Fprometheus-lolminer-exporter?ref=badge_shield)
[![Docker pulls](https://img.shields.io/docker/pulls/hon95/prometheus-lolminer-exporter?label=Docker%20Hub)](https://hub.docker.com/r/hon95/prometheus-lolminer-exporter)

![Dashboard](https://grafana.com/api/dashboards/14296/images/10340/image)

## Usage

### lolMiner

Specify `--apiport=<port>` for lolMiner to enable the API server on the specified port.

### Exporter (Docker)

Example `docker-compose.yml`:

```yaml
version: "3.7"

services:
  lolminer-exporter:
    image: hon95/hon95/prometheus-lolminer-exporter:1
    #command:
    #  - '--endpoint=:8080'
    #  - '--debug'
    user: 1000:1000
    environment:
      - TZ=Europe/Oslo
    ports:
      - "8080:8080/tcp"
```

### Prometheus

Example `prometheus.yml`:

```yaml
global:
    scrape_interval: 15s
    scrape_timeout: 10s

scrape_configs:
  - job_name: "lolminer"
    static_configs:
      # Insert lolminer address here
      - targets: ["lolminer:3493"]
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        # Insert lolMiner exporter address here
        replacement: lolminer-exporter:8080
```

### Grafana

[Example dashboard](https://grafana.com/grafana/dashboards/14296).

## Configuration

### Docker Image Versions

Use `1` for stable v1.Y.Z releases and `latest` for bleeding/unstable releases.

## Metrics

See the [example output](examples/output.txt) (I'm too lazy to create a pretty table).

### Docker

See the dev/example Docker Compose file: [docker-compose.yml](dev/docker-compose.yml)

## Development

- Build (Go): `go build -o prometheus-lolminer-exporter`
- Lint: `golint ./..`
- Build and run along Traefik (Docker Compose): `docker-compose -f dev/docker-compose.yml up --force-recreate --build`

## License

GNU General Public License version 3 (GPLv3).
