// Package recognition provides an embedded, versioned ruleset for classifying
// PR/MR authors as human, bot, or a recognized AI agent. The ruleset follows
// the same go:embed pattern as metrics/bands/bands.json.
package recognition

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed airuleset.json
var rulesetFS embed.FS

// Entry maps a known agent login to its canonical agent id.
type Entry struct {
	Login string `json:"login"`
	Agent string `json:"agent"`
}

// Ruleset is the top-level structure of the embedded airuleset.json.
type Ruleset struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

var (
	once     sync.Once
	loginMap map[string]string // login -> canonical agent id
	version  int
)

// load reads and parses the embedded airuleset.json once.
func load() {
	raw, err := rulesetFS.ReadFile("airuleset.json")
	if err != nil {
		panic(fmt.Sprintf("recognition: embedded airuleset.json: %v", err))
	}
	var rs Ruleset
	if err := json.Unmarshal(raw, &rs); err != nil {
		panic(fmt.Sprintf("recognition: parse embedded airuleset.json: %v", err))
	}
	version = rs.Version
	loginMap = make(map[string]string, len(rs.Entries))
	for _, e := range rs.Entries {
		loginMap[strings.ToLower(e.Login)] = e.Agent
	}
}

// agentIDForLogin returns the canonical agent id for a known agent login, or
// false if the login is not in the curated ruleset.
func agentIDForLogin(login string) (string, bool) {
	once.Do(load)
	aid, ok := loginMap[strings.ToLower(login)]
	return aid, ok
}

// Classify returns the canonical agent id when the given login+type matches a
// curated ruleset entry, "bot" when the match is structural (GitHub type:"Bot"
// or *[bot] login pattern), or "" for a human author. The login comparison is
// case-insensitive.
func Classify(login, userType string) string {
	if login == "" {
		return ""
	}
	// Layer 1: curated agent ruleset.
	if aid, ok := agentIDForLogin(login); ok {
		return aid
	}
	// Layer 2: structural bot-type.
	// GitHub API returns type:"Bot" for bot accounts.
	if userType == "Bot" {
		return "bot"
	}
	// GitHub bot logins follow the *[bot] pattern (e.g. "copilot[bot]").
	// This also catches ruleset entries that didn't match above, though
	// that's redundant — ruleset entries use [bot] logins.
	if strings.HasSuffix(login, "[bot]") {
		return "bot"
	}
	return ""
}

// Version returns the embedded airuleset.json version number.
func Version() int {
	once.Do(load)
	return version
}
