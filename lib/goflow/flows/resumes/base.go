package resumes

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/flows/triggers"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/goflow/utils/dates"
	"github.com/nyaruka/goflow/utils/jsonx"

	"github.com/pkg/errors"
)

// ReadFunc is a function that can read a resume from JSON
type ReadFunc func(flows.SessionAssets, json.RawMessage, assets.MissingCallback) (flows.Resume, error)

var registeredTypes = map[string]ReadFunc{}

// registers a new type of resume
func registerType(name string, f ReadFunc) {
	registeredTypes[name] = f
}

// RegisteredTypes gets the registered types of resumes
func RegisteredTypes() map[string]ReadFunc {
	return registeredTypes
}

// base of all resume types
type baseResume struct {
	type_       string
	environment envs.Environment
	contact     *flows.Contact
	resumedOn   time.Time
}

// creates a new base resume
func newBaseResume(typeName string, env envs.Environment, contact *flows.Contact) baseResume {
	return baseResume{type_: typeName, environment: env, contact: contact, resumedOn: dates.Now()}
}

// Type returns the type of this resume
func (r *baseResume) Type() string { return r.type_ }

func (r *baseResume) Environment() envs.Environment { return r.environment }
func (r *baseResume) Contact() *flows.Contact       { return r.contact }
func (r *baseResume) ResumedOn() time.Time          { return r.resumedOn }

// Apply applies our state changes and saves any events to the run
func (r *baseResume) Apply(run flows.FlowRun, logEvent flows.EventCallback) error {
	if r.environment != nil {
		if !run.Session().Environment().Equal(r.environment) {
			logEvent(events.NewEnvironmentRefreshed(r.environment))
		}

		run.Session().SetEnvironment(r.environment)
	}
	if r.contact != nil {
		if !run.Session().Contact().Equal(r.contact) {
			logEvent(events.NewContactRefreshed(r.contact))
		}

		run.Session().SetContact(r.contact)

		triggers.EnsureDynamicGroups(run.Session(), logEvent)
	}

	if run.Status() == flows.RunStatusWaiting {
		run.SetStatus(flows.RunStatusActive)
	}

	return nil
}

//------------------------------------------------------------------------------------------
// JSON Encoding / Decoding
//------------------------------------------------------------------------------------------

type baseResumeEnvelope struct {
	Type        string          `json:"type" validate:"required"`
	Environment json.RawMessage `json:"environment,omitempty"`
	Contact     json.RawMessage `json:"contact,omitempty"`
	ResumedOn   time.Time       `json:"resumed_on" validate:"required"`
}

// ReadResume reads a resume from the given JSON
func ReadResume(sessionAssets flows.SessionAssets, data json.RawMessage, missing assets.MissingCallback) (flows.Resume, error) {
	typeName, err := utils.ReadTypeFromJSON(data)
	if err != nil {
		return nil, err
	}

	f := registeredTypes[typeName]
	if f == nil {
		return nil, errors.Errorf("unknown type: '%s'", typeName)
	}
	return f(sessionAssets, data, missing)
}

func (r *baseResume) unmarshal(sessionAssets flows.SessionAssets, e *baseResumeEnvelope, missing assets.MissingCallback) error {
	var err error

	r.type_ = e.Type
	r.resumedOn = e.ResumedOn

	if e.Environment != nil {
		if r.environment, err = envs.ReadEnvironment(e.Environment); err != nil {
			return errors.Wrap(err, "unable to read environment")
		}
	}
	if e.Contact != nil {
		if r.contact, err = flows.ReadContact(sessionAssets, e.Contact, missing); err != nil {
			return errors.Wrap(err, "unable to read contact")
		}
	}
	return nil
}

func (r *baseResume) marshal(e *baseResumeEnvelope) error {
	var err error
	e.Type = r.type_
	e.ResumedOn = r.resumedOn

	if r.environment != nil {
		e.Environment, err = jsonx.Marshal(r.environment)
		if err != nil {
			return err
		}
	}
	if r.contact != nil {
		e.Contact, err = jsonx.Marshal(r.contact)
		if err != nil {
			return err
		}
	}
	return nil
}
