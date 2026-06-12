package report_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/grkanitz/coderepute/report"
)

func TestBuildStampsTokenScopeClass(t *testing.T) {
	r := report.Build(activityFixture(), nil, nil, time.Now(),
		report.WithTokenScopeClass("app-installation"))

	if r.Coverage == nil || r.Coverage.TokenScopeClass != "app-installation" {
		t.Fatalf("coverage = %+v, want token_scope_class app-installation", r.Coverage)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("built report failed validation: %v", err)
	}

	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"token_scope_class":"app-installation"`) {
		t.Errorf("serialized report missing token_scope_class: %s", raw)
	}
	parsed, err := report.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Coverage.TokenScopeClass != "app-installation" {
		t.Errorf("round-trip lost token_scope_class: %+v", parsed.Coverage)
	}
}

func TestValidateRejectsEmptyCoverageRepoList(t *testing.T) {
	r := report.Build(activityFixture(), nil, nil, time.Now())
	r.Coverage.Repos = nil

	err := r.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error for empty coverage repo list")
	}
	if !strings.Contains(err.Error(), "repo") {
		t.Errorf("error %q does not mention the repo list", err)
	}
}
