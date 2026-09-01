package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

func TestLiveDoesNotMutateInput(t *testing.T) {
	in := bson.D{{Key: "_id", Value: "x"}}
	out := live(in)

	assert.Len(t, in, 1, "input filter must be untouched")
	assert.Len(t, out, 2)
	assert.Equal(t, "deleted_at", out[1].Key)
	assert.Nil(t, out[1].Value)

	// mutating the result must not reach back into the input's backing array
	out[0] = bson.E{Key: "_id", Value: "y"}
	assert.Equal(t, "x", in[0].Value)
}
