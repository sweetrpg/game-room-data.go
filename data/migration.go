package data

import (
	"context"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
)

// MigrateWishlistNames backfills DefaultWishlistName onto every pre-existing wishlist document
// that predates the multi-wishlist Name field (name missing or empty), turning each user's
// former implicit singular wishlist into their first named wishlist. Safe to run more than
// once - already-named wishlists are left untouched.
func MigrateWishlistNames(c context.Context) (int, error) {
	filter := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "name", Value: bson.D{{Key: "$exists", Value: false}}}},
		bson.D{{Key: "name", Value: ""}},
	}}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "name", Value: DefaultWishlistName}}}}

	result, err := database.Db.Collection(wishlistCollection).UpdateMany(c, filter, update)
	if err != nil {
		logging.Logger.Error("Error while migrating wishlist names", "error", err)
		return 0, err
	}

	logging.Logger.Info("Migrated wishlist names", "count", result.ModifiedCount)
	return int(result.ModifiedCount), nil
}

// MigrateTableVolumeTitles ensures every table document has a volume_titles object, setting it to
// an empty object on any document that predates the denormalized-title sidecar. It is structural
// only - it populates no titles; those fill in as volumes are re-added or as catalog
// volume.updated events arrive. Safe to run more than once: a document that already has the
// field matches nothing on a second pass.
func MigrateTableVolumeTitles(c context.Context) (int, error) {
	filter := bson.D{{Key: "volume_titles", Value: bson.D{{Key: "$exists", Value: false}}}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "volume_titles", Value: bson.D{}}}}}

	result, err := database.Db.Collection(tableCollection).UpdateMany(c, filter, update)
	if err != nil {
		logging.Logger.Error("Error while migrating table volume titles", "error", err)
		return 0, err
	}

	logging.Logger.Info("Migrated table volume titles", "count", result.ModifiedCount)
	return int(result.ModifiedCount), nil
}
