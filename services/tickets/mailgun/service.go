package mailgun

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"text/template"

	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/utils"
	"github.com/nyaruka/goflow/utils/httpx"
	"github.com/nyaruka/goflow/utils/uuids"
	"github.com/nyaruka/mailroom/models"
	"github.com/nyaruka/mailroom/services/tickets"

	"github.com/pkg/errors"
)

const (
	typeMailgun = "mailgun"

	configDomain    = "domain"
	configAPIKey    = "api_key"
	configToAddress = "to_address"
	configBrandName = "brand_name"
	configURLBase   = "url_base"

	ticketConfigContactUUID    = "contact-uuid"
	ticketConfigContactDisplay = "contact-display"
	ticketConfigLastMessageID  = "last-message-id"
)

// body template for new ticket being opened
var openBodyTemplate = newTemplate("open_body", `New ticket opened
------------------------------------------------

{{.body}}

------------------------------------------------
* Reply to the contact by replying to this email
* Close this ticket by replying with CLOSE
* View this contact at {{.contact_url}}
`)

// body template for message being forwarded from contact
var forwardBodyTemplate = newTemplate("forward_body", `{{.contact}} replied:
------------------------------------------------

{{.message}}

------------------------------------------------
* Reply to the contact by replying to this email
* Close this ticket by replying with CLOSE
* View this contact at {{.contact_url}}
`)

// body template for ticket being closed
var closedBodyTemplate = newTemplate("closed_body", `{{.message}}
* Ticket has been closed
* Replying to the contact will reopen this ticket
* View this contact at {{.contact_url}}
`)

// body template for ticket being reopened
var reopenedBodyTemplate = newTemplate("reopened_body", `{{.message}}
* Ticket has been reopened
* Close this ticket by replying with CLOSE
* View this contact at {{.contact_url}}
`)

func init() {
	models.RegisterTicketService(typeMailgun, NewService)
}

type service struct {
	client    *Client
	ticketer  *flows.Ticketer
	toAddress string
	brandName string
	urlBase   string
	redactor  utils.Redactor
}

// NewService creates a new mailgun email-based ticket service
func NewService(httpClient *http.Client, httpRetries *httpx.RetryConfig, ticketer *flows.Ticketer, config map[string]string) (models.TicketService, error) {
	domain := config[configDomain]
	apiKey := config[configAPIKey]
	toAddress := config[configToAddress]
	brandName := config[configBrandName]
	urlBase := config[configURLBase]

	if domain != "" && apiKey != "" && toAddress != "" && urlBase != "" {
		// need to redact the string used for basic auth
		basicAuth := base64.StdEncoding.EncodeToString([]byte("api:" + apiKey))

		return &service{
			client:    NewClient(httpClient, httpRetries, domain, apiKey),
			ticketer:  ticketer,
			toAddress: toAddress,
			brandName: brandName,
			urlBase:   urlBase,
			redactor:  utils.NewRedactor(flows.RedactionMask, apiKey, basicAuth),
		}, nil
	}
	return nil, errors.New("missing domain or api_key or to_address or url_base in mailgun config")
}

// Open opens a ticket which for mailgun means just sending an initial email
func (s *service) Open(session flows.Session, subject, body string, logHTTP flows.HTTPLogCallback) (*flows.Ticket, error) {
	ticketUUID := flows.TicketUUID(uuids.New())
	contactDisplay := tickets.GetContactDisplay(session.Environment(), session.Contact())

	from := s.ticketAddress(contactDisplay, ticketUUID)
	context := s.templateContext(subject, body, "", string(session.Contact().UUID()), contactDisplay)
	fullBody := evaluateTemplate(openBodyTemplate, context)

	msgID, trace, err := s.client.SendMessage(from, s.toAddress, subject, fullBody, nil)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return nil, errors.Wrap(err, "error calling mailgun API")
	}

	return flows.NewTicket(ticketUUID, s.ticketer.Reference(), subject, body, msgID), nil
}

func (s *service) Forward(ticket *models.Ticket, msgUUID flows.MsgUUID, text string, logHTTP flows.HTTPLogCallback) error {
	context := s.templateContext(ticket.Subject(), ticket.Body(), text, ticket.Config(ticketConfigContactUUID), ticket.Config(ticketConfigContactDisplay))
	body := evaluateTemplate(forwardBodyTemplate, context)

	_, err := s.sendInTicket(ticket, body, logHTTP)
	return err
}

func (s *service) Close(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	for _, ticket := range tickets {
		context := s.templateContext(ticket.Subject(), ticket.Body(), "", ticket.Config(ticketConfigContactUUID), ticket.Config(ticketConfigContactDisplay))
		body := evaluateTemplate(closedBodyTemplate, context)

		_, err := s.sendInTicket(ticket, body, logHTTP)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *service) Reopen(tickets []*models.Ticket, logHTTP flows.HTTPLogCallback) error {
	for _, ticket := range tickets {
		context := s.templateContext(ticket.Subject(), ticket.Body(), "", ticket.Config(ticketConfigContactUUID), ticket.Config(ticketConfigContactDisplay))
		body := evaluateTemplate(reopenedBodyTemplate, context)

		_, err := s.sendInTicket(ticket, body, logHTTP)
		if err != nil {
			return err
		}
	}
	return nil
}

// sends an email as part of the thread for the given ticket
func (s *service) sendInTicket(ticket *models.Ticket, text string, logHTTP flows.HTTPLogCallback) (string, error) {
	contactDisplay := ticket.Config(ticketConfigContactDisplay)
	lastMessageID := ticket.Config(ticketConfigLastMessageID)
	if lastMessageID == "" {
		lastMessageID = string(ticket.ExternalID()) // id of first message sent becomes external ID
	}
	headers := map[string]string{
		"In-Reply-To": lastMessageID,
		"References":  lastMessageID,
	}
	from := s.ticketAddress(contactDisplay, ticket.UUID())

	return s.send(from, s.toAddress, ticket.Subject(), text, headers, logHTTP)
}

func (s *service) send(from, to, subject, text string, headers map[string]string, logHTTP flows.HTTPLogCallback) (string, error) {
	msgID, trace, err := s.client.SendMessage(from, to, subject, text, headers)
	if trace != nil {
		logHTTP(flows.NewHTTPLog(trace, flows.HTTPStatusFromCode, s.redactor))
	}
	if err != nil {
		return "", errors.Wrap(err, "error calling mailgun API")
	}

	return msgID, nil
}

func (s *service) ticketAddress(contactDisplay string, ticketUUID flows.TicketUUID) string {
	address := fmt.Sprintf("ticket+%s@%s", ticketUUID, s.client.domain)
	return fmt.Sprintf("%s via %s <%s>", contactDisplay, s.brandName, address)
}

func (s *service) noReplyAddress() string {
	return fmt.Sprintf("no-reply@%s", s.client.domain)
}

func (s *service) templateContext(subject, body, message, contactUUID, contactDisplay string) map[string]string {
	return map[string]string{
		"brand":       s.brandName,                                                // rapidpro brand
		"subject":     subject,                                                    // original ticket subject
		"body":        body,                                                       // original ticket body
		"message":     message,                                                    // new message if this is a forward
		"contact":     contactDisplay,                                             // display name contact
		"contact_url": fmt.Sprintf("%s/contact/read/%s/", s.urlBase, contactUUID), // link to contact
	}
}

func newTemplate(name, value string) *template.Template {
	return template.Must(template.New(name).Parse(value))
}

func evaluateTemplate(t *template.Template, c map[string]string) string {
	b := &strings.Builder{}
	t.Execute(b, c)
	return b.String()
}
