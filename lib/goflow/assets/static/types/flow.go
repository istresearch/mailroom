package types

import (
	"encoding/json"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/utils/jsonx"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
)

// Flow is a JSON serializable implementation of a flow asset
type Flow struct {
	UUID_       assets.FlowUUID `json:"uuid" validate:"required,uuid4"`
	Name_       string          `json:"name"`
	Definition_ json.RawMessage
}

// UUID returns the UUID of the flow
func (f *Flow) UUID() assets.FlowUUID { return f.UUID_ }

// Name returns the name of the flow
func (f *Flow) Name() string { return f.Name_ }

func (f *Flow) Definition() json.RawMessage { return f.Definition_ }

func (f *Flow) UnmarshalJSON(data []byte) error {
	f.Definition_ = data

	// alias our type so we don't end up here again
	type alias Flow

	// try as new spec first
	err := jsonx.Unmarshal(data, (*alias)(f))
	if err == nil && f.UUID() != "" {
		return nil
	}

	// and then as legacy spec
	legacyMetadata, _, _, _ := jsonparser.Get(data, "metadata")
	if legacyMetadata != nil {
		err = jsonx.Unmarshal(legacyMetadata, (*alias)(f))
		if err == nil && f.UUID() != "" {
			return nil
		}
	}

	return errors.New("can't parse UUID from flow asset")
}
