package models

import (
	"context"
	"encoding/json"

	"github.com/jmoiron/sqlx"
	"github.com/juju/errors"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/legacy"
)

type FlowID int

// Flow is the mailroom type for a flow
type Flow struct {
	id         FlowID
	uuid       assets.FlowUUID
	name       string
	definition json.RawMessage
}

// ID returns the ID for this flow
func (f *Flow) ID() FlowID { return f.id }

// UUID returns the UUID for this flow
func (f *Flow) UUID() assets.FlowUUID { return f.uuid }

// Name returns the name of this flow
func (f *Flow) Name() string { return f.name }

// Definition returns the definition for this flow
func (f *Flow) Definition() json.RawMessage { return f.definition }

// loads the flow with the passed in UUID
func loadFlow(ctx context.Context, db *sqlx.DB, uuid assets.FlowUUID) (assets.Flow, error) {
	flow := &Flow{uuid: uuid}

	rows, err := db.Queryx(selectFlowSQL, uuid)
	if err != nil {
		return nil, errors.Annotatef(err, "error querying flow with uuid: %s", uuid)
	}
	defer rows.Close()

	// no row, no flow!
	if !rows.Next() {
		return nil, nil
	}

	var oldDefinition string
	err = rows.Scan(&flow.id, &flow.name, oldDefinition)
	if err != nil {
		return nil, errors.Annotatef(err, "error scanning flow with uuid: %s", uuid)
	}

	// load it in from our json
	legacyFlow, err := legacy.ReadLegacyFlow([]byte(oldDefinition))
	if err != nil {
		return nil, errors.Annotatef(err, "error reading flow into legacy format: %s", uuid)
	}

	// migrate forwards returning our final flow definition
	newFlow, err := legacyFlow.Migrate(false, false)
	if err != nil {
		return nil, errors.Annotatef(err, "error migrating flow: %s", uuid)
	}

	// write this flow back out in our new format
	flow.definition, err = json.Marshal(newFlow)
	if err != nil {
		return nil, errors.Annotatef(err, "error mashalling migrated flow definition: %s", uuid)
	}

	return flow, nil
}

const selectFlowSQL = `
SELECT 
	id, name,
	fr.definition::jsonb || 
	jsonb_build_object(
		'flow_type', f.flow_type, 
		'metadata', jsonb_build_object(
			'uuid', f.uuid, 
			'id', f.id,
			'name', f.name, 
			'revision', fr.revision, 
			'expires', f.expires_after_minutes
		)
	) as definition
FROM 
	flows_flowrevision fr, 
	flows_flow f 
WHERE 
	f.uuid = $1 AND 
	fr.flow_id = f.id AND 
	fr.is_active = TRUE AND
	f.is_active = TRUE 
ORDER BY 
	revision DESC 
LIMIT 1;`