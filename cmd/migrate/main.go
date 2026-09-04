package main

import (
	"context"
	"log"
	"os"
	"strings"
	"time"

	"ai-token/internal/db"
)

func main() {
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	migrationsDir := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR"))
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	conn, err := db.Open(ctx, databaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	if err := db.Migrate(ctx, conn, migrationsDir); err != nil {
		log.Fatal(err)
	}

	log.Printf("database migrations completed: %s", migrationsDir)
}
