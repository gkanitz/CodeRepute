// Package bands provides cited typical ranges for CodeRepute metric values,
// sourced from published research and industry benchmarks. The data is embedded
// at build time and accessible via Lookup.
package bands

import (
	"embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed bands.json
var bandsFS embed.FS

// Entry is one metric's cited typical range.
type Entry struct {
	Key         string  `json:"key"`
	RangeLo     float64 `json:"range_lo"`
	RangeHi     float64 `json:"range_hi"`
	Unit        string  `json:"unit"`
	Label       string  `json:"label"`
	SourceTitle string  `json:"source_title"`
	SourceURL   string  `json:"source_url"`
	SourceYear  string  `json:"source_year"`
	Caveat      string  `json:"caveat"`
}

// Bands is the top-level structure of the embedded bands.json.
type Bands struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

var (
	once    sync.Once
	entries map[string]Entry
	version int
)

// load reads and parses the embedded bands.json once.
func load() {
	raw, err := bandsFS.ReadFile("bands.json")
	if err != nil {
		panic(fmt.Sprintf("bands: embedded bands.json: %v", err))
	}
	var bands Bands
	if err := json.Unmarshal(raw, &bands); err != nil {
		panic(fmt.Sprintf("bands: parse embedded bands.json: %v", err))
	}
	version = bands.Version
	entries = make(map[string]Entry, len(bands.Entries))
	for _, e := range bands.Entries {
		entries[e.Key] = e
	}
}

// Lookup returns the band entry for the given metric key, or false if the
// key is unknown.
func Lookup(key string) (Entry, bool) {
	once.Do(load)
	e, ok := entries[key]
	return e, ok
}

// Version returns the embedded bands.json version number.
func Version() int {
	once.Do(load)
	return version
}
