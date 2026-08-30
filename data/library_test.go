package data

import (
	"context"
	"os"
	"testing"

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

	lib, err := AddLibraryEntry(ctx, userID, "vol-1", "Maus I")
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), lib)
	assert.Equal(suite.T(), "vol-1", lib.Entries[0].VolumeID)
	assert.Equal(suite.T(), "Maus I", lib.Entries[0].VolumeTitle)

	read, err := GetLibraryByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), read.Entries, 1)
	assert.Equal(suite.T(), "Maus I", read.Entries[0].VolumeTitle)
}

func (suite *LibraryTestSuite) TestAddLibraryEntryIsIdempotent() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "First")
	assert.NoError(suite.T(), err)
	lib, err := AddLibraryEntry(ctx, userID, "vol-1", "Second")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), lib.Entries, 1)
	assert.Equal(suite.T(), "First", lib.Entries[0].VolumeTitle)
}

func (suite *LibraryTestSuite) TestUpdateLibraryEntryTitle() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "Old Title")
	assert.NoError(suite.T(), err)

	lib, err := UpdateLibraryEntryTitle(ctx, userID, "vol-1", "New Title")
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "New Title", lib.Entries[0].VolumeTitle)

	read, err := GetLibraryByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "New Title", read.Entries[0].VolumeTitle)
}

func (suite *LibraryTestSuite) TestUpdateLibraryEntryTitleNotFound() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "Maus I")
	assert.NoError(suite.T(), err)

	lib, err := UpdateLibraryEntryTitle(ctx, userID, "vol-999", "Nope")
	assert.ErrorIs(suite.T(), err, ErrLibraryEntryNotFound)
	assert.Nil(suite.T(), lib)
}

func (suite *LibraryTestSuite) TestLibraryToVOCarriesTitle() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	_, err := AddLibraryEntry(ctx, userID, "vol-1", "Persepolis")
	assert.NoError(suite.T(), err)

	lib, err := GetLibraryByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	vo := LibraryToVO(lib, userID, false, false)
	assert.Len(suite.T(), vo.Entries, 1)
	assert.Equal(suite.T(), "vol-1", vo.Entries[0].VolumeID)
	assert.Equal(suite.T(), "Persepolis", vo.Entries[0].VolumeTitle)
}

func TestLibraryTestSuite(t *testing.T) {
	suite.Run(t, new(LibraryTestSuite))
}
