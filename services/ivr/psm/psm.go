package psm

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/mailroom/core/ivr"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/runtime"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
)

const (
	psmChannelType = models.ChannelType("PSM")
)

type service struct {
	channel *models.Channel
}

func init() {
	ivr.RegisterServiceType(psmChannelType, NewClientFromChannel)
}

// NewClientFromChannel creates a new Twilio IVR service for the passed in account and auth token
func NewClientFromChannel(httpClient *http.Client, channel *models.Channel) (ivr.Service, error) {
	return &service{
		channel: channel,
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

func (s *service) DownloadMedia(url string) (*http.Response, error) {
	return http.Get(url)
}

func (s *service) CheckStartRequest(r *http.Request) models.ConnectionError {
	r.ParseForm()
	answeredBy := r.Form.Get("AnsweredBy")
	if answeredBy == "machine_start" || answeredBy == "fax" {
		return models.ConnectionErrorMachine
	}
	return ""
}

func (s *service) CallIDForRequest(r *http.Request) (string, error) {
	return "", nil
}

func (s *service) URNForRequest(r *http.Request) (urns.URN, error) {
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

func (s *service) PreprocessStatus(ctx context.Context, rt *runtime.Runtime, r *http.Request) ([]byte, error) {
	return nil, nil
}

func (s *service) PreprocessResume(ctx context.Context, rt *runtime.Runtime, conn *models.ChannelConnection, r *http.Request) ([]byte, error) {
	return nil, nil
}

// RequestCall causes this service to request a new outgoing call for this provider
func (s *service) RequestCall(number urns.URN, handleURL string, statusURL string, machineDetection bool) (ivr.CallID, *httpx.Trace, error) {
	return "", nil, nil
}

// HangupCall asks PSM to hang up the call that is passed in
func (s *service) HangupCall(callID string) (*httpx.Trace, error) {
	return nil, nil
}

// InputForRequest returns the input for the passed in request, if any
func (s *service) InputForRequest(r *http.Request) (string, utils.Attachment, error) {
	return "", utils.Attachment(""), nil
}

// StatusForRequest returns the call status for the passed in request, and if it's an error the reason,
// and if available, the current call duration
func (s *service) StatusForRequest(r *http.Request) (models.ConnectionStatus, models.ConnectionError, int) {
	status := r.Form.Get("CallStatus")
	switch status {

	case "queued", "ringing":
		return models.ConnectionStatusWired, "", 0
	case "in-progress", "initiated":
		return models.ConnectionStatusInProgress, "", 0
	case "completed":
		duration, _ := strconv.Atoi(r.Form.Get("CallDuration"))
		return models.ConnectionStatusCompleted, "", duration

	case "busy":
		return models.ConnectionStatusErrored, models.ConnectionErrorBusy, 0
	case "no-answer":
		return models.ConnectionStatusErrored, models.ConnectionErrorNoAnswer, 0
	case "canceled", "failed":
		return models.ConnectionStatusErrored, models.ConnectionErrorProvider, 0

	default:
		logrus.WithField("call_status", status).Error("unknown call status in status callback")
		return models.ConnectionStatusFailed, models.ConnectionErrorProvider, 0
	}
}

// ValidateRequestSignature validates the signature on the passed in request, returning an error if it is invaled
func (s *service) ValidateRequestSignature(r *http.Request) error {
	return nil
}

// WriteSessionResponse writes a TWIML response for the events in the passed in session
func (s *service) WriteSessionResponse(ctx context.Context, rt *runtime.Runtime, channel *models.Channel, conn *models.ChannelConnection, session *models.Session, number urns.URN, resumeURL string, req *http.Request, w http.ResponseWriter) error {
	return nil
}

// WriteErrorResponse writes an error / unavailable response
func (s *service) WriteErrorResponse(w http.ResponseWriter, err error) error {
	return nil
}

// WriteEmptyResponse writes an empty (but valid) response
func (s *service) WriteEmptyResponse(w http.ResponseWriter, msg string) error {
	msgBody := map[string]string{
		"response": msg,
	}
	body, err := json.Marshal(msgBody)
	if err != nil {
		return errors.Wrapf(err, "error marshalling message")
	}

	_, err = w.Write(body)
	return err
}

// EventForCallDataRequest gets the channel event type and duration of a non-ivr call event
func (s *service) EventForCallDataRequest(r *http.Request) (models.ChannelEventType, int) {
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

func (s *service) ResumeForRequest(r *http.Request) (ivr.Resume, error) {
	return nil, nil
}
