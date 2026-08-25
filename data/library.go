package data

import (
	"context"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"github.com/sweetrpg/shelf-objects.go/models"
	"github.com/sweetrpg/shelf-objects.go/vo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetLibraryByUser returns the given user's library, or nil if they don't have one yet.
func GetLibraryByUser(c context.Context, userID string) (*models.Library, error) {
	results, err := database.Query[models.Library](libraryCollection, bson.D{{Key: "user_id", Value: userID}}, nil, nil, 0, 1)
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
// exists yet, per the "new library defaults to private" requirement.
func getOrCreateLibrary(c context.Context, userID string) (*models.Library, error) {
	lib, err := GetLibraryByUser(c, userID)
	if err != nil {
		return nil, err
	}
	if lib != nil {
		return lib, nil
	}

	newLib := models.NewLibrary(primitive.NewObjectID().Hex(), userID)
	newLib.Auditable.CreatedAt = time.Now()
	newLib.Auditable.CreatedBy = userID
	if _, err := database.Insert(libraryCollection, newLib); err != nil {
		logging.Logger.Error("Error while inserting library", "userID", userID, "error", err)
		return nil, err
	}
	return &newLib, nil
}

func replaceLibrary(c context.Context, lib *models.Library) error {
	lib.Auditable.UpdatedAt = time.Now()
	_, err := database.Db.Collection(libraryCollection).ReplaceOne(c, bson.D{{Key: "_id", Value: lib.ID}}, lib)
	if err != nil {
		logging.Logger.Error("Error while replacing library", "id", lib.ID, "error", err)
	}
	return err
}

// AddLibraryEntry links a catalog volume into the user's library, creating the library if it
// doesn't exist yet. A no-op if the volume is already present.
func AddLibraryEntry(c context.Context, userID, volumeID string) (*models.Library, error) {
	lib, err := getOrCreateLibrary(c, userID)
	if err != nil {
		return nil, err
	}
	for _, e := range lib.Entries {
		if e.VolumeID == volumeID {
			return lib, nil
		}
	}
	lib.Entries = append(lib.Entries, models.LibraryEntry{VolumeID: volumeID})
	if err := replaceLibrary(c, lib); err != nil {
		return nil, err
	}
	return lib, nil
}

// RemoveLibraryEntry unlinks a catalog volume from the user's library.
func RemoveLibraryEntry(c context.Context, userID, volumeID string) (*models.Library, error) {
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
	if err := replaceLibrary(c, lib); err != nil {
		return nil, err
	}
	return lib, nil
}

// SetLibraryDefaultVisibility updates the library's default visibility, creating the library
// (private default) first if the user doesn't have one yet.
func SetLibraryDefaultVisibility(c context.Context, userID string, newDefault models.Visibility) (*models.Library, error) {
	lib, err := getOrCreateLibrary(c, userID)
	if err != nil {
		return nil, err
	}
	lib.DefaultVisibility = newDefault
	if err := replaceLibrary(c, lib); err != nil {
		return nil, err
	}
	return lib, nil
}

// SetLibraryEntryVisibilityOverride sets or clears (override == nil) one entry's per-entry
// visibility override.
func SetLibraryEntryVisibilityOverride(c context.Context, userID, volumeID string, override *models.Visibility) (*models.Library, error) {
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
	if err := replaceLibrary(c, lib); err != nil {
		return nil, err
	}
	return lib, nil
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
		entries = append(entries, vo.LibraryEntryVO{VolumeID: e.VolumeID, VisibilityOverride: override})
	}

	return &vo.LibraryVO{
		ID:                lib.ID,
		UserID:            lib.UserID,
		DefaultVisibility: string(lib.DefaultVisibility),
		Entries:           entries,
	}
}
