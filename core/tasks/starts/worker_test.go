package starts

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nyaruka/gocommon/dbutil/assertdb"
	_ "github.com/nyaruka/mailroom/core/handlers"
	"github.com/nyaruka/mailroom/core/models"
	"github.com/nyaruka/mailroom/core/queue"
	"github.com/nyaruka/mailroom/core/runner"
	"github.com/nyaruka/mailroom/testsuite"
	"github.com/nyaruka/mailroom/testsuite/testdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStarts(t *testing.T) {
	ctx, rt, db, rp := testsuite.Get()
	rc := rp.Get()
	defer rc.Close()

	defer testsuite.Reset(testsuite.ResetAll)

	mockES := testsuite.NewMockElasticServer()
	defer mockES.Close()

	rt.ES = mockES.Client()

	// convert our single message flow to an actual background flow that shouldn't interrupt
	db.MustExec(`UPDATE flows_flow SET flow_type = 'B' WHERE id = $1`, testdata.SingleMessage.ID)

	sID := testdata.InsertWaitingSession(db, testdata.Org1, testdata.George, models.FlowTypeMessaging, testdata.Favorites, models.NilConnectionID, time.Now(), time.Now(), true, nil)
	testdata.InsertFlowRun(db, testdata.Org1, sID, testdata.George, testdata.Favorites, models.RunStatusWaiting)

	tcs := []struct {
		label                    string
		flowID                   models.FlowID
		groupIDs                 []models.GroupID
		excludeGroupIDs          []models.GroupID
		contactIDs               []models.ContactID
		createContact            bool
		query                    string
		queryResult              []models.ContactID
		excludeInAFlow           bool
		excludeStartedPreviously bool
		queue                    string
		expectedContactCount     int
		expectedBatchCount       int
		expectedTotalCount       int
		expectedStatus           models.StartStatus
		expectedActiveRuns       map[models.FlowID]int
	}{
		{
			label:                    "Empty flow start",
			flowID:                   testdata.Favorites.ID,
			excludeInAFlow:           true,
			excludeStartedPreviously: true,
			queue:                    queue.BatchQueue,
			expectedContactCount:     0,
			expectedBatchCount:       0,
			expectedTotalCount:       0,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 1, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Single group",
			flowID:                   testdata.Favorites.ID,
			groupIDs:                 []models.GroupID{testdata.DoctorsGroup.ID},
			excludeInAFlow:           true,
			excludeStartedPreviously: true,
			queue:                    queue.BatchQueue,
			expectedContactCount:     121,
			expectedBatchCount:       2,
			expectedTotalCount:       121,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 122, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Group and Contact (but all already active)",
			flowID:                   testdata.Favorites.ID,
			groupIDs:                 []models.GroupID{testdata.DoctorsGroup.ID},
			contactIDs:               []models.ContactID{testdata.Cathy.ID},
			excludeInAFlow:           true,
			excludeStartedPreviously: true,
			queue:                    queue.BatchQueue,
			expectedContactCount:     121,
			expectedBatchCount:       2,
			expectedTotalCount:       0,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 122, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Contact restart",
			flowID:                   testdata.Favorites.ID,
			contactIDs:               []models.ContactID{testdata.Cathy.ID},
			excludeInAFlow:           false,
			excludeStartedPreviously: false,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       1,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 122, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Previous group and one new contact",
			flowID:                   testdata.Favorites.ID,
			groupIDs:                 []models.GroupID{testdata.DoctorsGroup.ID},
			contactIDs:               []models.ContactID{testdata.Bob.ID},
			excludeStartedPreviously: true,
			queue:                    queue.BatchQueue,
			expectedContactCount:     122,
			expectedBatchCount:       2,
			expectedTotalCount:       1,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Single contact, no restart",
			flowID:                   testdata.Favorites.ID,
			contactIDs:               []models.ContactID{testdata.Bob.ID},
			excludeStartedPreviously: true,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       0,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Single contact, include active, but no restart",
			flowID:                   testdata.Favorites.ID,
			contactIDs:               []models.ContactID{testdata.Bob.ID},
			excludeInAFlow:           false,
			excludeStartedPreviously: true,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       0,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Single contact, include active and restart",
			flowID:                   testdata.Favorites.ID,
			contactIDs:               []models.ContactID{testdata.Bob.ID},
			excludeInAFlow:           false,
			excludeStartedPreviously: false,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       1,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Query start",
			flowID:                   testdata.Favorites.ID,
			query:                    "bob",
			queryResult:              []models.ContactID{testdata.Bob.ID},
			excludeInAFlow:           false,
			excludeStartedPreviously: false,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       1,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Query start with invalid query",
			flowID:                   testdata.Favorites.ID,
			query:                    "xyz = 45",
			excludeInAFlow:           false,
			excludeStartedPreviously: false,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     0,
			expectedBatchCount:       0,
			expectedTotalCount:       0,
			expectedStatus:           models.StartStatusFailed,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                "New Contact",
			flowID:               testdata.Favorites.ID,
			createContact:        true,
			queue:                queue.HandlerQueue,
			expectedContactCount: 1,
			expectedBatchCount:   1,
			expectedTotalCount:   1,
			expectedStatus:       models.StartStatusComplete,
			expectedActiveRuns:   map[models.FlowID]int{testdata.Favorites.ID: 124, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Other messaging flow",
			flowID:                   testdata.PickANumber.ID,
			contactIDs:               []models.ContactID{testdata.Bob.ID},
			excludeInAFlow:           false,
			excludeStartedPreviously: true,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       1,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 1, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Background flow",
			flowID:                   testdata.SingleMessage.ID,
			contactIDs:               []models.ContactID{testdata.Bob.ID},
			excludeInAFlow:           false,
			excludeStartedPreviously: true,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       1,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 123, testdata.PickANumber.ID: 1, testdata.SingleMessage.ID: 0},
		},
		{
			label:                    "Exclude group",
			flowID:                   testdata.Favorites.ID,
			contactIDs:               []models.ContactID{testdata.Cathy.ID, testdata.Bob.ID},
			excludeGroupIDs:          []models.GroupID{testdata.DoctorsGroup.ID}, // should exclude Cathy
			excludeInAFlow:           false,
			excludeStartedPreviously: false,
			queue:                    queue.HandlerQueue,
			expectedContactCount:     1,
			expectedBatchCount:       1,
			expectedTotalCount:       1,
			expectedStatus:           models.StartStatusComplete,
			expectedActiveRuns:       map[models.FlowID]int{testdata.Favorites.ID: 124, testdata.PickANumber.ID: 0, testdata.SingleMessage.ID: 0},
		},
	}

	for _, tc := range tcs {
		if tc.queryResult != nil {
			mockES.AddResponse(tc.queryResult...)
		}

		// handle our start task
		start := models.NewFlowStart(testdata.Org1.ID, models.StartTypeManual, models.FlowTypeMessaging, tc.flowID).
			WithGroupIDs(tc.groupIDs).
			WithExcludeGroupIDs(tc.excludeGroupIDs).
			WithContactIDs(tc.contactIDs).
			WithQuery(tc.query).
			WithExcludeInAFlow(tc.excludeInAFlow).
			WithExcludeStartedPreviously(tc.excludeStartedPreviously).
			WithCreateContact(tc.createContact)

		err := models.InsertFlowStarts(ctx, db, []*models.FlowStart{start})
		assert.NoError(t, err)

		startJSON, err := json.Marshal(start)
		require.NoError(t, err)

		err = handleFlowStart(ctx, rt, &queue.Task{Type: queue.StartFlow, Task: startJSON})
		assert.NoError(t, err)

		// pop all our tasks and execute them
		var task *queue.Task
		count := 0
		for {
			task, err = queue.PopNextTask(rc, tc.queue)
			assert.NoError(t, err)
			if task == nil {
				break
			}

			count++
			assert.Equal(t, queue.StartFlowBatch, task.Type)
			batch := &models.FlowStartBatch{}
			err = json.Unmarshal(task.Task, batch)
			assert.NoError(t, err)

			_, err = runner.StartFlowBatch(ctx, rt, batch)
			assert.NoError(t, err)
		}

		// assert our count of batches
		assert.Equal(t, tc.expectedBatchCount, count, "unexpected batch count in '%s'", tc.label)

		// assert our count of total flow runs created
		assertdb.Query(t, db, `SELECT count(*) FROM flows_flowrun WHERE flow_id = $1 AND start_id = $2`, tc.flowID, start.ID()).Returns(tc.expectedTotalCount, "unexpected total run count in '%s'", tc.label)

		// assert final status
		assertdb.Query(t, db, `SELECT count(*) FROM flows_flowstart where status = $2 AND id = $1`, start.ID(), tc.expectedStatus).Returns(1, "status mismatch in '%s'", tc.label)

		// assert final contact count
		if tc.expectedStatus != models.StartStatusFailed {
			assertdb.Query(t, db, `SELECT count(*) FROM flows_flowstart where contact_count = $2 AND id = $1`,
				[]interface{}{start.ID(), tc.expectedContactCount}, 1, "contact count mismatch in '%s'", tc.label)
		}

		// assert count of active runs by flow
		for flowID, activeRuns := range tc.expectedActiveRuns {
			assertdb.Query(t, db, `SELECT count(*) FROM flows_flowrun WHERE status = 'W' AND flow_id = $1`, flowID).Returns(activeRuns, "active runs mismatch for flow #%d in '%s'", flowID, tc.label)
		}
	}
}
