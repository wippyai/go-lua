// Package ref defines stable function identities for analysis summaries.
package ref

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// Kind identifies the namespace used by a FuncRef.
type Kind uint8

const (
	KindZero Kind = iota
	KindCFG
	KindSymbol
	KindRoot
)

// FuncRef is a compact, comparable function identity.
//
// CFG-backed refs are process-local and suitable for in-memory analysis
// summaries. Symbol-backed refs can name lexical function declarations when the
// caller has that identity available.
type FuncRef struct {
	Kind Kind
	ID   uint64
}

// Zero returns the zero function reference.
func Zero() FuncRef {
	return FuncRef{}
}

// FromCFG returns a reference for g. Nil graphs produce the zero reference.
func FromCFG(g cfg.Graph) FuncRef {
	if g == nil || g.ID() == 0 {
		return FuncRef{}
	}
	return FuncRef{Kind: KindCFG, ID: g.ID()}
}

// FromSymbol returns a reference for a function symbol. ID 0 produces zero.
func FromSymbol(id symbol.ID) FuncRef {
	if id == 0 {
		return FuncRef{}
	}
	return FuncRef{Kind: KindSymbol, ID: uint64(id)}
}

// Root returns the in-memory root reference for a chunk or entry body.
func Root() FuncRef {
	return FuncRef{Kind: KindRoot, ID: 1}
}

// IsZero reports whether r is the zero function reference.
func (r FuncRef) IsZero() bool {
	return r.Kind == KindZero || r.ID == 0
}

// Less reports whether r sorts before other.
func (r FuncRef) Less(other FuncRef) bool {
	if r.Kind != other.Kind {
		return r.Kind < other.Kind
	}
	return r.ID < other.ID
}

func (r FuncRef) String() string {
	if r.IsZero() {
		return "func:zero"
	}
	switch r.Kind {
	case KindCFG:
		return fmt.Sprintf("func:cfg:%d", r.ID)
	case KindSymbol:
		return fmt.Sprintf("func:symbol:%d", r.ID)
	case KindRoot:
		return fmt.Sprintf("func:root:%d", r.ID)
	default:
		return fmt.Sprintf("func:kind%d:%d", r.Kind, r.ID)
	}
}
