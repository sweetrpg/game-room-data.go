package data

import (
	"context"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-objects.go/models"
	"github.com/sweetrpg/game-room-objects.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetWishlistByUser returns the given user's wishlist, or nil if they don't have one yet.
func GetWishlistByUser(c context.Context, userID string) (*models.Wishlist, error) {
	results, err := database.Query[models.Wishlist](wishlistCollection, bson.D{{Key: "user_id", Value: userID}}, nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for wishlist", "userID", userID, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// getOrCreateWishlist returns the user's wishlist, creating one (defaulting to private) if none
// exists yet, per the "new wishlists default to private" requirement.
func getOrCreateWishlist(c context.Context, userID string) (*models.Wishlist, error) {
	wl, err := GetWishlistByUser(c, userID)
	if err != nil {
		return nil, err
	}
	if wl != nil {
		return wl, nil
	}

	newWl := models.NewWishlist(primitive.NewObjectID().Hex(), userID)
	newWl.CreatedAt = time.Now()
	newWl.CreatedBy = userID
	if _, err := database.Insert(wishlistCollection, newWl); err != nil {
		logging.Logger.Error("Error while inserting wishlist", "userID", userID, "error", err)
		return nil, err
	}
	return &newWl, nil
}

func replaceWishlist(c context.Context, wl *models.Wishlist) error {
	wl.UpdatedAt = time.Now()
	_, err := database.Db.Collection(wishlistCollection).ReplaceOne(c, bson.D{{Key: "_id", Value: wl.ID}}, wl)
	if err != nil {
		logging.Logger.Error("Error while replacing wishlist", "id", wl.ID, "error", err)
	}
	return err
}

// AddWishlistEntry links a catalog volume into the user's wishlist, creating the wishlist if it
// doesn't exist yet. A no-op if the volume is already present.
func AddWishlistEntry(c context.Context, userID, volumeID string) (*models.Wishlist, error) {
	wl, err := getOrCreateWishlist(c, userID)
	if err != nil {
		return nil, err
	}
	for _, e := range wl.Entries {
		if e.VolumeID == volumeID {
			return wl, nil
		}
	}
	wl.Entries = append(wl.Entries, models.WishlistEntry{VolumeID: volumeID})
	if err := replaceWishlist(c, wl); err != nil {
		return nil, err
	}
	return wl, nil
}

// RemoveWishlistEntry unlinks a catalog volume from the user's wishlist.
func RemoveWishlistEntry(c context.Context, userID, volumeID string) (*models.Wishlist, error) {
	wl, err := GetWishlistByUser(c, userID)
	if err != nil || wl == nil {
		return wl, err
	}
	kept := wl.Entries[:0]
	for _, e := range wl.Entries {
		if e.VolumeID != volumeID {
			kept = append(kept, e)
		}
	}
	wl.Entries = kept
	if err := replaceWishlist(c, wl); err != nil {
		return nil, err
	}
	return wl, nil
}

// SetWishlistVisibility updates the wishlist's visibility, creating the wishlist (private
// default) first if the user doesn't have one yet.
func SetWishlistVisibility(c context.Context, userID string, visibility models.Visibility) (*models.Wishlist, error) {
	wl, err := getOrCreateWishlist(c, userID)
	if err != nil {
		return nil, err
	}
	wl.Visibility = visibility
	if err := replaceWishlist(c, wl); err != nil {
		return nil, err
	}
	return wl, nil
}

// WishlistToVO converts a wishlist model into its VO, or nil if viewerID may not see it at all.
func WishlistToVO(wl *models.Wishlist, viewerID string, isFriend, isFriendOfFriend bool) *vo.WishlistVO {
	if !CanView(wl.Visibility, wl.UserID, viewerID, isFriend, isFriendOfFriend) {
		return nil
	}

	entries := make([]vo.WishlistEntryVO, 0, len(wl.Entries))
	for _, e := range wl.Entries {
		entries = append(entries, vo.WishlistEntryVO{VolumeID: e.VolumeID})
	}

	return &vo.WishlistVO{
		ID:         wl.ID,
		UserID:     wl.UserID,
		Visibility: string(wl.Visibility),
		Entries:    entries,
	}
}
