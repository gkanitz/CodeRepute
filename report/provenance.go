package report

import (
	"fmt"
	"time"
)

// SLSABuildTypeURI identifies the CodeRepute report build process for SLSA v1.2
// provenance. It is stamped into every provenance block produced in CI.
const SLSABuildTypeURI = "https://coderepute.dev/buildTypes/report@v1"

// SLSAProvenance carries SLSA v1.2 provenance fields describing the CI build
// that produced the report. The struct is nil (omitted from JSON) when the
// report was not generated in a recognized CI environment.
type SLSAProvenance struct {
	BuildType            string           `json:"build_type"`
	BuilderID            string           `json:"builder_id"`
	InvocationID         string           `json:"invocation_id"`
	StartedOn            *time.Time       `json:"started_on,omitempty"`
	FinishedOn           *time.Time       `json:"finished_on,omitempty"`
	ResolvedDependencies []SLSADependency `json:"resolved_dependencies,omitempty"`
}

// SLSADependency is one entry in the resolvedDependencies array of SLSA v1.2
// provenance. It carries a URI identifying the dependency.
type SLSADependency struct {
	URI string `json:"uri"`
}

// CIProvenance inspects the process environment via getenv and returns a
// populated SLSAProvenance block when GITHUB_ACTIONS is true, or nil otherwise.
// finishedAt is used as the provenance finishedOn timestamp.
// codeReputeVersion is the building binary's version string, used to construct
// a resolved dependency URI pointing at the CodeRepute release. When empty,
// resolved_dependencies is omitted.
func CIProvenance(getenv func(string) string, finishedAt time.Time, codeReputeVersion string) *SLSAProvenance {
	if getenv("GITHUB_ACTIONS") != "true" {
		return nil
	}
	server := getenv("GITHUB_SERVER_URL")
	repo := getenv("GITHUB_REPOSITORY")
	runID := getenv("GITHUB_RUN_ID")
	workflowRef := getenv("GITHUB_WORKFLOW_REF")

	invocationID := ""
	if server != "" && repo != "" && runID != "" {
		invocationID = fmt.Sprintf("%s/%s/actions/runs/%s", server, repo, runID)
	}

	builderID := ""
	if server != "" && workflowRef != "" {
		builderID = server + "/" + workflowRef
	}

	var deps []SLSADependency
	if codeReputeVersion != "" {
		deps = []SLSADependency{
			{URI: fmt.Sprintf("https://github.com/gkanitz/CodeRepute@%s", codeReputeVersion)},
		}
	}

	finishedUTC := finishedAt.UTC()

	return &SLSAProvenance{
		BuildType:            SLSABuildTypeURI,
		BuilderID:            builderID,
		InvocationID:         invocationID,
		FinishedOn:           &finishedUTC,
		ResolvedDependencies: deps,
	}
}
