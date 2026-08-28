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
