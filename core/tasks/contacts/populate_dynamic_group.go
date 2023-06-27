package contacts

import (
	"context"
	"fmt"
	"time"

	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/search"
	"github.com/nyaruka/mailroom/core/tasks"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/redisx"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TypePopulateDynamicGroup is the type of the populate group task
const TypePopulateDynamicGroup = "populate_dynamic_group"

const populateLockKey string = "lock:pop_dyn_group_%d"

func init() {
	tasks.RegisterType(TypePopulateDynamicGroup, func() tasks.Task { return &PopulateDynamicGroupTask{} })
}

// PopulateDynamicGroupTask is our task to populate the contacts for a dynamic group
type PopulateDynamicGroupTask struct {
	GroupID models.GroupID `json:"group_id"`
	Query   string         `json:"query"`
}

// Timeout is the maximum amount of time the task can run for
func (t *PopulateDynamicGroupTask) Timeout() time.Duration {
	return time.Hour
}

// Perform figures out the membership for a query based group then repopulates it
func (t *PopulateDynamicGroupTask) Perform(ctx context.Context, rt *runtime.Runtime, orgID models.OrgID) error {
	locker := redisx.NewLocker(fmt.Sprintf(populateLockKey, t.GroupID), time.Hour)
	lock, err := locker.Grab(rt.RP, time.Minute*5)
	if err != nil {
		return errors.Wrapf(err, "error grabbing lock to repopulate smart group: %d", t.GroupID)
	}
	defer locker.Release(rt.RP, lock)

	start := time.Now()
	log := logrus.WithFields(logrus.Fields{
		"group_id": t.GroupID,
		"org_id":   orgID,
		"query":    t.Query,
	})

	log.Info("starting population of smart group")

	oa, err := models.GetOrgAssets(ctx, rt, orgID)
	if err != nil {
		return errors.Wrapf(err, "unable to load org when populating group: %d", t.GroupID)
	}

	count, err := search.PopulateSmartGroup(ctx, rt.DB, rt.ES, oa, t.GroupID, t.Query)
	if err != nil {
		return errors.Wrapf(err, "error populating smart group: %d", t.GroupID)
	}
	logrus.WithField("elapsed", time.Since(start)).WithField("count", count).Info("completed populating smart group")

	return nil
}
