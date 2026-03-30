package bankdirectory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type Entry struct {
	BankCode      string `json:"bank_code"`
	BankName      string `json:"bank_name"`
	BankSwiftCode string `json:"bank_swift_code"`
}

type Directory struct {
	entries []Entry
	byCode  map[string][]Entry
}

var (
	defaultOnce sync.Once
	defaultDir  *Directory
	defaultErr  error
)

func LoadDefault() (*Directory, error) {
	defaultOnce.Do(func() {
		defaultDir, defaultErr = loadFromRepository()
	})

	return defaultDir, defaultErr
}

func MustLoadDefault() *Directory {
	directory, err := LoadDefault()
	if err != nil {
		panic(err)
	}

	return directory
}

func New(entries []Entry) *Directory {
	normalized := make([]Entry, 0, len(entries))
	byCode := make(map[string][]Entry, len(entries))

	for _, entry := range entries {
		entry.BankCode = normalizeCode(entry.BankCode)
		entry.BankName = strings.TrimSpace(entry.BankName)
		entry.BankSwiftCode = strings.TrimSpace(entry.BankSwiftCode)
		if entry.BankCode == "" || entry.BankName == "" {
			continue
		}

		normalized = append(normalized, entry)
		byCode[entry.BankCode] = append(byCode[entry.BankCode], entry)
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].BankCode == normalized[j].BankCode {
			return normalized[i].BankName < normalized[j].BankName
		}

		return normalized[i].BankCode < normalized[j].BankCode
	})

	return &Directory{
		entries: normalized,
		byCode:  byCode,
	}
}

func (d *Directory) Search(query string, limit int) []Entry {
	needle := normalizeQuery(query)
	max := sanitizeLimit(limit)
	results := make([]Entry, 0, max)

	for _, entry := range d.entries {
		if needle != "" &&
			!strings.Contains(entry.BankCode, needle) &&
			!strings.Contains(normalizeQuery(entry.BankName), needle) {
			continue
		}

		results = append(results, entry)
		if len(results) == max {
			break
		}
	}

	return results
}

func (d *Directory) HasCode(code string) bool {
	_, ok := d.PrimaryByCode(code)
	return ok
}

func (d *Directory) PrimaryByCode(code string) (Entry, bool) {
	entries := d.byCode[normalizeCode(code)]
	if len(entries) == 0 {
		return Entry{}, false
	}

	return entries[0], true
}

func loadFromRepository() (*Directory, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("resolve bank directory caller")
	}

	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "../../../"))
	path := filepath.Join(root, "docs", "Bank RTOL.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read bank directory JSON: %w", err)
	}

	normalizedPayload := strings.TrimSpace(string(payload))
	normalizedPayload = strings.TrimPrefix(normalizedPayload, "export default")
	normalizedPayload = strings.TrimSpace(strings.TrimSuffix(normalizedPayload, ";"))

	var entries []Entry
	if err := json.Unmarshal([]byte(normalizedPayload), &entries); err != nil {
		return nil, fmt.Errorf("decode bank directory JSON: %w", err)
	}

	return New(entries), nil
}

func normalizeCode(code string) string {
	return strings.TrimSpace(code)
}

func normalizeQuery(query string) string {
	replacer := strings.NewReplacer(".", "", ",", "", "(", "", ")", "", "/", "", "-", "", "  ", " ")
	return strings.ToLower(strings.TrimSpace(replacer.Replace(query)))
}

func sanitizeLimit(limit int) int {
	switch {
	case limit <= 0:
		return 20
	case limit > 50:
		return 50
	default:
		return limit
	}
}
