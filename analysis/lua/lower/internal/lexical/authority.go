// Package lexical owns atomic Body publication, symbol visibility, and closures.
package lexical

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	modulelower "github.com/wippyai/go-lua/analysis/lua/lower/internal/module"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Bodies is the one atomic authority for lexical scopes and their Program roots.
// Its fields stay private so restoring symbol visibility and publishing a Body
// cannot be split across packages.
type Bodies struct {
	collector  *assembly.Collector
	binding    *bind.Result
	sourceName string
	modules    *modulelower.Writer
	phases     *continuation.Stack
	values     *eval.Values
	statics    *continuation.Statics
	localSteps []localStep

	active map[bind.Symbol]keyspace.Term
	undo   []activeUndo
	frames []bodyFrame
	source []sourceItem

	cellInline   [4]keyspace.Term
	cellOverflow []keyspace.Term
	cellLen      int
	chunkVararg  keyspace.Term
	reserved     []reservedCell
	reservedIDs  map[bind.Symbol]struct{}
	captures     []flow.Capture
}

// New creates the one lexical authority for one unfinished Program. It binds
// the sole phase stack once; copied lexical state or caller-supplied alternate
// stacks could split Cell visibility from Body publication and are forbidden.
func New(
	phases *continuation.Stack,
	collector *assembly.Collector,
	binding *bind.Result,
	sourceName string,
	modules *modulelower.Writer,
	values *eval.Values,
	statics *continuation.Statics,
) *Bodies {
	return &Bodies{
		collector:  collector,
		binding:    binding,
		sourceName: sourceName,
		modules:    modules,
		phases:     phases,
		values:     values,
		statics:    statics,
	}
}

// Clean reports whether every lexical transaction and scratch range completed.
func (b *Bodies) Clean() bool {
	return len(b.frames) == 0 &&
		len(b.source) == 0 &&
		len(b.active) == 0 &&
		len(b.undo) == 0 &&
		b.cellLen == 0 &&
		len(b.cellOverflow) == 0 &&
		len(b.reserved) == 0 &&
		len(b.reservedIDs) == 0 &&
		len(b.captures) == 0 &&
		len(b.localSteps) == 0
}
