package execution

import (
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
)

// ProgramSource is the row-local capability a rule whose candidates are
// Program rows carries: the immutable publication its candidate ordinal
// addresses, and that ordinal.
//
// It is per row rather than per family because one generated Family holds rows
// from every mounted Program that placed the rule, and equal ordinals from
// different mounts are different rows. A row of a rule whose candidates come
// from a Factor axis carries the zero value and resolves through its axis
// owner as before.
//
// The value is opaque to the engine: it transports the capability and reads
// nothing through it. A typed child redeems it against the Program family it
// expects and no other.
type ProgramSource struct {
	state   programstate.State
	ordinal uint32
}

// NewProgramSource seals one row's capability. Only an authenticated
// publication can back a candidate ordinal, so an unavailable state is refused
// rather than carried as an empty capability beside a live ordinal.
func NewProgramSource(state programstate.State, ordinal uint32) (ProgramSource, bool) {
	if !state.Available() {
		return ProgramSource{}, false
	}
	return ProgramSource{state: state, ordinal: ordinal}, true
}

// Available reports whether this row carries a Program candidate row.
func (source ProgramSource) Available() bool { return source.state.Available() }

// State is the immutable publication the ordinal addresses. A typed child
// opens its own view over it; the engine never reads a row through it.
func (source ProgramSource) State() programstate.State { return source.state }

// Ordinal is the dense candidate row this placement was issued for. It is the
// same value FormRow.Candidate carries, restated here so a child that holds
// the capability holds the whole address.
func (source ProgramSource) Ordinal() (uint32, bool) {
	if !source.Available() {
		return 0, false
	}
	return source.ordinal, true
}
