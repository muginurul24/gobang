package bankdirectory

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestLoadDefaultContainsKnownBank(t *testing.T) {
	directory, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault returned error: %v", err)
	}

	entry, ok := directory.PrimaryByCode("014")
	if !ok {
		t.Fatal("PrimaryByCode(014) = missing, want known BCA entry")
	}

	if entry.BankName == "" {
		t.Fatal("PrimaryByCode(014) returned empty bank name")
	}
}

func TestSearchMatchesByCodeAndName(t *testing.T) {
	directory := New([]Entry{
		{BankCode: "014", BankName: "PT. BANK CENTRAL ASIA, TBK."},
		{BankCode: "542", BankName: "PT. BANK ARTOS INDONESIA"},
	})

	resultsByCode := directory.Search("542", 10)
	if len(resultsByCode) != 1 || resultsByCode[0].BankCode != "542" {
		t.Fatalf("Search by code = %#v, want single 542 result", resultsByCode)
	}

	resultsByName := directory.Search("central asia", 10)
	if len(resultsByName) != 1 || resultsByName[0].BankCode != "014" {
		t.Fatalf("Search by name = %#v, want single 014 result", resultsByName)
	}
}

func TestLoadDefaultUsesEnvOverride(t *testing.T) {
	t.Setenv("BANK_DIRECTORY_PATH", filepath.Join("testdata", "bank-directory.json"))
	resetDefaultDirectoryCache()
	t.Cleanup(resetDefaultDirectoryCache)

	directory, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault returned error: %v", err)
	}

	entry, ok := directory.PrimaryByCode("999")
	if !ok {
		t.Fatal("PrimaryByCode(999) = missing, want override entry")
	}

	if entry.BankName != "Override Bank" {
		t.Fatalf("PrimaryByCode(999).BankName = %q, want Override Bank", entry.BankName)
	}
}

func TestDefaultPathsIncludesAppDocsFallback(t *testing.T) {
	t.Setenv("BANK_DIRECTORY_PATH", "")

	paths := defaultPaths()
	if len(paths) == 0 {
		t.Fatal("defaultPaths() returned empty candidate list")
	}

	if paths[len(paths)-1] != "/app/docs/Bank RTOL.json" {
		t.Fatalf("defaultPaths() last = %q, want /app/docs/Bank RTOL.json", paths[len(paths)-1])
	}
}

func resetDefaultDirectoryCache() {
	defaultOnce = sync.Once{}
	defaultDir = nil
	defaultErr = nil
}
