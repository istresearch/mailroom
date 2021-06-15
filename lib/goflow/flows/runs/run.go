package runs

import (
	"encoding/json"
	"time"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/excellent"
	"github.com/nyaruka/goflow/excellent/types"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/events"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/goflow/utils/dates"
	"github.com/nyaruka/goflow/utils/jsonx"
	"github.com/nyaruka/goflow/utils/uuids"

	"github.com/pkg/errors"
)

type flowRun struct {
	uuid        flows.RunUUID
	session     flows.Session
	environment *runEnvironment

	flow    flows.Flow
	flowRef *assets.FlowReference

	parent  flows.FlowRun
	results flows.Results
	path    Path
	events  []flows.Event
	status  flows.RunStatus

	createdOn  time.Time
	modifiedOn time.Time
	expiresOn  *time.Time
	exitedOn   *time.Time

	webhook     types.XValue
	legacyExtra *legacyExtra
}

// NewRun initializes a new context and flow run for the passed in flow and contact
func NewRun(session flows.Session, flow flows.Flow, parent flows.FlowRun) flows.FlowRun {
	now := dates.Now()
	r := &flowRun{
		uuid:       flows.RunUUID(uuids.New()),
		session:    session,
		flow:       flow,
		flowRef:    flow.Reference(),
		parent:     parent,
		results:    flows.NewResults(),
		status:     flows.RunStatusActive,
		events:     make([]flows.Event, 0),
		createdOn:  now,
		modifiedOn: now,
	}

	r.environment = newRunEnvironment(session.Environment(), r)
	r.ResetExpiration(nil)

	r.webhook = types.XObjectEmpty
	r.legacyExtra = newLegacyExtra(r)

	return r
}

func (r *flowRun) UUID() flows.RunUUID           { return r.uuid }
func (r *flowRun) Session() flows.Session        { return r.session }
func (r *flowRun) Environment() envs.Environment { return r.environment }

func (r *flowRun) Flow() flows.Flow                     { return r.flow }
func (r *flowRun) FlowReference() *assets.FlowReference { return r.flowRef }
func (r *flowRun) Contact() *flows.Contact              { return r.session.Contact() }
func (r *flowRun) Events() []flows.Event                { return r.events }

func (r *flowRun) Results() flows.Results { return r.results }
func (r *flowRun) SaveResult(result *flows.Result) {
	// truncate value if necessary
	result.Value = utils.Truncate(result.Value, r.Environment().MaxValueLength())

	r.results.Save(result)
	r.modifiedOn = dates.Now()

	r.legacyExtra.addResult(result)
}

func (r *flowRun) Exit(status flows.RunStatus) {
	now := dates.Now()

	r.status = status
	r.exitedOn = &now
	r.modifiedOn = now
}
func (r *flowRun) Status() flows.RunStatus { return r.status }
func (r *flowRun) SetStatus(status flows.RunStatus) {
	r.status = status
	r.modifiedOn = dates.Now()
}

func (r *flowRun) Webhook() types.XValue {
	return r.webhook
}
func (r *flowRun) SetWebhook(value types.XValue) {
	r.webhook = value
}

// ParentInSession returns the parent of the run within the same session if one exists
func (r *flowRun) ParentInSession() flows.FlowRun { return r.parent }

// Parent returns either the same session parent or if this session was triggered from a trigger_flow action
// in another session, that run
func (r *flowRun) Parent() flows.RunSummary {
	if r.parent == nil {
		return r.session.ParentRun()
	}
	return r.ParentInSession()
}

func (r *flowRun) Ancestors() []flows.FlowRun {
	ancestors := make([]flows.FlowRun, 0)
	if r.parent != nil {
		run := r.parent.(*flowRun)
		ancestors = append(ancestors, run)

		for {
			if run.parent != nil {
				run = run.parent.(*flowRun)
				ancestors = append(ancestors, run)
			} else {
				break
			}
		}
	}

	return ancestors
}

func (r *flowRun) LogEvent(s flows.Step, event flows.Event) {
	if s != nil {
		event.SetStepUUID(s.UUID())
	}

	r.events = append(r.events, event)
	r.modifiedOn = dates.Now()
}

func (r *flowRun) LogError(step flows.Step, err error) {
	r.LogEvent(step, events.NewError(err))
}

// find the first event matching the given step UUID and type
func (r *flowRun) findEvent(stepUUID flows.StepUUID, eType string) flows.Event {
	for _, e := range r.events {
		if e.StepUUID() == stepUUID && e.Type() == eType {
			return e
		}
	}
	return nil
}

func (r *flowRun) Path() []flows.Step { return r.path }
func (r *flowRun) CreateStep(node flows.Node) flows.Step {
	now := dates.Now()
	step := NewStep(node, now)
	r.path = append(r.path, step)
	r.modifiedOn = now
	return step
}

func (r *flowRun) PathLocation() (flows.Step, flows.Node, error) {
	if r.Path() == nil {
		return nil, nil, errors.Errorf("run has no location as path is empty")
	}

	step := r.Path()[len(r.Path())-1]

	// check that we still have a node for this step
	node := r.Flow().GetNode(step.NodeUUID())
	if node == nil {
		return nil, nil, errors.Errorf("run is located at a flow node that no longer exists")
	}

	return step, node, nil
}

func (r *flowRun) CreatedOn() time.Time  { return r.createdOn }
func (r *flowRun) ModifiedOn() time.Time { return r.modifiedOn }
func (r *flowRun) ExpiresOn() *time.Time { return r.expiresOn }
func (r *flowRun) ResetExpiration(from *time.Time) {
	if r.Flow() != nil && r.Flow().ExpireAfterMinutes() >= 0 {
		if from == nil {
			now := dates.Now()
			from = &now
		}

		expiresAfterMinutes := time.Duration(r.Flow().ExpireAfterMinutes())
		expiresOn := from.Add(expiresAfterMinutes * time.Minute)

		r.expiresOn = &expiresOn
		r.modifiedOn = dates.Now()
	}

	if r.ParentInSession() != nil {
		r.ParentInSession().ResetExpiration(r.expiresOn)
	}
}

func (r *flowRun) ExitedOn() *time.Time { return r.exitedOn }

// RootContext returns the root context for expression evaluation
//
//   contact:contact -> the contact
//   fields:fields -> the custom field values of the contact
//   urns:urns -> the URN values of the contact
//   results:results -> the current run results
//   input:input -> the current input from the contact
//   run:run -> the current run
//   child:related_run -> the last child run
//   parent:related_run -> the parent of the run
//   webhook:any -> the parsed JSON response of the last webhook call
//   globals:globals -> the global values
//   trigger:trigger -> the trigger that started this session
//
// @context root
func (r *flowRun) RootContext(env envs.Environment) map[string]types.XValue {
	var urns, fields types.XValue
	if r.Contact() != nil {
		urns = flows.ContextFunc(env, r.Contact().URNs().MapContext)
		fields = flows.Context(env, r.Contact().Fields())
	}

	var child = newRelatedRunContext(r.Session().GetCurrentChild(r))
	var parent = newRelatedRunContext(r.Parent())

	return map[string]types.XValue{
		// the available runs
		"run":    flows.Context(env, r),
		"child":  flows.Context(env, child),
		"parent": flows.Context(env, parent),

		// shortcuts to things on the current run
		"contact": flows.Context(env, r.Contact()),
		"results": flows.Context(env, r.Results()),
		"urns":    urns,
		"fields":  fields,

		// other
		"trigger":      flows.Context(env, r.Session().Trigger()),
		"input":        flows.Context(env, r.Session().Input()),
		"globals":      flows.Context(env, r.Session().Assets().Globals()),
		"webhook":      r.webhook,
		"legacy_extra": r.legacyExtra.ToXValue(env),
	}
}

// Context returns the properties available in expressions
//
//   __default__:text -> the contact name and flow UUID
//   uuid:text -> the UUID of the run
//   contact:contact -> the contact of the run
//   flow:flow -> the flow of the run
//   status:text -> the current status of the run
//   results:results -> the results saved by the run
//   created_on:datetime -> the creation date of the run
//   exited_on:datetime -> the exit date of the run
//
// @context run
func (r *flowRun) Context(env envs.Environment) map[string]types.XValue {
	var exitedOn types.XValue
	if r.exitedOn != nil {
		exitedOn = types.NewXDateTime(*r.exitedOn)
	}

	return map[string]types.XValue{
		"__default__": types.NewXText(FormatRunSummary(env, r)),
		"uuid":        types.NewXText(string(r.UUID())),
		"contact":     flows.Context(env, r.Contact()),
		"flow":        flows.Context(env, r.Flow()),
		"status":      types.NewXText(string(r.Status())),
		"results":     flows.Context(env, r.Results()),
		"path":        r.path.ToXValue(env),
		"created_on":  types.NewXDateTime(r.CreatedOn()),
		"exited_on":   exitedOn,
	}
}

// EvaluateTemplate evaluates the given template in the context of this run
func (r *flowRun) EvaluateTemplateValue(template string) (types.XValue, error) {
	context := types.NewXObject(r.RootContext(r.Environment()))

	return excellent.EvaluateTemplateValue(r.Environment(), context, template)
}

// EvaluateTemplateText evaluates the given template as text in the context of this run
func (r *flowRun) EvaluateTemplateText(template string, escaping excellent.Escaping, truncate bool) (string, error) {
	context := types.NewXObject(r.RootContext(r.Environment()))

	value, err := excellent.EvaluateTemplate(r.Environment(), context, template, escaping)
	if truncate {
		value = utils.TruncateEllipsis(value, r.Session().Engine().MaxTemplateChars())
	}
	return value, err
}

// EvaluateTemplate is a convenience function for evaluating as text with no escaping
func (r *flowRun) EvaluateTemplate(template string) (string, error) {
	return r.EvaluateTemplateText(template, nil, true)
}

// get the ordered list of languages to be used for localization in this run
func (r *flowRun) getLanguages() []envs.Language {
	// TODO cache this this?

	contact := r.Contact()
	languages := make([]envs.Language, 0, 3)

	// if contact has a allowed language, it takes priority
	if contact != nil && contact.Language() != envs.NilLanguage {
		for _, l := range r.Environment().AllowedLanguages() {
			if l == contact.Language() {
				languages = append(languages, contact.Language())
				break
			}
		}
	}

	// next we include the default language if it's different to the contact language
	defaultLanguage := r.Environment().DefaultLanguage()
	if defaultLanguage != envs.NilLanguage && defaultLanguage != contact.Language() {
		languages = append(languages, defaultLanguage)
	}

	// finally we include the flow native language if it isn't an allowed language - because it's the only
	// one guaranteed to have translations
	return append(languages, r.flow.Language())
}

func (r *flowRun) GetText(uuid uuids.UUID, key string, native string) string {
	textArray, _ := r.GetTextArray(uuid, key, []string{native})
	return textArray[0]
}

func (r *flowRun) GetTextArray(uuid uuids.UUID, key string, native []string) ([]string, envs.Language) {
	return r.getTranslatedText(uuid, key, native, r.getLanguages())
}

func (r *flowRun) GetTranslatedTextArray(uuid uuids.UUID, key string, native []string, languages []envs.Language) []string {
	texts, _ := r.getTranslatedText(uuid, key, native, languages)
	return texts
}

func (r *flowRun) getTranslatedText(uuid uuids.UUID, key string, native []string, languages []envs.Language) ([]string, envs.Language) {
	nativeLang := r.Flow().Language()

	if languages == nil {
		languages = r.getLanguages()
	}

	for _, lang := range languages {
		if lang == r.Flow().Language() {
			return native, nativeLang
		}

		textArray := r.Flow().Localization().GetItemTranslation(lang, uuid, key)
		if textArray != nil {
			merged := make([]string, len(native))
			for i := range native {
				if i < len(textArray) && textArray[i] != "" {
					merged[i] = textArray[i]
				} else {
					merged[i] = native[i]
				}
			}
			return merged, lang
		}
	}
	return native, nativeLang
}

func (r *flowRun) Snapshot() flows.RunSummary {
	return newRunSummaryFromRun(r)
}

var _ flows.RunSummary = (*flowRun)(nil)

//------------------------------------------------------------------------------------------
// JSON Encoding / Decoding
//------------------------------------------------------------------------------------------

type runEnvelope struct {
	UUID       flows.RunUUID         `json:"uuid" validate:"required,uuid4"`
	Flow       *assets.FlowReference `json:"flow" validate:"required,dive"`
	Path       []*step               `json:"path" validate:"dive"`
	Events     []json.RawMessage     `json:"events,omitempty"`
	Results    flows.Results         `json:"results,omitempty" validate:"omitempty,dive"`
	Status     flows.RunStatus       `json:"status" validate:"required"`
	ParentUUID flows.RunUUID         `json:"parent_uuid,omitempty" validate:"omitempty,uuid4"`

	CreatedOn  time.Time  `json:"created_on" validate:"required"`
	ModifiedOn time.Time  `json:"modified_on" validate:"required"`
	ExpiresOn  *time.Time `json:"expires_on"`
	ExitedOn   *time.Time `json:"exited_on"`
}

// ReadRun decodes a run from the passed in JSON. Parent run UUID is returned separately as the
// run in question might be loaded yet from the session.
func ReadRun(session flows.Session, data json.RawMessage, missing assets.MissingCallback) (flows.FlowRun, error) {
	e := &runEnvelope{}
	var err error

	if err = utils.UnmarshalAndValidate(data, e); err != nil {
		return nil, errors.Wrap(err, "unable to read run")
	}

	r := &flowRun{
		session:    session,
		uuid:       e.UUID,
		flowRef:    e.Flow,
		status:     e.Status,
		createdOn:  e.CreatedOn,
		modifiedOn: e.ModifiedOn,
		expiresOn:  e.ExpiresOn,
		exitedOn:   e.ExitedOn,
	}

	// lookup actual flow
	if r.flow, err = session.Assets().Flows().Get(e.Flow.UUID); err != nil {
		missing(e.Flow, err)
	}

	// lookup parent run
	if e.ParentUUID != "" {
		if r.parent, err = session.GetRun(e.ParentUUID); err != nil {
			return nil, err
		}
	}

	if e.Results != nil {
		r.results = e.Results
	} else {
		r.results = flows.NewResults()
	}

	// read in our path
	r.path = make([]flows.Step, len(e.Path))
	for i, step := range e.Path {
		r.path[i] = step
	}

	// read in our events
	r.events = make([]flows.Event, len(e.Events))
	for i := range r.events {
		if r.events[i], err = events.ReadEvent(e.Events[i]); err != nil {
			return nil, errors.Wrap(err, "unable to read event")
		}
	}

	// create a run specific environment and context
	r.environment = newRunEnvironment(session.Environment(), r)
	r.webhook = lastWebhookSavedAsExtra(r)
	r.legacyExtra = newLegacyExtra(r)

	return r, nil
}

// MarshalJSON marshals this flow run into JSON
func (r *flowRun) MarshalJSON() ([]byte, error) {
	var err error

	e := &runEnvelope{
		UUID:       r.uuid,
		Flow:       r.flowRef,
		Status:     r.status,
		CreatedOn:  r.createdOn,
		ModifiedOn: r.modifiedOn,
		ExpiresOn:  r.expiresOn,
		ExitedOn:   r.exitedOn,
		Results:    r.results,
	}

	if r.parent != nil {
		e.ParentUUID = r.parent.UUID()
	}

	e.Path = make([]*step, len(r.path))
	for i, s := range r.path {
		e.Path[i] = s.(*step)
	}

	e.Events = make([]json.RawMessage, len(r.events))
	for i := range r.events {
		if e.Events[i], err = jsonx.Marshal(r.events[i]); err != nil {
			return nil, errors.Wrapf(err, "unable to marshal event[type=%s]", r.events[i].Type())
		}
	}

	return jsonx.Marshal(e)
}
