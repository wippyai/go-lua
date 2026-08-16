package position

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal derives the one sparse Source position batch from the already sealed
// semantic owners.  A non-Outcome Term is positioned iff its containment
// parent closure reaches a direct Source occurrence.  The closure is solved
// with dense per-family state and an iterative path-compression walk; no
// alternate graph or discovery-order index is retained.
//
// Body rows and Outcome origins are copied from their sealed owners.  The
// returned batch is complete or absent: an invalid foreign result never
// produces a partially usable IndexInput.
// The explicit Static and Module IDs fence the post-containment owners before
// any rows are read.  This projection emits only Source IndexInput and retains
// no identity, token, adapter, digest, or alternate source authority.
func Seal(
	preimage source.Preimage,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	outcomes *outcome.Result,
	entry keyspace.Term,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (source.IndexInput, error) {
	var empty source.IndexInput

	counts, total, err := validateInputs(preimage, flow, bodies, forest, outcomes, entry, staticID, moduleID)
	if err != nil {
		return empty, err
	}

	bodyRows, err := sealBodies(bodies, forest, counts, entry)
	if err != nil {
		return empty, err
	}

	direct, err := directSources(preimage, bodies, forest, counts)
	if err != nil {
		return empty, err
	}

	if err := sealLoops(flow, bodies, forest, counts, direct); err != nil {
		return empty, err
	}

	positionCount, err := anchorClosure(forest, counts, total, direct)
	if err != nil {
		return empty, err
	}
	positions, err := emitPositions(counts, direct, positionCount)
	if err != nil {
		return empty, err
	}

	origins, err := sealOutcomeOrigins(outcomes, counts)
	if err != nil {
		return empty, err
	}

	return source.IndexInput{
		SourceID:       preimage.Identity().ContentID(),
		Positions:      positions,
		Bodies:         bodyRows,
		OutcomeOrigins: origins,
		Entry:          entry,
	}, nil
}

// positionAnchor is the complete coordinate copied by every term in one
// direct-anchor closure.  directTable starts as the direct Source table and is
// consumed in place by anchorClosure: direct rows are never overwritten, and
// non-direct rows receive their resolved direct anchor. A zero anchor remains
// an unambiguous rootless result.
type positionAnchor struct {
	root           keyspace.Term
	body           keyspace.Term
	offset         uint32
	cursor         uint32
	frontierBody   keyspace.Term
	frontierCursor uint32
	repeat         bool
}

type directTable [keyspace.FamilyCount][]positionAnchor

func validateInputs(
	preimage source.Preimage,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	outcomes *outcome.Result,
	entry keyspace.Term,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) ([keyspace.FamilyCount]uint32, int, error) {
	var counts [keyspace.FamilyCount]uint32
	identity := preimage.Identity()
	sourceID := identity.ContentID()
	flowID := flow.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return counts, 0, errors.New("program/flow/position: Source preimage is unavailable")
	}
	if bodies == nil || forest == nil || outcomes == nil {
		return counts, 0, errors.New("program/flow/position: sealed owner is nil")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return counts, 0, errors.New("program/flow/position: Body provenance disagrees with Source or Flow")
	}
	if !containment.Matches(forest, sourceID, flowID, staticID, moduleID) {
		return counts, 0, errors.New("program/flow/position: containment provenance disagrees with Source, Flow, Static, or Module")
	}
	if !outcome.Matches(outcomes, sourceID, flowID, staticID, moduleID) {
		return counts, 0, errors.New("program/flow/position: Outcome provenance disagrees with Source, Flow, Static, or Module")
	}
	if identity.Name() == "" || identity.TermCount() == 0 {
		return counts, 0, errors.New("program/flow/position: Source preimage is unavailable")
	}

	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, 0, errors.New("program/flow/position: invalid Source family denominator")
		}
		if family == keyspace.FamilyOutcome {
			if count != 0 {
				return counts, 0, errors.New("program/flow/position: authored Outcome denominator is nonzero")
			}
			continue
		}
		counts[family] = uint32(count)
		total += uint64(count)
	}
	if total != uint64(identity.TermCount()) || total == 0 {
		return counts, 0, errors.New("program/flow/position: Source denominator mismatch")
	}
	if total > uint64(maxInt()) {
		return counts, 0, errors.New("program/flow/position: Source denominator is too large")
	}
	if bodies.BodyCount() != int(counts[keyspace.FamilyBody]) {
		return counts, 0, errors.New("program/flow/position: Body denominator mismatch")
	}
	if forest.Count() != int(total) {
		return counts, 0, errors.New("program/flow/position: containment denominator mismatch")
	}
	if !validTerm(entry, counts, keyspace.FamilyBody) {
		return counts, 0, errors.New("program/flow/position: invalid Entry Body")
	}
	if flow.Control().Loops().Count() != int(counts[keyspace.FamilyLoop]) {
		return counts, 0, errors.New("program/flow/position: Loop denominator mismatch")
	}

	// The containment result is the canonical pre-Outcome denominator. Each
	// expected Term must be a member of its own interval. Counts already match
	// the complete denominator, so this O(1)-per-term membership proof also
	// proves exact family distribution without rescanning the canonical At list.
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, ordinal)
			if !forest.Contains(term, term) {
				return counts, 0, errors.New("program/flow/position: containment denominator is not canonical")
			}
		}
	}
	return counts, int(total), nil
}

func maxInt() int {
	return int(^uint(0) >> 1)
}

func validTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32, family keyspace.Family) bool {
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount &&
		keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) <= counts[family]
}

func validPreOutcomeTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	family := keyspace.TermFamily(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && family != keyspace.FamilyOutcome &&
		keyspace.TermOrdinal(term) <= counts[family]
}

func sealBodies(
	bodies *body.Result,
	forest *containment.Result,
	counts [keyspace.FamilyCount]uint32,
	entry keyspace.Term,
) ([]source.BodyRoots, error) {
	rows := make([]source.BodyRoots, bodies.BodyCount())
	for index := range rows {
		bodyTerm, ok := bodies.BodyAt(index)
		if !ok || bodyTerm != keyspace.MakeTerm(keyspace.FamilyBody, uint32(index+1)) {
			return nil, errors.New("program/flow/position: Body result is not dense")
		}
		parent, hasParent := bodies.Parent(bodyTerm)
		if bodyTerm == entry {
			if hasParent || parent != 0 {
				return nil, errors.New("program/flow/position: Entry Body has a parent")
			}
		} else {
			if !hasParent || !validTerm(parent, counts, keyspace.FamilyBody) || parent == bodyTerm {
				return nil, errors.New("program/flow/position: malformed Body parent")
			}
		}
		forestParent, forestHasParent := forest.Parent(bodyTerm)
		if hasParent != forestHasParent || (hasParent && forestParent != parent) {
			return nil, errors.New("program/flow/position: Body and containment parents disagree")
		}

		rootCount, ok := bodies.RootCount(bodyTerm)
		if !ok || rootCount < 0 {
			return nil, errors.New("program/flow/position: Body roots unavailable")
		}
		roots := make([]keyspace.Term, rootCount)
		for rootIndex := range roots {
			root, ok := bodies.RootAt(bodyTerm, rootIndex)
			if !ok || !validPreOutcomeTerm(root, counts) {
				return nil, errors.New("program/flow/position: malformed Body root")
			}
			rootParent, rootHasParent := forest.Parent(root)
			if !rootHasParent || rootParent != bodyTerm {
				return nil, errors.New("program/flow/position: Body root lacks direct containment")
			}
			roots[rootIndex] = root
		}
		rows[index] = source.BodyRoots{Body: bodyTerm, Parent: parent, Roots: roots}
	}
	return rows, nil
}

func directSources(preimage source.Preimage, bodies *body.Result, forest *containment.Result, counts [keyspace.FamilyCount]uint32) (directTable, error) {
	var direct directTable
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		direct[family] = make([]positionAnchor, counts[family])
	}
	order := preimage.Order()
	for bodyOrdinal := uint32(1); bodyOrdinal <= counts[keyspace.FamilyBody]; bodyOrdinal++ {
		owner := keyspace.MakeTerm(keyspace.FamilyBody, bodyOrdinal)
		length, ok := order.BodyLen(owner)
		if !ok || length < 0 {
			return direct, errors.New("program/flow/position: Source Body order unavailable")
		}
		rootCount, ok := bodies.RootCount(owner)
		if !ok {
			return direct, errors.New("program/flow/position: Body roots unavailable")
		}
		cursor := 0
		for offset := 0; offset < length; offset++ {
			term, ok := order.BodyAt(owner, offset)
			if !ok || !validPreOutcomeTerm(term, counts) {
				return direct, errors.New("program/flow/position: malformed direct Source term")
			}
			family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
			row := &direct[family][ordinal-1]
			if row.root != 0 {
				return direct, errors.New("program/flow/position: duplicate direct Source term")
			}
			cursorBefore := cursor
			if cursor < rootCount {
				root, rootOK := bodies.RootAt(owner, cursor)
				if rootOK && root == term {
					cursor++
				}
			}
			row.root = term
			row.body = owner
			row.offset = uint32(offset)
			// cursor is the number of statement roots strictly preceding term.
			row.cursor = uint32(cursorBefore)
			row.frontierBody = owner
			row.frontierCursor = uint32(cursorBefore)
			if parent, hasParent := forest.Parent(term); !hasParent || parent != owner {
				return direct, errors.New("program/flow/position: direct Source containment mismatch")
			}
		}
		if cursor != rootCount {
			return direct, errors.New("program/flow/position: Body root is absent from Source order")
		}
	}
	return direct, nil
}

func sealLoops(
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	counts [keyspace.FamilyCount]uint32,
	direct directTable,
) error {
	loops := flow.Control().Loops()
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		if !ok || loop != keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1)) {
			return errors.New("program/flow/position: Loop result is not dense")
		}
		owner, loopBody, loopKind, _, ok := loops.Get(loop)
		if !ok || !validTerm(owner, counts, keyspace.FamilyBody) || !validTerm(loopBody, counts, keyspace.FamilyBody) || owner == loopBody {
			return errors.New("program/flow/position: malformed Loop owner or Body")
		}
		parent, hasParent := bodies.Parent(loopBody)
		forestParent, forestHasParent := forest.Parent(loopBody)
		if !hasParent || parent != owner || !forestHasParent || forestParent != owner {
			return errors.New("program/flow/position: Loop Body is not an exact owner child")
		}
		anchor := direct[keyspace.FamilyLoop][keyspace.TermOrdinal(loop)-1]
		if anchor.root != loop || anchor.body != owner {
			return errors.New("program/flow/position: Loop lacks its direct Source occurrence")
		}
		if loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor {
			return errors.New("program/flow/position: invalid Loop kind")
		}
		if loopKind != kind.LoopRepeat {
			continue
		}
		rootCount, ok := bodies.RootCount(loopBody)
		if !ok || rootCount < 0 {
			return errors.New("program/flow/position: Repeat frontier roots unavailable")
		}
		anchor.frontierBody = loopBody
		anchor.frontierCursor = uint32(rootCount)
		anchor.repeat = true
		direct[keyspace.FamilyLoop][keyspace.TermOrdinal(loop)-1] = anchor
	}
	return nil
}

func anchorClosure(
	forest *containment.Result,
	counts [keyspace.FamilyCount]uint32,
	total int,
	direct directTable,
) (int, error) {
	if total < 0 {
		return 0, errors.New("program/flow/position: negative containment denominator")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome || uint64(len(direct[family])) == uint64(counts[family]) {
			continue
		}
		return 0, errors.New("program/flow/position: direct Source table denominator mismatch")
	}
	var status [keyspace.FamilyCount][]uint8
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		status[family] = make([]uint8, counts[family])
	}
	path := make([]keyspace.Term, total)
	positionCount := 0
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			if status[family][ordinal-1] == 2 {
				continue
			}
			pathLength := 0
			current := keyspace.MakeTerm(family, ordinal)
			var resolved positionAnchor
			for {
				currentFamily, currentOrdinal := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
				if currentFamily <= keyspace.FamilyInvalid || currentFamily >= keyspace.FamilyCount || currentFamily == keyspace.FamilyOutcome || currentOrdinal == 0 || currentOrdinal > counts[currentFamily] {
					return 0, fmt.Errorf("program/flow/position: containment parent is outside pre-Outcome denominator: %v", current)
				}
				// Inspect the closure status before looking at the table. A status-2
				// row is already resolved, including a direct terminal; it must not
				// be mistaken for a new direct occurrence.
				state := status[currentFamily][currentOrdinal-1]
				switch state {
				case 1:
					return 0, errors.New("program/flow/position: containment cycle")
				case 2:
					resolved = direct[currentFamily][currentOrdinal-1]
					current = 0
				default:
					status[currentFamily][currentOrdinal-1] = 1
					if pathLength >= len(path) {
						return 0, errors.New("program/flow/position: containment path exceeds denominator")
					}
					path[pathLength] = current
					pathLength++
					// Only an unresolved status-0 row whose anchor root is itself is
					// a direct terminal. Direct rows are retained verbatim below;
					// non-direct rows are the only rows eligible for overwrite.
					anchor := direct[currentFamily][currentOrdinal-1]
					if anchor.root == current {
						resolved = anchor
						current = 0
					} else {
						parent, hasParent := forest.Parent(current)
						if !hasParent {
							resolved = positionAnchor{}
							current = 0
						} else {
							if !validPreOutcomeTerm(parent, counts) {
								return 0, fmt.Errorf("program/flow/position: containment parent is invalid: %v", parent)
							}
							current = parent
							continue
						}
					}
				}
				if current == 0 {
					break
				}
			}
			if resolved.root != 0 {
				if positionCount > maxInt()-pathLength {
					return 0, errors.New("program/flow/position: positioned count overflows")
				}
				positionCount += pathLength
			}
			for pathIndex := 0; pathIndex < pathLength; pathIndex++ {
				term := path[pathIndex]
				termFamily, termOrdinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
				// Direct rows are the original source-coordinate authority and
				// must never be overwritten. A rootless resolution intentionally
				// leaves non-direct rows zero. Every path member, including the
				// direct terminal, is nevertheless marked resolved.
				if direct[termFamily][termOrdinal-1].root != term {
					direct[termFamily][termOrdinal-1] = resolved
				}
				status[termFamily][termOrdinal-1] = 2
			}
		}
	}
	return positionCount, nil
}

func emitPositions(
	counts [keyspace.FamilyCount]uint32,
	direct directTable,
	positionCount int,
) ([]source.Position, error) {
	if positionCount < 0 {
		return nil, errors.New("program/flow/position: positioned count is negative")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome || uint64(len(direct[family])) == uint64(counts[family]) {
			continue
		}
		return nil, errors.New("program/flow/position: direct Source table denominator mismatch")
	}
	positions := make([]source.Position, positionCount)
	write := 0
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if family == keyspace.FamilyOutcome {
			continue
		}
		for ordinal := uint32(1); ordinal <= counts[family]; ordinal++ {
			anchor := direct[family][ordinal-1]
			if anchor.root == 0 {
				continue
			}
			anchorFamily, anchorOrdinal := keyspace.TermFamily(anchor.root), keyspace.TermOrdinal(anchor.root)
			if anchorFamily <= keyspace.FamilyInvalid || anchorFamily >= keyspace.FamilyCount || anchorFamily == keyspace.FamilyOutcome || anchorOrdinal == 0 || anchorOrdinal > counts[anchorFamily] || uint64(anchorOrdinal) > uint64(len(direct[anchorFamily])) || direct[anchorFamily][anchorOrdinal-1].root != anchor.root {
				return nil, errors.New("program/flow/position: anchor is not a direct Source term")
			}
			if write >= len(positions) {
				return nil, errors.New("program/flow/position: positioned count is short")
			}
			positions[write] = source.Position{
				Term:           keyspace.MakeTerm(family, ordinal),
				Root:           anchor.root,
				Body:           anchor.body,
				Offset:         anchor.offset,
				Cursor:         anchor.cursor,
				FrontierBody:   anchor.frontierBody,
				FrontierCursor: anchor.frontierCursor,
				Repeat:         anchor.repeat,
			}
			write++
		}
	}
	if write != positionCount {
		return nil, errors.New("program/flow/position: positioned count is inconsistent")
	}
	return positions, nil
}

func sealOutcomeOrigins(outcomes *outcome.Result, counts [keyspace.FamilyCount]uint32) ([]keyspace.Term, error) {
	count := outcomes.Count()
	if count == 0 {
		return nil, errors.New("program/flow/position: Outcome result is empty")
	}
	origins := make([]keyspace.Term, count)
	for index := range origins {
		term, ok := outcomes.At(index)
		if !ok {
			return nil, errors.New("program/flow/position: Outcome result is not dense")
		}
		origin, _, _, ok := outcomes.Get(term)
		if !ok || !validTerm(origin, counts, keyspace.FamilyBody) {
			return nil, errors.New("program/flow/position: malformed Outcome origin")
		}
		origins[index] = origin
	}
	return origins, nil
}
