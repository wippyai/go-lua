package publicationfreeze

import (
	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	calldomain "github.com/wippyai/go-lua/domain/call"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/heap/internal/recentplan"
	"github.com/wippyai/go-lua/domain/materialization"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// publicationFreezeRow is the declarative counterpart of freezeRow: one
// admitted FreezeSeal receipt of the candidate call, plus the span its
// subject members occupy in the call's own member vector rather than in
// Effect's global member column. HotRule addresses a member by the tag its
// selector read resolved; this derivation addresses it by the position the
// call's own member order already gives it, because the vector argument
// delivers exactly that order.
type publicationFreezeRow struct {
	id          identity.ContentID
	operation   vocabulary.Operation
	subjectOpen bool
	memberBase  int
	memberCount int
}

// operationGateForValue is prepareCall's operationGateForCall, restated over
// the Call authority and fact directly rather than over a HotRule's cached
// batch. The two must agree on what an alternative set of a mounted call
// authorizes, so this is the same walk of the same Value: an opaque or open
// Call widens the whole gate, a target with no operation makes the gate
// unsupported without refusing it, and any other target contributes its
// operation.
func operationGateForValue(calls *calldomain.Algebra, value calldomain.Value) (operationGate, bool) {
	if calls == nil {
		return operationGate{}, false
	}
	var gate operationGate
	if value.IsTop() {
		gate.opaque = true
		return gate, true
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		target, targetOK := value.KnownTargetAt(index)
		if !targetOK {
			return operationGate{}, false
		}
		operation, operationKind := calls.ClassifyTargetOperation(target)
		switch operationKind {
		case calldomain.TargetOperationInvalid:
			return operationGate{}, false
		case calldomain.TargetOperationNone:
			// Freeze is stricter than publication placement: a valid Call
			// alternative without an operation cannot justify strong freeze.
			gate.unsupported = true
		case calldomain.TargetOperationPresent:
			if !gate.add(operation) {
				return operationGate{}, false
			}
		}
	}
	gate.opaque = value.IsOpen()
	return gate, true
}

// actualOrdinalFor maps one authored subject member to the actual ordinal that
// carries it.
//
// A publication's subject is a semantic the program authored, and the call's
// actuals are the semantics Pack mounted for this call. The two meet by
// identity, which is Pack's correspondence to state and not this rule's: a
// member that is no actual of this call is not an error, it is a subject this
// vector does not carry, and the caller settles it as the empty valid plan.
func actualOrdinalFor(actual packdomain.MountedActualProjection, semantic identity.ContentID) (int, bool) {
	if !semantic.Available() {
		return 0, false
	}
	for ordinal := 0; ordinal < actual.ActualCount(); ordinal++ {
		source, sourceOK := actual.ActualAt(ordinal)
		if !sourceOK {
			return 0, false
		}
		if source.ID() == semantic {
			return ordinal, true
		}
	}
	return 0, false
}

// joinSubjectFact folds one receipt's subject members into the single fact
// planFor consumes, mirroring factBuffer.merge: any absent member makes the
// whole subject absent, and every present member is joined in vector order.
// A subject that names no member is the caller's job to recognize before
// calling this - it is the "no mounted semantic source" settlement in
// planFor, not a fact this fold can manufacture.
func joinSubjectFact(schema *valuedomain.Schema, publication effectfactor.PublicationCall, actual packdomain.MountedActualProjection, actuals execution.SummaryVector[valuedomain.Value], base, length int) (valuedomain.Value, bool, bool) {
	if schema == nil || base < 0 || length <= 0 {
		return valuedomain.Value{}, false, false
	}
	var joined valuedomain.Value
	haveValue := false
	present := true
	for offset := 0; offset < length; offset++ {
		subject, subjectOK := publication.MemberAt(base + offset)
		if !subjectOK {
			return valuedomain.Value{}, false, false
		}
		ordinal, carried := actualOrdinalFor(actual, subject.Row().Semantic)
		if !carried {
			// The subject is no actual of this call. The vector carries no
			// cell for it, so the receipt has no exact fact and the plan
			// settles empty rather than refusing.
			return valuedomain.Value{}, false, true
		}
		value, cellPresent, cellOK := actuals.At(ordinal)
		if !cellOK {
			return valuedomain.Value{}, false, false
		}
		if !cellPresent {
			present = false
			continue
		}
		if !present {
			continue
		}
		if !haveValue {
			joined = value
			haveValue = true
			continue
		}
		var joinOK bool
		joined, joinOK = schema.Join(joined, value)
		if !joinOK {
			return valuedomain.Value{}, false, false
		}
	}
	if !present {
		return valuedomain.Value{}, false, true
	}
	return joined, haveValue, true
}

// DerivePublicationFreezeRoutes is the declarative publication-freeze
// relation: the exact Heap routes one mounted call's sealed FreezeSeal
// receipts justify freezing.
//
// It folds prepareCall, operationGateForCall and planFor into one pass over
// the candidate's own receipts, because a relation Build does not get a
// hot-path cache to prepare ahead of it - it is handed the candidate, the
// call fact and the subject vector, and derives the same route set the
// authored rule maintains incrementally.
//
// Each known operation alternative of the call contributes the exact Recent
// allocation roots its own FreezeSeal receipts select, and the relation is
// their route-tag INTERSECTION: a mixed target set writes nothing strong,
// and aliasing targets still agree once their roots agree. Semantic
// uncertainty - an opaque or unsupported gate, a receipt with an open or
// empty subject, or a subject fact that is not an exact Recent allocation -
// is an empty, valid plan rather than a refusal: the rule settles its
// authenticated empty selection instead of fabricating a Heap route.
// Malformed owner authority - an owner mismatch, an inadmissible call, or a
// directory the candidate does not belong to - is a failed relation.
//
// The subject vector is the call's own member set in the call's own member
// order, one cell per member of publication.MemberAt: a route set computed
// from every subject cannot be built one subject at a time, so the whole
// vector is one argument and a receipt's members are addressed by the
// running position its span occupies in that order, never by a second tag
// this relation would have to mint.
func DerivePublicationFreezeRoutes(
	schema heapdomain.Schema,
	values *valuedomain.Schema,
	calls *calldomain.Algebra,
	effects *effectfactor.Algebra,
	packs *packdomain.Schema,
	candidate calldomain.CallCoordinate,
	callFact calldomain.Value,
	actuals execution.SummaryVector[valuedomain.Value],
) (recentplan.Plan, bool) {
	if calls == nil || !calls.Valid() || !schema.Valid() || values == nil || !values.Valid() ||
		effects == nil || !effects.Valid() || !values.OwnsHeapSchema(schema) ||
		!values.LinkOwner().Matches(calls.LinkOwner()) || !effects.LinkOwner().Available() ||
		!effects.LinkOwner().Matches(calls.LinkOwner()) {
		return recentplan.Plan{}, false
	}
	// The candidate is Effect's mounted call. Its publications are what this
	// relation is derived from, and the algebra sealed them once: this resolves
	// the call's directory row rather than re-deriving a directory per
	// invocation.
	mounted, mountedOK := candidate.MountedCall()
	_, occurrence, module, _, _, identityOK := calls.MountedCallIdentity(mounted)
	if !mountedOK || !identityOK || !calls.OwnsCallCoordinate(candidate) {
		return recentplan.Plan{}, false
	}
	// The candidate is every mounted call, because that is the directory
	// Value's actual member set corresponds to. A call Effect admitted no
	// publications on justifies no freeze, which is an empty valid plan and
	// not a refusal: the rule is a candidate of every call and answers for
	// every call, including the ones that authored nothing.
	publication, publicationOK := effects.PublicationCallForOccurrence(module, occurrence)
	if !publicationOK {
		return recentplan.Plan{}, true
	}
	row := publication.Row()
	if !row.Available() {
		return recentplan.Plan{}, false
	}
	// Pack owns which semantics this call mounted as actuals, and the vector
	// join 1 delivers is exactly that projection's cells.
	actual, actualOK := packs.MountedActualProjection(row.Module, row.Call)
	if !actualOK || !actual.Valid() || !actual.OwnedBy(packs) {
		return recentplan.Plan{}, false
	}
	_, key, keyOK := calls.MountedCallKeyForOccurrence(row.Module, row.Call)
	if !keyOK || !key.Valid() || !calls.Admits(key, callFact) {
		return recentplan.Plan{}, false
	}

	directory := effects.Publications()
	if int(row.RowOffset)+int(row.RowLength) > len(directory.Rows) {
		return recentplan.Plan{}, false
	}

	// One walk of this call's own receipts, in Effect's sealed order, builds
	// the kept FreezeSeal rows and advances the same member cursor Effect
	// advanced when it appended this call's members: every receipt's span
	// counts against the cursor, kept or not, because the subject vector
	// covers every receipt's members and not only the ones this relation
	// keeps.
	var kept []publicationFreezeRow
	var seen contentIDBuffer
	cursor := 0
	for _, receipt := range directory.Rows[row.RowOffset : row.RowOffset+row.RowLength] {
		if !receipt.MountedAt(row.Module, row.Call) {
			return recentplan.Plan{}, false
		}
		if !seen.add(receipt.ID) {
			return recentplan.Plan{}, false
		}
		base := cursor
		length := int(receipt.SubjectLength)
		cursor += length
		if receipt.Kind != vocabulary.PublicationEffectFreezeSeal || receipt.Mutability != vocabulary.PublicationMutabilitySeal {
			continue
		}
		if receipt.Operation == 0 {
			return recentplan.Plan{}, false
		}
		kept = append(kept, publicationFreezeRow{
			id: receipt.ID, operation: receipt.Operation, subjectOpen: receipt.SubjectOpen,
			memberBase: base, memberCount: length,
		})
	}
	// The vector is this call's own actuals, so its width is Pack's statement
	// about the call and disagreeing with it is malformed authority rather
	// than uncertainty.
	if cursor != publication.MemberCount() || !actuals.Valid() || actuals.Count() != actual.ActualCount() {
		return recentplan.Plan{}, false
	}

	gate, gateOK := operationGateForValue(calls, callFact)
	if !gateOK {
		return recentplan.Plan{}, false
	}
	if gate.opaque || gate.unsupported || gate.count == 0 {
		return recentplan.Plan{}, true
	}

	var intersection routePlan
	haveRoutes := false
	for gateIndex := 0; gateIndex < gate.count; gateIndex++ {
		operation, operationOK := gate.at(gateIndex)
		if !operationOK {
			return routePlan{}, false
		}
		var targetRoutes routePlan
		found := false
		for _, freezeRow := range kept {
			if freezeRow.operation != operation {
				continue
			}
			found = true
			if freezeRow.subjectOpen {
				return routePlan{}, true
			}
			if freezeRow.memberCount == 0 {
				// A subject with no member is either proven nil by Lua
				// under-application or statically unknown behind an open
				// pack. Neither authorizes a strong freeze, and both leave a
				// valid empty plan rather than refusing the batch.
				return routePlan{}, true
			}
			fact, factPresent, joinOK := joinSubjectFact(values, publication, actual, actuals, freezeRow.memberBase, freezeRow.memberCount)
			if !joinOK {
				return routePlan{}, false
			}
			candidateRoot, candidateOK := exactRecentAllocation(values, fact, factPresent)
			if !candidateOK {
				return routePlan{}, true
			}
			tag, tagOK := schema.RouteTag(candidateRoot, materialization.Recent)
			if !tagOK || tag == 0 || !targetRoutes.Add(route{Key: candidateRoot, Tag: tag}) {
				return routePlan{}, false
			}
		}
		if !found || targetRoutes.Count() == 0 {
			return routePlan{}, true
		}
		if !haveRoutes {
			intersection = targetRoutes
			haveRoutes = true
			continue
		}
		var intersectionOK bool
		intersection, intersectionOK = intersection.Intersection(targetRoutes)
		if !intersectionOK {
			return routePlan{}, false
		}
		// Once no exact root is common to the known alternatives, no later
		// operation can restore a strong write. This is the common
		// mixed-target case and avoids walking the remaining kept rows.
		if intersection.Count() == 0 {
			return routePlan{}, true
		}
	}
	if !haveRoutes || intersection.Count() == 0 {
		return routePlan{}, true
	}
	for index := 0; index < intersection.Count(); index++ {
		candidateRoute, candidateOK := intersection.At(index)
		if !candidateOK || !candidateRoute.Key.Valid() || candidateRoute.Key.Kind() != heapdomain.RootAllocation ||
			!schema.OwnsKey(candidateRoute.Key) || candidateRoute.Tag == 0 {
			return routePlan{}, false
		}
	}
	return intersection, true
}

// PublicationFreezeRouteCount is the direct composition accessor for a
// derived plan.
func PublicationFreezeRouteCount(plan recentplan.Plan) int { return plan.Count() }

// PublicationFreezeRouteAt is the direct composition accessor for one
// derived route, in the plan's canonical route-tag order.
func PublicationFreezeRouteAt(plan recentplan.Plan, index int) (recentplan.Route, bool) {
	return plan.At(index)
}
