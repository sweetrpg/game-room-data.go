package data

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-objects.go/models"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type BackfillTestSuite struct {
	suite.Suite
}

func (suite *BackfillTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
}

// insertPreAuditTable writes a table document straight to Mongo with no audit fields, standing in
// for a record created before the audit-fields convention.
func (suite *BackfillTestSuite) insertPreAuditTable(ctx context.Context, userID string) (string, primitive.DateTime) {
	oid := primitive.NewObjectID()
	_, err := database.Db.Collection(tableCollection).InsertOne(ctx, bson.D{
		{Key: "_id", Value: oid.Hex()},
		{Key: "user_id", Value: userID},
		{Key: "name", Value: "legacy"},
		{Key: "visibility", Value: string(models.VisibilityPrivate)},
	})
	assert.NoError(suite.T(), err)
	return oid.Hex(), primitive.NewDateTimeFromTime(oid.Timestamp())
}

func (suite *BackfillTestSuite) TestBackfillDryRunCountsButDoesNotWrite() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()
	id, _ := suite.insertPreAuditTable(ctx, userID)

	results, err := BackfillAuditFields(ctx, true)
	assert.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), collResult(results, tableCollection).Matched, 1)

	var raw bson.M
	err = database.Db.Collection(tableCollection).FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&raw)
	assert.NoError(suite.T(), err)
	_, hasCreatedAt := raw["created_at"]
	assert.False(suite.T(), hasCreatedAt, "dry run must not write")
}

func (suite *BackfillTestSuite) TestBackfillSeedsFieldsAndIsIdempotent() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()
	id, oidTS := suite.insertPreAuditTable(ctx, userID)

	_, err := BackfillAuditFields(ctx, false)
	assert.NoError(suite.T(), err)

	var raw struct {
		CreatedAt primitive.DateTime `bson:"created_at"`
		UpdatedAt primitive.DateTime `bson:"updated_at"`
		CreatedBy string             `bson:"created_by"`
		UpdatedBy string             `bson:"updated_by"`
	}
	err = database.Db.Collection(tableCollection).FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&raw)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), oidTS, raw.CreatedAt)
	assert.Equal(suite.T(), oidTS, raw.UpdatedAt)
	assert.Equal(suite.T(), userID, raw.CreatedBy)
	assert.Equal(suite.T(), userID, raw.UpdatedBy)

	// second run touches nothing
	second, err := BackfillAuditFields(ctx, false)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, collResult(second, tableCollection).Updated)
}

func collResult(rs []CollectionBackfillResult, name string) CollectionBackfillResult {
	for _, r := range rs {
		if r.Collection == name {
			return r
		}
	}
	return CollectionBackfillResult{}
}

func TestBackfillTestSuite(t *testing.T) {
	suite.Run(t, new(BackfillTestSuite))
}
