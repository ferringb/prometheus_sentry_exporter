package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/atlassian/go-sentry-api"
	"github.com/ferringb/prometheus_sentry_exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var (
	listen             = flag.String("web.listen-address", ":9096", "The host:port to listen on for HTTP requests")
	metricsPath        = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	sentryURL          = flag.String("sentry.url", "", "http url for the sentry instance to talk to.  Cal be specified via environment variable SENTRY_URL")
	sentryOrganization = flag.String("sentry.organization", "", "organization slug to exposed.  Can be specified via environment variable SENTRY_ORGANIZATION")
	sentryAuthToken    = flag.String("sentry.auth-token", "", "bearer token to use for authorization.  Can be specified via environment variable SENTRY_AUTH_TOKEN")
	sentryTimeout      = flag.Duration("sentry.timeout", time.Second*10, "http timeouts to enforce for sentry requests")
	logLevel           = flag.String("log.level", "info", "log level")
)

func integrateEnvAndCheckFlag(flagName string, envName string, flagValue *string) error {
	if *flagValue == "" {
		s := os.Getenv(envName)
		if s != "" {
			*flagValue = s
		}
	}
	if *flagValue == "" {
		return fmt.Errorf("neither %s nor environment variable %s was defined; this required", flagName, envName)
	}
	return nil
}

func main() {
	flag.Parse()
	if err := integrateEnvAndCheckFlag("-sentry.url", "SENTRY_URL", sentryURL); err != nil {
		log.Fatal(err.Error())
	}
	if err := integrateEnvAndCheckFlag("-sentry.organiation", "SENTRY_ORGANIZATION", sentryOrganization); err != nil {
		log.Fatal(err.Error())
	}
	if err := integrateEnvAndCheckFlag("-sentry.auth-token", "SENTRY_AUTH_TOKEN", sentryAuthToken); err != nil {
		log.Fatal(err.Error())
	}
	if err := log.Base().SetLevel(*logLevel); err != nil {
		log.Fatal(err.Error())
	}

	timeout := int(sentryTimeout.Seconds())
	apiURL := fmt.Sprintf("%s/api/0/", *sentryURL)
	client, err := sentry.NewClient(*sentryAuthToken, &apiURL, &timeout)
	if err != nil {
		log.Fatalf("failed to create sentry client: %s", err)
	}
	metricExporter, err := exporter.NewExporter(client, "sentry")
	if err != nil {
		log.Fatalf("failed to create exporter: %s", err)
	}
	prometheus.MustRegister(metricExporter)
	log.Infof("starting server; telemetry accessible at %s%s", *listen, *metricsPath)
	http.Handle(*metricsPath, prometheus.Handler())
	log.Fatal(http.ListenAndServe(*listen, nil))
}
