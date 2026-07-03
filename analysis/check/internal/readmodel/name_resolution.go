package readmodel

import (
	"strconv"

	"github.com/wippyai/go-lua/analysis/check/body"
)

// ForEachUnusedLocal visits local bindings whose symbol has no reachable read.
func (r Reader) ForEachUnusedLocal(visit func(UnusedLocal) bool) bool {
	if r.result == nil || visit == nil || r.result.Graph() == nil {
		return false
	}
	readsByPoint := r.result.ReachableSymbolReads()
	visited := false
	for _, binding := range r.result.LocalBindingOccurrences() {
		if readsByPoint.Has(binding.Symbol) {
			continue
		}
		item := UnusedLocal{
			Point: binding.Point,
			Name:  binding.Name,
			Key:   strconv.Itoa(int(binding.Symbol)),
			Span:  sourceSpanFromBody(binding.Span),
		}
		visited = true
		if !visit(item) {
			return true
		}
	}
	return visited
}

// ForEachUnresolvedValueReference visits reachable identifier reads that remain
// implicit globals after binding and are not known type-syntax references.
func (r Reader) ForEachUnresolvedValueReference(visit func(UnresolvedValueReference) bool) bool {
	if visit == nil || r.result == nil || r.result.Graph() == nil {
		return false
	}
	visited := false
	return r.result.ForEachUnresolvedValueReferenceOccurrence(func(occ body.UnresolvedValueReferenceOccurrence) bool {
		visited = true
		return visit(UnresolvedValueReference{
			Point: occ.Point,
			Name:  occ.Name,
			Key:   occ.Key,
			Span:  sourceSpanFromBody(occ.Span),
		})
	}) || visited
}

// ForEachUnresolvedTypeReference visits annotation type names that did not bind
// in the lexical/module type namespace. The query deliberately reports binding
// facts, not lowered type strings, so obligation producers do not need a second
// annotation resolver.
func (r Reader) ForEachUnresolvedTypeReference(visit func(UnresolvedTypeReference) bool) bool {
	if visit == nil || r.result == nil {
		return false
	}
	visited := false
	return r.result.ForEachUnresolvedTypeReferenceOccurrence(r.parents, func(occ body.UnresolvedTypeReferenceOccurrence) bool {
		visited = true
		return visit(UnresolvedTypeReference{
			Point: occ.Point,
			Name:  occ.Name,
			Key:   occ.Key,
			Span:  sourceSpanFromBody(occ.Span),
		})
	}) || visited
}
