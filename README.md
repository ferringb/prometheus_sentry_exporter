# Prometheus sentry exporter

A [Prometheus](http://prometheus.io) metrics exporter for [Sentry](https://sentry.io/).

This exporter specifically exports the per project stats available via endpoints like https://docs.sentry.io/api/projects/get-project-stats/ .

This in turn gives you per project event counts; effectively you can much more easily graph what projects are currently hammering your sentry instance
and trigger alerts via prometheus accordingly.

# Build status
[![Build Status](https://travis-ci.org/ferringb/prometheus_sentry_exporter.svg?branch=master)](https://travis-ci.org/ferringb/prometheus_sentry_exporter)

## Usage

```sh
Usage of ./prometheus_sentry_exporter:
  -log.level string
    	log level (default "info")
  -sentry.auth-token string
    	bearer token to use for authorization.  Can be specified via environment variable SENTRY_AUTH_TOKEN
  -sentry.concurrency int
    	level of concurrent stats requests to allow against the given sentry (default 40)
  -sentry.timeout duration
    	http timeouts to enforce for sentry requests (default 10s)
  -sentry.url string
    	http url for the sentry instance to talk to.  Cal be specified via environment variable SENTRY_URL
  -web.listen-address string
    	The host:port to listen on for HTTP requests (default ":9096")
  -web.telemetry-path string
    	Path under which to expose metrics (default "/metrics")
```

## Developing

This codebase uses [dep](https://github.com/golang/dep) for vendoring.