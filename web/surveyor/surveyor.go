package surveyor

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/engine"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/goflow"
	"github.com/nyaruka/mailroom/models"
	"github.com/nyaruka/mailroom/web"

	"github.com/pkg/errors"
)

func init() {
	web.RegisterJSONRoute(http.MethodPost, "/mr/surveyor/submit", web.RequireUserToken(handleSubmit))
}

// Represents a surveyor submission
//
//   {
//     "session": {...},
//     "events": [{...}],
//     "modifiers": [{...}]
//   }
//
type submitRequest struct {
	Session   json.RawMessage   `json:"session"    validate:"required"`
	Events    []json.RawMessage `json:"events"`
	Modifiers []json.RawMessage `json:"modifiers"`
}

type submitResponse struct {
	Session struct {
		ID     models.SessionID     `json:"id"`
		Status models.SessionStatus `json:"status"`
	} `json:"session"`
	Contact struct {
		ID   flows.ContactID   `json:"id"`
		UUID flows.ContactUUID `json:"uuid"`
	} `json:"contact"`
}

// handles a surveyor request
func handleSubmit(ctx context.Context, s *web.Server, r *http.Request) (interface{}, int, error) {
	request := &submitRequest{}
	if err := utils.UnmarshalAndValidateWithLimit(r.Body, request, web.MaxRequestBytes); err != nil {
		return nil, http.StatusBadRequest, errors.Wrapf(err, "request failed validation")
	}

	// grab our org assets
	orgID := ctx.Value(web.OrgIDKey).(models.OrgID)
	oa, err := models.GetOrgAssets(s.CTX, s.DB, orgID)
	if err != nil {
		return nil, http.StatusBadRequest, errors.Wrapf(err, "unable to load org assets")
	}

	// and our user id
	_, valid := ctx.Value(web.UserIDKey).(int64)
	if !valid {
		return nil, http.StatusInternalServerError, errors.Errorf("missing request user")
	}

	fs, err := goflow.Engine().ReadSession(oa.SessionAssets(), request.Session, assets.IgnoreMissing)
	if err != nil {
		return nil, http.StatusBadRequest, errors.Wrapf(err, "error reading session")
	}

	// and our events
	sessionEvents := make([]flows.Event, 0, len(request.Events))
	for _, e := range request.Events {
		event, err := events.ReadEvent(e)
		if err != nil {
			return nil, http.StatusBadRequest, errors.Wrapf(err, "error unmarshalling event: %s", string(e))
		}
		sessionEvents = append(sessionEvents, event)
	}

	// and our modifiers
	mods, err := goflow.ReadModifiers(oa.SessionAssets(), request.Modifiers, goflow.IgnoreMissing)
	if err != nil {
		return nil, http.StatusBadRequest, err
	}

	// create / assign our contact
	urn := urns.NilURN
	if len(fs.Contact().URNs()) > 0 {
		urn = fs.Contact().URNs()[0].URN()
	}

	// create / fetch our contact based on the highest priority URN
	contactID, err := models.CreateContact(ctx, s.DB, oa, urn)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "unable to look up contact")
	}

	// load that contact to get the current groups and UUID
	contacts, err := models.LoadContacts(ctx, s.DB, oa, []models.ContactID{contactID})
	if err == nil && len(contacts) == 0 {
		err = errors.Errorf("no contacts loaded")
	}
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error loading contact")
	}

	// load our flow contact
	flowContact, err := contacts[0].FlowContact(oa)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error loading flow contact")
	}

	modifierEvents := make([]flows.Event, 0, len(mods))
	appender := func(e flows.Event) {
		modifierEvents = append(modifierEvents, e)
	}

	// run through each contact modifier, applying it to our contact
	for _, m := range mods {
		m.Apply(oa.Env(), oa.SessionAssets(), flowContact, appender)
	}

	// set this updated contact on our session
	fs.SetContact(flowContact)

	// append our session events to our modifiers events, the union will be used to update the db/contact
	for _, e := range sessionEvents {
		modifierEvents = append(modifierEvents, e)
	}

	// create our sprint
	sprint := engine.NewSprint(mods, modifierEvents)

	// write our session out
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error starting transaction for session write")
	}
	sessions, err := models.WriteSessions(ctx, tx, s.RP, oa, []flows.Session{fs}, []flows.Sprint{sprint}, nil)
	if err == nil && len(sessions) == 0 {
		err = errors.Errorf("no sessions written")
	}
	if err != nil {
		tx.Rollback()
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error writing session")
	}
	err = tx.Commit()
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error committing sessions")
	}

	tx, err = s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error starting transaction for post commit hooks")
	}

	// write our post commit hooks
	err = models.ApplyEventPostCommitHooks(ctx, tx, s.RP, oa, []*models.Scene{sessions[0].Scene()})
	if err != nil {
		tx.Rollback()
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error applying post commit hooks")
	}
	err = tx.Commit()
	if err != nil {
		return nil, http.StatusInternalServerError, errors.Wrapf(err, "error committing post commit hooks")
	}

	response := &submitResponse{}
	response.Session.ID = sessions[0].ID()
	response.Session.Status = sessions[0].Status()
	response.Contact.ID = flowContact.ID()
	response.Contact.UUID = flowContact.UUID()

	return response, http.StatusCreated, nil
}
