package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/mugiew/onixggr/internal/platform/db"
)

const (
	seedProfileDemo    = "demo"
	seedProfileDevOnly = "dev-only"
)

func parseSeedProfile(raw string) (string, error) {
	profile := strings.TrimSpace(strings.ToLower(raw))
	switch profile {
	case seedProfileDemo, "":
		return seedProfileDemo, nil
	case seedProfileDevOnly:
		return seedProfileDevOnly, nil
	default:
		return "", fmt.Errorf("unsupported seed profile %q", raw)
	}
}

func parseMigrateFreshSeedArgs(args []string) (bool, string, error) {
	if len(args) == 0 {
		return false, "", nil
	}

	switch {
	case args[0] == "--seed":
		if len(args) == 1 {
			return true, seedProfileDemo, nil
		}
		if len(args) == 2 {
			profile, err := parseSeedProfile(args[1])
			if err != nil {
				return false, "", err
			}
			return true, profile, nil
		}
		return false, "", fmt.Errorf("unexpected extra arguments after --seed")
	case strings.HasPrefix(args[0], "--seed="):
		if len(args) != 1 {
			return false, "", fmt.Errorf("unexpected extra arguments after %s", args[0])
		}
		profile, err := parseSeedProfile(strings.TrimPrefix(args[0], "--seed="))
		if err != nil {
			return false, "", err
		}
		return true, profile, nil
	default:
		return false, "", fmt.Errorf("unsupported migrate fresh arguments: %s", strings.Join(args, " "))
	}
}

func applySeedProfile(ctx context.Context, pool *db.Pool, profile string, authEncryptionKey string) (int, error) {
	switch profile {
	case seedProfileDemo:
		return applyDemoSeed(ctx, pool, authEncryptionKey)
	case seedProfileDevOnly:
		return applyDevOnlySeed(ctx, pool)
	default:
		return 0, fmt.Errorf("unsupported seed profile %q", profile)
	}
}
