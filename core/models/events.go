package models

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/pkg/errors"
)

// Scene represents the context that events are occurring in
type Scene struct {
	contact *flows.Contact
	session *Session

	preCommits  map[EventCommitHook][]interface{}
	postCommits map[EventCommitHook][]interface{}
}

// NewSceneForSession creates a new scene for the passed in session
func NewSceneForSession(session *Session) *Scene {
	s := &Scene{
		contact: session.Contact(),
		session: session,

		preCommits:  make(map[EventCommitHook][]interface{}),
		postCommits: make(map[EventCommitHook][]interface{}),
	}
	return s
}

// NewSceneForContact creates a new scene for the passed in contact, session will be nil
func NewSceneForContact(contact *flows.Contact) *Scene {
	s := &Scene{
		contact: contact,

		preCommits:  make(map[EventCommitHook][]interface{}),
		postCommits: make(map[EventCommitHook][]interface{}),
	}
	return s
}

// SessionID returns the session id for this scene if any
func (s *Scene) SessionID() SessionID {
	if s.session == nil {
		return SessionID(0)
	}
	return s.session.ID()
}

func (s *Scene) Contact() *flows.Contact        { return s.contact }
func (s *Scene) ContactID() ContactID           { return ContactID(s.contact.ID()) }
func (s *Scene) ContactUUID() flows.ContactUUID { return s.contact.UUID() }

// Session returns the session for this scene if any
func (s *Scene) Session() *Session {
	return s.session
}

// AppendToEventPreCommitHook adds a new event to be handled by a pre commit hook
func (s *Scene) AppendToEventPreCommitHook(hook EventCommitHook, event interface{}) {
	s.preCommits[hook] = append(s.preCommits[hook], event)
}

// AppendToEventPostCommitHook adds a new event to be handled by a post commit hook
func (s *Scene) AppendToEventPostCommitHook(hook EventCommitHook, event interface{}) {
	s.postCommits[hook] = append(s.postCommits[hook], event)
}

// EventHandler defines a call for handling events that occur in a flow
type EventHandler func(context.Context, *runtime.Runtime, *sqlx.Tx, *OrgAssets, *Scene, flows.Event) error

// our registry of event type to internal handlers
var eventHandlers = make(map[string]EventHandler)

// our registry of event type to pre insert handlers
var preHandlers = make(map[string]EventHandler)

// RegisterEventHandler registers the passed in handler as being interested in the passed in type
func RegisterEventHandler(eventType string, handler EventHandler) {
	// it's a bug if we try to register more than one handler for a type
	_, found := eventHandlers[eventType]
	if found {
		panic(errors.Errorf("duplicate handler being registered for type: %s", eventType))
	}
	eventHandlers[eventType] = handler
}

// RegisterEventPreWriteHandler registers the passed in handler as being interested in the passed in type before session and run insertion
func RegisterEventPreWriteHandler(eventType string, handler EventHandler) {
	// it's a bug if we try to register more than one handler for a type
	_, found := preHandlers[eventType]
	if found {
		panic(errors.Errorf("duplicate handler being registered for type: %s", eventType))
	}
	preHandlers[eventType] = handler
}

// HandleEvents handles the passed in event, IE, creates the db objects required etc..
func HandleEvents(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *OrgAssets, scene *Scene, events []flows.Event) error {
	for _, e := range events {

		handler, found := eventHandlers[e.Type()]
		if !found {
			return errors.Errorf("unable to find handler for event type: %s", e.Type())
		}

		err := handler(ctx, rt, tx, oa, scene, e)
		if err != nil {
			return err
		}
	}
	return nil
}

// ApplyPreWriteEvent applies the passed in event before insertion or update, unlike normal event handlers it is not a requirement
// that all types have a handler.
func ApplyPreWriteEvent(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *OrgAssets, scene *Scene, e flows.Event) error {
	handler, found := preHandlers[e.Type()]
	if !found {
		return nil
	}

	return handler(ctx, rt, tx, oa, scene, e)
}

// EventCommitHook defines a callback that will accept a certain type of events across session, either before or after committing
type EventCommitHook interface {
	Apply(context.Context, *runtime.Runtime, *sqlx.Tx, *OrgAssets, map[*Scene][]interface{}) error
}

// ApplyEventPreCommitHooks runs through all the pre event hooks for the passed in sessions and applies their events
func ApplyEventPreCommitHooks(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *OrgAssets, scenes []*Scene) error {
	// gather all our hook events together across our sessions
	preHooks := make(map[EventCommitHook]map[*Scene][]interface{})
	for _, s := range scenes {
		for hook, args := range s.preCommits {
			sessionMap, found := preHooks[hook]
			if !found {
				sessionMap = make(map[*Scene][]interface{}, len(scenes))
				preHooks[hook] = sessionMap
			}
			sessionMap[s] = args
		}
	}

	// now fire each of our hooks
	for hook, args := range preHooks {
		err := hook.Apply(ctx, rt, tx, oa, args)
		if err != nil {
			return errors.Wrapf(err, "error applying pre commit hook: %T", hook)
		}
	}

	return nil
}

// ApplyEventPostCommitHooks runs through all the post event hooks for the passed in sessions and applies their events
func ApplyEventPostCommitHooks(ctx context.Context, rt *runtime.Runtime, tx *sqlx.Tx, oa *OrgAssets, scenes []*Scene) error {
	// gather all our hook events together across our sessions
	postHooks := make(map[EventCommitHook]map[*Scene][]interface{})
	for _, s := range scenes {
		for hook, args := range s.postCommits {
			sprintMap, found := postHooks[hook]
			if !found {
				sprintMap = make(map[*Scene][]interface{}, len(scenes))
				postHooks[hook] = sprintMap
			}
			sprintMap[s] = args
		}
	}

	// now fire each of our hooks
	for hook, args := range postHooks {
		err := hook.Apply(ctx, rt, tx, oa, args)
		if err != nil {
			return errors.Wrapf(err, "error applying post commit hook: %v", hook)
		}
	}

	return nil
}

// HandleAndCommitEvents takes a set of contacts and events, handles the events and applies any hooks, and commits everything
func HandleAndCommitEvents(ctx context.Context, rt *runtime.Runtime, oa *OrgAssets, contactEvents map[*flows.Contact][]flows.Event) error {
	// create scenes for each contact
	scenes := make([]*Scene, 0, len(contactEvents))
	for contact := range contactEvents {
		scene := NewSceneForContact(contact)
		scenes = append(scenes, scene)
	}

	// begin the transaction for pre-commit hooks
	tx, err := rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "error beginning transaction")
	}

	// handle the events to create the hooks on each scene
	for _, scene := range scenes {
		err := HandleEvents(ctx, rt, tx, oa, scene, contactEvents[scene.Contact()])
		if err != nil {
			return errors.Wrapf(err, "error applying events")
		}
	}

	// gather all our pre commit events, group them by hook and apply them
	err = ApplyEventPreCommitHooks(ctx, rt, tx, oa, scenes)
	if err != nil {
		return errors.Wrapf(err, "error applying pre commit hooks")
	}

	// commit the transaction
	if err := tx.Commit(); err != nil {
		return errors.Wrapf(err, "error committing pre commit hooks")
	}

	// begin the transaction for post-commit hooks
	tx, err = rt.DB.BeginTxx(ctx, nil)
	if err != nil {
		return errors.Wrapf(err, "error beginning transaction for post commit")
	}

	// apply the post commit hooks
	err = ApplyEventPostCommitHooks(ctx, rt, tx, oa, scenes)
	if err != nil {
		return errors.Wrapf(err, "error applying post commit hooks")
	}

	// commit the transaction
	if err := tx.Commit(); err != nil {
		return errors.Wrapf(err, "error committing post commit hooks")
	}
	return nil
}

// ApplyModifiers modifies contacts by applying modifiers and handling the resultant events
func ApplyModifiers(ctx context.Context, rt *runtime.Runtime, oa *OrgAssets, modifiersByContact map[*flows.Contact][]flows.Modifier) (map[*flows.Contact][]flows.Event, error) {
	// create an environment instance with location support
	env := flows.NewEnvironment(oa.Env(), oa.SessionAssets().Locations())

	eventsByContact := make(map[*flows.Contact][]flows.Event, len(modifiersByContact))

	// apply the modifiers to get the events for each contact
	for contact, mods := range modifiersByContact {
		events := make([]flows.Event, 0)
		for _, mod := range mods {
			mod.Apply(env, oa.SessionAssets(), contact, func(e flows.Event) { events = append(events, e) })
		}
		eventsByContact[contact] = events
	}

	err := HandleAndCommitEvents(ctx, rt, oa, eventsByContact)
	if err != nil {
		return nil, errors.Wrap(err, "error commiting events")
	}

	return eventsByContact, nil
}

// TypeSprintEnded is a pseudo event that lets add hooks for changes to a contacts current flow or flow history
const TypeSprintEnded string = "sprint_ended"

type SprintEndedEvent struct {
	events.BaseEvent

	Contact *Contact // model contact so we can access current flow
	Resumed bool     // whether this was a resume
}

// NewSprintEndedEvent creates a new sprint ended event
func NewSprintEndedEvent(c *Contact, resumed bool) *SprintEndedEvent {
	return &SprintEndedEvent{
		BaseEvent: events.NewBaseEvent(TypeSprintEnded),
		Contact:   c,
		Resumed:   resumed,
	}
}
