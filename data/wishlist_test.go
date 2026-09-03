package data

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-objects.go/models"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
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

	_, err := CreateWishlist(ctx, userID, "Birthday", userID)
	assert.NoError(suite.T(), err)
	_, err = CreateWishlist(ctx, userID, "Con haul", userID)
	assert.NoError(suite.T(), err)
	// noise: a different user's wishlist must not show up
	other := primitive.NewObjectID().Hex()
	_, err = CreateWishlist(ctx, other, "Someone else's", other)
	assert.NoError(suite.T(), err)

	wls, err := ListWishlistsByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), wls, 2)
}

func (suite *WishlistTestSuite) TestCreateWishlistRejectsEmptyName() {
	ctx := context.Background()
	u := primitive.NewObjectID().Hex()
	wl, err := CreateWishlist(ctx, u, "", u)
	assert.Error(suite.T(), err)
	assert.Nil(suite.T(), wl)
}

func (suite *WishlistTestSuite) TestCreateWishlistPersists() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, userID, "Someday", userID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), wl)
	assert.Equal(suite.T(), "Someday", wl.Name)
	assert.Equal(suite.T(), models.VisibilityPrivate, wl.Visibility)

	// create stamps all four audit fields; created_at == updated_at on a fresh record
	assert.False(suite.T(), wl.CreatedAt.IsZero())
	assert.Equal(suite.T(), userID, wl.CreatedBy)
	assert.Equal(suite.T(), userID, wl.UpdatedBy)
	assert.Equal(suite.T(), wl.CreatedAt, wl.UpdatedAt)
	assert.Nil(suite.T(), wl.DeletedAt)

	got, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), got)
	assert.Equal(suite.T(), "Someday", got.Name)
	assert.False(suite.T(), got.CreatedAt.IsZero())
	assert.Equal(suite.T(), userID, got.CreatedBy)
}

func (suite *WishlistTestSuite) TestUpdateWishlistAdvancesUpdateStampOnly() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, userID, "Someday", userID)
	assert.NoError(suite.T(), err)

	// baseline from the persisted record (post DB round-trip: UTC, millisecond precision)
	base, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)

	editor := primitive.NewObjectID().Hex()
	time.Sleep(5 * time.Millisecond)
	updated, err := SetWishlistVisibility(ctx, wl.ID, userID, models.VisibilityPublic, editor)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), base.CreatedAt.Equal(updated.CreatedAt), "created_at unchanged by update")
	assert.Equal(suite.T(), userID, updated.CreatedBy)
	assert.Equal(suite.T(), editor, updated.UpdatedBy)
	assert.True(suite.T(), updated.UpdatedAt.After(base.UpdatedAt), "updated_at advanced")
}

func (suite *WishlistTestSuite) TestDeleteWishlistIsSoftAndTargeted() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	keep, err := CreateWishlist(ctx, userID, "Keep me", userID)
	assert.NoError(suite.T(), err)
	remove, err := CreateWishlist(ctx, userID, "Remove me", userID)
	assert.NoError(suite.T(), err)

	deleted, err := DeleteWishlist(ctx, remove.ID, userID, userID)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), deleted)

	// absent from every read path
	got, err := GetWishlist(ctx, remove.ID)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)
	list, err := ListWishlistsByUser(ctx, userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), list, 1)
	assert.Equal(suite.T(), keep.ID, list[0].ID)

	// but the document still exists, with the delete stamp set
	var raw bson.M
	err = database.Db.Collection(wishlistCollection).FindOne(ctx, bson.D{{Key: "_id", Value: remove.ID}}).Decode(&raw)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), raw["deleted_at"])
	assert.Equal(suite.T(), userID, raw["deleted_by"])

	// re-deleting a soft-deleted wishlist reports nothing changed
	again, err := DeleteWishlist(ctx, remove.ID, userID, userID)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), again)
}

func (suite *WishlistTestSuite) TestDeleteWishlistRejectsNonOwner() {
	ctx := context.Background()
	owner := primitive.NewObjectID().Hex()
	intruder := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, owner, "Mine", owner)
	assert.NoError(suite.T(), err)

	deleted, err := DeleteWishlist(ctx, wl.ID, intruder, intruder)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), deleted)

	stillThere, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), stillThere)
}

func (suite *WishlistTestSuite) TestEntryAndVisibilityScopedToWishlistID() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	a, err := CreateWishlist(ctx, userID, "A", userID)
	assert.NoError(suite.T(), err)
	b, err := CreateWishlist(ctx, userID, "B", userID)
	assert.NoError(suite.T(), err)

	updated, err := AddWishlistEntry(ctx, a.ID, userID, "vol-1", "", userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), updated.Entries, 1)

	untouched, err := GetWishlist(ctx, b.ID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), untouched.Entries, 0)

	updated, err = SetWishlistVisibility(ctx, a.ID, userID, models.VisibilityPublic, userID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VisibilityPublic, updated.Visibility)

	untouched, err = GetWishlist(ctx, b.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VisibilityPrivate, untouched.Visibility)

	updated, err = RemoveWishlistEntry(ctx, a.ID, userID, "vol-1", userID)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), updated.Entries, 0)
}

func (suite *WishlistTestSuite) TestEntryMutatorsRejectNonOwner() {
	ctx := context.Background()
	owner := primitive.NewObjectID().Hex()
	intruder := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, owner, "Mine", owner)
	assert.NoError(suite.T(), err)

	got, err := AddWishlistEntry(ctx, wl.ID, intruder, "vol-1", "", intruder)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)

	got, err = SetWishlistVisibility(ctx, wl.ID, intruder, models.VisibilityPublic, intruder)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)

	untouched, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), models.VisibilityPrivate, untouched.Visibility)
	assert.Len(suite.T(), untouched.Entries, 0)
}

func (suite *WishlistTestSuite) TestWishlistToVOCarriesAuditFields() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, userID, "VO check", userID)
	assert.NoError(suite.T(), err)

	got, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	vo := WishlistToVO(got, userID, false, false)
	assert.NotNil(suite.T(), vo)
	assert.Equal(suite.T(), got.CreatedAt, vo.CreatedAt)
	assert.Equal(suite.T(), got.UpdatedAt, vo.UpdatedAt)
	assert.Equal(suite.T(), userID, vo.CreatedBy)
	assert.Equal(suite.T(), userID, vo.UpdatedBy)
}

func (suite *WishlistTestSuite) TestAddWishlistEntryStoresVolumeTitle() {
	ctx := context.Background()
	userID := primitive.NewObjectID().Hex()

	wl, err := CreateWishlist(ctx, userID, "Titled", userID)
	assert.NoError(suite.T(), err)

	updated, err := AddWishlistEntry(ctx, wl.ID, userID, "vol-1", "Pathfinder Core", userID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Pathfinder Core", updated.Entries[0].VolumeTitle)

	read, err := GetWishlist(ctx, wl.ID)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Pathfinder Core", read.Entries[0].VolumeTitle)

	vo := WishlistToVO(read, userID, false, false)
	assert.Equal(suite.T(), "Pathfinder Core", vo.Entries[0].VolumeTitle)
}

func (suite *WishlistTestSuite) TestUpdateWishlistEntryTitleByVolume() {
	ctx := context.Background()
	target := "vol-shared"
	other := "vol-other"

	changed := []string{primitive.NewObjectID().Hex(), primitive.NewObjectID().Hex()}
	for _, uid := range changed {
		wl, err := CreateWishlist(ctx, uid, "L", uid)
		assert.NoError(suite.T(), err)
		_, err = AddWishlistEntry(ctx, wl.ID, uid, target, "Old Title", uid)
		assert.NoError(suite.T(), err)
	}
	untouched := primitive.NewObjectID().Hex()
	uwl, err := CreateWishlist(ctx, untouched, "K", untouched)
	assert.NoError(suite.T(), err)
	_, err = AddWishlistEntry(ctx, uwl.ID, untouched, other, "Keep Me", untouched)
	assert.NoError(suite.T(), err)

	got, err := UpdateWishlistEntryTitleByVolume(ctx, target, "New Title")
	assert.NoError(suite.T(), err)
	assert.ElementsMatch(suite.T(), changed, got)

	// read-your-write: the refreshed title is visible through the same Query wrapper the
	// consumer's cache invalidation reads back with.
	for _, uid := range changed {
		wls, err := ListWishlistsByUser(ctx, uid)
		assert.NoError(suite.T(), err)
		assert.Equal(suite.T(), "New Title", wls[0].Entries[0].VolumeTitle)
		assert.Equal(suite.T(), SystemActor, wls[0].UpdatedBy)
	}
	uwls, err := ListWishlistsByUser(ctx, untouched)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Keep Me", uwls[0].Entries[0].VolumeTitle)

	again, err := UpdateWishlistEntryTitleByVolume(ctx, target, "New Title")
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), again)
}

func (suite *WishlistTestSuite) TestUpdateWishlistEntryTitleByVolumeNoReferences() {
	ctx := context.Background()

	got, err := UpdateWishlistEntryTitleByVolume(ctx, "vol-nobody-has", "Whatever")
	assert.NoError(suite.T(), err)
	assert.Empty(suite.T(), got)
}

func TestWishlistTestSuite(t *testing.T) {
	suite.Run(t, new(WishlistTestSuite))
}
