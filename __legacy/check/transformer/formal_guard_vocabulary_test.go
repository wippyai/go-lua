package transformer

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

type formalGuardLifetimeFixture struct {
	program                               *RelationProgram
	vocabulary                            *formalGuardVocabulary
	callerArena, targetArena              *Arena
	callerParam, targetParam              ValueTerm
	targetLocal                           ValueTerm
	callerGuard, callerFalsy, targetGuard Guard
	callerLoop, targetLoop                loopMuTerm
	callerApply                           formalRelationCell
	targetApply                           formalRelationCell
}

func TestFormalGuardVocabularySubstitutesAndClosesExactLifetimes(t *testing.T) {
	left := formalGuardLifetimeTestFixture(t, false)
	vocabulary := left.vocabulary
	if !vocabulary.valid() {
		t.Fatal("formal guard vocabulary is not closed")
	}
	formalGuardTestDeepSealRejectsMalformedRanks(t, left)

	callerRank, callerOK := vocabulary.lexicalRank(1, left.callerLoop, left.callerArena, left.callerParam)
	targetParamRank, targetParamOK := vocabulary.lexicalRank(2, left.targetLoop, left.targetArena, left.targetParam)
	targetLocalRank, targetLocalOK := vocabulary.lexicalRank(2, left.targetLoop, left.targetArena, left.targetLocal)
	if !callerOK || !targetParamOK || !targetLocalOK || targetParamRank == targetLocalRank {
		t.Fatalf("direct ranks caller=%d/%t target=%d/%t local=%d/%t", callerRank, callerOK, targetParamRank, targetParamOK, targetLocalRank, targetLocalOK)
	}

	callerBoundary, ok := vocabulary.applyBoundary(left.callerApply)
	if !ok {
		t.Fatal("caller Apply has no formal guard boundary")
	}
	if got, exact := callerBoundary.rename.target(targetParamRank); !exact || got != callerRank {
		t.Fatalf("callee boundary rank %d -> %d/%t, want caller rank %d", targetParamRank, got, exact, callerRank)
	}
	localAlpha, exact := callerBoundary.rename.target(targetLocalRank)
	if !exact || localAlpha == targetLocalRank || !callerBoundary.close.contains(localAlpha) || callerBoundary.close.contains(callerRank) {
		t.Fatalf("target-local alpha=%d/%t close=%#v caller=%d", localAlpha, exact, callerBoundary.close.ranks, callerRank)
	}
	loopLifetime, ok := vocabulary.loopLifetime(1, left.callerLoop)
	if !ok || !loopLifetime.contains(callerRank) || loopLifetime.contains(localAlpha) {
		t.Fatalf("caller loop lifetime = %#v", loopLifetime.ranks)
	}

	kernel := newDecisionKernel()
	kernel.resetBoolean()
	targetDecision, err := vocabulary.decision(context.Background(), &kernel, 2, left.targetLoop, left.targetArena, left.targetGuard)
	if err != nil {
		t.Fatal(err)
	}
	composed, err := callerBoundary.composeBoolean(context.Background(), &kernel, targetDecision)
	if err != nil {
		t.Fatal(err)
	}
	want, err := vocabulary.decision(context.Background(), &kernel, 1, left.callerLoop, left.callerArena, left.callerGuard)
	if err != nil || composed != want {
		t.Fatalf("Apply guard composition = %d, want caller predicate %d: %v", composed, want, err)
	}

	definition, ok := vocabulary.definitionBoundary(1)
	if !ok {
		t.Fatal("Definition has no formal guard boundary")
	}
	defined, err := definition.composeBoolean(context.Background(), &kernel, targetDecision)
	if err != nil || defined != want {
		t.Fatalf("Definition guard composition = %d, want %d: %v", defined, want, err)
	}

	recursive, ok := vocabulary.applyBoundary(left.targetApply)
	if !ok {
		t.Fatal("recursive Apply has no formal guard boundary")
	}
	if rebound, exact := recursive.rename.target(targetParamRank); !exact || rebound != targetParamRank {
		t.Fatalf("recursive param rank = %d/%t, want stable %d", rebound, exact, targetParamRank)
	}
	recursiveAlpha, exact := recursive.rename.target(targetLocalRank)
	if !exact || recursiveAlpha == targetLocalRank || !recursive.close.contains(recursiveAlpha) {
		t.Fatalf("recursive local alpha = %d/%t close=%#v", recursiveAlpha, exact, recursive.close.ranks)
	}

	closedLoop, err := vocabulary.closeLoopBoolean(context.Background(), &kernel, 1, left.callerLoop, want)
	if err != nil || closedLoop != decisionTrue {
		t.Fatalf("loop feedback/exit closure = %d, want true: %v", closedLoop, err)
	}
	truthy := want
	falsy, err := vocabulary.decision(context.Background(), &kernel, 1, left.callerLoop, left.callerArena, left.callerFalsy)
	if err != nil {
		t.Fatal(err)
	}
	and, err := kernel.apply(context.Background(), uint8(decisionAnd), true, truthy, falsy, decisionLeafAnd)
	if err != nil || and != decisionFalse {
		t.Fatalf("Truthy/Falsy intersection = %d: %v", and, err)
	}
	or, err := kernel.apply(context.Background(), uint8(decisionOr), true, truthy, falsy, decisionLeafOr)
	if err != nil || or != decisionTrue {
		t.Fatalf("Truthy/Falsy union = %d: %v", or, err)
	}

	foreign := kernel.branch(vocabulary.size, decisionFalse, decisionTrue)
	if _, err := callerBoundary.substituteDecision(context.Background(), &kernel, foreign); err == nil {
		t.Fatal("formal guard substitution accepted an unranked rank")
	}
	broken := callerBoundary
	for index, pair := range broken.rename.pairs {
		if pair.source == targetParamRank {
			broken.rename.pairs = append(append([]formalGuardRankPair(nil), broken.rename.pairs[:index]...), broken.rename.pairs[index+1:]...)
			break
		}
	}
	if !broken.valid() {
		t.Fatal("runtime boundary ownership check re-ran deep closure validation")
	}
	if _, err := broken.substituteDecision(context.Background(), &kernel, targetDecision); err == nil {
		t.Fatal("formal guard substitution let an omitted callee atom leak")
	}

	formalGuardTestIrreduciblePortalScope(t)

	right := formalGuardLifetimeTestFixture(t, true)
	if got, wantSnapshot := formalGuardVocabularySnapshot(right.vocabulary), formalGuardVocabularySnapshot(vocabulary); !reflect.DeepEqual(got, wantSnapshot) {
		t.Fatalf("definition permutation changed formal guard vocabulary:\n got %#v\nwant %#v", got, wantSnapshot)
	}
}

func TestSealFormalGuardBoundaryDomainsOnlyMappedCalleeAtoms(t *testing.T) {
	fixture := formalGuardLifetimeTestFixture(t, false)
	source := formalGuardRankKey{
		variable: 2, scope: fixture.targetLoop, arena: fixture.targetArena, term: fixture.targetParam,
	}
	target := formalGuardRankKey{
		variable: 1, scope: fixture.callerLoop, arena: fixture.callerArena, term: fixture.callerParam,
	}
	boundary, err := sealFormalGuardBoundary(fixture.vocabulary, formalGuardBoundaryDraft{
		target:  2,
		sources: map[formalGuardRankKey]formalGuardRankKey{source: target},
	})
	if err != nil || !boundary.validateClosure() {
		t.Fatalf("boundary = %#v, err = %v", boundary, err)
	}
	sourceRank, ok := fixture.vocabulary.ranks[source]
	if !ok || !boundary.domain.contains(sourceRank) {
		t.Fatalf("boundary domain = %#v, want mapped source rank %d", boundary.domain.ranks, sourceRank)
	}
	localRank, ok := fixture.vocabulary.ranks[formalGuardRankKey{
		variable: 2, scope: fixture.targetLoop, arena: fixture.targetArena, term: fixture.targetLocal,
	}]
	if !ok || boundary.domain.contains(localRank) {
		t.Fatalf("boundary domain = %#v, must exclude unrelated callee rank %d", boundary.domain.ranks, localRank)
	}
}

func formalGuardTestDeepSealRejectsMalformedRanks(t *testing.T, fixture formalGuardLifetimeFixture) {
	t.Helper()
	keys := make([]formalGuardRankKey, 0, 2)
	for key := range fixture.vocabulary.ranks {
		keys = append(keys, key)
		if len(keys) == 2 {
			break
		}
	}
	if len(keys) != 2 {
		t.Fatal("formal guard seal fixture has fewer than two ranks")
	}
	duplicate := &formalGuardVocabulary{
		ranks: map[formalGuardRankKey]uint32{keys[0]: 0, keys[1]: 0},
		apply: make(map[formalRelationCell]formalGuardBoundary), definitions: make(map[formalRelationDefinitionRef]formalGuardBoundary),
		loops: make(map[formalGuardLoopLifetime]formalGuardRankSet), size: 2,
	}
	if duplicate.validateClosure() {
		t.Fatal("formal guard seal accepted non-bijective rank values")
	}

	unsorted := &formalGuardVocabulary{
		ranks: map[formalGuardRankKey]uint32{keys[0]: 0, keys[1]: 1},
		apply: make(map[formalRelationCell]formalGuardBoundary), definitions: make(map[formalRelationDefinitionRef]formalGuardBoundary),
		loops: make(map[formalGuardLoopLifetime]formalGuardRankSet), size: 2,
	}
	boundary := formalGuardBoundary{
		owner:  unsorted,
		rename: formalGuardRankMap{owner: unsorted, pairs: []formalGuardRankPair{{source: 0, target: 0}, {source: 1, target: 1}}},
		domain: formalGuardRankSet{owner: unsorted, ranks: []uint32{1, 0}},
		close:  formalGuardRankSet{owner: unsorted},
	}
	unsorted.apply[fixture.callerApply] = boundary
	if unsorted.validateClosure() {
		t.Fatal("formal guard seal accepted a non-canonical boundary domain")
	}
}

func formalGuardLifetimeTestFixture(t *testing.T, reverseDefinitions bool) formalGuardLifetimeFixture {
	t.Helper()
	reg := standard.Registry()
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-guard-lifetimes"))
	callerArena, targetArena := NewArena(reg), NewArena(reg)
	if !callerArena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 1)) || !targetArena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 2)) {
		t.Fatal("bind formal guard owners")
	}
	shape := Shape{Params: 1}
	callerParam := callerArena.Root(Root{Kind: RootParam})
	targetParam := targetArena.Root(Root{Kind: RootParam})
	targetLocal := targetArena.bindEnvironmentSymbol(9101)
	if targetLocal == 0 {
		t.Fatal("bind target-local guard register")
	}
	if err := callerArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	if err := targetArena.sealMiddleRegisterSchema(); err != nil {
		t.Fatal(err)
	}
	callerLoop := callerArena.loopMu(2, 0, []cfg.Point{2, 3, 4}, []loopMuBackedge{{from: 3, to: 2}})
	targetLoop := targetArena.loopMu(12, 0, []cfg.Point{12, 13, 14}, []loopMuBackedge{{from: 13, to: 12}})
	if callerLoop == 0 || targetLoop == 0 {
		t.Fatal("freeze loop binders")
	}
	callerFrame := callerArena.relationFrame(2, 10, 1, shape, []ValueTerm{callerParam}, []PathTerm{0}, 0)
	definitionLeft := callerArena.relationFrame(2, 20, 2, shape, []ValueTerm{callerParam}, []PathTerm{0}, 0)
	definitionRight := callerArena.relationFrame(2, 21, 3, shape, []ValueTerm{callerParam}, []PathTerm{0}, 0)
	targetFrame := targetArena.relationFrame(2, 30, 1, shape, []ValueTerm{targetParam}, []PathTerm{0}, 0)
	if callerFrame == 0 || definitionLeft == 0 || definitionRight == 0 || targetFrame == 0 {
		t.Fatal("freeze formal guard call frames")
	}
	callerGuard := callerArena.Truthy(callerParam)
	callerFalsy := callerArena.Falsy(callerParam)
	targetGuard := targetArena.And(targetArena.Truthy(targetParam), targetArena.Falsy(targetLocal))
	callerEffects, targetEffects := NewEffectArena(callerArena), NewEffectArena(targetArena)
	callerCode := &relationCode{
		terms: callerArena, effects: callerEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes: []relationNode{
			{},
			{kind: relationNodeLoopMu, binder: callerLoop, body: 2, exits: []relationRootRef{5}},
			{kind: relationNodeChoice, guard: callerGuard, whenTrue: 3, whenFalse: 4},
			{kind: relationNodeSequence, steps: []boundaryStep{
				{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: callerFrame}},
				{kind: boundaryStepLoopFeedback, binder: callerLoop},
			}},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, guard: callerFalsy, binder: callerLoop, route: 0}}},
			{kind: relationNodeOutcome, outcome: 1},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}}, contributions: []semanticContribution{{}},
		publication: relationPublicationPlan{points: []relationPointPublication{{point: 20, ref: 3}, {point: 21, ref: 3}}},
	}
	targetCode := &relationCode{
		terms: targetArena, effects: targetEffects, descriptors: DefaultDescriptorRegistry(), shape: shape, root: 1,
		nodes: []relationNode{
			{},
			{kind: relationNodeLoopMu, binder: targetLoop, body: 2, exits: []relationRootRef{5}},
			{kind: relationNodeChoice, guard: targetGuard, whenTrue: 3, whenFalse: 4},
			{kind: relationNodeSequence, steps: []boundaryStep{
				{kind: boundaryStepApply, apply: relationApplyRef{variable: 2, frame: targetFrame}},
				{kind: boundaryStepLoopFeedback, binder: targetLoop},
			}},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopExit, binder: targetLoop, route: 0}}},
			{kind: relationNodeOutcome, outcome: 1},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}}, contributions: []semanticContribution{{}},
	}
	codes := []*relationCode{callerCode, targetCode}
	definitions := []relationProgramDefinition{
		{owner: 1, target: 2, point: 20, frame: definitionLeft},
		{owner: 1, target: 2, point: 21, frame: definitionRight},
	}
	if reverseDefinitions {
		definitions[0], definitions[1] = definitions[1], definitions[0]
	}
	closeAndFreezeRelationGuardTestForest(t, codes, definitions...)
	callerCode.sealed, targetCode.sealed = true, true
	program := formalRegionTestProgram(callerCode, targetCode)
	program.definitions = definitions
	program.recursiveSCCs = [][]relationVar{{2}}
	region, err := freezeFormalRelationRegionInventory(program)
	if err != nil {
		t.Fatal(err)
	}
	program.formalRegion = region
	vocabulary, err := freezeFormalGuardVocabulary(program)
	if err != nil {
		t.Fatal(err)
	}
	return formalGuardLifetimeFixture{
		program: program, vocabulary: vocabulary, callerArena: callerArena, targetArena: targetArena,
		callerParam: callerParam, targetParam: targetParam, targetLocal: targetLocal,
		callerGuard: callerGuard, callerFalsy: callerFalsy, targetGuard: targetGuard, callerLoop: callerLoop, targetLoop: targetLoop,
		callerApply: formalRelationCell{Variable: 1, Root: 3, Step: 1, Kind: formalRelationCellStep},
		targetApply: formalRelationCell{Variable: 2, Root: 3, Step: 1, Kind: formalRelationCellStep},
	}
}

func formalGuardVocabularySnapshot(vocabulary *formalGuardVocabulary) []string {
	if !vocabulary.valid() {
		return nil
	}
	out := make([]string, 0, len(vocabulary.ranks)+len(vocabulary.apply)+len(vocabulary.definitions))
	for key, rank := range vocabulary.ranks {
		out = append(out, formalGuardSnapshotRank(key, rank))
	}
	for site, boundary := range vocabulary.apply {
		out = append(out, formalGuardSnapshotBoundary("apply", uint32(site.Variable), uint32(site.Root), site.Step, boundary))
	}
	for definition, boundary := range vocabulary.definitions {
		out = append(out, formalGuardSnapshotBoundary("definition", uint32(definition), 0, 0, boundary))
	}
	sort.Strings(out)
	return out
}

func formalGuardSnapshotRank(key formalGuardRankKey, rank uint32) string {
	return fmt.Sprintf("%s:%d:%d:%d:%d:%d:%d", key.arena.canonicalValue(key.term), key.variable, key.scope, key.root, key.step, key.definition, rank)
}

func formalGuardSnapshotBoundary(prefix string, owner, root, step uint32, boundary formalGuardBoundary) string {
	out := fmt.Sprintf("%s:%d:%d:%d", prefix, owner, root, step)
	for _, pair := range boundary.rename.pairs {
		out += fmt.Sprintf(":%d>%d", pair.source, pair.target)
	}
	for _, rank := range boundary.domain.ranks {
		out += fmt.Sprintf(":d%d", rank)
	}
	for _, rank := range boundary.close.ranks {
		out += fmt.Sprintf(":x%d", rank)
	}
	return out
}

func formalGuardTestIrreduciblePortalScope(t *testing.T) {
	t.Helper()
	arena := NewArena(standard.Registry())
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-guard-portal"))
	if !arena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 1)) {
		t.Fatal("bind LoopPortal guard owner")
	}
	param := arena.Root(Root{Kind: RootParam})
	binder := arena.loopMu(2, 0, []cfg.Point{2, 3}, []loopMuBackedge{{from: 3, to: 2}})
	guard := arena.Truthy(param)
	code := &relationCode{terms: arena, root: 1, nodes: []relationNode{
		{},
		{kind: relationNodeLoopPortal, binder: binder, body: 2},
		{kind: relationNodeChoice, guard: guard, whenTrue: 3, whenFalse: 3},
		{kind: relationNodeOutcome, outcome: 1},
	}, outcomes: []boundaryOutcomeTuple{{}, {}}}
	scopes, parents, err := formalGuardLexicalScopes(code)
	if err != nil {
		t.Fatal(err)
	}
	if scopes[2] != binder || scopes[3] != binder || parents[binder] != 0 {
		t.Fatalf("LoopPortal scopes = %#v parents=%#v", scopes, parents)
	}
	arena.Seal()
	guards, err := reachableScopedRelationGuards(code)
	if err != nil || len(guards) != 1 || guards[0].guard != guard || guards[0].scope != binder {
		t.Fatalf("LoopPortal guard inventory = %#v: %v", guards, err)
	}
}

func TestFormalGuardOuterLoopLifetimeContainsNestedPortalScopeCone(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	namespace := lexicalidentity.UnitNamespaceFromContent([]byte("formal-guard-nested-portal-cone"))
	if !arena.bindLexicalOwner(lexicalidentity.FunctionBody(namespace, 1)) {
		t.Fatal("bind nested LoopPortal guard owner")
	}
	outerAtom := arena.Root(Root{Kind: RootParam, Index: 0})
	innerAtom := arena.Root(Root{Kind: RootParam, Index: 1})
	outerGuard, innerGuard := arena.Truthy(outerAtom), arena.Truthy(innerAtom)
	outer := arena.loopMu(2, 0, []cfg.Point{2, 3, 4, 5, 6, 7}, []loopMuBackedge{{from: 7, to: 2}})
	inner := arena.loopMu(5, 0, []cfg.Point{5, 6}, []loopMuBackedge{{from: 6, to: 5}})
	if outer == 0 || inner == 0 || outer == inner {
		t.Fatal("freeze nested loop binders")
	}
	code := &relationCode{
		terms: arena,
		root:  1,
		nodes: []relationNode{
			{},
			{kind: relationNodeLoopMu, binder: outer, body: 2, exits: []relationRootRef{8}},
			{kind: relationNodeChoice, guard: outerGuard, whenTrue: 3, whenFalse: 4},
			{kind: relationNodeLoopMu, binder: inner, body: 5, exits: []relationRootRef{7}},
			{kind: relationNodeLoopPortal, binder: inner, body: 5},
			{kind: relationNodeChoice, guard: innerGuard, whenTrue: 6, whenFalse: 6},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: inner}}},
			{kind: relationNodeSequence, steps: []boundaryStep{{kind: boundaryStepLoopFeedback, binder: outer}}},
			{kind: relationNodeOutcome, outcome: 1},
		},
		outcomes: []boundaryOutcomeTuple{{}, {}},
	}
	arena.Seal()
	code.sealed = true
	program := formalRegionTestProgram(code)
	program.formalRegion = &formalRelationRegionInventory{}
	vocabulary, err := freezeFormalGuardVocabulary(program)
	if err != nil {
		t.Fatal(err)
	}
	outerRank, outerOK := vocabulary.lexicalRank(1, outer, arena, outerAtom)
	innerRank, innerOK := vocabulary.lexicalRank(1, inner, arena, innerAtom)
	outerLifetime, outerLifetimeOK := vocabulary.loopLifetime(1, outer)
	innerLifetime, innerLifetimeOK := vocabulary.loopLifetime(1, inner)
	if !outerOK || !innerOK || !outerLifetimeOK || !innerLifetimeOK {
		t.Fatalf("nested ranks/lifetimes missing: outer=%d/%t/%t inner=%d/%t/%t", outerRank, outerOK, outerLifetimeOK, innerRank, innerOK, innerLifetimeOK)
	}
	if !outerLifetime.contains(outerRank) || !outerLifetime.contains(innerRank) {
		t.Fatalf("outer lifetime omitted descendant cone: ranks=%v outer=%d inner=%d", outerLifetime.ranks, outerRank, innerRank)
	}
	if innerLifetime.contains(outerRank) || !innerLifetime.contains(innerRank) {
		t.Fatalf("inner lifetime escaped its descendant cone: ranks=%v outer=%d inner=%d", innerLifetime.ranks, outerRank, innerRank)
	}

	kernel := newDecisionKernel()
	kernel.resetBoolean()
	outerDecision, err := vocabulary.decision(context.Background(), &kernel, 1, outer, arena, outerGuard)
	if err != nil {
		t.Fatal(err)
	}
	innerDecision, err := vocabulary.decision(context.Background(), &kernel, 1, inner, arena, innerGuard)
	if err != nil {
		t.Fatal(err)
	}
	both, err := kernel.apply(context.Background(), uint8(decisionAnd), true, outerDecision, innerDecision, decisionLeafAnd)
	if err != nil {
		t.Fatal(err)
	}
	innerClosed, err := vocabulary.closeLoopBoolean(context.Background(), &kernel, 1, inner, both)
	if err != nil || innerClosed != outerDecision {
		t.Fatalf("inner closure = %d, want outer atom %d: %v", innerClosed, outerDecision, err)
	}
	outerClosed, err := vocabulary.closeLoopBoolean(context.Background(), &kernel, 1, outer, both)
	if err != nil || outerClosed != decisionTrue {
		t.Fatalf("outer closure = %d, want true: %v", outerClosed, err)
	}
}
