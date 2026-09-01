package data

import (
	"context"
	"time"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// auditBackfillCollections are the game-room owner-scoped collections whose documents predate the
// audit-fields convention and need created_at/updated_at seeded from their ObjectID timestamp.
var auditBackfillCollections = []string{tableCollection, wishlistCollection, libraryCollection}

// CollectionBackfillResult reports what the backfill did (or would do, on a dry run) for one
// collection.
type CollectionBackfillResult struct {
	Collection string
	Matched    int
	Updated    int
}

// backfillCandidateFilter matches documents with no usable created_at: the field is absent or is
// the BSON zero date (0001-01-01). This is what makes the backfill idempotent - a second run
// matches nothing.
func backfillCandidateFilter() bson.D {
	return bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "created_at", Value: bson.D{{Key: "$exists", Value: false}}}},
		bson.D{{Key: "created_at", Value: time.Time{}}},
	}}}
}

// BackfillAuditFields seeds created_at/updated_at (from the ObjectID hex in _id) and
// created_by/updated_by (from the document's own user_id owner) on every table/wishlist/library
// document that is missing them. It is idempotent. When dryRun is true it only counts candidates
// and writes nothing.
//
// deleted_at/deleted_by are left untouched: a document that predates soft delete has neither
// field, which reads as null everywhere the live() filter is applied.
func BackfillAuditFields(c context.Context, dryRun bool) ([]CollectionBackfillResult, error) {
	results := make([]CollectionBackfillResult, 0, len(auditBackfillCollections))
	for _, name := range auditBackfillCollections {
		res, err := backfillCollection(c, name, dryRun)
		if err != nil {
			return results, err
		}
		results = append(results, res)
	}
	return results, nil
}

func backfillCollection(c context.Context, name string, dryRun bool) (CollectionBackfillResult, error) {
	res := CollectionBackfillResult{Collection: name}
	coll := database.Db.Collection(name)
	filter := backfillCandidateFilter()

	if dryRun {
		count, err := coll.CountDocuments(c, filter)
		if err != nil {
			logging.Logger.Error("Error counting audit-backfill candidates", "collection", name, "error", err)
			return res, err
		}
		res.Matched = int(count)
		return res, nil
	}

	cursor, err := coll.Find(c, filter, options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "user_id", Value: 1}}))
	if err != nil {
		logging.Logger.Error("Error finding audit-backfill candidates", "collection", name, "error", err)
		return res, err
	}
	defer cursor.Close(c)

	for cursor.Next(c) {
		var doc struct {
			ID     string `bson:"_id"`
			UserID string `bson:"user_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			logging.Logger.Error("Error decoding audit-backfill candidate", "collection", name, "error", err)
			return res, err
		}
		res.Matched++

		oid, err := primitive.ObjectIDFromHex(doc.ID)
		if err != nil {
			logging.Logger.Warn("Skipping audit-backfill for non-ObjectID _id", "collection", name, "id", doc.ID)
			continue
		}
		ts := oid.Timestamp()

		update := bson.D{{Key: "$set", Value: bson.D{
			{Key: "created_at", Value: ts},
			{Key: "updated_at", Value: ts},
			{Key: "created_by", Value: doc.UserID},
			{Key: "updated_by", Value: doc.UserID},
		}}}
		if _, err := coll.UpdateByID(c, doc.ID, update); err != nil {
			logging.Logger.Error("Error applying audit-backfill", "collection", name, "id", doc.ID, "error", err)
			return res, err
		}
		res.Updated++
	}
	if err := cursor.Err(); err != nil {
		logging.Logger.Error("Cursor error during audit-backfill", "collection", name, "error", err)
		return res, err
	}
	return res, nil
}
