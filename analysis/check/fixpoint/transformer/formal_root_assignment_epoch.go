package transformer

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/formal"
)

// formalRootAssignmentEpochContract is the freeze-time proof that a root
// assignment starts a new observation epoch for one immutable lexical class.
// The class is structural; guarded aliases and write alphabets deliberately do
// not participate in this contract.
type formalRootAssignmentEpochContract struct {
	target  formal.Root
	class   formal.LexicalClassID
	classes []formal.LexicalClassID
	members []formal.Root
	close   formalGuardRankSet
}

func (c formalRootAssignmentEpochContract) validFor(owner *formalGuardVocabulary, target formal.Root) bool {
	if owner == nil || !owner.valid() || !target.Valid() || !c.class.Valid() ||
		c.class.Owner() != target.Owner() || c.close.owner != owner || !c.close.validateClosure() || len(c.members) == 0 || len(c.classes) == 0 {
		return false
	}
	found := false
	classFound := false
	for index, class := range c.classes {
		if !class.Valid() || class.Owner() != target.Owner() || index != 0 && c.classes[index-1].Ordinal() >= class.Ordinal() {
			return false
		}
		classFound = classFound || class == c.class
	}
	for index, member := range c.members {
		if !member.Valid() || member.Owner() != target.Owner() || index != 0 && !c.members[index-1].Less(member) {
			return false
		}
		found = found || member == target
	}
	return found && classFound
}

// stableRoots returns the class only when the frozen program has a predicate
// whose condition observed that class.  A root write otherwise retains the
// ordinary concrete mutation law; it must not erase guarded consequences that
// merely happen to be stored on a class-tagged coordinate.
func (c formalRootAssignmentEpochContract) stableRoots() []formal.Root {
	if len(c.close.ranks) == 0 {
		return nil
	}
	return append([]formal.Root(nil), c.members...)
}

// freezeFormalRootAssignmentEpochContract binds every ranked predicate whose
// observed value reads the target's lexical class.  Input and Middle spellings
// are intentionally unified only through the registry's immutable class map.
func freezeFormalRootAssignmentEpochContract(
	program *RelationProgram,
	body *relationProgramBody,
	variable relationVar,
	target FormalSlot,
) (formalRootAssignmentEpochContract, error) {
	if program == nil || program.formalFibers == nil || program.formalGuards == nil || !program.formalGuards.valid() ||
		body == nil || body.variable != variable || !target.Valid() || target.Body() != body.body {
		return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch contract is unowned")
	}
	registryIndex := int(variable - 1)
	if registryIndex < 0 || registryIndex >= len(program.formalFibers.coordinateRegistries) {
		return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch registry is outside the formal forest")
	}
	registry := program.formalFibers.coordinateRegistries[registryIndex]
	targetRoot, exact := target.Root()
	if !exact || registry == nil {
		return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch target has no formal root")
	}
	class, classified := registry.class(targetRoot)
	if !classified {
		return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch target has no lexical class")
	}
	// Alias facts intentionally remain guarded and are not folded into this
	// lexical relation. A root write advances only its immutable class.
	classes := []formal.LexicalClassID{class}
	members := make([]formal.Root, 0)
	for _, memberClass := range classes {
		members = append(members, registry.classMembers(memberClass)...)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].Less(members[j]) })
	if len(members) == 0 {
		return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch class has no members")
	}

	ranks := make([]uint32, 0)
	for key, rank := range program.formalGuards.ranks {
		if key.variable != variable || key.arena != body.relation.code.terms {
			continue
		}
		readSlots, err := body.valueTermReadSlots(key.term)
		if err != nil {
			return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch guard reads: %w", err)
		}
		readsClass := false
		for _, readSlot := range readSlots {
			live, present := formalMiddleSlotForStateKey(program, body, readSlot)
			if !present {
				return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch guard read has no live formal slot")
			}
			root, rootPresent := live.Root()
			if !rootPresent {
				return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch guard read has no lexical class")
			}
			// A decision becomes stale only when its own observed root is
			// overwritten.  A class-tagged coordinate can participate in other
			// facts (including guarded aliases), but that alone does not make the
			// predicate's condition depend on this write's old epoch.
			readsClass = readsClass || root == targetRoot
		}
		if readsClass && formalEpochGuardObservesRoot(key) {
			ranks = append(ranks, rank)
		}
	}
	sort.Slice(ranks, func(i, j int) bool { return ranks[i] < ranks[j] })
	for index := 1; index < len(ranks); index++ {
		if ranks[index-1] == ranks[index] {
			return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch ranks are not unique")
		}
	}
	contract := formalRootAssignmentEpochContract{
		target: targetRoot, class: class, classes: classes, members: members,
		close: formalGuardRankSet{owner: program.formalGuards, ranks: ranks},
	}
	if !contract.validFor(program.formalGuards, targetRoot) {
		return formalRootAssignmentEpochContract{}, fmt.Errorf("transformer: RootAssignment epoch contract is malformed")
	}
	return contract, nil
}

// formalEpochGuardObservesRoot admits only predicates whose observed value is
// the root itself or one of its static members.  Derived guard expressions
// (for example `type(x) == "table"`) may read a class coordinate while their
// condition remains scoped to the expression result; closing their dependent
// consequences on a root write would globalize an unrelated guard epoch.
func formalEpochGuardObservesRoot(key formalGuardRankKey) bool {
	if key.arena == nil || key.term == 0 || int(key.term) >= len(key.arena.values) {
		return false
	}
	node := key.arena.values[key.term]
	if node.op == valuePredicateObservation {
		if len(node.args) != 1 || node.args[0] == 0 || int(node.args[0]) >= len(key.arena.values) {
			return false
		}
		node = key.arena.values[node.args[0]]
	}
	return node.op == valueRoot || node.op == valueStaticIndex
}

// advanceFormalRootAssignmentEpoch conditionally closes every predicate rank
// bound to the assigned lexical class.  The inactive route is retained before
// the closed route is joined back, so a guarded assignment never globalizes a
// branch-local alias fact.
func (a *formalTupleAlgebra) advanceFormalRootAssignmentEpoch(
	tuple formalRelationTuple,
	contract formalRootAssignmentEpochContract,
	execute decisionRef,
) (formalRelationTuple, error) {
	if a == nil || a.program == nil || a.program.formalGuards == nil || tuple.bottom() ||
		int(execute) >= len(a.decisions.nodes) {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	span, _, _, owned := a.span(tuple.variable)
	if !owned || !contract.validFor(a.program.formalGuards, contract.target) || span.variable != tuple.variable {
		return formalRelationTuple{}, errFormalComponentForeignOwner
	}
	if len(contract.close.ranks) == 0 || execute == decisionFalse {
		return tuple, nil
	}
	boundary := formalGuardBoundary{
		owner:  a.program.formalGuards,
		rename: formalGuardRankMap{owner: a.program.formalGuards},
		domain: formalGuardRankSet{owner: a.program.formalGuards},
		close:  contract.close,
	}
	if execute == decisionTrue {
		return a.composeGuardBoundary(tuple, boundary)
	}
	active, err := a.restrictTupleCare(tuple, execute)
	if err != nil {
		return formalRelationTuple{}, err
	}
	if active.bottom() {
		return tuple, nil
	}
	advanced, err := a.composeGuardBoundary(active, boundary)
	if err != nil {
		return formalRelationTuple{}, err
	}
	skipGuard, err := formalDecisionBooleanNot(a, execute)
	if err != nil {
		return formalRelationTuple{}, err
	}
	skip, err := a.restrictTupleCare(tuple, skipGuard)
	if err != nil {
		return formalRelationTuple{}, err
	}
	if skip.bottom() {
		return advanced, nil
	}
	return a.combine(formalComponentJoin, advanced, skip), a.err()
}
