package operationplan

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// DirectLexicalDeclaration is external whole-unit evidence that one local
// function value is used only as the complete set of direct lexical calls.
// The plan independently seals its expression/function/target structure.
type DirectLexicalDeclaration struct {
	Expression factflow.ExprRef
	Function   symbol.ID
	Target     symbol.ID
}

// DirectLexicalDeclarations is an opaque authority tied by pointer identity to
// the immutable plan it certifies. The zero value is not a proof.
type DirectLexicalDeclarations struct {
	owner    *Plan
	entries  []DirectLexicalDeclaration
	complete bool
}

// SealDirectLexicalDeclarations seals the complete binder-selected direct-call
// subset. Function expressions outside that subset remain ordinary function
// values; they are not evidence that the direct-call subset is incomplete.
// Duplicate, annotated, non-local, or structurally mismatched entries return
// the zero authority. Escape and call-set completeness are established by the
// binder before this method is called; sealing fences that evidence to the
// exact immutable fact snapshot consumed by the compiler.
func SealDirectLexicalDeclarations(p *Plan, input []DirectLexicalDeclaration) DirectLexicalDeclarations {
	if p == nil {
		return DirectLexicalDeclarations{}
	}

	functions := make(map[factflow.ExprRef]symbol.ID)
	p.facts.ForEachExpressionFunction(func(ref factflow.ExprRef, fn symbol.ID) bool {
		functions[ref] = fn
		return true
	})
	entries := append([]DirectLexicalDeclaration(nil), input...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Expression < entries[j].Expression })
	for i, entry := range entries {
		if entry.Expression == 0 || entry.Function == 0 || entry.Target == 0 ||
			i != 0 && entries[i-1].Expression == entry.Expression || functions[entry.Expression] != entry.Function ||
			!p.matchesDirectLexicalDeclaration(entry) {
			return DirectLexicalDeclarations{}
		}
	}
	return DirectLexicalDeclarations{owner: p, entries: entries, complete: true}
}

// SealEmptyDirectLexicalDeclarations seals the unique empty declaration
// census, but only when p contains no function-literal identity sidecar. It is
// the exact proof used by syntax-built RelationProgram tests and other units
// for which there is mathematically nothing for a binder to certify.
func SealEmptyDirectLexicalDeclarations(p *Plan) DirectLexicalDeclarations {
	if p == nil {
		return DirectLexicalDeclarations{}
	}
	empty := true
	p.facts.ForEachExpressionFunction(func(factflow.ExprRef, symbol.ID) bool {
		empty = false
		return false
	})
	if !empty {
		return DirectLexicalDeclarations{}
	}
	return DirectLexicalDeclarations{owner: p, complete: true}
}

func (p *Plan) matchesDirectLexicalDeclaration(want DirectLexicalDeclaration) bool {
	matches := 0
	for raw := 0; raw < p.PointCount(); raw++ {
		assignment, ok := p.facts.RootAssignment(cfg.Point(raw))
		if !ok {
			continue
		}
		source := assignment.Source()
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr || source.ExprRef != want.Expression {
			continue
		}
		if assignment.Kind() != factflow.RootAssignmentLocalDeclaration || assignment.TargetSymbol() != want.Target ||
			source.ResultIndex != 0 || source.Expanded || source.OpenTail {
			return false
		}
		if _, declared := assignment.DeclaredValue(); declared {
			return false
		}
		if _, annotated := assignment.DeclaredAnnotationValue(); annotated {
			return false
		}
		matches++
	}
	return matches == 1
}

// AdmitsDirectLexicalDeclaration reports whether candidate has the exact
// plan-owned declaration shape that may be erased in favor of direct lexical
// call composition. Binder use-closure proves stability and completeness;
// the Plan independently owns value-side conditions such as annotations and
// declared overlays. Keeping this predicate here prevents binder authority
// construction from duplicating (and drifting from) the semantic census.
func (p *Plan) AdmitsDirectLexicalDeclaration(candidate DirectLexicalDeclaration) bool {
	return p != nil && p.matchesDirectLexicalDeclaration(candidate)
}

// Contains reports whether ref is a member of the exact
// sealed declaration census.
func (a DirectLexicalDeclarations) Contains(p *Plan, ref factflow.ExprRef, function, target symbol.ID) bool {
	if p == nil || a.owner != p || !a.complete || ref == 0 || function == 0 || target == 0 {
		return false
	}
	index := sort.Search(len(a.entries), func(i int) bool {
		return a.entries[i].Expression >= ref
	})
	return index < len(a.entries) && a.entries[index] == (DirectLexicalDeclaration{
		Expression: ref, Function: function, Target: target,
	})
}

// FunctionForTarget returns the exact function-expression identity owned by a
// sealed direct-only local binding. This is the value producer for a captured
// sibling function: the declaration is intentionally absent from mutable
// State, but its exact function value remains part of a callee frame.
func (a DirectLexicalDeclarations) FunctionForTarget(p *Plan, target symbol.ID) (factflow.ExprRef, symbol.ID, bool) {
	if p == nil || a.owner != p || !a.complete || target == 0 {
		return 0, 0, false
	}
	for _, entry := range a.entries {
		if entry.Target == target {
			return entry.Expression, entry.Function, true
		}
	}
	return 0, 0, false
}

// Matches reports exact whole-plan authority for p.
func (a DirectLexicalDeclarations) Matches(p *Plan) bool {
	return p != nil && a.owner == p && a.complete
}
