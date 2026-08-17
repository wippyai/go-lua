// Package storage owns evaluated storage identities and mutation construction.
package storage

import (
	"github.com/wippyai/go-lua/analysis/lua/bind"
	lowercollector "github.com/wippyai/go-lua/analysis/lua/lower/internal/assembly"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/eval"
	"github.com/wippyai/go-lua/analysis/lua/lower/internal/lexical"
	staticlower "github.com/wippyai/go-lua/analysis/lua/lower/internal/static"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TargetMark is an opaque boundary around one ordered assignment-target range.
// A caller can retain and pass it back, but cannot manufacture a raw scratch
// offset or inspect the private range it denotes.
type TargetMark struct {
	owner  *Writer
	target int
}

// TableMark is an opaque boundary around one nested table-constructor range.
// Keys, field kinds, and completed fields always move together.
type TableMark struct {
	owner            *Writer
	key, kind, field int
}

// Writer is the sole lowering authority for storage selection, reads, lenses,
// writes, and table-field construction. It has direct concrete dependencies;
// it does not call back into a parent lowerer.
type Writer struct {
	stack       *continuation.Stack
	binding     *bind.Result
	lexical     *lexical.Bodies
	values      *eval.Values
	expressions *continuation.Expressions
	static      *staticlower.Writer
	collector   *lowercollector.Collector
	sourceName  string

	targets     []keyspace.Term
	targetSpans []source.Span
	tableKeys   []keyspace.Term
	tableKinds  []flowkind.FieldKind
	tableFields []keyspace.Term
	steps       []step
}

// New creates the one storage authority for a source Program assembly.
func New(
	stack *continuation.Stack,
	binding *bind.Result,
	lexical *lexical.Bodies,
	values *eval.Values,
	expressions *continuation.Expressions,
	static *staticlower.Writer,
	collector *lowercollector.Collector,
	sourceName string,
) *Writer {
	return &Writer{
		stack:       stack,
		binding:     binding,
		lexical:     lexical,
		values:      values,
		expressions: expressions,
		static:      static,
		collector:   collector,
		sourceName:  sourceName,
	}
}
