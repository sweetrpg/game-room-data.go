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
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type LibraryTestSuite struct {
	suite.Suite
}

func (suite *LibraryTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
}

func (suite *LibraryTestSuite) TestAddLibraryEntryPersistsTitle() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	lib, err := AddLibraryEntry(ctx, userID, "vol-1", "Maus I", userID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), lib)
	assert.Equal(suite.T(), "vol-1", lib.Entries[0].VolumeID)
	assert.Equal(suite.T(), "Maus I", lib.Entries[0].VolumeTitle)

	// the auto-created library carries a full create stamp
	assert.False(suite.T(), lib.CreatedAt.IsZero())
	assert.Equal(suite.T(), userID, lib.CreatedBy)
	assert.Equal(suite.T(), userID, lib.UpdatedBy)

	read, err := GetLibraryByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), read.Entries, 1)
	assert.Equal(suite.T(), "Maus I", read.Entries[0].VolumeTitle)
	assert.False(suite.T(), read.CreatedAt.IsZero())
}

func (suite *LibraryTestSuite) TestAddLibraryEntryIsIdempotent() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "First", userID)
	assert.NoError(suite.T(), err)
	lib, err := AddLibraryEntry(ctx, userID, "vol-1", "Second", userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), lib.Entries, 1)
	assert.Equal(suite.T(), "First", lib.Entries[0].VolumeTitle)
}

func (suite *LibraryTestSuite) TestUpdateLibraryEntryTitle() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "Old Title", userID)
	assert.NoError(suite.T(), err)

	lib, err := UpdateLibraryEntryTitle(ctx, userID, "vol-1", "New Title", userID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "New Title", lib.Entries[0].VolumeTitle)

	read, err := GetLibraryByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "New Title", read.Entries[0].VolumeTitle)
}

func (suite *LibraryTestSuite) TestUpdateLibraryEntryTitleNotFound() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "Maus I", userID)
	assert.NoError(suite.T(), err)

	lib, err := UpdateLibraryEntryTitle(ctx, userID, "vol-999", "Nope", userID)
	assert.ErrorIs(suite.T(), err, ErrLibraryEntryNotFound)
	assert.Nil(suite.T(), lib)
}

func (suite *LibraryTestSuite) TestLibraryUpdateAdvancesUpdateStampOnly() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "Old Title", userID)
	assert.NoError(suite.T(), err)

	base, err := GetLibraryByUser(ctx, userID)
	assert.NoError(suite.T(), err)

	editor := primitive.NewObjectID().Hex()
	time.Sleep(5 * time.Millisecond)
	updated, err := UpdateLibraryEntryTitle(ctx, userID, "vol-1", "New Title", editor)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), base.CreatedAt.Equal(updated.CreatedAt), "created_at unchanged by update")
	assert.Equal(suite.T(), userID, updated.CreatedBy)
	assert.Equal(suite.T(), editor, updated.UpdatedBy)
	assert.True(suite.T(), updated.UpdatedAt.After(base.UpdatedAt), "updated_at advanced")
}

func (suite *LibraryTestSuite) TestLibraryToVOCarriesTitleAndAuditFields() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "Persepolis", userID)
	assert.NoError(suite.T(), err)

	lib, err := GetLibraryByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	vo := LibraryToVO(lib, userID, false, false)
	assert.Len(suite.T(), vo.Entries, 1)
	assert.Equal(suite.T(), "vol-1", vo.Entries[0].VolumeID)
	assert.Equal(suite.T(), "Persepolis", vo.Entries[0].VolumeTitle)
	assert.Equal(suite.T(), lib.CreatedAt, vo.CreatedAt)
	assert.Equal(suite.T(), lib.UpdatedAt, vo.UpdatedAt)
	assert.Equal(suite.T(), userID, vo.CreatedBy)
}

func (suite *LibraryTestSuite) TestUpdateLibraryEntryTitleByVolume() {
	ctx := context.Background()
	target := "vol-shared"
	other := "vol-other"

	changed := []string{
		primitive.NewObjectID().Hex(),
		primitive.NewObjectID().Hex(),
		primitive.NewObjectID().Hex(),
	}
	for _, uid := range changed {
		_, err := AddLibraryEntry(ctx, uid, target, "Old Title")
		assert.NoError(suite.T(), err)
	}
	untouched := primitive.NewObjectID().Hex()
	_, err := AddLibraryEntry(ctx, untouched, other, "Keep Me")
	assert.NoError(suite.T(), err)

	got, err := UpdateLibraryEntryTitleByVolume(ctx, target, "New Title")
	assert.NoError(suite.T(), err)
	assert.ElementsMatch(suite.T(), changed, got)

	for _, uid := range changed {
		lib, err := GetLibraryByUser(ctx, uid)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "New Title", lib.Entries[0].VolumeTitle)
	}
	otherLib, err := GetLibraryByUser(ctx, untouched)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Keep Me", otherLib.Entries[0].VolumeTitle)

	// Replay is a no-op: the same title matches no stale entries.
	again, err := UpdateLibraryEntryTitleByVolume(ctx, target, "New Title")
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), again)
}

func (suite *LibraryTestSuite) TestUpdateLibraryEntryTitleByVolumeNoReferences() {
	ctx := context.Background()

	got, err := UpdateLibraryEntryTitleByVolume(ctx, "vol-nobody-has", "Whatever")
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), got)
}

func TestLibraryTestSuite(t *testing.T) {
	suite.Run(t, new(LibraryTestSuite))
}
