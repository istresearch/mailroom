package handlers_test

import (
	"testing"

	"github.com/nyaruka/gocommon/httpx"
	"github.com/nyaruka/gocommon/uuids"
	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/actions"
	"github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	_ "github.com/nyaruka/mailroom/services/tickets/mailgun"
	_ "github.com/nyaruka/mailroom/services/tickets/zendesk"

	"github.com/stretchr/testify/require"
)

func TestTicketOpened(t *testing.T) {
	ctx, _, db, _ := testsuite.Get()

	defer testsuite.Reset()
	defer httpx.SetRequestor(httpx.DefaultRequestor)

	httpx.SetRequestor(httpx.NewMockRequestor(map[string][]httpx.MockResponse{
		"https://api.mailgun.net/v3/tickets.rapidpro.io/messages": {
			httpx.NewMockResponse(200, nil, `{
				"id": "<20200426161758.1.590432020254B2BF@tickets.rapidpro.io>",
				"message": "Queued. Thank you."
			}`),
		},
		"https://nyaruka.zendesk.com/api/v2/any_channel/push.json": {
			httpx.NewMockResponse(201, nil, `{
				"results": [
					{
						"external_resource_id": "123",
						"status": {"code": "success"}
					}
				]
			}`),
		},
	}))

	// an existing ticket
	cathyTicket := models.NewTicket(flows.TicketUUID(uuids.New()), testdata.Org1.ID, testdata.Cathy.ID, testdata.Mailgun.ID, "748363", "Old Question", "Who?", models.NilUserID, nil)
	err := models.InsertTickets(ctx, db, []*models.Ticket{cathyTicket})
	require.NoError(t, err)

	tcs := []handlers.TestCase{
		{
			Actions: handlers.ContactActionMap{
				testdata.Cathy: []flows.Action{
					actions.NewOpenTicket(handlers.NewActionUUID(), assets.NewTicketerReference(testdata.Mailgun.UUID, "Mailgun (IT Support)"), "Need help", "Where are my cookies?", "Email Ticket"),
				},
				testdata.Bob: []flows.Action{
					actions.NewOpenTicket(handlers.NewActionUUID(), assets.NewTicketerReference(testdata.Zendesk.UUID, "Zendesk (Nyaruka)"), "Interesting", "I've found some cookies", "Zen Ticket"),
				},
			},
			SQLAssertions: []handlers.SQLAssertion{
				{ // cathy's old ticket will still be open and cathy's new ticket will have been created
					SQL:   "select count(*) from tickets_ticket where contact_id = $1 AND status = 'O' AND ticketer_id = $2",
					Args:  []interface{}{testdata.Cathy.ID, testdata.Mailgun.ID},
					Count: 2,
				},
				{ // and there's an HTTP log for that
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1",
					Args:  []interface{}{testdata.Mailgun.ID},
					Count: 1,
				},
				{ // which doesn't include our API token
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1 AND request like '%sesame%'",
					Args:  []interface{}{testdata.Mailgun.ID},
					Count: 0,
				},
				{ // bob's ticket will have been created too
					SQL:   "select count(*) from tickets_ticket where contact_id = $1 AND status = 'O' AND ticketer_id = $2",
					Args:  []interface{}{testdata.Bob.ID, testdata.Zendesk.ID},
					Count: 1,
				},
				{ // and there's an HTTP log for that
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1",
					Args:  []interface{}{testdata.Zendesk.ID},
					Count: 1,
				},
				{ // which doesn't include our API token
					SQL:   "select count(*) from request_logs_httplog where ticketer_id = $1 AND request like '%523562%'",
					Args:  []interface{}{testdata.Zendesk.ID},
					Count: 0,
				},
				{ // and we have 2 ticket opened events for the 2 tickets opened
					SQL:   "select count(*) from tickets_ticketevent where event_type = 'O'",
					Count: 2,
				},
			},
		},
	}

	handlers.RunTestCases(t, tcs)
}
