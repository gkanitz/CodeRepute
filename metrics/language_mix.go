package metrics

import (
	"math"
	"sort"

	"github.com/gkanitz/coderepute/provider"
	"github.com/gkanitz/coderepute/report"
)

func init() {
	Register("language_mix", computeLanguageMix)
}

// extLang maps canonical file extensions to their language or bucket name.
// Extensions are lowercase, no leading dot. The empty string ("") maps to
// Other. Bucket categories: Config, Docs, Other.
var extLang = map[string]string{
	// Common programming languages (~40)
	"go":            "Go",
	"rs":            "Rust",
	"py":            "Python",
	"js":            "JavaScript",
	"jsx":           "JavaScript",
	"ts":            "TypeScript",
	"tsx":           "TypeScript",
	"java":          "Java",
	"kt":            "Kotlin",
	"kts":           "Kotlin",
	"scala":         "Scala",
	"rb":            "Ruby",
	"php":           "PHP",
	"c":             "C",
	"h":             "C",
	"cpp":           "C++",
	"cc":            "C++",
	"cxx":           "C++",
	"hpp":           "C++",
	"hxx":           "C++",
	"cs":            "C#",
	"fs":            "F#",
	"swift":         "Swift",
	"m":             "Objective-C",
	"mm":            "Objective-C",
	"sh":            "Shell",
	"bash":          "Shell",
	"zsh":           "Shell",
	"ps1":           "PowerShell",
	"pl":            "Perl",
	"pm":            "Perl",
	"lua":           "Lua",
	"r":             "R",
	"dart":          "Dart",
	"elm":           "Elm",
	"clj":           "Clojure",
	"cljs":          "Clojure",
	"ex":            "Elixir",
	"exs":           "Elixir",
	"erl":           "Erlang",
	"hrl":           "Erlang",
	"hs":            "Haskell",
	"sql":           "SQL",
	"graphql":       "GraphQL",
	"gql":           "GraphQL",
	"proto":         "Protobuf",
	"zig":           "Zig",
	"vue":           "Vue",
	"svelte":        "Svelte",
	"astro":         "Astro",
	"dockerfile":    "Docker",
	"tf":            "Terraform",
	"tfvars":        "Terraform",
	"hcl":           "Terraform",
	"yaml":          "Config",
	"yml":           "Config",
	"json":          "Config",
	"toml":          "Config",
	"ini":           "Config",
	"cfg":           "Config",
	"conf":          "Config",
	"xml":           "Config",
	"plist":         "Config",
	"env":           "Config",
	"editorconfig":  "Config",
	"gitattributes": "Config",
	"gitignore":     "Config",
	"dockerignore":  "Config",
	"svg":           "Config",
	"md":            "Docs",
	"rst":           "Docs",
	"adoc":          "Docs",
	"asciidoc":      "Docs",
	"txt":           "Docs",
	"rtf":           "Docs",
	"org":           "Docs",
	"mkdn":          "Docs",
	"mdwn":          "Docs",
	"mdown":         "Docs",
	"markdown":      "Docs",
}

// LangForExt returns the language name for a canonical extension. Empty
// string and unknown extensions map to "Other".
func LangForExt(ext string) string {
	if ext == "" {
		return "Other"
	}
	if lang, ok := extLang[ext]; ok {
		return lang
	}
	return "Other"
}

// computeLanguageMix computes the language mix from the subject's merged PRs
// that have per-file diff-shape data (FileStats). Weight = additions + deletions
// per file. Languages with < 3% share fold into Other. Suppressed when fewer
// than 5 merged PRs have FileStats data.
func computeLanguageMix(as provider.ActivitySet, res *Result) {
	// Collect FileStats from merged PRs.
	type weightedFile struct {
		lang   string
		weight int
	}

	var files []weightedFile
	prWithData := 0
	totalLines := 0

	for _, pr := range as.PullRequests {
		if pr.MergedAt == nil {
			continue
		}
		if len(pr.FileStats) == 0 {
			continue
		}
		prWithData++
		for _, fs := range pr.FileStats {
			w := fs.Additions + fs.Deletions
			if w == 0 {
				continue
			}
			lang := LangForExt(fs.Ext)
			files = append(files, weightedFile{lang: lang, weight: w})
			totalLines += w
		}
	}

	if prWithData < 5 {
		if prWithData > 0 {
			res.Collaboration.Suppressed = append(res.Collaboration.Suppressed, report.SuppressedEntry{
				Section: "language_mix",
				Reason:  "sample too small: only " + plural(prWithData, "merged PR") + " with diff data (need ≥ 5)",
			})
		}
		return
	}

	if totalLines == 0 {
		return
	}

	// Aggregate by language.
	langWeight := make(map[string]int)
	for _, f := range files {
		langWeight[f.lang] += f.weight
	}

	// Build share list, applying 3% folding threshold.
	threshold := 3.0
	var shown []report.LangShare
	otherWeight := 0

	// Sort languages by weight descending for deterministic output.
	type langWeightPair struct {
		name   string
		weight int
	}
	var pairs []langWeightPair
	for name, w := range langWeight {
		pairs = append(pairs, langWeightPair{name: name, weight: w})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].weight != pairs[j].weight {
			return pairs[i].weight > pairs[j].weight
		}
		return pairs[i].name < pairs[j].name
	})

	for _, p := range pairs {
		share := float64(p.weight) / float64(totalLines) * 100
		rounded := math.Round(share)
		if rounded >= threshold {
			shown = append(shown, report.LangShare{
				Name:     p.name,
				SharePct: rounded,
			})
		} else {
			otherWeight += p.weight
		}
	}

	otherShare := math.Round(float64(otherWeight) / float64(totalLines) * 100)

	res.Collaboration.LanguageMix = &report.LanguageMixStats{
		Basis:      "merged_pr_diff_lines",
		PRCount:    prWithData,
		TotalLines: totalLines,
		Languages:  shown,
		OtherShare: otherShare,
	}
}
