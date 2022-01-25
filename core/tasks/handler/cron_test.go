package handler_test

import (
	"testing"
	"time"

	"github.com/nyaruka/gocommon/uuids"

	_ "github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/tasks/handler"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
)

func TestRetryMsgs(t *testing.T) {
	ctx, rt, db, rp := testsuite.Reset()
	rc := rp.Get()
	defer rc.Close()

	// noop does nothing
	err := handler.RetryPendingMsgs(ctx, db, rp, "test", "test")
	assert.NoError(t, err)

	testMsgs := []struct {
		Text      string
		Status    models.MsgStatus
		CreatedOn time.Time
	}{
		{"pending", models.MsgStatusPending, time.Now().Add(-time.Hour)},
		{"handled", models.MsgStatusHandled, time.Now().Add(-time.Hour)},
		{"recent", models.MsgStatusPending, time.Now()},
	}

	for _, msg := range testMsgs {
		db.MustExec(
			`INSERT INTO msgs_msg(uuid, org_id, channel_id, contact_id, contact_urn_id, text, direction, status, created_on, visibility, msg_count, error_count, next_attempt) 
						   VALUES($1,   $2,     $3,         $4,         $5,             $6,   $7,        $8,     $9,         'V',        1,         0,           NOW())`,
			uuids.New(), testdata.Org1.ID, testdata.TwilioChannel.ID, testdata.Cathy.ID, testdata.Cathy.URNID, msg.Text, models.DirectionIn, msg.Status, msg.CreatedOn)
	}

	err = handler.RetryPendingMsgs(ctx, db, rp, "test", "test")
	assert.NoError(t, err)

	// should have one message requeued
	task, _ := queue.PopNextTask(rc, queue.HandlerQueue)
	assert.NotNil(t, task)
	err = handler.HandleEvent(ctx, rt, task)
	assert.NoError(t, err)

	// message should be handled now
	testsuite.AssertQuery(t, db, `SELECT count(*) from msgs_msg WHERE text = 'pending' AND status = 'H'`).Returns(1)

	// only one message was queued
	task, _ = queue.PopNextTask(rc, queue.HandlerQueue)
	assert.Nil(t, task)
}
