// Command seed loads development fixtures into the database.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/20age1million/WaterlooStar-Backend/internal/config"
	"github.com/20age1million/WaterlooStar-Backend/internal/db"
	"github.com/20age1million/WaterlooStar-Backend/internal/db/seed"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Seeding rewrites every listing, so it must never be pointed at production.
	if !cfg.IsDevelopment() {
		log.Fatal("refusing to seed: ENVIRONMENT is not development")
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	count, err := seed.Run(ctx, pool)
	if err != nil {
		log.Fatalf("seed: %v", err)
	}
	fmt.Printf("seeded %d posts\n", count)
}
