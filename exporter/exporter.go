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

// Exporter exporter for sentry metrics
type Exporter struct {
	client                 *sentry.Client
	maxFetchConccurrency   uint32
	projectStatDesc        *prometheus.Desc
	statResolution         string
	statResolutionDuration time.Duration
	sentryUp               *prometheus.Desc
	scrapeDurationDesc     *prometheus.Desc
	totalScrapes           prometheus.Counter
}

// Describe visit all prometheus.Desc contained in this exporter
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.projectStatDesc
	ch <- e.sentryUp
	ch <- e.scrapeDurationDesc
	ch <- e.totalScrapes.Desc()
}

// Collect visit all prometheus metrics contained in this exporter
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	defer func() {
		ch <- prometheus.MustNewConstMetric(
			e.scrapeDurationDesc,
			prometheus.GaugeValue,
			time.Since(start).Seconds(),
		)
	}()
	e.collectOrganizations(ch)
	e.totalScrapes.Inc()
	ch <- e.totalScrapes
}

type projectFetchJob struct {
	organization sentry.Organization
	project      sentry.Project
	team         sentry.Team
}

func (e *Exporter) collectOrganizations(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	log.Debug("spawning organization")
	organizations, link, err := e.client.GetOrganizations()

	// note: go-sentry-api doesn't use pointers in a sane way, so this has to do
	// a *lot* of copying.  Upstream API has to improve for this to improve.
	workQueue := make(chan *projectFetchJob, e.maxFetchConccurrency)
	defer func() {
		close(workQueue)
		wg.Wait()
	}()

	for i := uint32(0); i < e.maxFetchConccurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				work, more := <-workQueue
				if !more {
					return
				}
				e.collectProjectStats(ch, &work.organization, &work.team, &work.project)
			}
		}()
	}

	for len(organizations) != 0 && err == nil {
		for orgIdx := range organizations {
			// repull the org; API doesn't give us useful results, but
			// GetOrganization gets the team/project listing we want.
			org, err := e.client.GetOrganization(*(organizations[orgIdx].Slug))
			if err != nil {
				log.Errorf("failed pulling organization details for %s: err %s", (*organizations[orgIdx].Slug), err)
				continue
			}
			for _, team := range *(org.Teams) {

				for _, project := range *(team.Projects) {
					workQueue <- &projectFetchJob{
						organization: org,
						project:      project,
						team:         team,
					}
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
	log.Debug("finished organizations")
	ch <- prometheus.MustNewConstMetric(
		e.sentryUp,
		prometheus.GaugeValue,
		upVal,
	)
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
					*(organization.ID),
					*(team.Slug),
					*(team.ID),
					*(project.Slug),
					project.ID,
					eventType,
				),
			)
		}
	}
	log.Debugf("finished project stats pull for organization %s, team %s, project %s", *(organization.Slug), *(team.Slug), *(project.Slug))
}

// NewExporter create a new sentry exporter
func NewExporter(client *sentry.Client, maxFetchConccurrency uint32, namespace string) (*Exporter, error) {
	projectLabels := []string{"organization_slug", "organization_id", "team_slug", "team_id", "project_slug", "project_id", "type"}
	return &Exporter{
		client:                 client,
		maxFetchConccurrency:   maxFetchConccurrency,
		statResolution:         "10s",
		statResolutionDuration: time.Second * 15,
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
		scrapeDurationDesc: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "exporter", "last_scrape_duration_seconds"),
			"duration in seconds for the last scrape",
			nil,
			nil,
		),
		totalScrapes: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: "exporter",
			Name:      "scrapes_total",
			Help:      "total number of scrapes",
		}),
	}, nil
}
