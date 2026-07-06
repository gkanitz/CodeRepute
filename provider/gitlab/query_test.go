package gitlab

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestQueryMatchesGolden verifies the exported query constant exactly
// matches the golden file. If this test fails, either the code or the
// golden must be updated — they are a matching pair.
func TestQueryMatchesGolden(t *testing.T) {
	golden := filepath.Join("testdata", "graphql_diffstats_query.golden")
	raw, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	goldenStr := strings.TrimSpace(string(raw))
	constStr := strings.TrimSpace(gitlabFilesQuery)

	if constStr != goldenStr {
		t.Errorf("gitlabFilesQuery constant does not match golden\n--- golden:\n%s\n\n--- constant:\n%s", goldenStr, constStr)
	}
}

// TestQueryNoContentFields is a defense-in-depth check that the GraphQL
// query never requests any content-bearing field.
func TestQueryNoContentFields(t *testing.T) {
	for _, forbidden := range []string{"patch", "hunk", "rawTextBlob", "content(", "diffs("} {
		if strings.Contains(gitlabFilesQuery, forbidden) {
			t.Errorf("gitlabFilesQuery contains forbidden content-bearing field %q", forbidden)
		}
	}
}
