package modulecomposition

import "github.com/wippyai/go-lua/analysis/identity"

// Composition is the immutable Link-owned module-composition catalog. It is
// built once before rule binding, consumed by domain binders, and published
// from the same sealed rows. Keeping those phases on one owner prevents a
// post-bind reconstruction from becoming a second authority.
type Composition struct {
	link        identity.ContentID
	imports     []ResolvedImport
	caches      []CacheIngress
	transitions []ModuleCallTransition
	generations []InitGeneration
	outcomes    []InitOutcome
	stateEdges  []ModuleReturnStateEdge
	terminals   []InitTerminal
	origins     []ModuleExportCallableOrigin
	ingresses   []ModuleExportCallableIngress
}

// SealComposition authenticates and freezes one complete Link catalog.
func SealComposition(
	link identity.ContentID,
	imports []ResolvedImport,
	caches []CacheIngress,
	transitions []ModuleCallTransition,
	generations []InitGeneration,
	outcomes []InitOutcome,
	stateEdges []ModuleReturnStateEdge,
	terminals []InitTerminal,
	origins []ModuleExportCallableOrigin,
	ingresses []ModuleExportCallableIngress,
) (Composition, bool) {
	if !link.Available() ||
		!allRows(imports, func(row ResolvedImport) bool { return row.Available() && row.LinkID() == link }) ||
		!allRows(caches, func(row CacheIngress) bool { return row.Available() && row.LinkID() == link }) ||
		!allRows(transitions, func(row ModuleCallTransition) bool { return row.Available() && row.LinkID() == link }) ||
		!allRows(generations, func(row InitGeneration) bool { return row.Available() && row.LinkID() == link }) ||
		!allRows(outcomes, func(row InitOutcome) bool { return row.Available() && row.LinkID() == link }) ||
		!allRows(stateEdges, func(row ModuleReturnStateEdge) bool { return row.Available() && row.LinkID() == link }) ||
		!allRows(terminals, func(row InitTerminal) bool { return row.Available() && row.LinkID() == link }) ||
		!allRows(origins, func(row ModuleExportCallableOrigin) bool { return row.Available() && row.LinkID() == link }) {
		return Composition{}, false
	}
	if !allRows(ingresses, func(row ModuleExportCallableIngress) bool { return row.Available() && row.LinkID() == link }) {
		return Composition{}, false
	}
	return Composition{
		link:        link,
		imports:     append([]ResolvedImport(nil), imports...),
		caches:      append([]CacheIngress(nil), caches...),
		transitions: append([]ModuleCallTransition(nil), transitions...),
		generations: append([]InitGeneration(nil), generations...),
		outcomes:    append([]InitOutcome(nil), outcomes...),
		stateEdges:  append([]ModuleReturnStateEdge(nil), stateEdges...),
		terminals:   append([]InitTerminal(nil), terminals...),
		origins:     append([]ModuleExportCallableOrigin(nil), origins...),
		ingresses:   append([]ModuleExportCallableIngress(nil), ingresses...),
	}, true
}

func allRows[T any](rows []T, valid func(T) bool) bool {
	for _, row := range rows {
		if !valid(row) {
			return false
		}
	}
	return true
}

// Available reports whether this catalog was sealed for one Link.
func (composition Composition) Available() bool { return composition.link.Available() }

// LinkID returns the exact Link owner.
func (composition Composition) LinkID() identity.ContentID {
	if !composition.Available() {
		return identity.ContentID{}
	}
	return composition.link
}

func (composition Composition) Imports() []ResolvedImport {
	return append([]ResolvedImport(nil), composition.imports...)
}

func (composition Composition) Caches() []CacheIngress {
	return append([]CacheIngress(nil), composition.caches...)
}

func (composition Composition) Transitions() []ModuleCallTransition {
	return append([]ModuleCallTransition(nil), composition.transitions...)
}

func (composition Composition) Generations() []InitGeneration {
	return append([]InitGeneration(nil), composition.generations...)
}

func (composition Composition) Outcomes() []InitOutcome {
	return append([]InitOutcome(nil), composition.outcomes...)
}

func (composition Composition) StateEdges() []ModuleReturnStateEdge {
	return append([]ModuleReturnStateEdge(nil), composition.stateEdges...)
}

func (composition Composition) Terminals() []InitTerminal {
	return append([]InitTerminal(nil), composition.terminals...)
}

func (composition Composition) CallableOrigins() []ModuleExportCallableOrigin {
	return append([]ModuleExportCallableOrigin(nil), composition.origins...)
}

func (composition Composition) CallableIngresses() []ModuleExportCallableIngress {
	return append([]ModuleExportCallableIngress(nil), composition.ingresses...)
}
