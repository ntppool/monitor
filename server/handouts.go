package server

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"go.ntppool.org/common/logger"
	"go.ntppool.org/monitor/monitorsettings"
	"go.ntppool.org/monitor/ntpdb"
	"go.ntppool.org/monitor/server/metrics"
)

// recordHandouts counts the rows GetServers handed out and, for rows that
// were handed out before, the time since the previous handout. Active and
// testing rows of a down server are counted but left out of the interval
// histogram: they come up on the fixed 120-minute cadence, not the
// status's interval, and would skew its quantiles.
func recordHandouts(m *metrics.Metrics, ipVersion string, rows []ntpdb.GetServersRow, now time.Time) {
	for _, row := range rows {
		status := string(row.Status)
		m.QueueHandouts.WithLabelValues(ipVersion, status).Inc()
		if row.PrevQueueTs.Valid && !row.Down {
			m.QueueHandoutInterval.WithLabelValues(ipVersion, status).
				Observe(now.Sub(row.PrevQueueTs.Time).Seconds())
		}
	}
}

// handoutSpanAttributes returns one "handouts.<status>" attribute with the
// number of rows for each status present in rows.
func handoutSpanAttributes(rows []ntpdb.GetServersRow) []attribute.KeyValue {
	counts := map[ntpdb.ServerScoresStatus]int{}
	for _, row := range rows {
		counts[row.Status]++
	}
	attrs := make([]attribute.KeyValue, 0, len(counts))
	for status, count := range counts {
		attrs = append(attrs, attribute.Int("handouts."+string(status), count))
	}
	return attrs
}

// trackQueueGauges refreshes queue_live_servers and queue_interval_seconds
// every interval until ctx is done.
func (srv *Server) trackQueueGauges(ctx context.Context, every time.Duration) {
	log := logger.FromContext(ctx)

	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		if err := srv.updateQueueGauges(ctx, srv.settings.Current()); err != nil {
			log.ErrorContext(ctx, "could not update queue gauges", "err", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (srv *Server) updateQueueGauges(ctx context.Context, settings monitorsettings.MonitorSettings) error {
	counts, err := srv.db.CountLiveServers(ctx)
	if err != nil {
		return err
	}
	for _, c := range counts {
		srv.m.QueueLiveServers.WithLabelValues(string(c.IpVersion)).Set(float64(c.Servers))
	}

	srv.m.QueueInterval.WithLabelValues(string(ntpdb.ServerScoresStatusActive)).Set(settings.IntervalActive.Seconds())
	srv.m.QueueInterval.WithLabelValues(string(ntpdb.ServerScoresStatusTesting)).Set(settings.IntervalTesting.Seconds())
	srv.m.QueueInterval.WithLabelValues(string(ntpdb.ServerScoresStatusCandidate)).Set(settings.IntervalCandidate.Seconds())
	return nil
}
