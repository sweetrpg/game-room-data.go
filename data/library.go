package data

import (
	"context"
	"errors"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-objects.go/models"
	"github.com/sweetrpg/game-room-objects.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GetLibraryByUser returns the given user's library, or nil if they don't have one yet or it has
// been soft-deleted.
func GetLibraryByUser(c context.Context, userID string) (*models.Library, error) {
	results, err := database.Query[models.Library](libraryCollection, live(bson.D{{Key: "user_id", Value: userID}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for library", "userID", userID, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// getOrCreateLibrary returns the user's library, creating one (defaulting to private) if none
// exists yet, per the "new library defaults to private" requirement. actingUserID is stamped
// into the audit fields of a newly created library.
func getOrCreateLibrary(c context.Context, userID, actingUserID string) (*models.Library, error) {
	lib, err := GetLibraryByUser(c, userID)
	if err != nil {
		return nil, err
	}
	if lib != nil {
		return lib, nil
	}

	newLib := models.NewLibrary(primitive.NewObjectID().Hex(), userID)
	stampCreate(&newLib.Auditable, actingUserID, time.Now())
	if _, err := database.Insert(libraryCollection, newLib); err != nil {
		logging.Logger.Error("Error while inserting library", "userID", userID, "error", err)
		return nil, err
	}
	return &newLib, nil
}

func replaceLibrary(c context.Context, lib *models.Library, actingUserID string) error {
	stampUpdate(&lib.Auditable, actingUserID, time.Now())
	_, err := database.Db.Collection(libraryCollection).ReplaceOne(c, live(bson.D{{Key: "_id", Value: lib.ID}}), lib)
	if err != nil {
		logging.Logger.Error("Error while replacing library", "id", lib.ID, "error", err)
	}
	return err
}

// AddLibraryEntry links a catalog volume into the user's library, creating the library if it
// doesn't exist yet. A no-op if the volume is already present. volumeTitle is a denormalized
// snapshot of the volume's title at add time so entries can be displayed without a catalog
// lookup.
func AddLibraryEntry(c context.Context, userID, volumeID, volumeTitle, actingUserID string) (*models.Library, error) {
	lib, err := getOrCreateLibrary(c, userID, actingUserID)
	if err != nil {
		return nil, err
	}
	for _, e := range lib.Entries {
		if e.VolumeID == volumeID {
			return lib, nil
		}
	}
	lib.Entries = append(lib.Entries, models.LibraryEntry{VolumeID: volumeID, AddedAt: time.Now(), VolumeTitle: volumeTitle})
	if err := replaceLibrary(c, lib, actingUserID); err != nil {
		return nil, err
	}
	return lib, nil
}

// RemoveLibraryEntry unlinks a catalog volume from the user's library.
func RemoveLibraryEntry(c context.Context, userID, volumeID, actingUserID string) (*models.Library, error) {
	lib, err := GetLibraryByUser(c, userID)
	if err != nil || lib == nil {
		return lib, err
	}
	kept := lib.Entries[:0]
	for _, e := range lib.Entries {
		if e.VolumeID != volumeID {
			kept = append(kept, e)
		}
	}
	lib.Entries = kept
	if err := replaceLibrary(c, lib, actingUserID); err != nil {
		return nil, err
	}
	return lib, nil
}

// SetLibraryDefaultVisibility updates the library's default visibility, creating the library
// (private default) first if the user doesn't have one yet.
func SetLibraryDefaultVisibility(c context.Context, userID string, newDefault models.Visibility, actingUserID string) (*models.Library, error) {
	lib, err := getOrCreateLibrary(c, userID, actingUserID)
	if err != nil {
		return nil, err
	}
	lib.DefaultVisibility = newDefault
	if err := replaceLibrary(c, lib, actingUserID); err != nil {
		return nil, err
	}
	return lib, nil
}

// SetLibraryEntryVisibilityOverride sets or clears (override == nil) one entry's per-entry
// visibility override.
func SetLibraryEntryVisibilityOverride(c context.Context, userID, volumeID string, override *models.Visibility, actingUserID string) (*models.Library, error) {
	lib, err := GetLibraryByUser(c, userID)
	if err != nil || lib == nil {
		return lib, err
	}
	for i, e := range lib.Entries {
		if e.VolumeID == volumeID {
			lib.Entries[i].VisibilityOverride = override
			break
		}
	}
	if err := replaceLibrary(c, lib, actingUserID); err != nil {
		return nil, err
	}
	return lib, nil
}

// ErrLibraryEntryNotFound is returned when an update targets an entry that isn't in the library.
var ErrLibraryEntryNotFound = errors.New("library entry not found")

// UpdateLibraryEntryTitle refreshes the denormalized title snapshot on a single library entry.
func UpdateLibraryEntryTitle(c context.Context, userID, volumeID, volumeTitle, actingUserID string) (*models.Library, error) {
	lib, err := GetLibraryByUser(c, userID)
	if err != nil || lib == nil {
		return lib, err
	}
	for i, e := range lib.Entries {
		if e.VolumeID == volumeID {
			lib.Entries[i].VolumeTitle = volumeTitle
			if err := replaceLibrary(c, lib, actingUserID); err != nil {
				return nil, err
			}
			return lib, nil
		}
	}
	return nil, ErrLibraryEntryNotFound
}

// UpdateLibraryEntryTitleByVolume refreshes the denormalized volume-title snapshot on every
// library entry across all users that references volumeID, in one bulk update, and returns the
// IDs of the users whose libraries actually changed. This is driven by a trusted catalog
// volume.updated event, not a user request, so it deliberately spans all owners - owner-scoping
// applies to user-initiated access, not to this event-driven denormalization refresh. Idempotent:
// a replay with an unchanged title matches no entries and returns an empty slice.
func UpdateLibraryEntryTitleByVolume(c context.Context, volumeID, volumeTitle string) ([]string, error) {
	stale, err := database.Query[models.Library](libraryCollection, bson.D{
		{Key: "entries", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
			{Key: "volume_id", Value: volumeID},
			{Key: "volume_title", Value: bson.D{{Key: "$ne", Value: volumeTitle}}},
		}}}},
	}, nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying libraries for volume title sync", "volumeID", volumeID, "error", err)
		return nil, err
	}
	if len(stale) == 0 {
		return []string{}, nil
	}

	userIDs := make([]string, 0, len(stale))
	for _, lib := range stale {
		userIDs = append(userIDs, lib.UserID)
	}

	_, err = database.Db.Collection(libraryCollection).UpdateMany(c,
		bson.D{{Key: "entries.volume_id", Value: volumeID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "entries.$[e].volume_title", Value: volumeTitle},
			{Key: "updated_at", Value: time.Now()},
		}}},
		options.Update().SetArrayFilters(options.ArrayFilters{
			Filters: []interface{}{bson.D{{Key: "e.volume_id", Value: volumeID}}},
		}),
	)
	if err != nil {
		logging.Logger.Error("Error while bulk-updating library entry titles", "volumeID", volumeID, "error", err)
		return nil, err
	}
	return userIDs, nil
}

// LibraryToVO converts a library model, filtered to what viewerID may see, into its VO. ownerID
// is the library's own user ID; isFriend/isFriendOfFriend describe the viewer's relationship to
// the owner (always false today - see CanView).
func LibraryToVO(lib *models.Library, viewerID string, isFriend, isFriendOfFriend bool) *vo.LibraryVO {
	entries := make([]vo.LibraryEntryVO, 0, len(lib.Entries))
	for _, e := range lib.Entries {
		effective := e.EffectiveVisibility(lib.DefaultVisibility)
		if !CanView(effective, lib.UserID, viewerID, isFriend, isFriendOfFriend) {
			continue
		}
		var override *string
		if e.VisibilityOverride != nil {
			s := string(*e.VisibilityOverride)
			override = &s
		}
		entries = append(entries, vo.LibraryEntryVO{VolumeID: e.VolumeID, VisibilityOverride: override, AddedAt: e.AddedAt, VolumeTitle: e.VolumeTitle})
	}

	return &vo.LibraryVO{
		ID:                lib.ID,
		UserID:            lib.UserID,
		DefaultVisibility: string(lib.DefaultVisibility),
		Entries:           entries,
		AuditableVO:       auditVO(lib.Auditable),
	}
}
