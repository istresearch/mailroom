package models

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/null"
)

type ChannelEventType string
type ChannelEventID int64

const (
	NewConversationEventType = ChannelEventType("new_conversation")
	WelcomeMessateEventType  = ChannelEventType("welcome_message")
	ReferralEventType        = ChannelEventType("referral")
	MOMissEventType          = ChannelEventType("mo_miss")
	MOCallEventType          = ChannelEventType("mo_call")
	MTCallEventType          = ChannelEventType("mt_call")
)

// ChannelEvent represents an event that occurred associated with a channel, such as a referral, missed call, etc..
type ChannelEvent struct {
	e struct {
		ID        ChannelEventID   `json:"id"           db:"id"`
		EventType ChannelEventType `json:"event_type"   db:"event_type"`
		OrgID     OrgID            `json:"org_id"       db:"org_id"`
		ChannelID ChannelID        `json:"channel_id"   db:"channel_id"`
		ContactID ContactID        `json:"contact_id"   db:"contact_id"`
		URNID     URNID            `json:"urn_id"       db:"contact_urn_id"`
		Extra     null.Map         `json:"extra"        db:"extra"`

		// only in JSON representation
		NewContact bool `json:"new_contact"`

		// only in DB representation
		CreatedOn  time.Time `db:"created_on"`
		OccurredOn time.Time `db:"occurred_on"`
	}
}

func (e *ChannelEvent) ID() ChannelEventID   { return e.e.ID }
func (e *ChannelEvent) ContactID() ContactID { return e.e.ContactID }
func (e *ChannelEvent) URNID() URNID         { return e.e.URNID }
func (e *ChannelEvent) OrgID() OrgID         { return e.e.OrgID }
func (e *ChannelEvent) ChannelID() ChannelID { return e.e.ChannelID }
func (e *ChannelEvent) IsNewContact() bool   { return e.e.NewContact }

func (e *ChannelEvent) Extra() map[string]interface{} {
	return e.e.Extra.Map()
}

func (e *ChannelEvent) ExtraValue(key string) string {
	return e.e.Extra.GetString(key, "")
}

// MarshalJSON is our custom marshaller so that our inner struct get output
func (e *ChannelEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.e)
}

// UnmarshalJSON is our custom marshaller so that our inner struct get output
func (e *ChannelEvent) UnmarshalJSON(b []byte) error {
	return json.Unmarshal(b, &e.e)
}

const insertChannelEventSQL = `
INSERT INTO
	channels_channelevent(event_type, extra, occurred_on, created_on, channel_id, contact_id, contact_urn_id, org_id)
	VALUES(:event_type, :extra, :occurred_on, :created_on, :channel_id, :contact_id, :contact_urn_id, :org_id)
RETURNING
	id
`

// Insert inserts this channel event to our DB. The ID of the channel event will be
// set if no error is returned
func (e *ChannelEvent) Insert(ctx context.Context, db *sqlx.DB) error {
	return BulkSQL(ctx, "insert channel event", db, insertChannelEventSQL, []interface{}{&e.e})
}

// NewChannelEvent creates a new channel event for the passed in parameters, returning it
func NewChannelEvent(eventType ChannelEventType, orgID OrgID, channelID ChannelID, contactID ContactID, urnID URNID, extra map[string]interface{}, isNewContact bool) *ChannelEvent {
	event := &ChannelEvent{}
	e := &event.e

	e.EventType = eventType
	e.OrgID = orgID
	e.ChannelID = channelID
	e.ContactID = contactID
	e.URNID = urnID
	e.NewContact = isNewContact

	if extra != nil {
		e.Extra = null.NewMap(extra)
	}

	now := time.Now()
	e.CreatedOn = now
	e.OccurredOn = now

	return event
}
