package data

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/mongodb.go/constants"
	"github.com/sweetrpg/mongodb.go/database"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type LoanTestSuite struct {
	suite.Suite
}

func (suite *LoanTestSuite) SetupTest() {
	_ = os.Setenv(constants.DB_URI, os.Getenv("TEST_DB_URI"))
	logging.Init()
	database.SetupDatabase()
}

func (suite *LoanTestSuite) TestListLoansLentByReturnsSeededLoans() {
	ctx := context.Background()
	lender := primitive.NewObjectID().Hex()
	volumeA := primitive.NewObjectID().Hex()
	volumeB := primitive.NewObjectID().Hex()

	_, err := CreateLoan(ctx, lender, volumeA, nil, "Alice", lender)
	assert.NoError(suite.T(), err)
	_, err = CreateLoan(ctx, lender, volumeB, nil, "Bob", lender)
	assert.NoError(suite.T(), err)

	loans, err := ListLoansLentBy(ctx, lender)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), loans, 2)
}

func (suite *LoanTestSuite) TestListLoansBorrowedByReturnsSeededLoans() {
	ctx := context.Background()
	lender := primitive.NewObjectID().Hex()
	borrower := primitive.NewObjectID().Hex()
	volumeA := primitive.NewObjectID().Hex()
	volumeB := primitive.NewObjectID().Hex()

	_, err := CreateLoan(ctx, lender, volumeA, &borrower, "Borrower Name", lender)
	assert.NoError(suite.T(), err)
	loan2, err := CreateLoan(ctx, lender, volumeB, &borrower, "Borrower Name", lender)
	assert.NoError(suite.T(), err)
	_, err = MarkLoanReturned(ctx, loan2.ID, lender, lender)
	assert.NoError(suite.T(), err)

	loans, err := ListLoansBorrowedBy(ctx, borrower)
	assert.NoError(suite.T(), err)
	assert.Len(suite.T(), loans, 2)
}

func (suite *LoanTestSuite) TestCreateLoanRejectsMissingBorrower() {
	ctx := context.Background()
	lender := primitive.NewObjectID().Hex()
	volumeID := primitive.NewObjectID().Hex()

	_, err := CreateLoan(ctx, lender, volumeID, nil, "", lender)
	assert.ErrorIs(suite.T(), err, ErrLoanBorrowerRequired)
}

func (suite *LoanTestSuite) TestCreateLoanRejectsDuplicateOpenLoanButAllowsAfterReturn() {
	ctx := context.Background()
	lender := primitive.NewObjectID().Hex()
	volumeID := primitive.NewObjectID().Hex()

	loan, err := CreateLoan(ctx, lender, volumeID, nil, "Carol", lender)
	assert.NoError(suite.T(), err)

	_, err = CreateLoan(ctx, lender, volumeID, nil, "Carol", lender)
	assert.ErrorIs(suite.T(), err, ErrLoanAlreadyOpen)

	_, err = MarkLoanReturned(ctx, loan.ID, lender, lender)
	assert.NoError(suite.T(), err)

	second, err := CreateLoan(ctx, lender, volumeID, nil, "Carol", lender)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), second)
}

func (suite *LoanTestSuite) TestMarkLoanReturnedRejectsNonLenderAndIsIdempotent() {
	ctx := context.Background()
	lender := primitive.NewObjectID().Hex()
	notLender := primitive.NewObjectID().Hex()
	volumeID := primitive.NewObjectID().Hex()

	loan, err := CreateLoan(ctx, lender, volumeID, nil, "Dave", lender)
	assert.NoError(suite.T(), err)

	got, err := MarkLoanReturned(ctx, loan.ID, notLender, notLender)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got, "non-lender return attempt resolves to owner-scoped miss (caller maps to 403)")

	returned, err := MarkLoanReturned(ctx, loan.ID, lender, lender)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), returned.ReturnedAt)

	again, err := MarkLoanReturned(ctx, loan.ID, lender, lender)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), returned.ReturnedAt, again.ReturnedAt, "second return is a no-op")
}

func (suite *LoanTestSuite) TestDeleteLoanRejectsNonOwner() {
	ctx := context.Background()
	lender := primitive.NewObjectID().Hex()
	notLender := primitive.NewObjectID().Hex()
	volumeID := primitive.NewObjectID().Hex()

	loan, err := CreateLoan(ctx, lender, volumeID, nil, "Erin", lender)
	assert.NoError(suite.T(), err)

	deleted, err := DeleteLoan(ctx, loan.ID, notLender, notLender)
	assert.NoError(suite.T(), err)
	assert.False(suite.T(), deleted)

	deleted, err = DeleteLoan(ctx, loan.ID, lender, lender)
	assert.NoError(suite.T(), err)
	assert.True(suite.T(), deleted)

	got, err := GetLoan(ctx, loan.ID)
	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), got)
}

func TestLoanTestSuite(t *testing.T) {
	suite.Run(t, new(LoanTestSuite))
}
