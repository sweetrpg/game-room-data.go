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
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type WishlistTestSuite struct {
	suite.Suite
}

func (suite *WishlistTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
}

func (suite *WishlistTestSuite) TestListWishlistsByUser() {
	userID := primitive.NewObjectID().Hex()
	ctx := context.Background()

	_, err := CreateWishlist(ctx, userID, "Birthday")
	assert.NoError(suite.T(), err)
	_, err = CreateWishlist(ctx, userID, "Con haul")
	assert.NoError(suite.T(), err)
	// noise: a different user's wishlist must not show up
	_, err = CreateWishlist(ctx, primitive.NewObjectID().Hex(), "Someone else's")
	assert.NoError(suite.T(), err)

	wls, err := ListWishlistsByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), wls, 2)
}

func (suite *WishlistTestSuite) TestCreateWishlistRejectsEmptyName() {
	ctx := context.Background()
	wl, err := CreateWishlist(ctx, primitive.NewObjectID().Hex(), "")
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), wl)
}

func (suite *WishlistTestSuite) TestCreateWishlistPersists() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, userID, "Someday")
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), wl)
	assert.Equal(suite.T(), "Someday", wl.Name)
	assert.Equal(suite.T(), models.VisibilityPrivate, wl.Visibility)

	got, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), got)
	assert.Equal(suite.T(), "Someday", got.Name)
}

func (suite *WishlistTestSuite) TestDeleteWishlistRemovesOnlyTargeted() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	keep, err := CreateWishlist(ctx, userID, "Keep me")
	assert.NoError(suite.T(), err)
	remove, err := CreateWishlist(ctx, userID, "Remove me")
	assert.NoError(suite.T(), err)

	deleted, err := DeleteWishlist(ctx, remove.ID, userID)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), deleted)

	got, err := GetWishlist(ctx, remove.ID)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)

	stillThere, err := GetWishlist(ctx, keep.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), stillThere)
}

func (suite *WishlistTestSuite) TestDeleteWishlistRejectsNonOwner() {
	ctx := context.Background()
	owner := primitive.NewObjectID().Hex()
	intruder := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, owner, "Mine")
	assert.NoError(suite.T(), err)

	deleted, err := DeleteWishlist(ctx, wl.ID, intruder)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), deleted)

	stillThere, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), stillThere)
}

func (suite *WishlistTestSuite) TestEntryAndVisibilityScopedToWishlistID() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	a, err := CreateWishlist(ctx, userID, "A")
	assert.NoError(suite.T(), err)
	b, err := CreateWishlist(ctx, userID, "B")
	assert.NoError(suite.T(), err)

	updated, err := AddWishlistEntry(ctx, a.ID, userID, "vol-1")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), updated.Entries, 1)

	untouched, err := GetWishlist(ctx, b.ID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), untouched.Entries, 0)

	updated, err = SetWishlistVisibility(ctx, a.ID, userID, models.VisibilityPublic)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VisibilityPublic, updated.Visibility)

	untouched, err = GetWishlist(ctx, b.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VisibilityPrivate, untouched.Visibility)

	updated, err = RemoveWishlistEntry(ctx, a.ID, userID, "vol-1")
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), updated.Entries, 0)
}

func (suite *WishlistTestSuite) TestEntryMutatorsRejectNonOwner() {
	ctx := context.Background()
	owner := primitive.NewObjectID().Hex()
	intruder := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, owner, "Mine")
	assert.NoError(suite.T(), err)

	got, err := AddWishlistEntry(ctx, wl.ID, intruder, "vol-1")
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)

	got, err = SetWishlistVisibility(ctx, wl.ID, intruder, models.VisibilityPublic)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)

	untouched, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VisibilityPrivate, untouched.Visibility)
	assert.Len(suite.T(), untouched.Entries, 0)
}

func TestWishlistTestSuite(t *testing.T) {
	suite.Run(t, new(WishlistTestSuite))
}
