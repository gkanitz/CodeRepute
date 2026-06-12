package report_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/grkanitz/coderepute/report"
)

func TestValidateRejections(t *testing.T) {
	valid := func() report.Report {
		return report.Build(activityFixture(), nil, nil, time.Now())
	}

	tests := []struct {
		name    string
		mutate  func(*report.Report)
		wantErr string
	}{
		{
			name:    "missing coverage stamp",
			mutate:  func(r *report.Report) { r.Coverage = nil },
			wantErr: "coverage",
		},
		{
			name:    "missing verification block",
			mutate:  func(r *report.Report) { r.Verification = nil },
			wantErr: "verification",
		},
		{
			name:    "coverage without window",
			mutate:  func(r *report.Report) { r.Coverage.Window = report.Window{} },
			wantErr: "window",
		},
		{
			name:    "inverted window",
			mutate:  func(r *report.Report) { r.Coverage.Window.Since, r.Coverage.Window.Until = r.Coverage.Window.Until, r.Coverage.Window.Since },
			wantErr: "since",
		},
		{
			name:    "unknown verification status",
			mutate:  func(r *report.Report) { r.Verification.Status = "trust-me" },
			wantErr: "status",
		},
		{
			name:    "unsupported schema version",
			mutate:  func(r *report.Report) { r.SchemaVersion = "v99" },
			wantErr: "schema_version",
		},
		{
			name:    "subject without account id",
			mutate:  func(r *report.Report) { r.Subject.AccountID = "" },
			wantErr: "account_id",
		},
		{
			name:    "subject without platform",
			mutate:  func(r *report.Report) { r.Subject.Platform = "" },
			wantErr: "subject",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := valid()
			tt.mutate(&r)
			err := r.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}

			raw, merr := json.Marshal(r)
			if merr != nil {
				t.Fatalf("marshal: %v", merr)
			}
			if _, perr := report.Parse(raw); perr == nil {
				t.Error("Parse accepted an invalid document")
			}
		})
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := report.Parse([]byte(`{not json`)); err == nil {
		t.Error("Parse accepted malformed JSON")
	}
	if _, err := report.Parse([]byte(`{}`)); err == nil {
		t.Error("Parse accepted an empty document")
	}
}
