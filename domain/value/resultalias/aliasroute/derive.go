// Package aliasroute derives the mounted actual coordinates one call site's
// selected Target operations alias its first result to.
//
// It is the declared relation the result-alias rule selects over: the members
// are the actuals THIS call aliases, ordered by the authored input formal each
// one answers, and the tag a member carries is that formal's ordinal. Nothing
// here reads a Factor cell - which formals an operation aliases is the Target's
// own declaration, and what each actual IS is the cell the selection observes
// at the member this derivation named.
package aliasroute

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// Route is one aliased actual of this call: the Value coordinate its fact is
// observed at, and the tag it is correlated by. The tag is the authored input
// formal ordinal raised by one, so zero stays the absent tag and a consumer
// holding the call's mounted actuals recovers the formal from it.
type Route struct {
	coordinate valuedomain.Coordinate
	tag        uint64
	set        bool
}

// Coordinate is the Value coordinate this member's cell is observed at.
func (route Route) Coordinate() (valuedomain.Coordinate, bool) { return route.coordinate, route.set }

// Predicate is the owner-issued tag this member is correlated by.
func (route Route) Predicate() (uint64, bool) { return route.tag, route.set }

// Plan is one call site's whole ordered member set.
type Plan struct{ routes []Route }

// Count is the member census this selection spans.
func Count(plan Plan) int { return len(plan.routes) }

// At projects one member in the canonical order Derive fixed.
func At(plan Plan, index int) (Route, bool) {
	if index < 0 || index >= len(plan.routes) {
		return Route{}, false
	}
	return plan.routes[index], true
}

// Selection is one call site's whole reading of the Target alias declarations:
// the authored input formals its selected operations alias the first result
// to, whether any selected operation aliases at all, and whether the evidence
// is beyond enumeration.
type Selection struct {
	sources []uint32
	aliased bool
	top     bool
}

// Top reports that the aliased actuals are beyond enumeration, so the site's
// result admits every value rather than the ones named here.
func (selection Selection) Top() bool { return selection.top }

// Aliased reports that at least one selected operation declares a result-zero
// alias. A site that aliases nothing carries no evidence of this kind.
func (selection Selection) Aliased() bool { return selection.aliased }

// Sources is the strictly ascending set of authored input formal ordinals this
// site aliases its first result to.
func (selection Selection) Sources() []uint32 { return selection.sources }

// AliasSources answers the authored input formals one Target operation
// declares its FIRST result an alias of, across every outcome that operation
// admits.
//
// Result ordinals other than zero belong to another output geometry and
// contribute nothing here. The compiler admits only ValueFormal aliases, so an
// alias of another source kind, or one naming a formal the operation does not
// declare, is malformed Target geometry and is refused rather than skipped.
func AliasSources(target *contract.Contract, operation vocabulary.Operation) ([]uint32, bool) {
	if target == nil || operation == 0 {
		return nil, false
	}
	var sources []uint32
	for outcome := 0; outcome < target.Operations.OutcomeCount(operation); outcome++ {
		for aliasIndex := 0; aliasIndex < target.Operations.ResultAliasCount(operation, outcome); aliasIndex++ {
			result, kind, source, aliasOK := target.Operations.ResultAliasAt(operation, outcome, aliasIndex)
			if !aliasOK {
				return nil, false
			}
			if result != 0 {
				continue
			}
			if kind != vocabulary.InputSourceValueFormal || int(source) >= target.Operations.InputFormalCount(operation) {
				return nil, false
			}
			if !containsSource(sources, source) {
				sources = append(sources, source)
			}
		}
	}
	sortSources(sources)
	return sources, true
}

// Site resolves the mounted actual projection one result-zero slot's call
// applies. Pack seals the fixed endpoint list; the Call coordinate is resolved
// beside it so the row is authenticated against the occurrence both owners
// address it by.
func Site(
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	packs *packdomain.Schema,
	candidate valuedomain.MountedCallResultSlot,
) (packdomain.MountedActualProjection, bool) {
	if values == nil || !values.Valid() || calls == nil || !calls.Valid() || packs == nil ||
		!values.LinkOwner().Matches(calls.LinkOwner()) || !packs.LinkOwner().Matches(calls.LinkOwner()) ||
		!values.OwnsMountedCallResultSlot(candidate) {
		return packdomain.MountedActualProjection{}, false
	}
	module, moduleOK := candidate.Module()
	occurrence, occurrenceOK := candidate.CallID()
	if !moduleOK || !occurrenceOK || !module.Available() || !occurrence.Available() {
		return packdomain.MountedActualProjection{}, false
	}
	_, coordinateOK := calls.CallCoordinateForOccurrence(module, occurrence)
	actual, actualOK := packs.MountedActualProjection(module, occurrence)
	if !coordinateOK || !actualOK || !actual.Valid() || !actual.OwnedBy(packs) {
		return packdomain.MountedActualProjection{}, false
	}
	return actual, true
}

// Select validates one exact Call fact and answers the union of every
// result-zero input formal ordinal named by the operations that fact selects.
//
// Body targets and operations with no alias are intentionally ignored: they
// carry no result-alias declaration. Opaque Call alternatives are conservative
// Top, and so is a mounted runtime suffix - an unrepresented tail can carry
// further call evidence, and a fixed projection that omits it is not complete.
// A declared alias whose mounted Value coordinate is absent is unresolved
// evidence rather than proof of no alias, and widens the same way.
func Select(
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	packs *packdomain.Schema,
	candidate valuedomain.MountedCallResultSlot,
	fact calldomain.Value,
) (Selection, packdomain.MountedActualProjection, bool) {
	actual, actualOK := Site(values, calls, packs, candidate)
	if !actualOK || !callValueValid(fact) {
		return Selection{}, packdomain.MountedActualProjection{}, false
	}
	target, targetOK := calls.TargetContract()
	if !targetOK || target == nil {
		return Selection{}, packdomain.MountedActualProjection{}, false
	}
	if fact.IsTop() || fact.HasOpaqueAlternative() {
		return Selection{top: true}, actual, true
	}
	if fact.IsEmpty() || fact.KnownTargetCount() == 0 {
		return Selection{}, actual, true
	}
	selection := Selection{}
	for index := 0; index < fact.KnownTargetCount(); index++ {
		declared, declaredOK := fact.KnownTargetAt(index)
		if !declaredOK || !calls.OwnsTarget(declared) {
			return Selection{}, packdomain.MountedActualProjection{}, false
		}
		operation, operationOK := declared.Operation()
		if !operationOK {
			// Body targets are handled by the body-result consumer and carry
			// no ResultAlias declaration.
			continue
		}
		sources, sourcesOK := AliasSources(target, operation)
		if !sourcesOK {
			return Selection{}, packdomain.MountedActualProjection{}, false
		}
		if len(sources) == 0 {
			continue
		}
		selection.aliased = true
		selection.sources = append(selection.sources, sources...)
	}
	if !selection.aliased {
		return selection, actual, true
	}
	if _, hasTail := actual.TailID(); hasTail {
		selection.top = true
		return selection, actual, true
	}
	selection.sources = dedupeSources(selection.sources)
	for _, source := range selection.sources {
		if uint64(source) >= uint64(actual.ActualCount()) {
			selection.top = true
			return selection, actual, true
		}
		semantic, semanticOK := actual.ActualAt(int(source))
		if !semanticOK {
			selection.top = true
			return selection, actual, true
		}
		if _, coordinateOK := values.CoordinateForMountedSemantic(semantic.Module(), semantic.ID()); !coordinateOK {
			selection.top = true
			return selection, actual, true
		}
	}
	return selection, actual, true
}

// Derive answers the actuals this call site aliases its first result to.
//
// A site whose evidence is beyond enumeration, and one that aliases nothing,
// both name no member: what such a site publishes is the fold's answer over an
// empty delivery, not a wider observation. The order is ascending tag, which
// is the order the engine canonicalizes a selection by, and the tags are the
// strictly ascending formal ordinals the selection named.
func Derive(
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	packs *packdomain.Schema,
	candidate valuedomain.MountedCallResultSlot,
	fact calldomain.Value,
) (Plan, bool) {
	selection, actual, selectionOK := Select(values, calls, packs, candidate, fact)
	if !selectionOK {
		return Plan{}, false
	}
	if selection.top || !selection.aliased {
		return Plan{}, true
	}
	routes := make([]Route, 0, len(selection.sources))
	for _, source := range selection.sources {
		semantic, semanticOK := actual.ActualAt(int(source))
		if !semanticOK {
			return Plan{}, false
		}
		coordinate, coordinateOK := values.CoordinateForMountedSemantic(semantic.Module(), semantic.ID())
		if !coordinateOK {
			return Plan{}, false
		}
		routes = append(routes, Route{coordinate: coordinate, tag: uint64(source) + 1, set: true})
	}
	return Plan{routes: routes}, true
}

// callValueValid states the shape a Call fact must have before this relation
// reads it: one of the four dispositions Call's algebra publishes.
func callValueValid(fact calldomain.Value) bool {
	return fact.IsTop() || fact.IsOpen() || fact.IsComplete() || fact.IsEmpty()
}

func containsSource(sources []uint32, source uint32) bool {
	for _, existing := range sources {
		if existing == source {
			return true
		}
	}
	return false
}

func sortSources(sources []uint32) {
	sort.Slice(sources, func(left, right int) bool { return sources[left] < sources[right] })
}

// dedupeSources fixes the canonical member order of one site's whole formal
// set. Two operations naming one formal address one member twice, and the
// selection has no second ordinal for the repeat.
func dedupeSources(sources []uint32) []uint32 {
	if len(sources) < 2 {
		return sources
	}
	sortSources(sources)
	write := 1
	for _, source := range sources[1:] {
		if source != sources[write-1] {
			sources[write] = source
			write++
		}
	}
	return sources[:write]
}
