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

// preMigrationWishlist mirrors the pre-Name document shape (no name field at all), simulating
// a seeded pre-migration dataset.
type preMigrationWishlist struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"user_id"`
	CreatedAt time.Time `bson:"created_at"`
}

type MigrationTestSuite struct {
	suite.Suite
}

func (suite *MigrationTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
}

func (suite *MigrationTestSuite) TestMigrateWishlistNamesBackfillsUnnamed() {
	ctx := context.Background()

	unnamed := preMigrationWishlist{ID: primitive.NewObjectID().Hex(), UserID: primitive.NewObjectID().Hex(), CreatedAt: time.Now()}
	_, err := database.Insert(wishlistCollection, unnamed)
	assert.NoError(suite.T(), err)

	namer := primitive.NewObjectID().Hex()
	alreadyNamed, err := CreateWishlist(ctx, namer, "Already named", namer)
	assert.NoError(suite.T(), err)

	modified, err := MigrateWishlistNames(ctx)
	assert.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), modified, 1)

	migrated, err := GetWishlist(ctx, unnamed.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), DefaultWishlistName, migrated.Name)

	untouched, err := GetWishlist(ctx, alreadyNamed.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Already named", untouched.Name)
}

func (suite *MigrationTestSuite) TestMigrateWishlistNamesSafeToRunTwice() {
	ctx := context.Background()

	unnamed := preMigrationWishlist{ID: primitive.NewObjectID().Hex(), UserID: primitive.NewObjectID().Hex(), CreatedAt: time.Now()}
	_, err := database.Insert(wishlistCollection, unnamed)
	assert.NoError(suite.T(), err)

	_, err = MigrateWishlistNames(ctx)
	assert.NoError(suite.T(), err)

	secondPass, err := MigrateWishlistNames(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, secondPass)

	got, err := GetWishlist(ctx, unnamed.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), DefaultWishlistName, got.Name)
}

// preSidecarTable mirrors a table document written before the volume_titles sidecar existed.
type preSidecarTable struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"user_id"`
	VolumeIDs []string  `bson:"volume_ids"`
	CreatedAt time.Time `bson:"created_at"`
}

func (suite *MigrationTestSuite) TestMigrateTableVolumeTitlesBackfillsMissing() {
	ctx := context.Background()

	old := preSidecarTable{ID: primitive.NewObjectID().Hex(), UserID: primitive.NewObjectID().Hex(), VolumeIDs: []string{"vol-1"}, CreatedAt: time.Now()}
	_, err := database.Insert(tableCollection, old)
	assert.NoError(suite.T(), err)

	modified, err := MigrateTableVolumeTitles(ctx)
	assert.NoError(suite.T(), err)
	assert.GreaterOrEqual(suite.T(), modified, 1)

	got, err := GetTable(ctx, old.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), got.VolumeTitles)
	assert.Len(suite.T(), got.VolumeTitles, 0)
}

func (suite *MigrationTestSuite) TestMigrateTableVolumeTitlesSafeToRunTwice() {
	ctx := context.Background()

	old := preSidecarTable{ID: primitive.NewObjectID().Hex(), UserID: primitive.NewObjectID().Hex(), VolumeIDs: []string{"vol-1"}, CreatedAt: time.Now()}
	_, err := database.Insert(tableCollection, old)
	assert.NoError(suite.T(), err)

	_, err = MigrateTableVolumeTitles(ctx)
	assert.NoError(suite.T(), err)

	secondPass, err := MigrateTableVolumeTitles(ctx)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), 0, secondPass)
}

func TestMigrationTestSuite(t *testing.T) {
	suite.Run(t, new(MigrationTestSuite))
}
