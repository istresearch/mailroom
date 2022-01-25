package handlers_test

import (
	"testing"

	"github.com/nyaruka/goflow/flows"
	"github.com/nyaruka/goflow/flows/modifiers"
	"github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"
)

func TestContactStatusChanged(t *testing.T) {
	_, _, db, _ := testsuite.Get()

	defer testsuite.Reset()

	// make sure cathyID contact is active
	db.Exec(`UPDATE contacts_contact SET status = 'A' WHERE id = $1`, testdata.Cathy.ID)

	tcs := []handlers.TestCase{
		{
			Modifiers: handlers.ContactModifierMap{
				testdata.Cathy: []flows.Modifier{modifiers.NewStatus(flows.ContactStatusBlocked)},
			},
			SQLAssertions: []handlers.SQLAssertion{
				{
					SQL:   `select count(*) from contacts_contact where id = $1 AND status = 'B'`,
					Args:  []interface{}{testdata.Cathy.ID},
					Count: 1,
				},
			},
		},
		{
			Modifiers: handlers.ContactModifierMap{
				testdata.Cathy: []flows.Modifier{modifiers.NewStatus(flows.ContactStatusStopped)},
			},
			SQLAssertions: []handlers.SQLAssertion{
				{
					SQL:   `select count(*) from contacts_contact where id = $1 AND status = 'S'`,
					Args:  []interface{}{testdata.Cathy.ID},
					Count: 1,
				},
			},
		},
		{
			Modifiers: handlers.ContactModifierMap{
				testdata.Cathy: []flows.Modifier{modifiers.NewStatus(flows.ContactStatusActive)},
			},
			SQLAssertions: []handlers.SQLAssertion{
				{
					SQL:   `select count(*) from contacts_contact where id = $1 AND status = 'A'`,
					Args:  []interface{}{testdata.Cathy.ID},
					Count: 1,
				},
				{
					SQL:   `select count(*) from contacts_contact where id = $1 AND status = 'A'`,
					Args:  []interface{}{testdata.Cathy.ID},
					Count: 1,
				},
			},
		},
	}

	handlers.RunTestCases(t, tcs)
}
