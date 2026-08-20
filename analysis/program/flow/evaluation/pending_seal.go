package evaluation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// SealPending derives the immutable pending projection from one committed
// Source view, authored Flow, executable proof, and complete candidate proof.
// Static and Module are scalar provenance fences supplied by the post-
// containment assembly; they are matched exactly but no owner authority is
// retained. Source order is consumed only at this seal boundary.
func SealPending(
	sourceView source.View,
	view authored.View,
	executableResult *executable.Result,
	candidateResult *candidates.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
) (*Pending, error) {
	identity := sourceView.Identity()
	counts, err := pendingCounts(identity, view)
	if err != nil {
		return nil, err
	}
	flowID := view.ContentID()
	if executableResult == nil || !executable.Matches(executableResult, identity.ContentID(), flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/evaluation: executable provenance is unavailable")
	}
	if candidateResult == nil || !candidates.Matches(candidateResult, identity.ContentID(), flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/evaluation: candidate provenance is unavailable")
	}
	if !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/evaluation: Static/Module provenance is unavailable")
	}
	builder := &pendingBuilder{
		view:       view,
		executable: executableResult,
		candidates: candidateResult,
		store:      newPendingTermStore(),
	}
	for index, family := range pendingAncestorFamilyKeys {
		count := counts[family]
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return nil, errors.New("program/flow/evaluation: invalid pending family cardinality")
		}
		builder.demand[index] = make([]bool, count+1)
	}
	for index, family := range pendingAncestorFamilyKeys {
		count := counts[family]
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return nil, errors.New("program/flow/evaluation: invalid pending parent cardinality")
		}
		builder.parents[index] = make([]keyspace.Term, count+1)
	}
	for index, family := range pendingClaimFamilyKeys {
		count := counts[family]
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return nil, errors.New("program/flow/evaluation: invalid pending payload cardinality")
		}
		builder.claimed[index] = make([]bool, count+1)
	}
	for _, family := range pendingSubjectFamilies {
		builder.roots[family] = make([]uint32, counts[family]+1)
	}

	walker, err := New(view)
	if err != nil {
		return nil, err
	}
	if err := discoverPendingParents(walker, builder, counts); err != nil {
		return nil, err
	}
	scratch, err := newPendingProofScratch(counts)
	if err != nil {
		return nil, err
	}
	if err := validatePendingParentForestWithScratch(builder, counts, scratch); err != nil {
		return nil, err
	}
	if err := markPendingDemandWithScratch(builder, counts, scratch); err != nil {
		return nil, err
	}
	resetEvaluationSeen(walker)
	walker.pending = builder
	if err := walkPendingSourceRoots(walker, sourceView, counts); err != nil {
		return nil, err
	}
	if err := validatePendingStorage(builder.store.nodes, builder.roots); err != nil {
		return nil, err
	}
	return &Pending{
		sourceID: identity.ContentID(),
		flowID:   flowID,
		staticID: staticID,
		moduleID: moduleID,
		nodes:    builder.store.nodes,
		roots:    builder.roots,
		sealed:   true,
	}, nil
}

// validatePendingParentForest proves that discovery's retained relation is
// acyclic. The sealed Executable input can only be issued after canonical
// Containment has already validated the complete authored forest, including
// dead runtime exact-key edges. Pending therefore does not reconstruct those
// non-executable edges as a second structural authority. The state plane is
// keyed by the same exact ancestor vocabulary and the iterative path is
// discarded after each walk.
// pendingProofScratch is Seal-local working storage for the two iterative
// ancestor proofs. The state bytes and path backing array are reset between
// proofs; neither is retained by pendingBuilder or the published Pending.
type pendingProofScratch struct {
	state [pendingAncestorFamilyCount][]uint8
	path  []keyspace.Term
}

func newPendingProofScratch(counts [keyspace.FamilyCount]int) (*pendingProofScratch, error) {
	scratch := &pendingProofScratch{}
	for index, family := range pendingAncestorFamilyKeys {
		count := counts[family]
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return nil, errors.New("program/flow/evaluation: invalid pending proof cardinality")
		}
		scratch.state[index] = make([]uint8, count+1)
	}
	return scratch, nil
}

func (scratch *pendingProofScratch) reset() {
	if scratch == nil {
		return
	}
	for index := range scratch.state {
		clear(scratch.state[index])
	}
	scratch.path = scratch.path[:0]
}

func validatePendingParentForestWithScratch(builder *pendingBuilder, counts [keyspace.FamilyCount]int, scratch *pendingProofScratch) error {
	if builder == nil {
		return errors.New("program/flow/evaluation: pending parent owner is unavailable")
	}
	if scratch == nil {
		return errors.New("program/flow/evaluation: pending proof scratch is unavailable")
	}
	scratch.reset()
	state := scratch.state
	path := scratch.path
	for index, family := range pendingAncestorFamilyKeys {
		for ordinal := 1; ordinal <= counts[family]; ordinal++ {
			if state[index][ordinal] == 2 {
				continue
			}
			current := keyspace.MakeTerm(family, uint32(ordinal))
			path = path[:0]
			for current != 0 {
				currentFamily, currentOrdinal := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
				currentIndex, ok := pendingAncestorIndex(currentFamily)
				if !ok || currentOrdinal == 0 || uint64(currentOrdinal) >= uint64(len(state[currentIndex])) {
					return errors.New("program/flow/evaluation: pending parent leaves dense universe")
				}
				switch state[currentIndex][currentOrdinal] {
				case 1:
					return errors.New("program/flow/evaluation: cyclic pending containment")
				case 2:
					current = 0
					continue
				}
				state[currentIndex][currentOrdinal] = 1
				path = append(path, current)
				current = builder.parents[currentIndex][currentOrdinal]
			}
			for _, term := range path {
				termIndex, ok := pendingAncestorIndex(keyspace.TermFamily(term))
				if !ok {
					return errors.New("program/flow/evaluation: pending parent leaves dense universe")
				}
				state[termIndex][keyspace.TermOrdinal(term)] = 2
			}
		}
	}
	scratch.path = path
	return nil
}

var pendingSubjectFamilies = [...]keyspace.Family{
	keyspace.FamilyUnary, keyspace.FamilyBinary, keyspace.FamilyRead,
	keyspace.FamilyWrite, keyspace.FamilyCall, keyspace.FamilyLoop,
}

func pendingCounts(identity source.Identity, view authored.View) ([keyspace.FamilyCount]int, error) {
	counts, err := pendingSourceCounts(identity)
	if err != nil {
		return counts, err
	}
	if !view.ContentID().Available() {
		return counts, errors.New("program/flow/evaluation: authored view is unavailable")
	}
	if err := validateAuthoredCounts(view, counts); err != nil {
		return counts, err
	}
	return counts, nil
}

// pendingSourceCounts accepts the committed Source identity, whose derived
// Outcome family is installed after Flow position sealing. Pending's universe
// remains the authored pre-Outcome denominator, so the derived family is
// validated in the total but deliberately omitted from every dense plane.
func pendingSourceCounts(identity source.Identity) ([keyspace.FamilyCount]int, error) {
	var counts [keyspace.FamilyCount]int
	if !identity.ContentID().Available() || identity.Name() == "" || identity.TermCount() == 0 {
		return counts, errors.New("program/flow/evaluation: Source identity is unavailable")
	}
	outcomeCount := identity.FamilyCount(keyspace.FamilyOutcome)
	if outcomeCount < 0 || !keyspace.TermOrdinalFits(outcomeCount) {
		return counts, errors.New("program/flow/evaluation: invalid Outcome cardinality")
	}
	var authoredTotal uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, errors.New("program/flow/evaluation: invalid Source family cardinality")
		}
		if family == keyspace.FamilyOutcome {
			continue
		}
		counts[family] = count
		authoredTotal += uint64(count)
	}
	if authoredTotal+uint64(outcomeCount) != uint64(identity.TermCount()) || counts[keyspace.FamilyBody] == 0 {
		return counts, errors.New("program/flow/evaluation: Source family cardinality mismatch")
	}
	return counts, nil
}

func discoverPendingParents(walker *Session, builder *pendingBuilder, counts [keyspace.FamilyCount]int) error {
	if walker == nil || builder == nil {
		return errors.New("program/flow/evaluation: pending discovery owner is unavailable")
	}
	builder.discover = true
	walker.pending = builder
	for _, family := range pendingAncestorFamilyKeys {
		for ordinal := 1; ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			owner, hasOwner, err := walker.owner(term)
			if err != nil {
				return err
			}
			if !hasOwner {
				continue
			}
			plane := walker.seen[family]
			if uint64(ordinal) >= uint64(len(plane)) || plane[ordinal] {
				continue
			}
			// Process one canonical seed at a time. Keeping all seeds on the
			// iterative stack makes the discovery frontier proportional to the
			// entire authored universe, while a one-seed drain keeps it bounded
			// by the currently walked path. Child edges are still recorded before
			// pushWithPrefix applies the occurrence plane, so already-seen child
			// terms retain their exact authored edge without a second walk.
			plane[ordinal] = true
			walker.done = false
			walker.stack = append(walker.stack, frame{term: term, owner: owner})
			for len(walker.stack) != 0 {
				if _, _, err := walker.Next(); err != nil {
					return err
				}
			}
		}
	}
	walker.done = true
	builder.discover = false
	return nil
}

func markPendingDemandWithScratch(builder *pendingBuilder, counts [keyspace.FamilyCount]int, scratch *pendingProofScratch) error {
	if builder == nil {
		return errors.New("program/flow/evaluation: pending demand owner is unavailable")
	}
	if scratch == nil {
		return errors.New("program/flow/evaluation: pending proof scratch is unavailable")
	}
	scratch.reset()
	state := scratch.state
	path := scratch.path
	for _, family := range pendingSubjectFamilies {
		for ordinal := 1; ordinal <= counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, uint32(ordinal))
			if !pendingSubject(builder.view, builder.executable, builder.candidates, term) {
				continue
			}
			builder.subjectsExpected++
			current := term
			path = path[:0]
			for current != 0 {
				currentFamily, currentOrdinal := keyspace.TermFamily(current), keyspace.TermOrdinal(current)
				currentIndex, currentIsAncestor := pendingAncestorIndex(currentFamily)
				if !currentIsAncestor || currentOrdinal == 0 || uint64(currentOrdinal) >= uint64(len(state[currentIndex])) {
					return errors.New("program/flow/evaluation: pending demand leaves dense universe")
				}
				if builder.executable == nil || !builder.executable.Contains(current) {
					return errors.New("program/flow/evaluation: pending demand crosses a non-executable parent")
				}
				if state[currentIndex][currentOrdinal] == 1 {
					return errors.New("program/flow/evaluation: cyclic pending containment")
				}
				if state[currentIndex][currentOrdinal] == 2 {
					break
				}
				state[currentIndex][currentOrdinal] = 1
				builder.demand[currentIndex][currentOrdinal] = true
				path = append(path, current)
				current = builder.parents[currentIndex][currentOrdinal]
			}
			for index := len(path) - 1; index >= 0; index-- {
				term := path[index]
				familyIndex, ok := pendingAncestorIndex(keyspace.TermFamily(term))
				if !ok {
					return errors.New("program/flow/evaluation: pending demand leaves dense universe")
				}
				state[familyIndex][keyspace.TermOrdinal(term)] = 2
			}
		}
	}
	scratch.path = path
	return nil
}

func resetEvaluationSeen(walker *Session) {
	if walker == nil {
		return
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := range walker.seen[family] {
			walker.seen[family][ordinal] = false
		}
	}
	walker.stack = walker.stack[:0]
	walker.done = true
	walker.failed = nil
	walker.event = Event{}
	walker.emitted = false
}

func walkPendingSourceRoots(walker *Session, sourceView source.View, counts [keyspace.FamilyCount]int) error {
	for ordinal := 1; ordinal <= counts[keyspace.FamilyBody]; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		length, ok := sourceView.Order().BodyLen(body)
		if !ok || length < 0 {
			return errors.New("program/flow/evaluation: Source Body order is unavailable")
		}
		for offset := 0; offset < length; offset++ {
			root, rootOK := sourceView.Order().BodyAt(body, offset)
			if !rootOK {
				return errors.New("program/flow/evaluation: Source Body root is unavailable")
			}
			family, rootOrdinal := keyspace.TermFamily(root), keyspace.TermOrdinal(root)
			if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || rootOrdinal == 0 || uint64(rootOrdinal) > uint64(counts[family]) {
				return errors.New("program/flow/evaluation: Source Body root leaves Source denominator")
			}
			// Source.Index is the direct-root authority. Position(root) must
			// identify this exact Body/offset; a repeated dead or static root
			// therefore fails here without a second seen-root plane or scan.
			indexedRoot, indexed := sourceView.Index().Root(root)
			rootBody, rootOffset, _, positioned := sourceView.Index().Position(root)
			if !indexed || indexedRoot != root || !positioned || rootBody != body || rootOffset != offset {
				return errors.New("program/flow/evaluation: duplicate or conflicting Source direct root")
			}
			// Source order also carries direct non-expression evidence (type
			// declarations and control faults). Those rows are outside the
			// evaluation vocabulary and cannot be pending subjects.
			if family == keyspace.FamilyTypeAlias || family == keyspace.FamilyTypeInterface || family == keyspace.FamilyControlFault {
				continue
			}
			if !walker.validTerm(root) {
				return errors.New("program/flow/evaluation: Source Body root is unavailable")
			}
			_, isAncestor := pendingAncestorIndex(family)
			if !isAncestor {
				// Source order also contains scalar/value and declaration roots;
				// they cannot be evaluation ancestors and require no scratch plane.
				continue
			}
			if !walker.pending.needed(root) {
				continue
			}
			if err := walker.Start(root); err != nil {
				return err
			}
			for {
				if _, ok, err := walker.Next(); err != nil {
					return err
				} else if !ok {
					break
				}
			}
		}
	}
	if walker.pending.subjectsWalked != walker.pending.subjectsExpected {
		return errors.New("program/flow/evaluation: pending subject root was not walked")
	}
	return nil
}
