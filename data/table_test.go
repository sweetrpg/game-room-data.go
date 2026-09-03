package data

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TableTestSuite struct {
	suite.Suite
}

func (suite *TableTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
}

func (suite *TableTestSuite) TestCreateTableStampsAllFourAuditFields() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	tbl, err := CreateTable(ctx, userID, "Curse of Strahd", userID)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), tbl.CreatedAt.IsZero())
	assert.Equal(suite.T(), userID, tbl.CreatedBy)
	assert.Equal(suite.T(), userID, tbl.UpdatedBy)
	assert.Equal(suite.T(), tbl.CreatedAt, tbl.UpdatedAt)
	assert.Nil(suite.T(), tbl.DeletedAt)

	got, err := GetTable(ctx, tbl.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), got)
	assert.Equal(suite.T(), userID, got.CreatedBy)
}

func (suite *TableTestSuite) TestUpdateTableAdvancesUpdateStampOnly() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	tbl, err := CreateTable(ctx, userID, "Original", userID)
	assert.NoError(suite.T(), err)

	base, err := GetTable(ctx, tbl.ID)
	assert.NoError(suite.T(), err)

	editor := primitive.NewObjectID().Hex()
	time.Sleep(5 * time.Millisecond)
	updated, err := UpdateTableName(ctx, tbl.ID, userID, "Renamed", editor)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), base.CreatedAt.Equal(updated.CreatedAt), "created_at unchanged by update")
	assert.Equal(suite.T(), userID, updated.CreatedBy)
	assert.Equal(suite.T(), editor, updated.UpdatedBy)
	assert.True(suite.T(), updated.UpdatedAt.After(base.UpdatedAt), "updated_at advanced")
}

func (suite *TableTestSuite) TestDeleteTableIsSoftAndExcludedFromReads() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	tbl, err := CreateTable(ctx, userID, "Doomed", userID)
	assert.NoError(suite.T(), err)

	deleted, err := DeleteTable(ctx, tbl.ID, userID, userID)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), deleted)

	// gone from every read path, including another user's filtered view
	got, err := GetTable(ctx, tbl.ID)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)
	mine, err := ListTablesByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), mine)

	// document persists with the delete stamp
	var raw bson.M
	err = database.Db.Collection(tableCollection).FindOne(ctx, bson.D{{Key: "_id", Value: tbl.ID}}).Decode(&raw)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), raw["deleted_at"])
	assert.Equal(suite.T(), userID, raw["deleted_by"])

	again, err := DeleteTable(ctx, tbl.ID, userID, userID)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), again)
}

func (suite *TableTestSuite) TestTableToVOCarriesAuditFields() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	tbl, err := CreateTable(ctx, userID, "VO check", userID)
	assert.NoError(suite.T(), err)

	got, err := GetTable(ctx, tbl.ID)
	assert.NoError(suite.T(), err)
	vo := TableToVO(got, userID, false, false)
	assert.NotNil(suite.T(), vo)
	assert.Equal(suite.T(), got.CreatedAt, vo.CreatedAt)
	assert.Equal(suite.T(), got.UpdatedAt, vo.UpdatedAt)
	assert.Equal(suite.T(), userID, vo.CreatedBy)
	assert.Equal(suite.T(), userID, vo.UpdatedBy)
}

func (suite *TableTestSuite) TestAddTableVolumeStoresTitleSidecar() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	tbl, err := CreateTable(ctx, userID, "Sidecar", userID)
	assert.NoError(suite.T(), err)

	// add with a title -> sidecar has it
	updated, err := AddTableVolume(ctx, tbl.ID, userID, "vol-1", "Pathfinder Core", userID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Pathfinder Core", updated.VolumeTitles["vol-1"])

	// add with an empty title -> no sidecar entry
	updated, err = AddTableVolume(ctx, tbl.ID, userID, "vol-2", "", userID)
	assert.NoError(suite.T(), err)
	_, present := updated.VolumeTitles["vol-2"]
	assert.False(suite.T(), present)

	// remove -> key gone, volume ID gone
	updated, err = RemoveTableVolume(ctx, tbl.ID, userID, "vol-1", userID)
	assert.NoError(suite.T(), err)
	_, present = updated.VolumeTitles["vol-1"]
	assert.False(suite.T(), present)
	assert.NotContains(suite.T(), updated.VolumeIDs, "vol-1")

	read, err := GetTable(ctx, tbl.ID)
	assert.NoError(suite.T(), err)
	vo := TableToVO(read, userID, false, false)
	assert.NotContains(suite.T(), vo.VolumeTitles, "vol-1")
}

func (suite *TableTestSuite) TestUpdateTableVolumeTitleByVolume() {
	ctx := context.Background()
	target := "vol-shared"
	other := "vol-other"

	changed := []string{primitive.NewObjectID().Hex(), primitive.NewObjectID().Hex()}
	for _, uid := range changed {
		tbl, err := CreateTable(ctx, uid, "T", uid)
		assert.NoError(suite.T(), err)
		_, err = AddTableVolume(ctx, tbl.ID, uid, target, "Old Title", uid)
		assert.NoError(suite.T(), err)
	}
	untouched := primitive.NewObjectID().Hex()
	utbl, err := CreateTable(ctx, untouched, "U", untouched)
	assert.NoError(suite.T(), err)
	_, err = AddTableVolume(ctx, utbl.ID, untouched, other, "Keep Me", untouched)
	assert.NoError(suite.T(), err)

	got, err := UpdateTableVolumeTitleByVolume(ctx, target, "New Title")
	assert.NoError(suite.T(), err)
	assert.ElementsMatch(suite.T(), changed, got)

	for _, uid := range changed {
		tbls, err := ListTablesByUser(ctx, uid)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "New Title", tbls[0].VolumeTitles[target])
		assert.Equal(suite.T(), SystemActor, tbls[0].UpdatedBy)
	}
	utbls, err := ListTablesByUser(ctx, untouched)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Keep Me", utbls[0].VolumeTitles[other])

	// replay with the same title is a no-op
	again, err := UpdateTableVolumeTitleByVolume(ctx, target, "New Title")
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), again)
}

func (suite *TableTestSuite) TestUpdateTableVolumeTitleByVolumeSkipsDottedVolumeID() {
	ctx := context.Background()

	got, err := UpdateTableVolumeTitleByVolume(ctx, "bad.id", "Whatever")
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), got)
}

func TestTableTestSuite(t *testing.T) {
	suite.Run(t, new(TableTestSuite))
}
