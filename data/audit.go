package data

import (
	"time"

	modelcore "github.com/sweetrpg/model-core.go/models"
	modelcorevo "github.com/sweetrpg/model-core.go/vo"
	"go.mongodb.org/mongo-driver/bson"
)

// SystemActor is stamped into updated_by for writes not driven by a user request - currently the
// event-driven volume-title denormalization refresh, which spans all owners on a trusted catalog
// event rather than a caller action.
const SystemActor = "system"

// stampCreate sets all four create/update audit fields on a record being inserted, so a freshly
// created document has created_at == updated_at and both *_by set to the acting user.
func stampCreate(a *modelcore.Auditable, actingUserID string, now time.Time) {
	a.CreatedAt = now
	a.CreatedBy = actingUserID
	a.UpdatedAt = now
	a.UpdatedBy = actingUserID
}

// stampUpdate advances updated_at/updated_by, leaving created_* untouched.
func stampUpdate(a *modelcore.Auditable, actingUserID string, now time.Time) {
	a.UpdatedAt = now
	a.UpdatedBy = actingUserID
}

// auditVO copies a model's audit block into its VO form for passthrough to the API response.
func auditVO(a modelcore.Auditable) modelcorevo.AuditableVO {
	return modelcorevo.AuditableVO{
		CreatedAt: a.CreatedAt,
		CreatedBy: a.CreatedBy,
		UpdatedAt: a.UpdatedAt,
		UpdatedBy: a.UpdatedBy,
		DeletedAt: a.DeletedAt,
		DeletedBy: a.DeletedBy,
	}
}

// live appends the soft-delete exclusion to a query filter. {deleted_at: nil} matches both an
// absent field (docs not yet backfilled) and an explicit null (live docs post-backfill).
func live(filter bson.D) bson.D {
	return append(filter, bson.E{Key: "deleted_at", Value: nil})
}

// softDeleteSet is the $set document for a soft delete: marks the record deleted and advances
// the update stamp in the same write.
func softDeleteSet(actingUserID string, now time.Time) bson.D {
	return bson.D{{Key: "$set", Value: bson.D{
		{Key: "deleted_at", Value: now},
		{Key: "deleted_by", Value: actingUserID},
		{Key: "updated_at", Value: now},
		{Key: "updated_by", Value: actingUserID},
	}}}
}
