package data

import (
	"context"
	"fmt"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-objects.go/models"
	"github.com/sweetrpg/game-room-objects.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetWishlist returns one wishlist by ID, or nil if it doesn't exist or has been soft-deleted.
// Unscoped by owner - only for read paths, which apply visibility filtering separately via
// WishlistToVO/CanView. Write paths must use getOwnedWishlist instead.
func GetWishlist(c context.Context, id string) (*models.Wishlist, error) {
	results, err := database.Query[models.Wishlist](wishlistCollection, live(bson.D{{Key: "_id", Value: id}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for wishlist", "id", id, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// getOwnedWishlist returns the wishlist only if it exists, is not soft-deleted, and is owned by
// ownerUserID, via a single query filtered on both fields. Returns (nil, nil) both when the
// wishlist doesn't exist and when it's owned by someone else - callers needing to tell those
// apart (to choose between a 403 and a 404 response) should follow up with GetWishlist.
func getOwnedWishlist(c context.Context, id, ownerUserID string) (*models.Wishlist, error) {
	results, err := database.Query[models.Wishlist](wishlistCollection, live(bson.D{{Key: "_id", Value: id}, {Key: "user_id", Value: ownerUserID}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for owned wishlist", "id", id, "ownerUserID", ownerUserID, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// ListWishlistsByUser returns every live wishlist owned by the given user.
func ListWishlistsByUser(c context.Context, userID string) ([]*models.Wishlist, error) {
	results, err := database.Query[models.Wishlist](wishlistCollection, live(bson.D{{Key: "user_id", Value: userID}}), nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying database for wishlists", "userID", userID, "error", err)
		return nil, err
	}
	return results, nil
}

// CreateWishlist creates a new named wishlist, defaulting to private visibility per the "new
// wishlists default to private" requirement. actingUserID is the caller stamped into the audit
// fields.
func CreateWishlist(c context.Context, userID, name, actingUserID string) (*models.Wishlist, error) {
	if name == "" {
		return nil, fmt.Errorf("wishlist name must not be empty")
	}

	wl := models.NewWishlist(primitive.NewObjectID().Hex(), userID, name)
	stampCreate(&wl.Auditable, actingUserID, time.Now())
	if _, err := database.Insert(wishlistCollection, wl); err != nil {
		logging.Logger.Error("Error while inserting wishlist", "userID", userID, "error", err)
		return nil, err
	}
	return &wl, nil
}

// DeleteWishlist soft-deletes a wishlist owned by ownerUserID: it sets deleted_at/deleted_by via
// $set rather than removing the document (its entries go with it). Returns whether a live
// document was actually marked, so callers can tell "not found" (or already deleted) apart from
// "found but not owned by ownerUserID".
func DeleteWishlist(c context.Context, id, ownerUserID, actingUserID string) (bool, error) {
	result, err := database.Db.Collection(wishlistCollection).UpdateOne(c,
		live(bson.D{{Key: "_id", Value: id}, {Key: "user_id", Value: ownerUserID}}),
		softDeleteSet(actingUserID, time.Now()))
	if err != nil {
		logging.Logger.Error("Error while deleting wishlist", "id", id, "error", err)
		return false, err
	}
	return result.ModifiedCount > 0, nil
}

func replaceWishlist(c context.Context, wl *models.Wishlist, actingUserID string) error {
	stampUpdate(&wl.Auditable, actingUserID, time.Now())
	_, err := database.Db.Collection(wishlistCollection).ReplaceOne(c, live(bson.D{{Key: "_id", Value: wl.ID}, {Key: "user_id", Value: wl.UserID}}), wl)
	if err != nil {
		logging.Logger.Error("Error while replacing wishlist", "id", wl.ID, "error", err)
	}
	return err
}

// AddWishlistEntry links a catalog volume into a wishlist owned by ownerUserID. A no-op if the
// volume is already present. volumeTitle is a denormalized snapshot of the volume's title at add
// time (mirrors LibraryEntry) so entries can be displayed without a catalog lookup.
func AddWishlistEntry(c context.Context, wishlistID, ownerUserID, volumeID, volumeTitle, actingUserID string) (*models.Wishlist, error) {
	wl, err := getOwnedWishlist(c, wishlistID, ownerUserID)
	if err != nil || wl == nil {
		return wl, err
	}
	for _, e := range wl.Entries {
		if e.VolumeID == volumeID {
			return wl, nil
		}
	}
	wl.Entries = append(wl.Entries, models.WishlistEntry{VolumeID: volumeID, VolumeTitle: volumeTitle, AddedAt: time.Now()})
	if err := replaceWishlist(c, wl, actingUserID); err != nil {
		return nil, err
	}
	return wl, nil
}

// UpdateWishlistEntryTitleByVolume refreshes the denormalized volume-title snapshot on every
// wishlist entry across all users that references volumeID, and returns the IDs of the users
// whose wishlists actually changed. Like the library equivalent this is driven by a trusted
// catalog volume.updated event, not a user request, so it deliberately spans all owners.
// Idempotent: a replay with an unchanged title matches no entries and returns an empty slice.
func UpdateWishlistEntryTitleByVolume(c context.Context, volumeID, volumeTitle string) ([]string, error) {
	stale, err := database.Query[models.Wishlist](wishlistCollection, live(bson.D{
		{Key: "entries", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
			{Key: "volume_id", Value: volumeID},
			{Key: "volume_title", Value: bson.D{{Key: "$ne", Value: volumeTitle}}},
		}}}},
	}), nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying wishlists for volume title sync", "volumeID", volumeID, "error", err)
		return nil, err
	}
	if len(stale) == 0 {
		return []string{}, nil
	}

	// ponytail: whole-document replace per affected wishlist, mirroring
	// UpdateLibraryEntryTitleByVolume. This is also the read-your-write-safe choice for an
	// embedded-array field update - a positional-array $set through the Query wrapper has
	// returned stale reads before (see game-room platform notes).
	userIDs := make([]string, 0, len(stale))
	for _, wl := range stale {
		for i := range wl.Entries {
			if wl.Entries[i].VolumeID == volumeID {
				wl.Entries[i].VolumeTitle = volumeTitle
			}
		}
		if err := replaceWishlist(c, wl, SystemActor); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, wl.UserID)
	}
	return userIDs, nil
}

// RemoveWishlistEntry unlinks a catalog volume from a wishlist owned by ownerUserID.
func RemoveWishlistEntry(c context.Context, wishlistID, ownerUserID, volumeID, actingUserID string) (*models.Wishlist, error) {
	wl, err := getOwnedWishlist(c, wishlistID, ownerUserID)
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
	if err := replaceWishlist(c, wl, actingUserID); err != nil {
		return nil, err
	}
	return wl, nil
}

// SetWishlistVisibility updates the visibility of a wishlist owned by ownerUserID.
func SetWishlistVisibility(c context.Context, wishlistID, ownerUserID string, visibility models.Visibility, actingUserID string) (*models.Wishlist, error) {
	wl, err := getOwnedWishlist(c, wishlistID, ownerUserID)
	if err != nil || wl == nil {
		return wl, err
	}
	wl.Visibility = visibility
	if err := replaceWishlist(c, wl, actingUserID); err != nil {
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
		entries = append(entries, vo.WishlistEntryVO{VolumeID: e.VolumeID, VolumeTitle: e.VolumeTitle, AddedAt: e.AddedAt})
	}

	return &vo.WishlistVO{
		ID:          wl.ID,
		UserID:      wl.UserID,
		Name:        wl.Name,
		Visibility:  string(wl.Visibility),
		Entries:     entries,
		AuditableVO: auditVO(wl.Auditable),
	}
}
