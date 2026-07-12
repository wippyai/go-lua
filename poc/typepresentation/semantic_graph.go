package typepresentation

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

// TypePair is the eager representation: presentation and semantic nodes are
// materialized together bottom-up, so selecting either graph is O(1).
type TypePair struct {
	Presentation typ.Type
	Semantic     typ.Type
}

type PairedField struct {
	Name     string
	Type     TypePair
	Optional bool
	Readonly bool
}

func PairFunction(fn *typ.Function) TypePair {
	if fn == nil {
		return TypePair{}
	}
	return TypePair{Presentation: fn, Semantic: fn.SemanticType()}
}

func PairRecord(fields []PairedField) TypePair {
	presented := make([]typ.Field, len(fields))
	semantic := make([]typ.Field, len(fields))
	for i, field := range fields {
		presented[i] = typ.Field{Name: field.Name, Type: field.Type.Presentation, Optional: field.Optional, Readonly: field.Readonly}
		semantic[i] = typ.Field{Name: field.Name, Type: field.Type.Semantic, Optional: field.Optional, Readonly: field.Readonly}
	}
	return TypePair{
		Presentation: typ.RebuildRecord(typ.RecordParts{Fields: presented}),
		Semantic:     typ.RebuildRecord(typ.RecordParts{Fields: semantic}),
	}
}

func PairUnion(members ...TypePair) TypePair {
	presented := make([]typ.Type, len(members))
	semantic := make([]typ.Type, len(members))
	for i, member := range members {
		presented[i] = member.Presentation
		semantic[i] = member.Semantic
	}
	return TypePair{Presentation: typ.MaterializeUnion(presented), Semantic: typ.MaterializeUnion(semantic)}
}

// PairedRecursive creates both placeholders before either body is installed,
// preserving recursive identity in both graphs without a post-construction
// traversal.
type PairedRecursive struct {
	presentation *typ.Recursive
	semantic     *typ.Recursive
}

func NewPairedRecursive(name string) *PairedRecursive {
	return &PairedRecursive{
		presentation: typ.NewRecursivePlaceholder(name),
		semantic:     typ.NewRecursivePlaceholder(name),
	}
}

func (r *PairedRecursive) Refs() TypePair {
	if r == nil {
		return TypePair{}
	}
	return TypePair{Presentation: r.presentation, Semantic: r.semantic}
}

func (r *PairedRecursive) SetBody(body TypePair) {
	if r == nil {
		return
	}
	r.presentation.SetBody(body.Presentation)
	r.semantic.SetBody(body.Semantic)
}

type semanticBox struct{ value typ.Type }

// LazySemanticGraph stores only the presentation root until semantic selection
// is requested. Selection is O(1) after the first graph walk.
type LazySemanticGraph struct {
	presentation typ.Type
	semantic     atomic.Pointer[semanticBox]
}

func NewLazySemanticGraph(presentation typ.Type) *LazySemanticGraph {
	return &LazySemanticGraph{presentation: presentation}
}

func (g *LazySemanticGraph) Presentation() typ.Type {
	if g == nil {
		return nil
	}
	return g.presentation
}

func (g *LazySemanticGraph) Semantic() typ.Type {
	if g == nil {
		return nil
	}
	if box := g.semantic.Load(); box != nil {
		return box.value
	}
	candidate := &semanticBox{value: semanticizeGraph(g.presentation, make(map[typ.Type]typ.Type))}
	if g.semantic.CompareAndSwap(nil, candidate) {
		return candidate.value
	}
	return g.semantic.Load().value
}

func semanticizeGraph(t typ.Type, memo map[typ.Type]typ.Type) typ.Type {
	if t == nil {
		return nil
	}
	if got := memo[t]; got != nil {
		return got
	}
	switch node := t.(type) {
	case *typ.Function:
		return node.SemanticType()
	case *typ.Record:
		fields := make([]typ.Field, len(node.Fields))
		for i, field := range node.Fields {
			field.Type = semanticizeGraph(field.Type, memo)
			fields[i] = field
		}
		semantic := typ.RebuildRecord(typ.RecordParts{Fields: fields, Open: node.Open})
		memo[t] = semantic
		return semantic
	case *typ.Union:
		members := make([]typ.Type, len(node.Members))
		for i, member := range node.Members {
			members[i] = semanticizeGraph(member, memo)
		}
		semantic := typ.MaterializeUnion(members)
		memo[t] = semantic
		return semantic
	case *typ.Recursive:
		placeholder := typ.NewRecursivePlaceholder(node.Name)
		memo[t] = placeholder
		placeholder.SetBody(semanticizeGraph(node.Body, memo))
		return placeholder
	default:
		return t
	}
}
