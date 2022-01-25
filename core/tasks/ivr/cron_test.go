package ivr

import (
	"encoding/json"
	"testing"

	"github.com/nyaruka/mailroom/config"
	"github.com/nyaruka/mailroom/core/ivr"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/starts"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
)

func TestRetries(t *testing.T) {
	ctx, _, db, rp := testsuite.Reset()
	rc := rp.Get()
	defer rc.Close()

	// register our mock client
	ivr.RegisterClientType(models.ChannelType("ZZ"), newMockClient)

	// update our twilio channel to be of type 'ZZ' and set max_concurrent_events to 1
	db.MustExec(`UPDATE channels_channel SET channel_type = 'ZZ', config = '{"max_concurrent_events": 1}' WHERE id = $1`, testdata.TwilioChannel.ID)

	// create a flow start for cathy
	start := models.NewFlowStart(testdata.Org1.ID, models.StartTypeTrigger, models.FlowTypeVoice, testdata.IVRFlow.ID, models.DoRestartParticipants, models.DoIncludeActive).
		WithContactIDs([]models.ContactID{testdata.Cathy.ID})

	// call our master starter
	err := starts.CreateFlowBatches(ctx, db, rp, nil, start)
	assert.NoError(t, err)

	// should have one task in our ivr queue
	task, err := queue.PopNextTask(rc, queue.HandlerQueue)
	assert.NoError(t, err)
	batch := &models.FlowStartBatch{}
	err = json.Unmarshal(task.Task, batch)
	assert.NoError(t, err)

	client.callError = nil
	client.callID = ivr.CallID("call1")
	err = HandleFlowStartBatch(ctx, config.Mailroom, db, rp, batch)
	assert.NoError(t, err)
	testsuite.AssertQuery(t, db, `SELECT COUNT(*) FROM channels_channelconnection WHERE contact_id = $1 AND status = $2 AND external_id = $3`,
		testdata.Cathy.ID, models.ConnectionStatusWired, "call1").Returns(1)

	// change our call to be errored instead of wired
	db.MustExec(`UPDATE channels_channelconnection SET status = 'E', next_attempt = NOW() WHERE external_id = 'call1';`)

	// fire our retries
	err = retryCalls(ctx, config.Mailroom, db, rp, "retry_test", "retry_test")
	assert.NoError(t, err)

	// should now be in wired state
	testsuite.AssertQuery(t, db, `SELECT COUNT(*) FROM channels_channelconnection WHERE contact_id = $1 AND status = $2 AND external_id = $3`,
		testdata.Cathy.ID, models.ConnectionStatusWired, "call1").Returns(1)

	// back to retry and make the channel inactive
	db.MustExec(`UPDATE channels_channelconnection SET status = 'E', next_attempt = NOW() WHERE external_id = 'call1';`)
	db.MustExec(`UPDATE channels_channel SET is_active = FALSE WHERE id = $1`, testdata.TwilioChannel.ID)

	models.FlushCache()
	err = retryCalls(ctx, config.Mailroom, db, rp, "retry_test", "retry_test")
	assert.NoError(t, err)

	// this time should be failed
	testsuite.AssertQuery(t, db, `SELECT COUNT(*) FROM channels_channelconnection WHERE contact_id = $1 AND status = $2 AND external_id = $3`,
		testdata.Cathy.ID, models.ConnectionStatusFailed, "call1").Returns(1)
}
