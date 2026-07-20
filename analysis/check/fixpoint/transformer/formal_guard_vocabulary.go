package transformer

import (
	"context"
	"fmt"
	"sort"
)

// formalGuardRankKey is one Boolean variable in the canonical formal tuple
// algebra. A zero site is lexical body syntax. A Step or Definition site is
// an alpha name whose lifetime is exactly one boundary composition.
type formalGuardRankKey struct {
	variable   relationVar
	scope      loopMuTerm
	root       relationRootRef
	step       uint32
	definition formalRelationDefinitionRef
	arena      *Arena
	term       ValueTerm
}

func (k formalGuardRankKey) valid() bool {
	if k.variable == 0 || k.arena == nil || k.term == 0 {
		return false
	}
	if k.definition != 0 {
		return k.root == 0 && k.step == 0
	}
	return k.root == 0 && k.step == 0 || k.root != 0 && k.step != 0
}

type formalGuardRankPair struct{ source, target uint32 }

// formalGuardRankMap is a closed exact variable substitution. Every rank in
// the boundary's callee domain must be present; only non-domain ranks already
// owned by a mixed caller root retain identity. Pairs are source-sorted and
// duplicate-free.
type formalGuardRankMap struct {
	owner *formalGuardVocabulary
	pairs []formalGuardRankPair
}

func (m formalGuardRankMap) target(source uint32) (uint32, bool) {
	if m.owner == nil || source >= m.owner.size {
		return 0, false
	}
	index := sort.Search(len(m.pairs), func(index int) bool { return m.pairs[index].source >= source })
	if index < len(m.pairs) && m.pairs[index].source == source {
		return m.pairs[index].target, m.pairs[index].target < m.owner.size
	}
	return 0, false
}

type formalGuardRankSet struct {
	owner *formalGuardVocabulary
	ranks []uint32
}

func (s formalGuardRankSet) contains(rank uint32) bool {
	if s.owner == nil || rank >= s.owner.size {
		return false
	}
	index := sort.Search(len(s.ranks), func(index int) bool { return s.ranks[index] >= rank })
	return index < len(s.ranks) && s.ranks[index] == rank
}

// formalGuardBoundary is the complete Boolean transaction for one exact
// Apply or Definition occurrence. Rename binds callee syntax into caller
// ranks. Close removes only invocation-local alpha ranks after composition.
type formalGuardBoundary struct {
	owner  *formalGuardVocabulary
	rename formalGuardRankMap
	domain formalGuardRankSet
	close  formalGuardRankSet
}

func (b formalGuardBoundary) valid() bool {
	return b.owner != nil && b.owner.valid() && b.rename.owner == b.owner && b.domain.owner == b.owner && b.close.owner == b.owner
}

// validateClosure proves the complete immutable boundary representation once,
// before its owner is sealed. Runtime users call valid, whose ownership check
// is deliberately O(1).
func (b formalGuardBoundary) validateClosure() bool {
	if b.owner == nil || b.rename.owner != b.owner || b.domain.owner != b.owner || b.close.owner != b.owner {
		return false
	}
	for index, pair := range b.rename.pairs {
		if pair.source >= b.owner.size || pair.target >= b.owner.size || index != 0 && b.rename.pairs[index-1].source >= pair.source {
			return false
		}
	}
	if !b.domain.validateClosure() || !b.close.validateClosure() {
		return false
	}
	for _, rank := range b.domain.ranks {
		if _, mapped := b.rename.target(rank); !mapped {
			return false
		}
	}
	return true
}

type formalGuardLoopLifetime struct {
	variable relationVar
	binder   loopMuTerm
}

// formalGuardVocabulary is the one immutable Boolean variable order for the
// caller-independent formal equation forest. It owns no route, invocation,
// concrete State, guard AST, or evaluator. Guards remain Arena syntax and the
// tuple decisionKernel remains the sole ROBDD representation.
type formalGuardVocabulary struct {
	ranks       map[formalGuardRankKey]uint32
	apply       map[formalRelationCell]formalGuardBoundary
	definitions map[formalRelationDefinitionRef]formalGuardBoundary
	loops       map[formalGuardLoopLifetime]formalGuardRankSet
	size        uint32
	sealed      bool
}

// valid is the executor's O(1) ownership check. sealed is set only after
// validateClosure has proved every rank and boundary deeply.
func (v *formalGuardVocabulary) valid() bool {
	return v != nil && v.sealed && v.ranks != nil && v.apply != nil && v.definitions != nil && v.loops != nil && uint32(len(v.ranks)) == v.size
}

func (s formalGuardRankSet) validateClosure() bool {
	if s.owner == nil {
		return false
	}
	for index, rank := range s.ranks {
		if rank >= s.owner.size || index != 0 && s.ranks[index-1] >= rank {
			return false
		}
	}
	return true
}

// validateClosure is the sole vocabulary-wide proof. It is intentionally
// freeze-only: executor lookups must never pay work proportional to the
// vocabulary or boundary inventory.
func (v *formalGuardVocabulary) validateClosure() bool {
	if v == nil || v.sealed || v.ranks == nil || v.apply == nil || v.definitions == nil || v.loops == nil || uint32(len(v.ranks)) != v.size {
		return false
	}
	seenRanks := make([]bool, v.size)
	for key, rank := range v.ranks {
		if !key.valid() || rank >= v.size || seenRanks[rank] {
			return false
		}
		seenRanks[rank] = true
	}
	for _, present := range seenRanks {
		if !present {
			return false
		}
	}
	for site, boundary := range v.apply {
		if site.Kind != formalRelationCellStep || boundary.owner != v || !boundary.validateClosure() {
			return false
		}
	}
	for definition, boundary := range v.definitions {
		if definition == 0 || boundary.owner != v || !boundary.validateClosure() {
			return false
		}
	}
	for lifetime, ranks := range v.loops {
		if lifetime.variable == 0 || lifetime.binder == 0 || ranks.owner != v || !ranks.validateClosure() {
			return false
		}
	}
	return true
}

func (v *formalGuardVocabulary) lexicalRank(variable relationVar, scope loopMuTerm, arena *Arena, term ValueTerm) (uint32, bool) {
	if !v.valid() {
		return 0, false
	}
	rank, ok := v.ranks[formalGuardRankKey{variable: variable, scope: scope, arena: arena, term: term}]
	return rank, ok
}

func (v *formalGuardVocabulary) applyBoundary(site formalRelationCell) (formalGuardBoundary, bool) {
	if !v.valid() || site.Kind != formalRelationCellStep {
		return formalGuardBoundary{}, false
	}
	boundary, ok := v.apply[site]
	return boundary, ok && boundary.valid()
}

func (v *formalGuardVocabulary) definitionBoundary(definition formalRelationDefinitionRef) (formalGuardBoundary, bool) {
	if !v.valid() || definition == 0 {
		return formalGuardBoundary{}, false
	}
	boundary, ok := v.definitions[definition]
	return boundary, ok && boundary.valid()
}

func (v *formalGuardVocabulary) loopLifetime(variable relationVar, binder loopMuTerm) (formalGuardRankSet, bool) {
	if !v.valid() || variable == 0 || binder == 0 {
		return formalGuardRankSet{}, false
	}
	ranks, ok := v.loops[formalGuardLoopLifetime{variable: variable, binder: binder}]
	return ranks, ok && ranks.owner == v
}

type formalGuardRankRecord struct {
	key       formalGuardRankKey
	canonical string
}

type formalGuardBoundaryDraft struct {
	target  relationVar
	sources map[formalGuardRankKey]formalGuardRankKey
}

func freezeFormalGuardVocabulary(program *RelationProgram) (*formalGuardVocabulary, error) {
	if program == nil || program.formalRegion == nil || len(program.bodies) == 0 {
		return nil, fmt.Errorf("transformer: formal guard vocabulary is unowned")
	}
	records := make([]formalGuardRankRecord, 0)
	seen := make(map[formalGuardRankKey]string)
	add := func(key formalGuardRankKey) error {
		if !key.valid() || !key.arena.Sealed() || int(key.term) >= len(key.arena.values) {
			return fmt.Errorf("transformer: formal guard rank has foreign syntax")
		}
		canonical := key.arena.canonicalValue(key.term)
		if canonical == "" {
			return fmt.Errorf("transformer: formal guard rank has no canonical term")
		}
		if prior, exists := seen[key]; exists {
			if prior != canonical {
				return fmt.Errorf("transformer: formal guard rank changed canonical syntax")
			}
			return nil
		}
		seen[key] = canonical
		records = append(records, formalGuardRankRecord{key: key, canonical: canonical})
		return nil
	}

	scopes := make([][]loopMuTerm, len(program.bodies))
	parents := make([]map[loopMuTerm]loopMuTerm, len(program.bodies))
	for bodyIndex := range program.bodies {
		variable, code := relationVar(bodyIndex+1), program.bodies[bodyIndex].relation.code
		if code == nil || !code.sealed || code.terms == nil || !code.terms.Sealed() {
			return nil, fmt.Errorf("transformer: formal guard member %d is unsealed", variable)
		}
		var err error
		scopes[bodyIndex], parents[bodyIndex], err = formalGuardLexicalScopes(code)
		if err != nil {
			return nil, fmt.Errorf("transformer: formal guard member %d: %w", variable, err)
		}
		guards, err := reachableScopedRelationGuards(code)
		if err != nil {
			return nil, err
		}
		for _, scoped := range guards {
			atoms := make(map[ValueTerm]struct{})
			if err := collectRelationGuardAtoms(code.terms, scoped.guard, atoms, make(map[Guard]uint8)); err != nil {
				return nil, err
			}
			for term := range atoms {
				if err := add(formalGuardRankKey{variable: variable, scope: scoped.scope, arena: code.terms, term: term}); err != nil {
					return nil, err
				}
			}
		}
	}

	applyDrafts := make(map[formalRelationCell]formalGuardBoundaryDraft)
	definitionDrafts := make(map[formalRelationDefinitionRef]formalGuardBoundaryDraft)
	for bodyIndex := range program.bodies {
		caller, code := relationVar(bodyIndex+1), program.bodies[bodyIndex].relation.code
		for root := relationRootRef(1); int(root) < len(code.nodes); root++ {
			for stepIndex, step := range code.nodes[root].steps {
				if step.kind != boundaryStepApply {
					continue
				}
				if int(root) >= len(scopes[bodyIndex]) || step.apply.frame == 0 || int(step.apply.frame) >= len(code.applicationGuards) {
					return nil, fmt.Errorf("transformer: formal guard Apply site is unranked")
				}
				plan := code.applicationGuards[step.apply.frame]
				if plan.definition || !plan.validFor(step.apply.frame, step.apply.variable) {
					return nil, fmt.Errorf("transformer: formal guard Apply site has no frozen binding")
				}
				site := formalRelationCell{Variable: caller, Root: root, Step: uint32(stepIndex + 1), Kind: formalRelationCellStep}
				draft, err := freezeFormalGuardBoundaryDraft(add, caller, scopes[bodyIndex][root], site, 0, plan)
				if err != nil {
					return nil, fmt.Errorf("transformer: formal guard Apply %+v: %w", site, err)
				}
				applyDrafts[site] = draft
			}
		}
	}
	for ref := formalRelationDefinitionRef(1); int(ref) < len(program.formalRegion.definitions); ref++ {
		definition := program.formalRegion.definitions[ref]
		ownerCode := program.bodies[definition.owner-1].relation.code
		if definition.frame == 0 || int(definition.frame) >= len(ownerCode.applicationGuards) {
			return nil, fmt.Errorf("transformer: formal guard Definition %d has no frame", ref)
		}
		plan := ownerCode.applicationGuards[definition.frame]
		if !plan.definition || !plan.validFor(definition.frame, definition.target) {
			return nil, fmt.Errorf("transformer: formal guard Definition %d has no frozen binding", ref)
		}
		scope, err := formalGuardDefinitionScope(ownerCode, scopes[definition.owner-1], definition)
		if err != nil {
			return nil, fmt.Errorf("transformer: formal guard Definition %d: %w", ref, err)
		}
		draft, err := freezeFormalGuardBoundaryDraft(add, definition.owner, scope, formalRelationCell{}, ref, plan)
		if err != nil {
			return nil, fmt.Errorf("transformer: formal guard Definition %d: %w", ref, err)
		}
		definitionDrafts[ref] = draft
	}

	sort.Slice(records, func(left, right int) bool { return formalGuardRankRecordLess(records[left], records[right]) })
	ranks := make(map[formalGuardRankKey]uint32, len(records))
	for index, record := range records {
		if index != 0 {
			prior := records[index-1]
			if formalGuardRankCoordinatesEqual(prior.key, record.key) && prior.canonical == record.canonical && prior.key != record.key {
				return nil, fmt.Errorf("transformer: formal guard canonical identity collision")
			}
		}
		ranks[record.key] = uint32(index)
	}
	vocabulary := &formalGuardVocabulary{
		ranks: ranks, apply: make(map[formalRelationCell]formalGuardBoundary, len(applyDrafts)),
		definitions: make(map[formalRelationDefinitionRef]formalGuardBoundary, len(definitionDrafts)),
		loops:       make(map[formalGuardLoopLifetime]formalGuardRankSet), size: uint32(len(records)),
	}
	for site, draft := range applyDrafts {
		boundary, err := sealFormalGuardBoundary(vocabulary, draft)
		if err != nil {
			return nil, err
		}
		vocabulary.apply[site] = boundary
	}
	for definition, draft := range definitionDrafts {
		boundary, err := sealFormalGuardBoundary(vocabulary, draft)
		if err != nil {
			return nil, err
		}
		vocabulary.definitions[definition] = boundary
	}
	for bodyIndex := range program.bodies {
		variable := relationVar(bodyIndex + 1)
		for binder := range parents[bodyIndex] {
			var loopRanks []uint32
			for key, rank := range ranks {
				if key.variable != variable || key.root != 0 || key.definition != 0 {
					continue
				}
				descends, err := formalGuardScopeDescendsFrom(key.scope, binder, parents[bodyIndex])
				if err != nil {
					return nil, fmt.Errorf("transformer: formal guard member %d: %w", variable, err)
				}
				if descends {
					loopRanks = append(loopRanks, rank)
				}
			}
			vocabulary.loops[formalGuardLoopLifetime{variable: variable, binder: binder}] = freezeFormalGuardRankSet(vocabulary, loopRanks)
		}
	}
	if !vocabulary.validateClosure() {
		return nil, fmt.Errorf("transformer: formal guard vocabulary failed closure")
	}
	vocabulary.sealed = true
	return vocabulary, nil
}

// formalGuardScopeDescendsFrom proves lexical mu containment using the scope
// parent tree frozen from relationCode. A feedback edge for an outer binder
// ends every guard lifetime created anywhere in that iteration, including
// atoms owned by nested reducible loops and irreducible LoopPortal entries.
// The walk is structural and finite: a malformed parent cycle rejects freeze.
func formalGuardScopeDescendsFrom(scope, ancestor loopMuTerm, parents map[loopMuTerm]loopMuTerm) (bool, error) {
	if scope == 0 || ancestor == 0 || parents == nil {
		return false, nil
	}
	seen := make(map[loopMuTerm]struct{})
	for scope != 0 {
		if scope == ancestor {
			return true, nil
		}
		if _, cycle := seen[scope]; cycle {
			return false, fmt.Errorf("formal guard loop scope tree contains a cycle at %d", scope)
		}
		seen[scope] = struct{}{}
		next, ok := parents[scope]
		if !ok || next == scope {
			if next == scope {
				return false, fmt.Errorf("formal guard loop scope %d is its own parent", scope)
			}
			return false, nil
		}
		scope = next
	}
	return false, nil
}

func freezeFormalGuardBoundaryDraft(
	add func(formalGuardRankKey) error,
	caller relationVar,
	callerScope loopMuTerm,
	site formalRelationCell,
	definition formalRelationDefinitionRef,
	plan relationApplicationGuardPlan,
) (formalGuardBoundaryDraft, error) {
	if add == nil || caller == 0 || !plan.validFor(plan.frame, plan.target) || site.valid() == (definition != 0) {
		return formalGuardBoundaryDraft{}, fmt.Errorf("boundary draft is malformed")
	}
	bound := make(map[ValueTerm]struct{}, len(plan.boundAtoms))
	for _, term := range plan.boundAtoms {
		bound[term] = struct{}{}
	}
	draft := formalGuardBoundaryDraft{target: plan.target, sources: make(map[formalGuardRankKey]formalGuardRankKey)}
	for _, pair := range plan.guards {
		for _, atom := range pair.atoms {
			source := formalGuardRankKey{variable: plan.target, scope: pair.targetScope, arena: plan.binding.targetArena, term: atom.source}
			local := atom.targetLocal
			if _, invocationLocal := bound[atom.substituted]; invocationLocal {
				local = true
			}
			var target formalGuardRankKey
			if local {
				target = formalGuardRankKey{
					variable: caller, scope: callerScope, root: site.Root, step: site.Step, definition: definition,
					arena: plan.binding.targetArena, term: atom.source,
				}
			} else {
				target = formalGuardRankKey{variable: caller, scope: callerScope, arena: plan.binding.callerArena, term: atom.substituted}
			}
			if err := add(target); err != nil {
				return formalGuardBoundaryDraft{}, err
			}
			if prior, duplicate := draft.sources[source]; duplicate && prior != target {
				return formalGuardBoundaryDraft{}, fmt.Errorf("callee guard rank has conflicting bindings")
			}
			draft.sources[source] = target
		}
	}
	return draft, nil
}

func sealFormalGuardBoundary(vocabulary *formalGuardVocabulary, draft formalGuardBoundaryDraft) (formalGuardBoundary, error) {
	if vocabulary == nil || draft.target == 0 || draft.sources == nil {
		return formalGuardBoundary{}, fmt.Errorf("transformer: formal guard boundary is unowned")
	}
	pairs := make([]formalGuardRankPair, 0, len(draft.sources))
	var closeRanks []uint32
	var domainRanks []uint32
	for key, rank := range vocabulary.ranks {
		if key.variable == draft.target && key.root == 0 && key.definition == 0 {
			domainRanks = append(domainRanks, rank)
		}
	}
	for source, target := range draft.sources {
		sourceRank, sourceOK := vocabulary.ranks[source]
		targetRank, targetOK := vocabulary.ranks[target]
		if !sourceOK || !targetOK {
			return formalGuardBoundary{}, fmt.Errorf("transformer: formal guard boundary contains an unranked atom")
		}
		pairs = append(pairs, formalGuardRankPair{source: sourceRank, target: targetRank})
		if target.root != 0 || target.definition != 0 {
			closeRanks = append(closeRanks, targetRank)
		}
	}
	sort.Slice(pairs, func(left, right int) bool { return pairs[left].source < pairs[right].source })
	boundary := formalGuardBoundary{
		owner: vocabulary, rename: formalGuardRankMap{owner: vocabulary, pairs: pairs},
		domain: freezeFormalGuardRankSet(vocabulary, domainRanks),
		close:  freezeFormalGuardRankSet(vocabulary, closeRanks),
	}
	if !boundary.validateClosure() {
		return formalGuardBoundary{}, fmt.Errorf("transformer: formal guard boundary failed closure")
	}
	return boundary, nil
}

func freezeFormalGuardRankSet(owner *formalGuardVocabulary, ranks []uint32) formalGuardRankSet {
	ordered := append([]uint32(nil), ranks...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	write := 0
	for _, rank := range ordered {
		if write != 0 && ordered[write-1] == rank {
			continue
		}
		ordered[write] = rank
		write++
	}
	return formalGuardRankSet{owner: owner, ranks: ordered[:write]}
}

func formalGuardRankRecordLess(left, right formalGuardRankRecord) bool {
	a, b := left.key, right.key
	if a.variable != b.variable {
		return a.variable < b.variable
	}
	if a.scope != b.scope {
		return a.scope < b.scope
	}
	if a.definition != b.definition {
		return a.definition < b.definition
	}
	if a.root != b.root {
		return a.root < b.root
	}
	if a.step != b.step {
		return a.step < b.step
	}
	return left.canonical < right.canonical
}

func formalGuardRankCoordinatesEqual(left, right formalGuardRankKey) bool {
	return left.variable == right.variable && left.scope == right.scope && left.root == right.root && left.step == right.step && left.definition == right.definition
}

func formalGuardLexicalScopes(code *relationCode) ([]loopMuTerm, map[loopMuTerm]loopMuTerm, error) {
	if code == nil || code.root == 0 || int(code.root) >= len(code.nodes) {
		return nil, nil, fmt.Errorf("lexical scope inventory is unowned")
	}
	type visit struct {
		root  relationRootRef
		scope loopMuTerm
	}
	scopes := make([]loopMuTerm, len(code.nodes))
	arrived := make([]bool, len(code.nodes))
	parents := make(map[loopMuTerm]loopMuTerm)
	queue := []visit{{root: code.root}}
	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if current.root == 0 || int(current.root) >= len(code.nodes) {
			return nil, nil, fmt.Errorf("lexical scope escaped relationCode")
		}
		if arrived[current.root] {
			if scopes[current.root] != current.scope {
				return nil, nil, fmt.Errorf("node %d has incompatible loop lifetimes", current.root)
			}
			continue
		}
		arrived[current.root], scopes[current.root] = true, current.scope
		node := code.nodes[current.root]
		switch node.kind {
		case relationNodeSequence:
			if node.next != 0 {
				queue = append(queue, visit{root: node.next, scope: current.scope})
			}
		case relationNodeChoice:
			if node.whenTrue != 0 {
				queue = append(queue, visit{root: node.whenTrue, scope: current.scope})
			}
			if node.whenFalse != 0 {
				queue = append(queue, visit{root: node.whenFalse, scope: current.scope})
			}
		case relationNodeLoopMu, relationNodeLoopPortal:
			if node.binder == 0 {
				return nil, nil, fmt.Errorf("loop node %d has no binder", current.root)
			}
			if parent, exists := parents[node.binder]; exists && parent != current.scope {
				return nil, nil, fmt.Errorf("binder %d has incompatible parents", node.binder)
			}
			parents[node.binder] = current.scope
			queue = append(queue, visit{root: node.body, scope: node.binder})
			if node.kind == relationNodeLoopMu {
				for _, exit := range node.exits {
					if exit != 0 {
						queue = append(queue, visit{root: exit, scope: current.scope})
					}
				}
			}
		}
	}
	return scopes, parents, nil
}

func formalGuardDefinitionScope(code *relationCode, scopes []loopMuTerm, definition formalRelationDefinition) (loopMuTerm, error) {
	if definition.external {
		return 0, nil
	}
	var scope loopMuTerm
	found := false
	for _, publication := range code.publication.points {
		if publication.point != definition.point || publication.ref == 0 || int(publication.ref) >= len(scopes) {
			continue
		}
		if found && scope != scopes[publication.ref] {
			return 0, fmt.Errorf("publication crosses loop lifetimes")
		}
		scope, found = scopes[publication.ref], true
	}
	if !found {
		// A relation with no point-publication inventory has only its lexical
		// root lifetime. This is not an inferred CFG location: absence of every
		// publication makes loop-local ownership structurally impossible, so the
		// definition is exactly root-scoped.
		if len(code.publication.points) == 0 {
			return 0, nil
		}
		return 0, fmt.Errorf("definition has no lexical publication")
	}
	return scope, nil
}

// decision compiles existing Arena guard syntax directly into the formal
// tuple's ROBDD kernel. Truthy(v) and Falsy(v) are the two branches of the
// same ranked atom and are therefore exact complements by construction.
func (v *formalGuardVocabulary) decision(ctx context.Context, kernel *decisionKernel, variable relationVar, scope loopMuTerm, arena *Arena, guard Guard) (decisionRef, error) {
	if ctx == nil || !v.valid() || kernel == nil || arena == nil || guard == 0 || int(guard) >= len(arena.guards) {
		return 0, fmt.Errorf("transformer: formal guard decision is unowned")
	}
	type frame struct {
		guard    Guard
		expanded bool
	}
	mark := kernel.checkpoint()
	memo := make(map[Guard]decisionRef)
	stack := []frame{{guard: guard}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := memo[current.guard]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if len(memo)&255 == 0 {
			if err := ctx.Err(); err != nil {
				kernel.rollback(mark)
				return 0, err
			}
		}
		if current.guard == 0 || int(current.guard) >= len(arena.guards) {
			kernel.rollback(mark)
			return 0, fmt.Errorf("transformer: formal guard decision contains foreign syntax")
		}
		node := arena.guards[current.guard]
		switch node.op {
		case guardTrue:
			memo[current.guard] = decisionTrue
		case guardFalse:
			memo[current.guard] = decisionFalse
		case guardTruthy, guardFalsy:
			rank, ok := v.lexicalRank(variable, scope, arena, node.value)
			if !ok {
				kernel.rollback(mark)
				return 0, fmt.Errorf("transformer: formal guard atom is unranked")
			}
			if node.op == guardTruthy {
				memo[current.guard] = kernel.branch(rank, decisionFalse, decisionTrue)
			} else {
				memo[current.guard] = kernel.branch(rank, decisionTrue, decisionFalse)
			}
		case guardAnd, guardOr:
			if !current.expanded {
				current.expanded = true
			}
			pending := false
			for _, child := range node.args {
				if child == 0 || int(child) >= len(arena.guards) {
					kernel.rollback(mark)
					return 0, fmt.Errorf("transformer: formal guard connective has foreign child")
				}
				if _, done := memo[child]; !done {
					stack = append(stack, frame{guard: child})
					pending = true
					break
				}
			}
			if pending {
				continue
			}
			children := make([]decisionRef, len(node.args))
			for index, child := range node.args {
				children[index] = memo[child]
			}
			op, identity, leaves := uint8(decisionAnd), decisionTrue, decisionLeafAnd
			if node.op == guardOr {
				op, identity, leaves = uint8(decisionOr), decisionFalse, decisionLeafOr
			}
			decision, err := kernel.reduce(ctx, op, identity, true, children, leaves)
			if err != nil {
				kernel.rollback(mark)
				return 0, err
			}
			memo[current.guard] = decision
		default:
			kernel.rollback(mark)
			return 0, fmt.Errorf("transformer: formal guard decision has invalid syntax")
		}
	}
	root := memo[guard]
	if err := v.validateDecisionRoot(kernel, root, formalGuardRankSet{}); err != nil {
		kernel.rollback(mark)
		return 0, err
	}
	return root, nil
}

// substituteDecision alpha-renames every ranked variable in an arbitrary
// formal component root. ITE reconstruction preserves the global order even
// when a target rank sorts before its source rank.
func (b formalGuardBoundary) substituteDecision(ctx context.Context, kernel *decisionKernel, root decisionRef) (decisionRef, error) {
	if ctx == nil || kernel == nil || !b.valid() {
		return 0, fmt.Errorf("transformer: formal guard substitution is unowned")
	}
	mark := kernel.checkpoint()
	mapped, err := formalRewriteDecisionRoot(ctx, kernel, root, func(variable uint32, low, high decisionRef) (decisionRef, error) {
		target, mapped := b.rename.target(variable)
		if !mapped && b.domain.contains(variable) {
			return 0, fmt.Errorf("transformer: formal guard substitution omitted callee rank %d", variable)
		}
		if !mapped {
			if variable >= b.owner.size {
				return 0, fmt.Errorf("transformer: formal guard substitution encountered foreign rank")
			}
			target = variable
		}
		if target >= b.owner.size {
			return 0, fmt.Errorf("transformer: formal guard substitution encountered foreign rank")
		}
		condition := kernel.branch(target, decisionFalse, decisionTrue)
		return kernel.condition(ctx, condition, high, low)
	})
	if err == nil {
		err = b.owner.validateDecisionRoot(kernel, mapped, formalGuardRankSet{})
	}
	if err != nil {
		kernel.rollback(mark)
		return 0, err
	}
	return mapped, nil
}

// closeBoolean existentially closes a Boolean guard root. It is used for
// guards themselves; tuple components close through their registered lattice
// Join so non-Boolean terminal semantics are never guessed here.
func (b formalGuardBoundary) closeBoolean(ctx context.Context, kernel *decisionKernel, root decisionRef) (decisionRef, error) {
	if !b.valid() {
		return 0, fmt.Errorf("transformer: formal guard boundary is unowned")
	}
	return b.owner.closeBooleanRanks(ctx, kernel, root, b.close)
}

func (b formalGuardBoundary) composeBoolean(ctx context.Context, kernel *decisionKernel, root decisionRef) (decisionRef, error) {
	mapped, err := b.substituteDecision(ctx, kernel, root)
	if err != nil {
		return 0, err
	}
	return b.closeBoolean(ctx, kernel, mapped)
}

func (v *formalGuardVocabulary) closeLoopBoolean(ctx context.Context, kernel *decisionKernel, variable relationVar, binder loopMuTerm, root decisionRef) (decisionRef, error) {
	lifetime, ok := v.loopLifetime(variable, binder)
	if !ok {
		return 0, fmt.Errorf("transformer: formal guard loop lifetime is unranked")
	}
	return v.closeBooleanRanks(ctx, kernel, root, lifetime)
}

func (v *formalGuardVocabulary) closeBooleanRanks(ctx context.Context, kernel *decisionKernel, root decisionRef, ranks formalGuardRankSet) (decisionRef, error) {
	if ctx == nil || !v.valid() || kernel == nil || ranks.owner != v {
		return 0, fmt.Errorf("transformer: formal guard existential is unowned")
	}
	mark := kernel.checkpoint()
	closed, err := formalRewriteDecisionRoot(ctx, kernel, root, func(variable uint32, low, high decisionRef) (decisionRef, error) {
		if !ranks.contains(variable) {
			return kernel.branch(variable, low, high), nil
		}
		return kernel.apply(ctx, uint8(decisionOr), true, low, high, decisionLeafOr)
	})
	if err == nil {
		err = v.validateDecisionRoot(kernel, closed, ranks)
	}
	if err != nil {
		kernel.rollback(mark)
		return 0, err
	}
	return closed, nil
}

func (v *formalGuardVocabulary) validateDecisionRoot(kernel *decisionKernel, root decisionRef, forbidden formalGuardRankSet) error {
	if !v.valid() || kernel == nil || forbidden.owner != nil && forbidden.owner != v {
		return fmt.Errorf("transformer: formal guard rank validation is unowned")
	}
	seen := make(map[decisionRef]struct{})
	stack := []decisionRef{root}
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		node, ok := kernel.node(ref)
		if !ok {
			return fmt.Errorf("transformer: formal guard decision has foreign ROBDD node")
		}
		if node.terminal {
			continue
		}
		if node.variable >= v.size {
			return fmt.Errorf("transformer: formal guard decision has unranked variable %d", node.variable)
		}
		if forbidden.owner != nil && forbidden.contains(node.variable) {
			return fmt.Errorf("transformer: formal guard decision leaked closed rank %d", node.variable)
		}
		stack = append(stack, node.low, node.high)
	}
	return nil
}

func formalRewriteDecisionRoot(
	ctx context.Context,
	kernel *decisionKernel,
	root decisionRef,
	branch func(uint32, decisionRef, decisionRef) (decisionRef, error),
) (decisionRef, error) {
	if ctx == nil || kernel == nil || branch == nil {
		return 0, fmt.Errorf("transformer: formal guard ROBDD rewrite is unowned")
	}
	type frame struct {
		ref      decisionRef
		expanded bool
	}
	memo := make(map[decisionRef]decisionRef)
	stack := []frame{{ref: root}}
	for len(stack) != 0 {
		current := &stack[len(stack)-1]
		if _, done := memo[current.ref]; done {
			stack = stack[:len(stack)-1]
			continue
		}
		if len(memo)&255 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		node, ok := kernel.node(current.ref)
		if !ok {
			return 0, fmt.Errorf("transformer: formal guard ROBDD rewrite has foreign node")
		}
		if node.terminal {
			memo[current.ref] = current.ref
			stack = stack[:len(stack)-1]
			continue
		}
		if !current.expanded {
			current.expanded = true
		}
		if _, done := memo[node.low]; !done {
			stack = append(stack, frame{ref: node.low})
			continue
		}
		if _, done := memo[node.high]; !done {
			stack = append(stack, frame{ref: node.high})
			continue
		}
		mapped, err := branch(node.variable, memo[node.low], memo[node.high])
		if err != nil {
			return 0, err
		}
		memo[current.ref] = mapped
		stack = stack[:len(stack)-1]
	}
	return memo[root], nil
}
