package psm

import (
	"bytes"
	"context"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/ivr"
	"github.com/nyaruka/mailroom/models"
	"io/ioutil"
	"net/http"

	"github.com/buger/jsonparser"
	"github.com/gomodule/redigo/redis"
	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

const (
	psmChannelType = models.ChannelType("PSM")
)

type client struct {
	channel    *models.Channel
}

func init() {
	ivr.RegisterClientType(psmChannelType, NewClientFromChannel)
}

// NewClientFromChannel creates a new Twilio IVR client for the passed in account and and auth token
func NewClientFromChannel(channel *models.Channel) (ivr.Client, error) {
	return &client{
		channel:    channel,
	}, nil
}

func readBody(r *http.Request) ([]byte, error) {
	if r.Body == http.NoBody {
		return nil, nil
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, nil
	}
	r.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	return body, nil
}

func (c *client) CallIDForRequest(r *http.Request) (string, error) {
	return "", nil
}

func (c *client) URNForRequest(r *http.Request) (urns.URN, error) {
	// get our recording url out
	body, err := readBody(r)
	if err != nil {
		return "", errors.Wrapf(err, "error reading body from request")
	}

	urn, err := jsonparser.GetString(body, "urn")
	if err != nil {
		return "", errors.Errorf("invalid json body")
	}

	if urn == "" {
		return "", errors.Errorf("no urn found in body")
	}
	return urns.NewTelURNForCountry("+"+urn, "")
}

func (c *client) DownloadMedia(url string) (*http.Response, error) {
	return nil, nil
}

func (c *client) PreprocessResume(ctx context.Context, db *sqlx.DB, rp *redis.Pool, conn *models.ChannelConnection, r *http.Request) ([]byte, error) {
	return nil, nil
}

// RequestCall causes this client to request a new outgoing call for this provider
func (c *client) RequestCall(client *http.Client, number urns.URN, resumeURL string, statusURL string) (ivr.CallID, error) {
	return ivr.CallID(""), nil
}

// HangupCall asks PSM to hang up the call that is passed in
func (c *client) HangupCall(client *http.Client, callID string) error {
	return nil
}

// InputForRequest returns the input for the passed in request, if any
func (c *client) InputForRequest(r *http.Request) (string, utils.Attachment, error) {
	return "", utils.Attachment(""), nil
}

// StatusForRequest returns the current call status for the passed in status (and optional duration if known)
func (c *client) StatusForRequest(r *http.Request) (models.ConnectionStatus, int) {
	return "", 0
}

// ValidateRequestSignature validates the signature on the passed in request, returning an error if it is invaled
func (c *client) ValidateRequestSignature(r *http.Request) error {
	return nil
}

// WriteSessionResponse writes a TWIML response for the events in the passed in session
func (c *client) WriteSessionResponse(session *models.Session, number urns.URN, resumeURL string, r *http.Request, w http.ResponseWriter) error {
	return nil
}

// WriteErrorResponse writes an error / unavailable response
func (c *client) WriteErrorResponse(w http.ResponseWriter, err error) error {
	return nil
}

// WriteEmptyResponse writes an empty (but valid) response
func (c *client) WriteEmptyResponse(w http.ResponseWriter, msg string) error {
	return nil
}

// Get the channel event type and duration of a non-ivr call event
func (c *client) EventForCallDataRequest(r *http.Request) (models.ChannelEventType, int) {
	// get our recording url out
	body, err := readBody(r)
	if err != nil {
		return "", 0
	}

	status, err := jsonparser.GetString(body, "status")
	if err != nil {
		return "", 0
	}

	if status == "" {
		status = "missed"
	}

	duration, err := jsonparser.GetInt(body, "duration")
	if err != nil {
		duration = 0
	}

	switch status {
	case "miss":
		return models.MOMissEventType, 0
	case "in":
		return models.MOCallEventType, int(duration)
	case "out":
		return models.MTCallEventType, int(duration)
	}

	return "", 0
}