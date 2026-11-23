package data

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-objects.go/models"
	"github.com/sweetrpg/game-room-objects.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
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
	if result.ModifiedCount > 0 {
		releaseWishlistClaims(c, id, ownerUserID)
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

// WishlistConflictError is returned by AddWishlistEntry when the volume is already on another of
// the same user's wishlists. It names that wishlist so the caller can offer to move the volume.
type WishlistConflictError struct {
	WishlistID   string
	WishlistName string
}

func (e *WishlistConflictError) Error() string {
	return fmt.Sprintf("volume is already on wishlist %q (%s)", e.WishlistName, e.WishlistID)
}

// AddWishlistEntry links a catalog volume into a wishlist owned by ownerUserID. A no-op if the
// volume is already present. A volume may be on at most one of a user's wishlists: if it is on
// another one, the add fails with a *WishlistConflictError naming it. volumeTitle is a
// denormalized snapshot of the volume's title at add time (mirrors LibraryEntry) so entries can
// be displayed without a catalog lookup.
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
	if err := claimWishlistVolume(c, wl, volumeID); err != nil {
		return nil, err
	}
	wl.Entries = append(wl.Entries, models.WishlistEntry{VolumeID: volumeID, VolumeTitle: volumeTitle, AddedAt: time.Now()})
	if err := replaceWishlist(c, wl, actingUserID); err != nil {
		releaseWishlistVolumeClaim(c, ownerUserID, volumeID, wishlistID)
		return nil, err
	}
	return wl, nil
}

func wishlistClaimID(ownerUserID, volumeID string) string {
	return ownerUserID + ":" + volumeID
}

// claimWishlistVolume reserves volumeID for wl by inserting the (user, volume) claim document;
// the _id collision on a concurrent add is what makes the check atomic. A claim whose wishlist
// no longer holds the volume (a crash between claim and write, or a stale release) is replaced.
// Entries written before claims existed have no claim, so after winning the claim it also
// checks the user's other wishlists directly.
func claimWishlistVolume(c context.Context, wl *models.Wishlist, volumeID string) error {
	claims := database.Db.Collection(wishlistClaimCollection)
	id := wishlistClaimID(wl.UserID, volumeID)
	doc := bson.D{{Key: "_id", Value: id}, {Key: "user_id", Value: wl.UserID}, {Key: "volume_id", Value: volumeID}, {Key: "wishlist_id", Value: wl.ID}}

	for attempt := 0; attempt < 3; attempt++ {
		_, err := claims.InsertOne(c, doc)
		if err == nil {
			return checkNoOtherWishlistHolds(c, wl, volumeID)
		}
		if !mongo.IsDuplicateKeyError(err) {
			logging.Logger.Error("Error while claiming wishlist volume", "id", id, "error", err)
			return err
		}
		var held struct {
			WishlistID string `bson:"wishlist_id"`
		}
		if err := claims.FindOne(c, bson.D{{Key: "_id", Value: id}}).Decode(&held); err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				continue // released between our insert and read
			}
			return err
		}
		if held.WishlistID == wl.ID {
			return nil
		}
		holder, err := FindWishlistByVolume(c, wl.UserID, volumeID)
		if err != nil {
			return err
		}
		if holder != nil && holder.ID != wl.ID {
			return &WishlistConflictError{WishlistID: holder.ID, WishlistName: holder.Name}
		}
		// Stale claim: take it over only if it is still the one we read.
		if _, err := claims.DeleteOne(c, bson.D{{Key: "_id", Value: id}, {Key: "wishlist_id", Value: held.WishlistID}}); err != nil {
			return err
		}
	}
	return fmt.Errorf("could not claim volume %s for wishlist %s", volumeID, wl.ID)
}

func checkNoOtherWishlistHolds(c context.Context, wl *models.Wishlist, volumeID string) error {
	holder, err := FindWishlistByVolume(c, wl.UserID, volumeID)
	if err != nil {
		releaseWishlistVolumeClaim(c, wl.UserID, volumeID, wl.ID)
		return err
	}
	if holder != nil && holder.ID != wl.ID {
		releaseWishlistVolumeClaim(c, wl.UserID, volumeID, wl.ID)
		return &WishlistConflictError{WishlistID: holder.ID, WishlistName: holder.Name}
	}
	return nil
}

// releaseWishlistVolumeClaim drops the claim only if wishlistID still holds it. Best effort: a
// failed release leaves a stale claim, which claimWishlistVolume replaces on the next add.
func releaseWishlistVolumeClaim(c context.Context, ownerUserID, volumeID, wishlistID string) {
	_, err := database.Db.Collection(wishlistClaimCollection).DeleteOne(c, bson.D{
		{Key: "_id", Value: wishlistClaimID(ownerUserID, volumeID)}, {Key: "wishlist_id", Value: wishlistID}})
	if err != nil {
		logging.Logger.Warn("Error while releasing wishlist volume claim", "volumeID", volumeID, "wishlistID", wishlistID, "error", err)
	}
}

func releaseWishlistClaims(c context.Context, wishlistID, ownerUserID string) {
	_, err := database.Db.Collection(wishlistClaimCollection).DeleteMany(c, bson.D{
		{Key: "user_id", Value: ownerUserID}, {Key: "wishlist_id", Value: wishlistID}})
	if err != nil {
		logging.Logger.Warn("Error while releasing wishlist claims", "wishlistID", wishlistID, "error", err)
	}
}

// FindWishlistByVolume returns the owner's live wishlist that contains volumeID, or nil. The
// filter runs in the query, scoped to ownerUserID.
func FindWishlistByVolume(c context.Context, ownerUserID, volumeID string) (*models.Wishlist, error) {
	results, err := database.Query[models.Wishlist](wishlistCollection, live(bson.D{
		{Key: "user_id", Value: ownerUserID}, {Key: "entries.volume_id", Value: volumeID}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying wishlists by volume", "ownerUserID", ownerUserID, "volumeID", volumeID, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// MultiWishlistVolume is one (user, volume) pair that sits on more than one live wishlist.
type MultiWishlistVolume struct {
	UserID      string   `bson:"user_id"`
	VolumeID    string   `bson:"volume_id"`
	WishlistIDs []string `bson:"wishlist_ids"`
}

// ListMultiWishlistVolumes reports every (user, volume) pair already on more than one live
// wishlist, i.e. data written before the one-wishlist-per-volume rule. Read-only.
func ListMultiWishlistVolumes(c context.Context) ([]MultiWishlistVolume, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: live(bson.D{})}},
		{{Key: "$unwind", Value: "$entries"}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "user_id", Value: "$user_id"}, {Key: "volume_id", Value: "$entries.volume_id"}}},
			{Key: "wishlist_ids", Value: bson.D{{Key: "$addToSet", Value: "$_id"}}},
		}}},
		{{Key: "$match", Value: bson.D{{Key: "wishlist_ids.1", Value: bson.D{{Key: "$exists", Value: true}}}}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0}, {Key: "user_id", Value: "$_id.user_id"},
			{Key: "volume_id", Value: "$_id.volume_id"}, {Key: "wishlist_ids", Value: 1}}}},
	}
	cur, err := database.Db.Collection(wishlistCollection).Aggregate(c, pipeline)
	if err != nil {
		return nil, err
	}
	var out []MultiWishlistVolume
	if err := cur.All(c, &out); err != nil {
		return nil, err
	}
	return out, nil
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
	releaseWishlistVolumeClaim(c, ownerUserID, volumeID, wishlistID)
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
