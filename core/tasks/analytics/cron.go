package analytics

import (
	"context"
	"time"

	"github.com/nyaruka/gocommon/analytics"
	"github.com/nyaruka/mailroom"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/sirupsen/logrus"
)

func init() {
	mailroom.RegisterCron("analytics", time.Second*60, true, reportAnalytics)
}

var (
	// both sqlx and redis provide wait stats which are cummulative that we need to make into increments
	dbWaitDuration    time.Duration
	dbWaitCount       int64
	redisWaitDuration time.Duration
	redisWaitCount    int64
)

// calculates a bunch of stats every minute and both logs them and sends them to librato
func reportAnalytics(ctx context.Context, rt *runtime.Runtime) error {
	// We wait 15 seconds since we fire at the top of the minute, the same as expirations.
	// That way any metrics related to the size of our queue are a bit more accurate (all expirations can
	// usually be handled in 15 seconds). Something more complicated would take into account the age of
	// the items in our queues.
	time.Sleep(time.Second * 15)

	rc := rt.RP.Get()
	defer rc.Close()

	// calculate size of batch queue
	batchSize, err := queue.Size(rc, queue.BatchQueue)
	if err != nil {
		logrus.WithError(err).Error("error calculating batch queue size")
	}

	// and size of handler queue
	handlerSize, err := queue.Size(rc, queue.HandlerQueue)
	if err != nil {
		logrus.WithError(err).Error("error calculating handler queue size")
	}

	// get our DB and redis stats
	dbStats := rt.DB.Stats()
	redisStats := rt.RP.Stats()

	dbWaitDurationInPeriod := dbStats.WaitDuration - dbWaitDuration
	dbWaitCountInPeriod := dbStats.WaitCount - dbWaitCount
	redisWaitDurationInPeriod := redisStats.WaitDuration - redisWaitDuration
	redisWaitCountInPeriod := redisStats.WaitCount - redisWaitCount

	dbWaitDuration = dbStats.WaitDuration
	dbWaitCount = dbStats.WaitCount
	redisWaitDuration = redisStats.WaitDuration
	redisWaitCount = redisStats.WaitCount

	analytics.Gauge("mr.db_busy", float64(dbStats.InUse))
	analytics.Gauge("mr.db_idle", float64(dbStats.Idle))
	analytics.Gauge("mr.db_wait_ms", float64(dbWaitDurationInPeriod/time.Millisecond))
	analytics.Gauge("mr.db_wait_count", float64(dbWaitCountInPeriod))
	analytics.Gauge("mr.redis_wait_ms", float64(redisWaitDurationInPeriod/time.Millisecond))
	analytics.Gauge("mr.redis_wait_count", float64(redisWaitCountInPeriod))
	analytics.Gauge("mr.handler_queue", float64(handlerSize))
	analytics.Gauge("mr.batch_queue", float64(batchSize))

	logrus.WithFields(logrus.Fields{
		"db_busy":          dbStats.InUse,
		"db_idle":          dbStats.Idle,
		"db_wait_time":     dbWaitDurationInPeriod,
		"db_wait_count":    dbWaitCountInPeriod,
		"redis_wait_time":  dbWaitDurationInPeriod,
		"redis_wait_count": dbWaitCountInPeriod,
		"handler_size":     handlerSize,
		"batch_size":       batchSize,
	}).Info("current analytics")

	return nil
}
