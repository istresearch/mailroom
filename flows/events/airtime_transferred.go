package events

import (
	"github.com/nyaruka/gocommon/urns"
	"github.com/nyaruka/goflow/flows"

	"github.com/shopspring/decimal"
)

func init() {
	registerType(TypeAirtimeTransferred, func() flows.Event { return &AirtimeTransferredEvent{} })
}

// TypeAirtimeTransferred is the type of our airtime transferred event
const TypeAirtimeTransferred string = "airtime_transferred"

// AirtimeTransferredEvent events are created when airtime has been transferred to the contact.
//
//   {
//     "type": "airtime_transferred",
//     "created_on": "2006-01-02T15:04:05Z",
//     "sender": "tel:4748",
//     "recipient": "tel:+1242563637",
//     "currency": "RWF",
//     "desired_amount": 120,
//     "actual_amount": 100,
//     "http_logs": [
//       {
//         "url": "https://airtime-api.dtone.com/cgi-bin/shop/topup",
//         "status": "success",
//         "request": "POST /topup HTTP/1.1\r\n\r\naction=ping",
//         "response": "HTTP/1.1 200 OK\r\n\r\ninfo_txt=pong\r\n",
//         "created_on": "2006-01-02T15:04:05Z",
//         "elapsed_ms": 123
//       }
//     ]
//   }
//
// @event airtime_transferred
type AirtimeTransferredEvent struct {
	baseEvent

	Sender        urns.URN         `json:"sender"`
	Recipient     urns.URN         `json:"recipient"`
	Currency      string           `json:"currency"`
	DesiredAmount decimal.Decimal  `json:"desired_amount"`
	ActualAmount  decimal.Decimal  `json:"actual_amount"`
	HTTPLogs      []*flows.HTTPLog `json:"http_logs"`
}

// NewAirtimeTransferred creates a new airtime transferred event
func NewAirtimeTransferred(t *flows.AirtimeTransfer, httpLogs []*flows.HTTPLog) *AirtimeTransferredEvent {
	return &AirtimeTransferredEvent{
		baseEvent:     newBaseEvent(TypeAirtimeTransferred),
		Sender:        t.Sender,
		Recipient:     t.Recipient,
		Currency:      t.Currency,
		DesiredAmount: t.DesiredAmount,
		ActualAmount:  t.ActualAmount,
		HTTPLogs:      httpLogs,
	}
}
