// Package metrics turns an ActivitySet into collaboration metrics.
// It is pure: no I/O, deterministic output.
//
// Each metric concern lives in its own file and registers itself into the
// package registry from an init func; follow-up concerns extend the
// pipeline by adding files, not editing this one.
package metrics

import (
	"fmt"
	"sort"

	"github.com/grkanitz/coderepute/provider"
	"github.com/grkanitz/coderepute/report"
)

// Result is the computed metric sections of a report.
type Result struct {
	Collaboration report.Collaboration
	Cadence       report.Cadence
}

// Func computes one metric concern from an ActivitySet into the result.
type Func func(provider.ActivitySet, *Result)

var registry = map[string]Func{}

// Register adds a metric concern to the registry. It panics on duplicate
// names; concerns register from init funcs, one file per concern.
func Register(name string, fn Func) {
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("metrics: duplicate registration of %q", name))
	}
	registry[name] = fn
}

// Compute runs every registered metric concern over the activity set.
func Compute(as provider.ActivitySet) Result {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)

	var res Result
	for _, name := range names {
		registry[name](as, &res)
	}
	return res
}
