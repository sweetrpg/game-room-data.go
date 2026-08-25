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

// GetTable returns one table by ID, or nil if it doesn't exist.
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
	tbl.Auditable.CreatedAt = time.Now()
	tbl.Auditable.CreatedBy = userID
	if _, err := database.Insert(tableCollection, tbl); err != nil {
		logging.Logger.Error("Error while inserting table", "userID", userID, "error", err)
		return nil, err
	}
	return &tbl, nil
}

func replaceTable(c context.Context, tbl *models.Table) error {
	tbl.Auditable.UpdatedAt = time.Now()
	_, err := database.Db.Collection(tableCollection).ReplaceOne(c, bson.D{{Key: "_id", Value: tbl.ID}}, tbl)
	if err != nil {
		logging.Logger.Error("Error while replacing table", "id", tbl.ID, "error", err)
	}
	return err
}

// UpdateTableName renames a table.
func UpdateTableName(c context.Context, id, name string) (*models.Table, error) {
	tbl, err := GetTable(c, id)
	if err != nil || tbl == nil {
		return tbl, err
	}
	tbl.Name = name
	if err := replaceTable(c, tbl); err != nil {
		return nil, err
	}
	return tbl, nil
}

// SetTableVisibility updates a table's visibility.
func SetTableVisibility(c context.Context, id string, visibility models.Visibility) (*models.Table, error) {
	tbl, err := GetTable(c, id)
	if err != nil || tbl == nil {
		return tbl, err
	}
	tbl.Visibility = visibility
	if err := replaceTable(c, tbl); err != nil {
		return nil, err
	}
	return tbl, nil
}

// AddTableVolume links a catalog volume into a table. A no-op if already present.
func AddTableVolume(c context.Context, id, volumeID string) (*models.Table, error) {
	tbl, err := GetTable(c, id)
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

// RemoveTableVolume unlinks a catalog volume from a table.
func RemoveTableVolume(c context.Context, id, volumeID string) (*models.Table, error) {
	tbl, err := GetTable(c, id)
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

// DeleteTable removes a table entirely.
func DeleteTable(c context.Context, id string) error {
	_, err := database.Db.Collection(tableCollection).DeleteOne(c, bson.D{{Key: "_id", Value: id}})
	if err != nil {
		logging.Logger.Error("Error while deleting table", "id", id, "error", err)
	}
	return err
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
