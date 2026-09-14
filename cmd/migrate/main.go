// Command migrate applies and rolls back database migrations.
//
// golang-migrate is used as a library rather than through its CLI: the CLI needs
// build tags to compile in the Postgres driver, which makes it awkward to install
// reproducibly. Driving it from here keeps the migration files identical while
// letting any developer run "go run ./cmd/migrate up" with no extra tooling.
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"

	"github.com/20age1million/WaterlooStar-Backend/internal/config"
)

const usage = `usage: go run ./cmd/migrate <command>

commands:
  up              apply all pending migrations
  down            roll back every migration (destructive)
  down-one        roll back the most recent migration
  goto <version>  migrate to an exact version
  version         print the current version and dirty state
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	m, err := migrate.New("file://migrations", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open migrations: %v", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if srcErr != nil {
			log.Printf("close migration source: %v", srcErr)
		}
		if dbErr != nil {
			log.Printf("close migration database: %v", dbErr)
		}
	}()

	switch cmd := os.Args[1]; cmd {
	case "up":
		err = m.Up()
	case "down":
		err = m.Down()
	case "down-one":
		err = m.Steps(-1)
	case "goto":
		if len(os.Args) < 3 {
			fmt.Fprint(os.Stderr, usage)
			os.Exit(2)
		}
		var v uint64
		v, err = strconv.ParseUint(os.Args[2], 10, 32)
		if err != nil {
			log.Fatalf("goto: %q is not a version number", os.Args[2])
		}
		err = m.Migrate(uint(v))
	case "version":
		version, dirty, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			fmt.Println("no migrations applied")
			return
		}
		if verr != nil {
			log.Fatalf("version: %v", verr)
		}
		fmt.Printf("version %d (dirty=%t)\n", version, dirty)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	if errors.Is(err, migrate.ErrNoChange) {
		fmt.Println("no change")
		return
	}
	if err != nil {
		log.Fatalf("migrate: %v", err)
	}
	fmt.Println("ok")
}
