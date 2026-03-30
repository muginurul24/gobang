package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/mugiew/onixggr/internal/modules/providercatalog"
	"github.com/mugiew/onixggr/internal/platform/config"
	"github.com/mugiew/onixggr/internal/platform/db"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return usageError()
	}

	switch args[0] {
	case "migrate":
		return runMigrate(args[1:])
	case "seed":
		return runSeed(args[1:])
	case "sync":
		return runSync(args[1:])
	default:
		return usageError()
	}
}

func runMigrate(args []string) error {
	switch args[0] {
	case "up":
		return withDatabase(func(ctx context.Context, pool *db.Pool) error {
			migrator := db.NewMigrator(pool, "migrations")
			applied, err := migrator.Up(ctx)
			if err != nil {
				return err
			}

			log.Printf("migrate up complete: %d migration(s) applied", applied)
			return nil
		})
	case "down":
		return withDatabase(func(ctx context.Context, pool *db.Pool) error {
			migrator := db.NewMigrator(pool, "migrations")
			applied, err := migrator.Down(ctx)
			if err != nil {
				return err
			}

			log.Printf("migrate down complete: %d migration(s) rolled back", applied)
			return nil
		})
	case "fresh":
		seedAfter := len(args) > 1 && args[1] == "--seed"
		return withDatabase(func(ctx context.Context, pool *db.Pool) error {
			migrator := db.NewMigrator(pool, "migrations")
			applied, err := migrator.Fresh(ctx)
			if err != nil {
				return err
			}

			log.Printf("migrate fresh complete: %d migration(s) applied", applied)

			if seedAfter {
				appliedSeeds, err := db.ApplySQLDir(ctx, pool, "seeds/demo")
				if err != nil {
					return fmt.Errorf("run demo seeds: %w", err)
				}

				log.Printf("demo seeds complete: %d file(s) applied", appliedSeeds)
			}

			return nil
		})
	default:
		return usageError()
	}
}

func runSeed(args []string) error {
	if len(args) != 1 || args[0] != "demo" {
		return usageError()
	}

	return withDatabase(func(ctx context.Context, pool *db.Pool) error {
		applied, err := db.ApplySQLDir(ctx, pool, "seeds/demo")
		if err != nil {
			return fmt.Errorf("run demo seeds: %w", err)
		}

		log.Printf("seed demo complete: %d file(s) applied", applied)
		return nil
	})
}

func runSync(args []string) error {
	if len(args) != 1 || args[0] != "providers" {
		return usageError()
	}

	return withDatabase(func(ctx context.Context, pool *db.Pool) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		service := providercatalog.NewService(providercatalog.Options{
			Repository: providercatalog.NewRepository(pool),
			Upstream: nexusggr.NewClient(nexusggr.Config{
				BaseURL:    cfg.NexusGGR.BaseURL,
				AgentCode:  cfg.NexusGGR.AgentCode,
				AgentToken: cfg.NexusGGR.AgentToken,
				Timeout:    cfg.NexusGGR.Timeout,
			}, slog.Default(), nil, nil),
		})

		summary, err := service.Sync(ctx)
		if err != nil {
			return fmt.Errorf("sync providers: %w", err)
		}

		log.Printf("sync providers complete: %d provider(s), %d game(s)", summary.ProvidersSynced, summary.GamesSynced)
		return nil
	})
}

func withDatabase(callback func(context.Context, *db.Pool) error) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := db.Open(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer pool.Close()

	return callback(ctx, pool)
}

func usageError() error {
	return errors.New("usage: go run ./apps/manage migrate <up|down|fresh [--seed]> | go run ./apps/manage seed demo | go run ./apps/manage sync providers")
}
