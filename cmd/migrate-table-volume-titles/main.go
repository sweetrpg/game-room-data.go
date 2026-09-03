// Command migrate-table-volume-titles ensures every game-room table document has a volume_titles
// object, setting it to an empty object on documents that predate the denormalized-title
// sidecar. It is structural only (populates no titles) and idempotent.
//
// Usage:
//
//	migrate-table-volume-titles
//
// Connection comes from the standard mongodb.go env vars (DB_URI, or DB_SCHEME/DB_HOST/... ).
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/sweetrpg/common.go/logging"
	"github.com/sweetrpg/game-room-data.go/data"
	"github.com/sweetrpg/mongodb.go/database"
)

func main() {
	logging.Init()
	database.SetupDatabase()

	count, err := data.MigrateTableVolumeTitles(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "migrate-table-volume-titles failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("tables migrated: %d\n", count)
}
