package zendesk_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static/types"
	"github.com/nyaruka/goflow/envs"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/test"
	"github.com/nyaruka/goflow/utils/dates"
	"github.com/nyaruka/goflow/utils/httpx"
	"github.com/nyaruka/goflow/utils/uuids"
	"github.com/nyaruka/mailroom/models"
	"github.com/nyaruka/mailroom/services/tickets/zendesk"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAndForward(t *testing.T) {
	session, _, err := test.CreateTestSession("", envs.RedactionPolicyNone)
	require.NoError(t, err)

	defer uuids.SetGenerator(uuids.DefaultGenerator)
	defer dates.SetNowSource(dates.DefaultNowSource)
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	uuids.SetGenerator(uuids.NewSeededGenerator(12345))
	dates.SetNowSource(dates.NewSequentialNowSource(time.Date(2019, 10, 7, 15, 21, 30, 0, time.UTC)))
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://nyaruka.zendesk.com/api/v2/any_channel/push.json": {
			httpx.MockConnectionError,
			httpx.NewMockResponse(201, nil, `{
				"results": [
					{
						"external_resource_id": "123",
						"status": {"code": "success"}
					}
				]
			}`),
			httpx.NewMockResponse(201, nil, `{
				"results": [
					{
						"external_resource_id": "124",
						"status": {"code": "success"}
					}
				]
			}`),
		},
	}))

	ticketer := flows.NewTicketer(types.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "zendesk"))

	_, err = zendesk.NewService(
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{},
	)
	assert.EqualError(t, err, "missing subdomain or oauth_token or push_id or push_token in zendesk config")

	svc, err := zendesk.NewService(
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"subdomain":   "nyaruka",
			"oauth_token": "987654321",
			"push_id":     "1234-abcd",
			"push_token":  "123456789",
		},
	)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}

	// try with connection failure
	_, err = svc.Open(session, "Need help", "Where are my cookies?", logger.Log)
	assert.EqualError(t, err, "error pushing message to zendesk: unable to connect to server")

	logger = &flows.HTTPLogger{}

	ticket, err := svc.Open(session, "Need help", "Where are my cookies?", logger.Log)

	assert.NoError(t, err)
	assert.Equal(t, &flows.Ticket{
		UUID:       flows.TicketUUID("59d74b86-3e2f-4a93-aece-b05d2fdcde0c"),
		Ticketer:   ticketer.Reference(),
		Subject:    "Need help",
		Body:       "Where are my cookies?",
		ExternalID: "",
	}, ticket)

	assert.Equal(t, 1, len(logger.Logs))
	assert.Equal(t, "https://nyaruka.zendesk.com/api/v2/any_channel/push.json", logger.Logs[0].URL)
	assert.Equal(t, "POST /api/v2/any_channel/push.json HTTP/1.1\r\nHost: nyaruka.zendesk.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 429\r\nAuthorization: Bearer ****************\r\nContent-Type: application/json\r\nAccept-Encoding: gzip\r\n\r\n{\"instance_push_id\":\"1234-abcd\",\"external_resources\":[{\"external_id\":\"59d74b86-3e2f-4a93-aece-b05d2fdcde0c\",\"message\":\"Where are my cookies?\",\"thread_id\":\"59d74b86-3e2f-4a93-aece-b05d2fdcde0c\",\"created_at\":\"2019-10-07T15:21:33Z\",\"author\":{\"external_id\":\"5d76d86b-3bb9-4d5a-b822-c9d86f5d8e4f\",\"name\":\"Ryan Lewis\"},\"display_info\":[{\"type\":\"temba\",\"data\":{\"uuid\":\"59d74b86-3e2f-4a93-aece-b05d2fdcde0c\"}}],\"allow_channelback\":true}]}", logger.Logs[0].Request)

	dbTicket := models.NewTicket(ticket.UUID, models.Org1, models.CathyID, models.ZendeskID, "", "Need help", "Where are my cookies?", map[string]interface{}{
		"contact-uuid":    string(models.CathyUUID),
		"contact-display": "Cathy",
	})

	logger = &flows.HTTPLogger{}
	err = svc.Forward(dbTicket, flows.MsgUUID("ca5607f0-cba8-4c94-9cd5-c4fbc24aa767"), "It's urgent", logger.Log)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(logger.Logs))
	assert.Equal(t, "POST /api/v2/any_channel/push.json HTTP/1.1\r\nHost: nyaruka.zendesk.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 421\r\nAuthorization: Bearer ****************\r\nContent-Type: application/json\r\nAccept-Encoding: gzip\r\n\r\n{\"instance_push_id\":\"1234-abcd\",\"external_resources\":[{\"external_id\":\"ca5607f0-cba8-4c94-9cd5-c4fbc24aa767\",\"message\":\"It's urgent\",\"thread_id\":\"59d74b86-3e2f-4a93-aece-b05d2fdcde0c\",\"created_at\":\"2019-10-07T15:21:36Z\",\"author\":{\"external_id\":\"6393abc0-283d-4c9b-a1b3-641a035c34bf\",\"name\":\"Cathy\"},\"display_info\":[{\"type\":\"temba-ticket\",\"data\":{\"uuid\":\"59d74b86-3e2f-4a93-aece-b05d2fdcde0c\"}}],\"allow_channelback\":true}]}", logger.Logs[0].Request)
}

func TestCloseAndReopen(t *testing.T) {
	defer httpx.SetRequestor(httpx.DefaultRequestor)
	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://nyaruka.zendesk.com/api/v2/tickets/update_many.json?ids=12,14": {
			httpx.NewMockResponse(201, nil, `{
				"job_status": {
					"id": "1234-abcd",
					"url": "http://zendesk.com",
					"status": "queued"
				}
			}`),
		},
		"https://nyaruka.zendesk.com/api/v2/tickets/update_many.json?ids=14": {
			httpx.NewMockResponse(201, nil, `{
				"job_status": {
					"id": "1234-abcd",
					"url": "http://zendesk.com",
					"status": "queued"
				}
			}`),
		},
	}))

	ticketer := flows.NewTicketer(types.NewTicketer(assets.TicketerUUID(uuids.New()), "Support", "zendesk"))
	svc, err := zendesk.NewService(
		http.DefaultClient,
		nil,
		ticketer,
		map[string]string{
			"subdomain":   "nyaruka",
			"oauth_token": "987654321",
			"push_id":     "1234-abcd",
			"push_token":  "123456789",
		},
	)
	require.NoError(t, err)

	logger := &flows.HTTPLogger{}
	ticket1 := models.NewTicket("88bfa1dc-be33-45c2-b469-294ecb0eba90", models.Org1, models.CathyID, models.ZendeskID, "12", "New ticket", "Where my cookies?", nil)
	ticket2 := models.NewTicket("645eee60-7e84-4a9e-ade3-4fce01ae28f1", models.Org1, models.BobID, models.ZendeskID, "14", "Second ticket", "Where my shoes?", nil)

	err = svc.Close([]*models.Ticket{ticket1, ticket2}, logger.Log)

	assert.NoError(t, err)
	assert.Equal(t, "PUT /api/v2/tickets/update_many.json?ids=12,14 HTTP/1.1\r\nHost: nyaruka.zendesk.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 30\r\nAuthorization: Bearer ****************\r\nContent-Type: application/json\r\nAccept-Encoding: gzip\r\n\r\n{\"ticket\":{\"status\":\"solved\"}}", logger.Logs[0].Request)

	err = svc.Reopen([]*models.Ticket{ticket2}, logger.Log)

	assert.NoError(t, err)
	assert.Equal(t, "PUT /api/v2/tickets/update_many.json?ids=14 HTTP/1.1\r\nHost: nyaruka.zendesk.com\r\nUser-Agent: Go-http-client/1.1\r\nContent-Length: 28\r\nAuthorization: Bearer ****************\r\nContent-Type: application/json\r\nAccept-Encoding: gzip\r\n\r\n{\"ticket\":{\"status\":\"open\"}}", logger.Logs[1].Request)
}