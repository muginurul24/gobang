package main

import "testing"

func TestParseSeedProfile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default demo", input: "", want: seedProfileDemo},
		{name: "demo", input: "demo", want: seedProfileDemo},
		{name: "demo uppercase", input: "DEMO", want: seedProfileDemo},
		{name: "dev only", input: "dev-only", want: seedProfileDevOnly},
		{name: "invalid", input: "full-demo", wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseSeedProfile(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseSeedProfile(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parseSeedProfile(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseMigrateFreshSeedArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		args        []string
		wantEnabled bool
		wantProfile string
		wantErr     bool
	}{
		{name: "no seed", args: nil, wantEnabled: false},
		{name: "default seed", args: []string{"--seed"}, wantEnabled: true, wantProfile: seedProfileDemo},
		{name: "default seed explicit empty", args: []string{"--seed="}, wantEnabled: true, wantProfile: seedProfileDemo},
		{name: "seed demo via equals", args: []string{"--seed=demo"}, wantEnabled: true, wantProfile: seedProfileDemo},
		{name: "seed dev only via equals", args: []string{"--seed=dev-only"}, wantEnabled: true, wantProfile: seedProfileDevOnly},
		{name: "seed dev only via separate arg", args: []string{"--seed", "dev-only"}, wantEnabled: true, wantProfile: seedProfileDevOnly},
		{name: "invalid flag", args: []string{"--bogus"}, wantErr: true},
		{name: "invalid profile", args: []string{"--seed=full-demo"}, wantErr: true},
		{name: "too many args", args: []string{"--seed", "dev-only", "extra"}, wantErr: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			enabled, profile, err := parseMigrateFreshSeedArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for args %v", tc.args)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseMigrateFreshSeedArgs(%v) error = %v", tc.args, err)
			}
			if enabled != tc.wantEnabled {
				t.Fatalf("enabled = %v, want %v", enabled, tc.wantEnabled)
			}
			if profile != tc.wantProfile {
				t.Fatalf("profile = %q, want %q", profile, tc.wantProfile)
			}
		})
	}
}
