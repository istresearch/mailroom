package hooks

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/nyaruka/null"
)

// CommitNameChangesHook is our hook for name changes
var CommitNameChangesHook models.EventCommitHook = &commitNameChangesHook{}

type commitNameChangesHook struct{}

// Apply commits our contact name changes as a bulk update for the passed in map of scene
func (h *commitNameChangesHook) Apply(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *models.OrgAssets, scenes map[*models.Scene][]interface{}) error {
	// build up our list of pairs of contact id and contact name
	updates := make([]*nameUpdate, 0, len(scenes))
	for s, e := range scenes {
		// we only care about the last name change
		event := e[len(e)-1].(*events.ContactNameChangedEvent)
		updates = append(updates, &nameUpdate{s.ContactID(), null.String(fmt.Sprintf("%.128s", event.Name))})
	}

	// do our update
	return models.BulkQuery(ctx, "updating contact name", tx, sqlUpdateContactName, updates)
}

// struct used for our bulk insert
type nameUpdate struct {
	ContactID models.ContactID `db:"id"`
	Name      null.String      `db:"name"`
}

const sqlUpdateContactName = `
UPDATE contacts_contact c
   SET name = r.name
  FROM (VALUES(:id, :name)) AS r(id, name)
 WHERE c.id = r.id::int`
