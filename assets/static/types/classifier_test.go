package types_test

import (
	"testing"

	"github.com/nyaruka/goflow/assets"
	"github.com/nyaruka/goflow/assets/static/types"

	"github.com/stretchr/testify/assert"
)

func TestClassifier(t *testing.T) {
	classifier := types.NewClassifier(
		assets.ClassifierUUID("37657cf7-5eab-4286-9cb0-bbf270587bad"),
		"Booking",
		"wit",
		[]string{"book_flight", "book_hotel"},
	)
	assert.Equal(t, assets.ClassifierUUID("37657cf7-5eab-4286-9cb0-bbf270587bad"), classifier.UUID())
	assert.Equal(t, "Booking", classifier.Name())
	assert.Equal(t, "wit", classifier.Type())
	assert.Equal(t, []string{"book_flight", "book_hotel"}, classifier.Intents())
}
