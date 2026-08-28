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

// GetTable returns one table by ID, or nil if it doesn't exist. Unscoped by owner - only for
// read paths, which apply visibility filtering separately via TableToVO/CanView. Write paths
// must use getOwnedTable instead.
func GetTable(c context.Context, id string) (*models.Table, error) {
	results, err := database.Query[models.Table](tableCollection, bson.D{{Key: "_id", Value: id}}, nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for table", "id", id, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// getOwnedTable returns the table only if it exists and is owned by ownerUserID, via a single
// query filtered on both fields. Returns (nil, nil) both when the table doesn't exist and when
// it's owned by someone else - callers needing to tell those apart (to choose between a 403 and
// a 404 response) should follow up with GetTable.
func getOwnedTable(c context.Context, id, ownerUserID string) (*models.Table, error) {
	results, err := database.Query[models.Table](tableCollection, bson.D{{Key: "_id", Value: id}, {Key: "user_id", Value: ownerUserID}}, nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for owned table", "id", id, "ownerUserID", ownerUserID, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// ListTablesByUser returns every table owned by the given user.
func ListTablesByUser(c context.Context, userID string) ([]*models.Table, error) {
	results, err := database.Query[models.Table](tableCollection, bson.D{{Key: "user_id", Value: userID}}, nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying database for tables", "userID", userID, "error", err)
		return nil, err
	}
	return results, nil
}

// CreateTable creates a new table, defaulting to private visibility per the "new tables default
// to private" requirement.
func CreateTable(c context.Context, userID, name string) (*models.Table, error) {
	tbl := models.NewTable(primitive.NewObjectID().Hex(), userID, name)
	tbl.CreatedAt = time.Now()
	tbl.CreatedBy = userID
	if _, err := database.Insert(tableCollection, tbl); err != nil {
		logging.Logger.Error("Error while inserting table", "userID", userID, "error", err)
		return nil, err
	}
	return &tbl, nil
}

func replaceTable(c context.Context, tbl *models.Table) error {
	tbl.UpdatedAt = time.Now()
	_, err := database.Db.Collection(tableCollection).ReplaceOne(c, bson.D{{Key: "_id", Value: tbl.ID}, {Key: "user_id", Value: tbl.UserID}}, tbl)
	if err != nil {
		logging.Logger.Error("Error while replacing table", "id", tbl.ID, "error", err)
	}
	return err
}

// UpdateTableName renames a table owned by ownerUserID.
func UpdateTableName(c context.Context, id, ownerUserID, name string) (*models.Table, error) {
	tbl, err := getOwnedTable(c, id, ownerUserID)
	if err != nil || tbl == nil {
		return tbl, err
	}
	tbl.Name = name
	if err := replaceTable(c, tbl); err != nil {
		return nil, err
	}
	return tbl, nil
}

// SetTableVisibility updates the visibility of a table owned by ownerUserID.
func SetTableVisibility(c context.Context, id, ownerUserID string, visibility models.Visibility) (*models.Table, error) {
	tbl, err := getOwnedTable(c, id, ownerUserID)
	if err != nil || tbl == nil {
		return tbl, err
	}
	tbl.Visibility = visibility
	if err := replaceTable(c, tbl); err != nil {
		return nil, err
	}
	return tbl, nil
}

// AddTableVolume links a catalog volume into a table owned by ownerUserID. A no-op if already
// present.
func AddTableVolume(c context.Context, id, ownerUserID, volumeID string) (*models.Table, error) {
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
	if err := replaceTable(c, tbl); err != nil {
		return nil, err
	}
	return tbl, nil
}

// RemoveTableVolume unlinks a catalog volume from a table owned by ownerUserID.
func RemoveTableVolume(c context.Context, id, ownerUserID, volumeID string) (*models.Table, error) {
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
	if err := replaceTable(c, tbl); err != nil {
		return nil, err
	}
	return tbl, nil
}

// DeleteTable removes a table owned by ownerUserID. Returns whether a document was actually
// deleted, so callers can tell "not found" apart from "found but not owned by ownerUserID".
func DeleteTable(c context.Context, id, ownerUserID string) (bool, error) {
	result, err := database.Db.Collection(tableCollection).DeleteOne(c, bson.D{{Key: "_id", Value: id}, {Key: "user_id", Value: ownerUserID}})
	if err != nil {
		logging.Logger.Error("Error while deleting table", "id", id, "error", err)
		return false, err
	}
	return result.DeletedCount > 0, nil
}

// TableToVO converts a table model into its VO, or nil if viewerID may not see it at all.
func TableToVO(tbl *models.Table, viewerID string, isFriend, isFriendOfFriend bool) *vo.TableVO {
	if !CanView(tbl.Visibility, tbl.UserID, viewerID, isFriend, isFriendOfFriend) {
		return nil
	}

	return &vo.TableVO{
		ID:         tbl.ID,
		UserID:     tbl.UserID,
		Name:       tbl.Name,
		VolumeIDs:  tbl.VolumeIDs,
		Visibility: string(tbl.Visibility),
	}
}
