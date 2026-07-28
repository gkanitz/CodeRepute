package recognition_test

import (
	"testing"

	"github.com/gkanitz/coderepute/provider/recognition"
)

func TestClassifyCuratedAgentReturnsAgentID(t *testing.T) {
	tests := []struct {
		login string
		want  string
	}{
		{"copilot[bot]", "copilot"},
		{"devin[bot]", "devin"},
		{"dependabot[bot]", "dependabot"},
		{"github-actions[bot]", "github-actions"},
		{"coderabbit[bot]", "coderabbit"},
		{"CODEIUM[bot]", "codeium"},               // case-insensitive
		{"GitHub-Actions[bot]", "github-actions"}, // case-insensitive
	}
	for _, tc := range tests {
		got := recognition.Classify(tc.login, "")
		if got != tc.want {
			t.Errorf("Classify(%q, \"\") = %q, want %q", tc.login, got, tc.want)
		}
	}
}

func TestClassifyStructuralBotByType(t *testing.T) {
	// Author type:"Bot" without a curated ruleset entry should return "bot".
	got := recognition.Classify("some-unknown-bot", "Bot")
	if got != "bot" {
		t.Errorf("Classify(%q, \"Bot\") = %q, want %q", "some-unknown-bot", got, "bot")
	}
}

func TestClassifyStructuralBotByLoginPattern(t *testing.T) {
	// Login matching *[bot] pattern without a curated ruleset entry
	// should return "bot".
	got := recognition.Classify("some-unknown-bot[bot]", "")
	if got != "bot" {
		t.Errorf("Classify(%q, \"\") = %q, want %q", "some-unknown-bot[bot]", got, "bot")
	}
}

func TestClassifyHumanReturnsEmpty(t *testing.T) {
	// A regular user (no [bot] login, no Bot type) should return "".
	got := recognition.Classify("octocat", "")
	if got != "" {
		t.Errorf("Classify(%q, \"\") = %q, want %q", "octocat", got, "")
	}
}

func TestClassifyEmptyLoginReturnsEmpty(t *testing.T) {
	got := recognition.Classify("", "")
	if got != "" {
		t.Errorf("Classify(%q, \"\") = %q, want %q", "", got, "")
	}
}

func TestVersionNonZero(t *testing.T) {
	if v := recognition.Version(); v == 0 {
		t.Error("Version() = 0, want > 0")
	}
}

func TestVersionConsistent(t *testing.T) {
	v1 := recognition.Version()
	v2 := recognition.Version()
	if v1 != v2 {
		t.Errorf("Version() returned different values: %d then %d", v1, v2)
	}
}
