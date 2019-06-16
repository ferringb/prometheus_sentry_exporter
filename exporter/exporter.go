package exporter

import (
	"fmt"
	"sync"
	"time"

	"github.com/atlassian/go-sentry-api"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var collectedProjectStats = map[string]sentry.StatQuery{
	"received":    sentry.StatReceived,
	"rejected":    sentry.StatRejected,
	"blacklisted": sentry.StatBlacklisted,
}

// Exporter exporter for
type Exporter struct {
	client                 *sentry.Client
	projectStatDesc        *prometheus.Desc
	statResolution         string
	statResolutionDuration time.Duration
	sentryUp               *prometheus.Desc
}

// Describe visit all prometheus.Desc contained in this exporter
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.projectStatDesc
	ch <- e.sentryUp
}

// Collect visit all prometheus metrics contained in this exporter
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.collectOrganizations(ch)
}

func (e *Exporter) collectOrganizations(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	log.Debug("spawning organization")
	organizations, link, err := e.client.GetOrganizations()

	for len(organizations) != 0 && err == nil {
		for _, organization := range organizations {
			// repull the org; API doesn't give us useful results, but
			// GetOrganization gets the team/project listing we want.
			org, err := e.client.GetOrganization(*(organization.Slug))
			if err != nil {
				log.Errorf("failed pulling organization details for %s: err %s", (*organization.Slug), err)
				continue
			}
			for _, team := range *(org.Teams) {
				for _, project := range *(team.Projects) {
					wg.Add(1)
					go func(organization sentry.Organization, project sentry.Project, team sentry.Team) {
						defer wg.Done()
						e.collectProjectStats(ch, &organization, &team, &project)
					}(org, project, team)
				}
			}

		}
		if !link.Next.Results {
			break
		}
		link, err = e.client.GetPage(link.Next, organizations)
		log.Debugf("organization pagination results were %v, err=%v", link, err)
	}
	upVal := float64(1)
	if err != nil {
		log.Errorf("failed spawning organizations: %s", err)
		upVal = 0
	}
	ch <- prometheus.MustNewConstMetric(
		e.sentryUp,
		prometheus.GaugeValue,
		upVal,
	)
	wg.Wait()
	log.Debug("finished organizations")
}

func (e *Exporter) collectProjectStats(ch chan<- prometheus.Metric, organization *sentry.Organization, team *sentry.Team, project *sentry.Project) {
	log.Debugf("spawning project stats pull for organization %s, team %s, project %s", *(organization.Slug), *(team.Slug), *(project.Slug))
	until := time.Now()
	since := until.Add(-e.statResolutionDuration)
	for eventType, statQuery := range collectedProjectStats {
		stats, err := e.client.GetProjectStats(
			*organization,
			*project,
			statQuery,
			since.Unix(),
			until.Unix(),
			&e.statResolution,
		)
		if err != nil {
			log.Warnf("failed fetching stat type %s for project %s; err %s", eventType, *project.Slug, err)
		} else if len(stats) == 0 {
			log.Warnf("requested stat type %s for project %s returned no results", eventType, *project.Slug)
		} else {
			log.Debugf("stat type %s for project %s returned %v", eventType, *project.Slug, stats)
			lastStat := stats[len(stats)-1]
			ch <- prometheus.NewMetricWithTimestamp(
				time.Unix(int64(lastStat[0]), 0),
				prometheus.MustNewConstMetric(
					e.projectStatDesc,
					prometheus.GaugeValue,
					lastStat[1],
					*(organization.Slug),
					*(team.Slug),
					*(project.Slug),
					eventType,
				),
			)
		}
	}
	log.Debugf("finished project stats pull for organization %s, team %s, project %s", *(organization.Slug), *(team.Slug), *(project.Slug))
}

// NewExporter create a new sentry exporter
func NewExporter(client *sentry.Client, namespace string) (*Exporter, error) {
	projectLabels := []string{"organization_slug", "team_slug", "project_slug", "type"}
	return &Exporter{
		client:                 client,
		statResolution:         "10s",
		statResolutionDuration: time.Minute,
		projectStatDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "project", "events_count"),
			"project count for received events of a given type",
			projectLabels,
			nil,
		),
		sentryUp: prometheus.NewDesc(
			fmt.Sprintf("%s_up", namespace),
			"boolean, 1 if the sentry instance was reachable, zero if not",
			nil,
			nil,
		),
	}, nil
}
