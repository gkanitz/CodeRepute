package bands

import (
	"encoding/json"
	"os"
	"testing"
)

// TestLookup verifies that every key in bands.json is resolvable via Lookup
// and that an unknown key returns ok == false.
func TestLookup(t *testing.T) {
	// Read the embedded file to enumerate all expected keys.
	raw, err := os.ReadFile("bands.json")
	if err != nil {
		t.Fatalf("read bands.json: %v", err)
	}
	var doc struct {
		Entries []struct {
			Key string `json:"key"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal bands.json: %v", err)
	}

	for _, e := range doc.Entries {
		entry, ok := Lookup(e.Key)
		if !ok {
			t.Errorf("Lookup(%q) = _, false, want true", e.Key)
		}
		if entry.Key != e.Key {
			t.Errorf("Lookup(%q).Key = %q, want %q", e.Key, entry.Key, e.Key)
		}
	}

	// Unknown key must return false.
	if _, ok := Lookup("nonexistent_metric"); ok {
		t.Error("Lookup(nonexistent_metric) = _, true, want false")
	}
}

// TestEntryCompleteness verifies that every bands.json entry has all required
// fields populated. This is a schema-completeness test over the embedded file.
func TestEntryCompleteness(t *testing.T) {
	raw, err := os.ReadFile("bands.json")
	if err != nil {
		t.Fatalf("read bands.json: %v", err)
	}
	var doc struct {
		Version int     `json:"version"`
		Entries []Entry `json:"entries"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal bands.json: %v", err)
	}

	if doc.Version != 1 {
		t.Errorf("bands version = %d, want 1", doc.Version)
	}

	if len(doc.Entries) == 0 {
		t.Fatal("bands.json has no entries")
	}

	for _, e := range doc.Entries {
		if e.Key == "" {
			t.Error("found entry with empty key")
		}
		if e.RangeLo == 0 && e.RangeHi == 0 {
			t.Errorf("entry %q: range_lo and range_hi are both zero", e.Key)
		}
		if e.Unit == "" {
			t.Errorf("entry %q: empty unit", e.Key)
		}
		if e.Label == "" {
			t.Errorf("entry %q: empty label", e.Key)
		}
		if e.SourceTitle == "" {
			t.Errorf("entry %q: empty source_title", e.Key)
		}
		if e.SourceURL == "" {
			t.Errorf("entry %q: empty source_url", e.Key)
		}
		if e.SourceYear == "" {
			t.Errorf("entry %q: empty source_year", e.Key)
		}
		if e.Caveat == "" {
			t.Errorf("entry %q: empty caveat", e.Key)
		}

		// Unit must be one of the known values.
		switch e.Unit {
		case "hours", "share", "lines":
			// valid
		default:
			t.Errorf("entry %q: unknown unit %q", e.Key, e.Unit)
		}
	}
}
