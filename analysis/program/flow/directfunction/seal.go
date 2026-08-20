package directfunction

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal proves occurrence-specific direct Function identity. Source Bind order
// is paired with each evaluated Values position (including nil-fill and open
// tails); every lexical Assign write is counted at its terminal capture Cell;
// and source-control strict dominance decides each dynamic occurrence.
// Construction work is discarded before the dense Read/Call/GenericFor
// projection is returned.
func Seal(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	bindings binding.Result,
	forest *containment.Result,
	control *sourcecontrol.Result,
	executableResult *executable.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	counts, err := validateOwners(sourceView, flow, staticID, moduleID, bodies, bindings, forest, control, executableResult)
	if err != nil {
		return nil, err
	}

	cellCount := int(counts[keyspace.FamilyCell])
	captureOuter := make([]keyspace.Term, cellCount+1)
	if err := validateCells(flow, bindings, counts); err != nil {
		return nil, err
	}
	if err := validateFunctions(flow, bodies, bindings, counts, captureOuter); err != nil {
		return nil, err
	}
	terminal, err := terminalCells(captureOuter)
	if err != nil {
		return nil, err
	}
	if err := validateOccurrences(flow, bodies, bindings, forest, counts); err != nil {
		return nil, err
	}
	functionForCell, functionOrigin, cellForFunction, recursiveSelf, err := buildInstallations(
		sourceView, flow, bodies, bindings, counts, terminal, forest, executableResult,
	)
	if err != nil {
		return nil, err
	}

	proof := &solver{
		source:          sourceView,
		flow:            flow,
		bodies:          bodies,
		bindings:        bindings,
		forest:          forest,
		control:         control,
		executable:      executableResult,
		counts:          counts,
		terminal:        terminal,
		functionForCell: functionForCell,
		functionOrigin:  functionOrigin,
		cellForFunction: cellForFunction,
		recursiveSelf:   recursiveSelf,
	}

	result := &Result{
		sourceID:      sourceView.Identity().ContentID(),
		flowID:        flow.Cold().ContentID(),
		staticID:      staticID,
		moduleID:      moduleID,
		readFunctions: make([]keyspace.Term, int(counts[keyspace.FamilyRead])+1),
		callFunctions: make([]keyspace.Term, int(counts[keyspace.FamilyCall])+1),
		loopFunctions: make([]keyspace.Term, int(counts[keyspace.FamilyLoop])+1),
		functionCount: counts[keyspace.FamilyFunction],
	}
	if err := proof.populate(result); err != nil {
		return nil, err
	}
	return result, nil
}
