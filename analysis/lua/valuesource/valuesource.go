package valuesource

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// Kind classifies where a Lua value-list slot comes from.
type Kind uint8

// Value source kinds describe how a value was produced.
const (
	Unknown Kind = iota
	Expression
	Call
	Vararg
	Nil
)

// NoIndex marks an index field that does not point at a source.
const NoIndex = -1

// Source describes Lua AST provenance for one value-list slot.
type Source struct {
	Kind Kind
	Expr ast.Expr

	ExprIndex    int
	TargetIndex  int
	ResultIndex  int
	CallPoint    cfg.Point
	HasCallPoint bool

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}
