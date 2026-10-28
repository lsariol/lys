// Command rehearse runs what Lys's startup does to the database against a
// restored copy of prod: the migrations (twice) and the schema check.
// rehearse.sh runs it.
package main

import (
	"context"
	"log"
	"os"

	"github.com/lsariol/lys/internal/config"
	"github.com/lsariol/lys/internal/database"
)

func main() {
	ctx := context.Background()
	migrator, app := os.Getenv("REHEARSAL_MIGRATOR_URL"), os.Getenv("REHEARSAL_APP_URL")
	if migrator == "" || app == "" {
		log.Fatal("✗ set REHEARSAL_MIGRATOR_URL and REHEARSAL_APP_URL")
	}

	for i := 1; i <= 2; i++ {
		log.Printf("== migrate, run %d (run 2 should apply nothing)", i)
		if err := database.Migrate(ctx, migrator); err != nil {
			log.Fatalf("✗ %v", err)
		}
	}
	if err := database.PrintMigrationStatus(ctx, migrator, os.Stdout); err != nil {
		log.Fatalf("✗ %v", err)
	}

	db := database.NewDatabase(&config.DatabaseConfig{ConnectionString: app})
	if err := db.Connect(ctx); err != nil {
		log.Fatalf("✗ %v", err)
	}
	defer db.Close()
	if err := db.CheckSchemaVersion(ctx); err != nil {
		log.Fatalf("✗ %v", err)
	}
	log.Println("✓ startup steps passed")
}
