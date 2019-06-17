package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/atlassian/go-sentry-api"
	"github.com/ferringb/prometheus_sentry_exporter/exporter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var (
	listen            = flag.String("web.listen-address", ":9096", "The host:port to listen on for HTTP requests")
	metricsPath       = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics")
	sentryURL         = flag.String("sentry.url", "", "http url for the sentry instance to talk to.  Cal be specified via environment variable SENTRY_URL")
	sentryAuthToken   = flag.String("sentry.auth-token", "", "bearer token to use for authorization.  Can be specified via environment variable SENTRY_AUTH_TOKEN")
	sentryTimeout     = flag.Duration("sentry.timeout", time.Second*10, "http timeouts to enforce for sentry requests")
	sentryConcurrency = flag.Int("sentry.concurrency", 40, "level of concurrent stats requests to allow against the given sentry")
	logLevel          = flag.String("log.level", "info", "log level")
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

const metricsIndexPage = `<html>
	<head><title>prometheus_sentry_exporter</title</head>
	<body>
		<li>prometheus metrics endpoint: <a href="/metrics"><code>/metrics</code></a></li>
	</body>
</html>
`

func main() {
	flag.Parse()
	if err := integrateEnvAndCheckFlag("-sentry.url", "SENTRY_URL", sentryURL); err != nil {
		log.Fatal(err.Error())
	}
	if err := integrateEnvAndCheckFlag("-sentry.auth-token", "SENTRY_AUTH_TOKEN", sentryAuthToken); err != nil {
		log.Fatal(err.Error())
	}
	if *sentryConcurrency <= 0 {
		log.Fatalf("-senty.concurency needs to be >= 1, got %d", *sentryConcurrency)
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
	metricExporter, err := exporter.NewExporter(client, uint32(*sentryConcurrency), "sentry")
	if err != nil {
		log.Fatalf("failed to create exporter: %s", err)
	}
	prometheus.MustRegister(metricExporter)
	log.Infof("starting server; telemetry accessible at %s%s", *listen, *metricsPath)
	http.Handle(*metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, metricsIndexPage)
	})
	log.Fatal(http.ListenAndServe(*listen, nil))
}
