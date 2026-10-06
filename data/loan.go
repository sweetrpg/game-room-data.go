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
)

// ErrLoanBorrowerRequired is returned when a loan is created with neither a borrower user ID nor
// a borrower name - a loan requires a borrower (design.md: XOR of the two, both stored).
var ErrLoanBorrowerRequired = errors.New("loan requires a borrower_user_id or a borrower_name")

// ErrLoanAlreadyOpen is returned when creating a loan would duplicate an existing open loan for
// the same lender, volume, and borrower identity.
var ErrLoanAlreadyOpen = errors.New("an open loan already exists for this lender, volume, and borrower")

// ListLoansLentBy returns every live loan lent by the given user, lent and returned alike.
func ListLoansLentBy(c context.Context, userID string) ([]*models.Loan, error) {
	results, err := database.Query[models.Loan](loanCollection, live(bson.D{{Key: "lender_user_id", Value: userID}}), nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying database for loans lent by user", "userID", userID, "error", err)
		return nil, err
	}
	return results, nil
}

// ListLoansBorrowedBy returns every live loan borrowed by the given platform-linked user, lent
// and returned alike. Free-form borrowers have no user ID and so never appear here.
func ListLoansBorrowedBy(c context.Context, userID string) ([]*models.Loan, error) {
	results, err := database.Query[models.Loan](loanCollection, live(bson.D{{Key: "borrower_user_id", Value: userID}}), nil, nil, 0, 0)
	if err != nil {
		logging.Logger.Error("Error while querying database for loans borrowed by user", "userID", userID, "error", err)
		return nil, err
	}
	return results, nil
}

// getOwnedLoan returns the loan only if it exists, is not soft-deleted, and is owned (lent) by
// ownerUserID, via a single query filtered on both fields. Returns (nil, nil) both when the loan
// doesn't exist and when it's owned by someone else - callers needing to tell those apart (to
// choose between a 403 and a 404 response) should follow up with GetLoan.
func getOwnedLoan(c context.Context, id, ownerUserID string) (*models.Loan, error) {
	results, err := database.Query[models.Loan](loanCollection, live(bson.D{{Key: "_id", Value: id}, {Key: "lender_user_id", Value: ownerUserID}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for owned loan", "id", id, "ownerUserID", ownerUserID, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// GetLoan returns one loan by ID, or nil if it doesn't exist or has been soft-deleted. Unscoped
// by owner - write paths must use getOwnedLoan instead.
func GetLoan(c context.Context, id string) (*models.Loan, error) {
	results, err := database.Query[models.Loan](loanCollection, live(bson.D{{Key: "_id", Value: id}}), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for loan", "id", id, "error", err)
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// openLoanExists reports whether a live, still-open (status=lent) loan already exists for the
// same lender, volume, and borrower identity - a linked borrower is matched by user ID, a
// free-form borrower by name.
func openLoanExists(c context.Context, lenderUserID, volumeID string, borrowerUserID *string, borrowerName string) (bool, error) {
	filter := bson.D{
		{Key: "lender_user_id", Value: lenderUserID},
		{Key: "volume_id", Value: volumeID},
		{Key: "status", Value: models.LoanStatusLent},
	}
	if borrowerUserID != nil {
		filter = append(filter, bson.E{Key: "borrower_user_id", Value: *borrowerUserID})
	} else {
		filter = append(filter, bson.E{Key: "borrower_name", Value: borrowerName})
	}
	results, err := database.Query[models.Loan](loanCollection, live(filter), nil, nil, 0, 1)
	if err != nil {
		logging.Logger.Error("Error while querying database for open loan", "lenderUserID", lenderUserID, "volumeID", volumeID, "error", err)
		return false, err
	}
	return len(results) > 0, nil
}

// CreateLoan creates a new loan lent by lenderUserID, requiring exactly one of borrowerUserID or
// borrowerName (the caller resolves borrowerUserID to a display name and passes it as
// borrowerName regardless of borrower kind, per design.md's always-populated name rule). Rejects
// the create if an open loan already exists for the same lender, volume, and borrower identity.
func CreateLoan(c context.Context, lenderUserID, volumeID string, borrowerUserID *string, borrowerName, actingUserID string) (*models.Loan, error) {
	if borrowerUserID == nil && borrowerName == "" {
		return nil, ErrLoanBorrowerRequired
	}

	exists, err := openLoanExists(c, lenderUserID, volumeID, borrowerUserID, borrowerName)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrLoanAlreadyOpen
	}

	loan := models.NewLoan(primitive.NewObjectID().Hex(), lenderUserID, volumeID, borrowerUserID, borrowerName)
	stampCreate(&loan.Auditable, actingUserID, time.Now())
	if _, err := database.Insert(loanCollection, loan); err != nil {
		logging.Logger.Error("Error while inserting loan", "lenderUserID", lenderUserID, "volumeID", volumeID, "error", err)
		return nil, err
	}
	return &loan, nil
}

func replaceLoan(c context.Context, loan *models.Loan, actingUserID string) error {
	stampUpdate(&loan.Auditable, actingUserID, time.Now())
	_, err := database.Db.Collection(loanCollection).ReplaceOne(c, live(bson.D{{Key: "_id", Value: loan.ID}, {Key: "lender_user_id", Value: loan.LenderUserID}}), loan)
	if err != nil {
		logging.Logger.Error("Error while replacing loan", "id", loan.ID, "error", err)
	}
	return err
}

// MarkLoanReturned marks a loan owned (lent) by ownerUserID as returned. A no-op if the loan is
// already returned. Returns (nil, nil) if the loan doesn't exist or isn't owned by ownerUserID.
func MarkLoanReturned(c context.Context, id, ownerUserID, actingUserID string) (*models.Loan, error) {
	loan, err := getOwnedLoan(c, id, ownerUserID)
	if err != nil || loan == nil {
		return loan, err
	}
	if loan.Status == models.LoanStatusReturned {
		return loan, nil
	}
	// Truncated to millisecond precision to match what BSON round-trips through Mongo - without
	// this, the in-memory value returned here (nanosecond precision) never equals a freshly
	// fetched copy of the same document, which TestMarkLoanReturnedRejectsNonLenderAndIsIdempotent
	// caught comparing the no-op second return against the first.
	now := time.Now().Truncate(time.Millisecond)
	loan.Status = models.LoanStatusReturned
	loan.ReturnedAt = &now
	if err := replaceLoan(c, loan, actingUserID); err != nil {
		return nil, err
	}
	return loan, nil
}

// DeleteLoan soft-deletes a loan owned (lent) by ownerUserID: it sets deleted_at/deleted_by via
// $set rather than removing the document. Returns whether a live document was actually marked, so
// callers can tell "not found" (or already deleted) apart from "found but not owned by
// ownerUserID".
func DeleteLoan(c context.Context, id, ownerUserID, actingUserID string) (bool, error) {
	result, err := database.Db.Collection(loanCollection).UpdateOne(c,
		live(bson.D{{Key: "_id", Value: id}, {Key: "lender_user_id", Value: ownerUserID}}),
		softDeleteSet(actingUserID, time.Now()))
	if err != nil {
		logging.Logger.Error("Error while deleting loan", "id", id, "error", err)
		return false, err
	}
	return result.ModifiedCount > 0, nil
}

// LoanToVO converts a loan model into its VO.
func LoanToVO(loan *models.Loan) *vo.LoanVO {
	return &vo.LoanVO{
		ID:             loan.ID,
		LenderUserID:   loan.LenderUserID,
		VolumeID:       loan.VolumeID,
		BorrowerUserID: loan.BorrowerUserID,
		BorrowerName:   loan.BorrowerName,
		Status:         string(loan.Status),
		LentAt:         loan.LentAt,
		ReturnedAt:     loan.ReturnedAt,
		AuditableVO:    auditVO(loan.Auditable),
	}
}
