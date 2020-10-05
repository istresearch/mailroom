package tasks_test

import (
	"testing"

	"github.com/nyaruka/mailroom/models"
	"github.com/nyaruka/mailroom/tasks"
	"github.com/nyaruka/mailroom/tasks/groups"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadTask(t *testing.T) {
	task, err := tasks.ReadTask("populate_dynamic_group", []byte(`{
		"group_id": 23,
		"query": "gender = F"
	}`))
	require.NoError(t, err)

	typedTask := task.(*groups.PopulateDynamicGroupTask)
	assert.Equal(t, models.GroupID(23), typedTask.GroupID)
	assert.Equal(t, "gender = F", typedTask.Query)
}
