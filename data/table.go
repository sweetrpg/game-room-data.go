package data

import (
	"context"
	"strings"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-objects.go/models"
	"github.com/sweetrpg/game-room-objects.go/vo"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// GetTable returns one table by ID, or nil if it doesn't exist or has been soft-deleted.
// Unscoped by owner - only for read paths, which apply visibility filtering separately via
// TableToVO/CanView. Write paths must use getOwnedTable instead.
func GetTable(c context.Context, id string) (*models.Table, error) {
	results, err := database.Query[models.Table](tableCollection, live(bson.D{{Key: "_id", Value: id}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for table", "id", id, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// getOwnedTable returns the table only if it exists, is not soft-deleted, and is owned by
// ownerUserID, via a single query filtered on both fields. Returns (nil, nil) both when the table
// doesn't exist and when it's owned by someone else - callers needing to tell those apart (to
// choose between a 403 and a 404 response) should follow up with GetTable.
func getOwnedTable(c context.Context, id, ownerUserID string) (*models.Table, error) {
	results, err := database.Query[models.Table](tableCollection, live(bson.D{{Key: "_id", Value: id}, {Key: "user_id", Value: ownerUserID}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for owned table", "id", id, "ownerUserID", ownerUserID, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// ListTablesByUser returns every live table owned by the given user.
func ListTablesByUser(c context.Context, userID string) ([]*models.Table, error) {
	results, err := database.Query[models.Table](tableCollection, live(bson.D{{Key: "user_id", Value: userID}}), nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying database for tables", "userID", userID, "error", err)
		return nil, err
	}
	return results, nil
}

// CreateTable creates a new table, defaulting to private visibility per the "new tables default
// to private" requirement. actingUserID is the caller stamped into the audit fields.
func CreateTable(c context.Context, userID, name, actingUserID string) (*models.Table, error) {
	tbl := models.NewTable(primitive.NewObjectID().Hex(), userID, name)
	stampCreate(&tbl.Auditable, actingUserID, time.Now())
	if _, err := database.Insert(tableCollection, tbl); err != nil {
		logging.Logger.Error("Error while inserting table", "userID", userID, "error", err)
		return nil, err
	}
	return &tbl, nil
}

func replaceTable(c context.Context, tbl *models.Table, actingUserID string) error {
	stampUpdate(&tbl.Auditable, actingUserID, time.Now())
	_, err := database.Db.Collection(tableCollection).ReplaceOne(c, live(bson.D{{Key: "_id", Value: tbl.ID}, {Key: "user_id", Value: tbl.UserID}}), tbl)
	if err != nil {
		logging.Logger.Error("Error while replacing table", "id", tbl.ID, "error", err)
	}
	return err
}

// UpdateTableName renames a table owned by ownerUserID.
func UpdateTableName(c context.Context, id, ownerUserID, name, actingUserID string) (*models.Table, error) {
	tbl, err := getOwnedTable(c, id, ownerUserID)
	if err != nil || tbl == nil {
		return tbl, err
	}
	tbl.Name = name
	if err := replaceTable(c, tbl, actingUserID); err != nil {
		return nil, err
	}
	return tbl, nil
}

// SetTableVisibility updates the visibility of a table owned by ownerUserID.
func SetTableVisibility(c context.Context, id, ownerUserID string, visibility models.Visibility, actingUserID string) (*models.Table, error) {
	tbl, err := getOwnedTable(c, id, ownerUserID)
	if err != nil || tbl == nil {
		return tbl, err
	}
	tbl.Visibility = visibility
	if err := replaceTable(c, tbl, actingUserID); err != nil {
		return nil, err
	}
	return tbl, nil
}

// AddTableVolume links a catalog volume into a table owned by ownerUserID. A no-op if already
// present. volumeTitle is a denormalized snapshot of the volume's title at add time, stored in
// the VolumeTitles sidecar so the volume can be displayed without a catalog lookup; an empty
// title leaves no sidecar entry.
func AddTableVolume(c context.Context, id, ownerUserID, volumeID, volumeTitle, actingUserID string) (*models.Table, error) {
	tbl, err := getOwnedTable(c, id, ownerUserID)
	if err != nil || tbl == nil {
		return tbl, err
	}
	for _, v := range tbl.VolumeIDs {
		if v == volumeID {
			return tbl, nil
		}
	}
	tbl.VolumeIDs = append(tbl.VolumeIDs, volumeID)
	if volumeTitle != "" {
		if tbl.VolumeTitles == nil {
			tbl.VolumeTitles = map[string]string{}
		}
		tbl.VolumeTitles[volumeID] = volumeTitle
	}
	if err := replaceTable(c, tbl, actingUserID); err != nil {
		return nil, err
	}
	return tbl, nil
}

// RemoveTableVolume unlinks a catalog volume from a table owned by ownerUserID, dropping its
// denormalized title sidecar entry along with it.
func RemoveTableVolume(c context.Context, id, ownerUserID, volumeID, actingUserID string) (*models.Table, error) {
	tbl, err := getOwnedTable(c, id, ownerUserID)
	if err != nil || tbl == nil {
		return tbl, err
	}
	kept := tbl.VolumeIDs[:0]
	for _, v := range tbl.VolumeIDs {
		if v != volumeID {
			kept = append(kept, v)
		}
	}
	tbl.VolumeIDs = kept
	delete(tbl.VolumeTitles, volumeID)
	if err := replaceTable(c, tbl, actingUserID); err != nil {
		return nil, err
	}
	return tbl, nil
}

// UpdateTableVolumeTitleByVolume refreshes the denormalized volume-title snapshot in the
// VolumeTitles sidecar of every table across all users that references volumeID, and returns the
// IDs of the users whose tables actually changed. Like the library equivalent this is driven by
// a trusted catalog volume.updated event, not a user request, so it deliberately spans all
// owners. Idempotent: a replay with an unchanged title matches no tables and returns an empty
// slice. A volumeID containing "." is skipped (with a log) rather than written, since "." is not
// a legal BSON map-key character - real volume IDs are hex ObjectIDs and never contain one.
func UpdateTableVolumeTitleByVolume(c context.Context, volumeID, volumeTitle string) ([]string, error) {
	if strings.Contains(volumeID, ".") {
		logging.Logger.Warn("Skipping table volume-title sync for volume ID containing '.'", "volumeID", volumeID)
		return []string{}, nil
	}

	stale, err := database.Query[models.Table](tableCollection, live(bson.D{
		{Key: "volume_ids", Value: volumeID},
		{Key: "volume_titles." + volumeID, Value: bson.D{{Key: "$ne", Value: volumeTitle}}},
	}), nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying tables for volume title sync", "volumeID", volumeID, "error", err)
		return nil, err
	}
	if len(stale) == 0 {
		return []string{}, nil
	}

	// ponytail: one whole-document replace per affected table, mirroring
	// UpdateLibraryEntryTitleByVolume - a volume retitle is a rare catalog event with a small
	// fan-out, and this reuses replaceTable's audit stamping. Revisit with a bulk write if a
	// single volume ever lands in tens of thousands of tables.
	userIDs := make([]string, 0, len(stale))
	for _, tbl := range stale {
		if tbl.VolumeTitles == nil {
			tbl.VolumeTitles = map[string]string{}
		}
		tbl.VolumeTitles[volumeID] = volumeTitle
		if err := replaceTable(c, tbl, SystemActor); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, tbl.UserID)
	}
	return userIDs, nil
}

// DeleteTable soft-deletes a table owned by ownerUserID: it sets deleted_at/deleted_by via $set
// rather than removing the document. Returns whether a live document was actually marked, so
// callers can tell "not found" (or already deleted) apart from "found but not owned by
// ownerUserID".
func DeleteTable(c context.Context, id, ownerUserID, actingUserID string) (bool, error) {
	result, err := database.Db.Collection(tableCollection).UpdateOne(c,
		live(bson.D{{Key: "_id", Value: id}, {Key: "user_id", Value: ownerUserID}}),
		softDeleteSet(actingUserID, time.Now()))
	if err != nil {
		logging.Logger.Error("Error while deleting table", "id", id, "error", err)
		return false, err
	}
	return result.ModifiedCount > 0, nil
}

// TableToVO converts a table model into its VO, or nil if viewerID may not see it at all.
func TableToVO(tbl *models.Table, viewerID string, isFriend, isFriendOfFriend bool) *vo.TableVO {
	if !CanView(tbl.Visibility, tbl.UserID, viewerID, isFriend, isFriendOfFriend) {
		return nil
	}

	return &vo.TableVO{
		ID:           tbl.ID,
		UserID:       tbl.UserID,
		Name:         tbl.Name,
		VolumeIDs:    tbl.VolumeIDs,
		VolumeTitles: tbl.VolumeTitles,
		Visibility:   string(tbl.Visibility),
		AuditableVO:  auditVO(tbl.Auditable),
	}
}
