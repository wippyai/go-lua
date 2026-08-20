package outcome

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/control"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal derives the complete canonical Outcome relation from the already
// proved Body and lexical-control projections.  It is deliberately a
// pre-execution pass: no normal successor, CFG edge, reachability, recurrence,
// Mu, continuation, or domain fact is constructed here.
//
// The pass is iterative. Return closure marks each Body once; Break and Goto
// requests are grouped by target and mark each path Body once per target. The
// only superlinear operation is the final deterministic semantic-key sort:
// O(B+C+K log K) time and O(B+C+K) temporary/published storage.
func Seal(
	identity source.Identity,
	view authored.View,
	bodies *body.Result,
	shape *control.Shape,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Result, error) {
	counts, bodyCount, err := validateInputs(identity, view, bodies, shape, staticID, moduleID)
	if err != nil {
		return nil, err
	}
	controlView := view.Control()
	returns := controlView.Returns()
	breaks := controlView.Breaks()
	labels := controlView.Labels()
	gotos := controlView.Gotos()
	loops := controlView.Loops()

	returnNeeded, err := returnClosure(returns, bodies, counts, bodyCount)
	if err != nil {
		return nil, err
	}

	keys := make([]outcomeKey, 0)
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		for _, outcomeKind := range mandatoryKinds {
			keys = append(keys, outcomeKey{body: bodyTerm, kind: outcomeKind})
		}
	}
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		if returnNeeded[ordinal] {
			keys = append(keys, outcomeKey{
				body: keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal)),
				kind: kind.OutcomeReturn,
			})
		}
	}

	breakRequests, err := collectBreakRequests(breaks, loops, shape, bodies, counts)
	if err != nil {
		return nil, err
	}
	if err := appendBreakKeys(&keys, breakRequests, loops, bodies, bodyCount); err != nil {
		return nil, err
	}

	gotoRequests, gotoDirect, err := collectGotoRequests(gotos, labels, shape, bodies, counts)
	if err != nil {
		return nil, err
	}
	if err := appendGotoKeys(&keys, gotoRequests, bodies, loops, bodyCount); err != nil {
		return nil, err
	}

	sort.Slice(keys, func(left, right int) bool { return compareKey(keys[left], keys[right]) < 0 })
	keys = deduplicate(keys)
	if !keyspace.TermOrdinalFits(len(keys)) {
		return nil, errors.New("program/flow/outcome: Outcome cardinality exceeds Term representation")
	}

	result, err := materialize(keys, bodyCount, returns.Count(), breaks.Count(), gotos.Count())
	if err != nil {
		return nil, err
	}
	result.sourceID = identity.ContentID()
	result.flowID = view.ContentID()
	result.staticID = staticID
	result.moduleID = moduleID
	if err := resolvePropagation(result, keys, bodies, loops, labels, counts); err != nil {
		return nil, err
	}
	if err := resolveOccurrenceExits(result, returns, breaks, gotos, shape, gotoDirect, counts); err != nil {
		return nil, err
	}
	return result, nil
}

var mandatoryKinds = [...]kind.OutcomeKind{
	kind.OutcomeNormal,
	kind.OutcomeThrow,
	kind.OutcomeYield,
	kind.OutcomeCancel,
}

func validateInputs(
	identity source.Identity,
	view authored.View,
	bodies *body.Result,
	shape *control.Shape,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) ([keyspace.FamilyCount]int, int, error) {
	var counts [keyspace.FamilyCount]int
	sourceID := identity.ContentID()
	flowID := view.ContentID()
	if !sourceID.Available() || !staticID.Available() || !moduleID.Available() || identity.Name() == "" || identity.TermCount() == 0 || bodies == nil || shape == nil {
		return counts, 0, errors.New("program/flow/outcome: owner or structural proof is unavailable")
	}
	if !flowID.Available() {
		return counts, 0, errors.New("program/flow/outcome: authored identity is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return counts, 0, errors.New("program/flow/outcome: Body provenance disagrees with Source or Flow")
	}
	if !control.Matches(shape, sourceID, flowID, staticID, moduleID) {
		return counts, 0, errors.New("program/flow/outcome: Control provenance disagrees with Source, Flow, Static, or Module")
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, 0, errors.New("program/flow/outcome: invalid Source family cardinality")
		}
		counts[family] = count
		total += uint64(count)
	}
	if total != uint64(identity.TermCount()) || counts[keyspace.FamilyOutcome] != 0 {
		return counts, 0, errors.New("program/flow/outcome: invalid pre-Outcome Source denominator")
	}
	bodyCount := counts[keyspace.FamilyBody]
	if bodyCount == 0 || bodies.BodyCount() != bodyCount {
		return counts, 0, errors.New("program/flow/outcome: Body denominator mismatch")
	}
	if err := validateAuthoredCounts(view, counts); err != nil {
		return counts, 0, err
	}
	return counts, bodyCount, nil
}

func validateAuthoredCounts(view authored.View, counts [keyspace.FamilyCount]int) error {
	storage := view.Storage()
	access := view.Access()
	operators := view.Operators()
	control := view.Control()
	checks := [...]struct {
		actual int
		family keyspace.Family
	}{
		{view.Values().Count(), keyspace.FamilyValues},
		{access.Exact().Count(), keyspace.FamilyLensExact},
		{access.Dynamic().Count(), keyspace.FamilyLensKey},
		{storage.Cells().Count(), keyspace.FamilyCell},
		{storage.Reads().Count(), keyspace.FamilyRead},
		{storage.Varargs().Count(), keyspace.FamilyVararg},
		{storage.Binds().Count(), keyspace.FamilyBind},
		{storage.Assigns().Count(), keyspace.FamilyAssign},
		{storage.Writes().Count(), keyspace.FamilyWrite},
		{view.Tables().Count(), keyspace.FamilyTable},
		{view.Fields().Count(), keyspace.FamilyTableField},
		{operators.Unaries().Count(), keyspace.FamilyUnary},
		{operators.Binaries().Count(), keyspace.FamilyBinary},
		{operators.Selects().Count(), keyspace.FamilySelect},
		{view.Functions().Count(), keyspace.FamilyFunction},
		{view.Calls().Count(), keyspace.FamilyCall},
		{control.Returns().Count(), keyspace.FamilyReturn},
		{control.Breaks().Count(), keyspace.FamilyBreak},
		{control.Labels().Count(), keyspace.FamilyLabel},
		{control.Gotos().Count(), keyspace.FamilyGoto},
		{control.Branches().Count(), keyspace.FamilyBranch},
		{control.Loops().Count(), keyspace.FamilyLoop},
		{view.Claims().Count(), keyspace.FamilyValueClaim},
		{view.TypeValues().Count(), keyspace.FamilyTypeValue},
	}
	for _, check := range checks {
		if check.actual != counts[check.family] {
			return errors.New("program/flow/outcome: authored family denominator mismatch")
		}
	}
	return nil
}

func returnClosure(returns authored.Returns, bodies *body.Result, counts [keyspace.FamilyCount]int, bodyCount int) ([]bool, error) {
	needed := make([]bool, bodyCount+1)
	queue := make([]keyspace.Term, 0, returns.Count())
	for index := 0; index < returns.Count(); index++ {
		term, ok := returns.At(index)
		owner, values, rowOK := returns.Get(term)
		if !ok || !rowOK || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(values, counts, keyspace.FamilyValues) {
			return nil, errors.New("program/flow/outcome: invalid Return row")
		}
		ordinal := keyspace.TermOrdinal(owner)
		if !needed[ordinal] {
			needed[ordinal] = true
			queue = append(queue, owner)
		}
	}
	for head := 0; head < len(queue); head++ {
		current := queue[head]
		parent, hasParent := bodies.Parent(current)
		if !hasParent {
			if parent != 0 {
				return nil, errors.New("program/flow/outcome: malformed Body parent")
			}
			continue
		}
		if parent == 0 {
			return nil, errors.New("program/flow/outcome: malformed Body parent")
		}
		if !sameActivation(bodies, current, parent) {
			continue
		}
		ordinal := keyspace.TermOrdinal(parent)
		if uint64(ordinal) > uint64(bodyCount) {
			return nil, errors.New("program/flow/outcome: Body parent is out of range")
		}
		if !needed[ordinal] {
			needed[ordinal] = true
			queue = append(queue, parent)
		}
	}
	return needed, nil
}

type pathRequest struct {
	occurrence keyspace.Term
	owner      keyspace.Term
	target     keyspace.Term
	targetBody keyspace.Term
}

func collectBreakRequests(breaks authored.Breaks, loops authored.Loops, shape *control.Shape, bodies *body.Result, counts [keyspace.FamilyCount]int) ([]pathRequest, error) {
	requests := make([]pathRequest, 0, breaks.Count())
	for index := 0; index < breaks.Count(); index++ {
		term, ok := breaks.At(index)
		owner, _, rowOK := breaks.Get(term)
		loop, shapeOK := shape.BreakLoop(term)
		loopOwner, loopBody, loopKind, _, loopOK := loops.Get(loop)
		if !ok || !rowOK || !shapeOK || !loopOK || !validTerm(owner, counts, keyspace.FamilyBody) ||
			!validTerm(loop, counts, keyspace.FamilyLoop) || !validLoopKind(loopKind) ||
			!validTerm(loopOwner, counts, keyspace.FamilyBody) || !validTerm(loopBody, counts, keyspace.FamilyBody) {
			return nil, errors.New("program/flow/outcome: invalid Break target")
		}
		parent, parentOK := bodies.Parent(loopBody)
		if !parentOK || parent != loopOwner || !sameActivation(bodies, loopOwner, loopBody) {
			return nil, errors.New("program/flow/outcome: Break Loop disagrees with Body topology")
		}
		requests = append(requests, pathRequest{occurrence: term, owner: owner, target: loop, targetBody: loopBody})
	}
	sort.Slice(requests, func(left, right int) bool {
		if keyspace.TermOrdinal(requests[left].target) != keyspace.TermOrdinal(requests[right].target) {
			return keyspace.TermOrdinal(requests[left].target) < keyspace.TermOrdinal(requests[right].target)
		}
		if keyspace.TermOrdinal(requests[left].owner) != keyspace.TermOrdinal(requests[right].owner) {
			return keyspace.TermOrdinal(requests[left].owner) < keyspace.TermOrdinal(requests[right].owner)
		}
		return keyspace.TermOrdinal(requests[left].occurrence) < keyspace.TermOrdinal(requests[right].occurrence)
	})
	return requests, nil
}

func appendBreakKeys(keys *[]outcomeKey, requests []pathRequest, loops authored.Loops, bodies *body.Result, bodyCount int) error {
	if len(requests) == 0 {
		return nil
	}
	seen := make([]keyspace.Term, bodyCount+1)
	for start := 0; start < len(requests); {
		end := start + 1
		for end < len(requests) && requests[end].target == requests[start].target {
			end++
		}
		loop := requests[start].target
		_, loopBody, _, _, ok := loops.Get(loop)
		if !ok || loopBody != requests[start].targetBody {
			return errors.New("program/flow/outcome: Break target changed during seal")
		}
		for index := start; index < end; index++ {
			current := requests[index].owner
			for {
				if !validBody(current) || uint64(keyspace.TermOrdinal(current)) > uint64(bodyCount) {
					return errors.New("program/flow/outcome: Break path leaves Body forest")
				}
				ordinal := keyspace.TermOrdinal(current)
				if seen[ordinal] == loop {
					break
				}
				seen[ordinal] = loop
				*keys = append(*keys, outcomeKey{body: current, kind: kind.OutcomeBreak, target: loop})
				if current == loopBody {
					break
				}
				parent, parentOK := bodies.Parent(current)
				if !parentOK || parent == 0 || !sameActivation(bodies, current, parent) {
					return errors.New("program/flow/outcome: Break path crosses its Loop")
				}
				current = parent
			}
		}
		start = end
	}
	return nil
}

func collectGotoRequests(gotos authored.Gotos, labels authored.Labels, shape *control.Shape, bodies *body.Result, counts [keyspace.FamilyCount]int) ([]pathRequest, []keyspace.Term, error) {
	requests := make([]pathRequest, 0, gotos.Count())
	direct := make([]keyspace.Term, gotos.Count()+1)
	for index := 0; index < gotos.Count(); index++ {
		term, ok := gotos.At(index)
		owner, target, rowOK := gotos.Get(term)
		targetBody, shapeOK := shape.GotoTargetBody(term)
		labelOwner, labelOK := labels.Get(target)
		if !ok || !rowOK || !shapeOK || !labelOK || !validTerm(owner, counts, keyspace.FamilyBody) ||
			!validTerm(target, counts, keyspace.FamilyLabel) || !validTerm(targetBody, counts, keyspace.FamilyBody) || labelOwner != targetBody {
			return nil, nil, errors.New("program/flow/outcome: invalid Goto target")
		}
		if owner == targetBody {
			direct[keyspace.TermOrdinal(term)] = target
			continue
		}
		requests = append(requests, pathRequest{occurrence: term, owner: owner, target: target, targetBody: targetBody})
	}
	sort.Slice(requests, func(left, right int) bool {
		if keyspace.TermOrdinal(requests[left].target) != keyspace.TermOrdinal(requests[right].target) {
			return keyspace.TermOrdinal(requests[left].target) < keyspace.TermOrdinal(requests[right].target)
		}
		if keyspace.TermOrdinal(requests[left].owner) != keyspace.TermOrdinal(requests[right].owner) {
			return keyspace.TermOrdinal(requests[left].owner) < keyspace.TermOrdinal(requests[right].owner)
		}
		return keyspace.TermOrdinal(requests[left].occurrence) < keyspace.TermOrdinal(requests[right].occurrence)
	})
	return requests, direct, nil
}

func appendGotoKeys(keys *[]outcomeKey, requests []pathRequest, bodies *body.Result, loops authored.Loops, bodyCount int) error {
	if len(requests) == 0 {
		return nil
	}
	// A Goto normally stops at the target Body: that Body is the direct
	// Label-resume boundary, so publishing an extra Outcome row there would
	// turn a direct resume into a synthetic propagation step. A loop Body is
	// different. Its enclosing loop route is an intermediate lexical boundary
	// for an outward Goto from a nested Body, and therefore needs its typed
	// Outcome row so the route can carry the jump through the loop Body before
	// resuming at the Label.
	loopBodies := make([]bool, bodyCount+1)
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		_, loopBody, _, _, rowOK := loops.Get(loop)
		if !ok || !rowOK || !validBody(loopBody) || uint64(keyspace.TermOrdinal(loopBody)) > uint64(bodyCount) {
			return errors.New("program/flow/outcome: Goto loop Body is unavailable")
		}
		loopBodies[keyspace.TermOrdinal(loopBody)] = true
	}
	seen := make([]keyspace.Term, bodyCount+1)
	for start := 0; start < len(requests); {
		end := start + 1
		for end < len(requests) && requests[end].target == requests[start].target {
			end++
		}
		for index := start; index < end; index++ {
			current := requests[index].owner
			targetBody := requests[index].targetBody
			targetOrdinal := keyspace.TermOrdinal(targetBody)
			if !validBody(targetBody) || uint64(targetOrdinal) > uint64(bodyCount) {
				return errors.New("program/flow/outcome: Goto target Body is unavailable")
			}
			includeTargetBody := loopBodies[targetOrdinal]
			for {
				if !validBody(current) || uint64(keyspace.TermOrdinal(current)) > uint64(bodyCount) {
					return errors.New("program/flow/outcome: Goto path leaves Body forest")
				}
				if current == targetBody && !includeTargetBody {
					break
				}
				ordinal := keyspace.TermOrdinal(current)
				if seen[ordinal] == requests[index].target {
					break
				}
				seen[ordinal] = requests[index].target
				*keys = append(*keys, outcomeKey{body: current, kind: kind.OutcomeGoto, target: requests[index].target})
				if current == targetBody {
					break
				}
				parent, parentOK := bodies.Parent(current)
				if !parentOK || parent == 0 || !sameActivation(bodies, current, parent) {
					return errors.New("program/flow/outcome: Goto target is not an enclosing Body")
				}
				current = parent
			}
		}
		start = end
	}
	return nil
}

func materialize(keys []outcomeKey, bodyCount, returnCount, breakCount, gotoCount int) (*Result, error) {
	result := &Result{
		bodies:      make([]keyspace.Term, len(keys)+1),
		kinds:       make([]kind.OutcomeKind, len(keys)+1),
		targets:     make([]keyspace.Term, len(keys)+1),
		propagation: make([]keyspace.Term, len(keys)+1),
		returnExit:  make([]keyspace.Term, returnCount+1),
		breakExit:   make([]keyspace.Term, breakCount+1),
		gotoExit:    make([]keyspace.Term, gotoCount+1),
	}
	for _, outcomeKind := range mandatoryKinds {
		result.base[outcomeKind] = make([]keyspace.Term, bodyCount+1)
	}
	for index, key := range keys {
		ordinal := uint32(index + 1)
		term := keyspace.MakeTerm(keyspace.FamilyOutcome, ordinal)
		if term == 0 || !validBody(key.body) || !validKind(key.kind) || !validTarget(key.kind, key.target) {
			return nil, errors.New("program/flow/outcome: invalid canonical Outcome key")
		}
		result.bodies[ordinal] = key.body
		result.kinds[ordinal] = key.kind
		result.targets[ordinal] = key.target
		if key.kind == kind.OutcomeNormal || key.kind == kind.OutcomeThrow || key.kind == kind.OutcomeYield || key.kind == kind.OutcomeCancel {
			bodyOrdinal := keyspace.TermOrdinal(key.body)
			plane := result.base[key.kind]
			if bodyOrdinal == 0 || uint64(bodyOrdinal) >= uint64(len(plane)) || plane[bodyOrdinal] != 0 {
				return nil, errors.New("program/flow/outcome: duplicate mandatory Body exit")
			}
			plane[bodyOrdinal] = term
		}
	}
	for _, outcomeKind := range mandatoryKinds {
		for bodyOrdinal := 1; bodyOrdinal <= bodyCount; bodyOrdinal++ {
			if result.base[outcomeKind][bodyOrdinal] == 0 {
				return nil, errors.New("program/flow/outcome: missing mandatory Body exit")
			}
		}
	}
	return result, nil
}

func resolvePropagation(result *Result, keys []outcomeKey, bodies *body.Result, loops authored.Loops, labels authored.Labels, counts [keyspace.FamilyCount]int) error {
	for index := range keys {
		currentOrdinal := uint32(index + 1)
		currentBody := result.bodies[currentOrdinal]
		key := keys[index]
		parent, parentOK := bodies.Parent(currentBody)
		if key.kind == kind.OutcomeNormal {
			continue
		}
		if key.kind == kind.OutcomeBreak {
			_, loopBody, _, _, ok := loops.Get(key.target)
			if !ok || !validTerm(loopBody, counts, keyspace.FamilyBody) {
				return errors.New("program/flow/outcome: Break target disappeared")
			}
			if currentBody == loopBody {
				continue
			}
			if !parentOK || parent == 0 || !sameActivation(bodies, currentBody, parent) {
				return errors.New("program/flow/outcome: incomplete Break propagation")
			}
			next, ok := result.Find(parent, key.kind, key.target)
			if !ok {
				return errors.New("program/flow/outcome: missing Break propagation target")
			}
			result.propagation[currentOrdinal] = next
			continue
		}
		if key.kind == kind.OutcomeGoto {
			targetBody, ok := labels.Get(key.target)
			if !ok || !validTerm(targetBody, counts, keyspace.FamilyBody) {
				return errors.New("program/flow/outcome: Goto target disappeared")
			}
			if currentBody == targetBody {
				continue
			}
			if !parentOK || parent == 0 {
				return errors.New("program/flow/outcome: incomplete Goto propagation")
			}
			if parent == targetBody {
				if next, found := result.Find(parent, key.kind, key.target); found {
					result.propagation[currentOrdinal] = next
				}
				continue
			}
			if !sameActivation(bodies, currentBody, parent) {
				return errors.New("program/flow/outcome: Goto propagation crosses activation")
			}
			next, ok := result.Find(parent, key.kind, key.target)
			if !ok {
				return errors.New("program/flow/outcome: missing Goto propagation target")
			}
			result.propagation[currentOrdinal] = next
			continue
		}
		if !parentOK || parent == 0 || !sameActivation(bodies, currentBody, parent) {
			continue
		}
		next, ok := result.Find(parent, key.kind, key.target)
		if !ok {
			return errors.New("program/flow/outcome: missing lexical propagation target")
		}
		result.propagation[currentOrdinal] = next
	}
	return nil
}

func resolveOccurrenceExits(result *Result, returns authored.Returns, breaks authored.Breaks, gotos authored.Gotos, shape *control.Shape, direct []keyspace.Term, counts [keyspace.FamilyCount]int) error {
	for index := 0; index < returns.Count(); index++ {
		term, ok := returns.At(index)
		owner, _, rowOK := returns.Get(term)
		if !ok || !rowOK || !validTerm(owner, counts, keyspace.FamilyBody) {
			return errors.New("program/flow/outcome: invalid Return exit row")
		}
		exit, found := result.Find(owner, kind.OutcomeReturn, 0)
		if !found {
			return errors.New("program/flow/outcome: missing Return exit")
		}
		result.returnExit[keyspace.TermOrdinal(term)] = exit
	}
	for index := 0; index < breaks.Count(); index++ {
		term, ok := breaks.At(index)
		owner, _, rowOK := breaks.Get(term)
		loop, shapeOK := shape.BreakLoop(term)
		if !ok || !rowOK || !shapeOK || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(loop, counts, keyspace.FamilyLoop) {
			return errors.New("program/flow/outcome: invalid Break exit row")
		}
		exit, found := result.Find(owner, kind.OutcomeBreak, loop)
		if !found {
			return errors.New("program/flow/outcome: missing Break exit")
		}
		result.breakExit[keyspace.TermOrdinal(term)] = exit
	}
	for index := 0; index < gotos.Count(); index++ {
		term, ok := gotos.At(index)
		owner, target, rowOK := gotos.Get(term)
		targetBody, shapeOK := shape.GotoTargetBody(term)
		if !ok || !rowOK || !shapeOK || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(target, counts, keyspace.FamilyLabel) || !validTerm(targetBody, counts, keyspace.FamilyBody) {
			return errors.New("program/flow/outcome: invalid Goto exit row")
		}
		if direct != nil && keyspace.TermOrdinal(term) < uint32(len(direct)) && direct[keyspace.TermOrdinal(term)] != 0 {
			if direct[keyspace.TermOrdinal(term)] != target {
				return errors.New("program/flow/outcome: inconsistent same-Body Goto")
			}
			result.gotoExit[keyspace.TermOrdinal(term)] = target
			continue
		}
		exit, found := result.Find(owner, kind.OutcomeGoto, target)
		if !found {
			return errors.New("program/flow/outcome: missing Goto exit")
		}
		result.gotoExit[keyspace.TermOrdinal(term)] = exit
	}
	return nil
}

func sameActivation(bodies *body.Result, left, right keyspace.Term) bool {
	leftActivation, leftOK := bodies.Activation(left)
	rightActivation, rightOK := bodies.Activation(right)
	return leftOK && rightOK && leftActivation == rightActivation
}

func validTerm(term keyspace.Term, termCounts [keyspace.FamilyCount]int, family keyspace.Family) bool {
	ordinal := keyspace.TermOrdinal(term)
	return keyspace.TermFamily(term) == family && ordinal != 0 && uint64(ordinal) <= uint64(termCounts[family])
}

func validLoopKind(loopKind kind.LoopKind) bool {
	return loopKind >= kind.LoopWhile && loopKind <= kind.LoopGenericFor
}

func deduplicate(keys []outcomeKey) []outcomeKey {
	if len(keys) < 2 {
		return keys
	}
	write := 1
	for read := 1; read < len(keys); read++ {
		if compareKey(keys[read-1], keys[read]) == 0 {
			continue
		}
		keys[write] = keys[read]
		write++
	}
	return keys[:write]
}
