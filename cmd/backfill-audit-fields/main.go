// Command backfill-audit-fields seeds model-core audit fields (created_at/updated_at from the
// ObjectID timestamp, created_by/updated_by from the document owner) on game-room table,
// wishlist, and library documents that predate the audit-fields convention. It is idempotent.
//
// Usage:
//
//	backfill-audit-fields            # apply
//	backfill-audit-fields -dry-run   # report candidate counts, write nothing
//
// Connection comes from the standard mongodb.go env vars (DB_URI, or DB_SCHEME/DB_HOST/... ).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-data.go/data"
	"github.com/sweetrpg/mongodb.go/database"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "report candidate counts without writing")
	flag.Parse()

	logging.Init()
	database.SetupDatabase()

	results, err := data.BackfillAuditFields(context.Background(), *dryRun)
	for _, r := range results {
		if *dryRun {
			fmt.Printf("%-10s candidates: %d\n", r.Collection, r.Matched)
		} else {
			fmt.Printf("%-10s matched: %d  updated: %d\n", r.Collection, r.Matched, r.Updated)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "backfill failed: %v\n", err)
		os.Exit(1)
	}
}
