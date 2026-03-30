package bankdirectory

import "testing"

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
