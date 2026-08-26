package obligation

import (
	"testing"

	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	"github.com/wippyai/go-lua/domain/typestate"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// judgmentFixture is the sealed stack one typestate judgment is derived from:
// the Value schema that owns the actuals, the Call algebra that classifies the
// operations, and the Pack schema that says where a declared operation input
// lands in a call row. It is the same seal the mounted-call-argument laws
// build, so this judgment is proved against the analyzer's own derivation
// rather than against a second model of it.
type judgmentFixture struct {
	values *valuedomain.Schema
	calls  *calldomain.Algebra
	packs  *packdomain.Schema
}

func buildJudgmentFixture(t *testing.T, source string) judgmentFixture {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "typestate_obligation.lua", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema unavailable")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("mount unavailable")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	snapshot := snapshottest.MustLower(t, artifact)
	types, err := typeauthority.SealProgramRows(linked.ContentID(), []programschema.Program{artifact.Program()}, nil)
	if err != nil || types == nil {
		t.Fatalf("seal types: %v", err)
	}
	statics, _, err := staticdomain.SealMountedPrograms(
		staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types,
		[]staticdomain.MountedProgram{{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal static: %v", err)
	}
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary unavailable")
	}
	mounted := func() programmount.MountedArtifact {
		artifactMount, ok := programmount.MountedArtifactFromSnapshot(snapshot, module)
		if !ok {
			t.Fatal("artifact mount unavailable")
		}
		return artifactMount
	}
	packs, packsOK := packdomain.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{mounted()})
	if !packsOK || packs == nil {
		t.Fatal("seal pack")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mounted()})
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("seal heap: %s", heapFailure)
	}
	calls := calltest.MustSeal(t, linked, []programmount.MountedArtifact{mounted()})
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calls, []programmount.MountedArtifact{mounted()}, structural)
	if valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal value: %s", valueFailure)
	}
	return judgmentFixture{values: values, calls: calls, packs: packs}
}

const judgmentSource = "local function f(a, b) return a end\nf(1, 2)\n"

// The judgment is issued by the declarations the link sealed. Deriving it
// reads every declared protocol out of the sealed authority the Call target
// contract owns, so the state machine a call is judged against is the one the
// manifest declared rather than one this domain kept beside it.
func TestDerivedJudgmentHoldsTheDeclaredProtocols(t *testing.T) {
	fixture := buildJudgmentFixture(t, judgmentSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok || !judgment.Valid() {
		t.Fatal("judgment not derived from the sealed link")
	}
	contract, contractOK := fixture.calls.TargetContract()
	if !contractOK {
		t.Fatal("target contract unavailable")
	}
	protocols := contract.Protocols()
	if protocols.ProtocolCount() == 0 {
		t.Fatal("the fixture target declares no protocol, so this law would prove nothing")
	}
	for index := 0; index < protocols.ProtocolCount(); index++ {
		handle, handleOK := protocols.ProtocolAt(index)
		if !handleOK {
			t.Fatalf("protocol %d unavailable", index)
		}
		definition, definitionOK := judgment.sealed.definitionFor(handle)
		if !definitionOK {
			t.Fatalf("protocol %d is declared and the judgment holds no state machine for it", handle)
		}
		if len(definition.States) != protocols.StateCount(handle) {
			t.Fatalf("protocol %d holds %d states, the declaration has %d", handle, len(definition.States), protocols.StateCount(handle))
		}
		for stateIndex := 0; stateIndex < protocols.StateCount(handle); stateIndex++ {
			state, stateOK := protocols.StateAt(handle, stateIndex)
			if !stateOK {
				t.Fatalf("protocol %d state %d unavailable", handle, stateIndex)
			}
			spelling, spellingOK := protocols.StateName(handle, state)
			if !spellingOK {
				t.Fatalf("protocol %d state %d has no declared name", handle, state)
			}
			if !definition.HasState(typestate.State(spelling)) {
				t.Fatalf("protocol %d is judged without its declared state %q", handle, spelling)
			}
		}
	}
}

// A callee this analysis cannot follow is judged rather than dropped.
//
// The call fact reaches the fold as authenticated opaque evidence, and the
// fold answers for the actual it was indexed by: the declared escape is
// applied, so every proof about the resource is discharged, and the verdict is
// the unproven one. Reporting the call clean, or refusing the row so that it
// leaves the population, are the two answers a soundness judgment may not
// give, and both are what this law excludes.
func TestOpaqueCalleeReachesAnUnprovenVerdict(t *testing.T) {
	fixture := buildJudgmentFixture(t, judgmentSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	candidate, candidateOK := fixture.values.MountedCallArgumentAt(0)
	if !candidateOK {
		t.Fatal("the fixture publishes no mounted call actual")
	}
	opaque := opaqueCallValue(t, fixture.calls)
	protocol := firstProtocol(t, judgment)
	current, currentOK := typestate.Exactly(firstState(t, judgment, protocol))
	if !currentOK {
		t.Fatal("declared state unavailable")
	}

	successor, verdict, outcome := judgment.decide(candidate, fixture.values.Top(), opaque, uint64(protocol), current)
	if outcome != structure.AuthenticatedOpaque {
		t.Fatalf("outcome = %d, want the authenticated-opaque admission the read delivered", outcome)
	}
	if !successor.IsUnknown() {
		t.Fatalf("successor = %+v, want the declared escape's top", successor)
	}
	if !verdict.Reports() {
		t.Fatalf("verdict = %q, want a reported unproven finding", verdict.Spelling())
	}
	if verdict != typestate.VerdictUnprovenTransition && verdict != typestate.VerdictUnprovenRequirement {
		t.Fatalf("verdict = %q, want one of the two unproven answers", verdict.Spelling())
	}
	if verdict == typestate.VerdictConforms || verdict == typestate.VerdictAbstain {
		t.Fatalf("verdict = %q, which certifies a call the analysis could not follow", verdict.Spelling())
	}
}

// The population is the actual's own. An opaque callee changes what a read
// delivers and never which rows exist, so the same candidate that is judged
// against a followable callee is judged against an unfollowable one.
func TestOpaqueCalleeLeavesThePopulationUnchanged(t *testing.T) {
	fixture := buildJudgmentFixture(t, judgmentSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	protocol := firstProtocol(t, judgment)
	current, currentOK := typestate.Exactly(firstState(t, judgment, protocol))
	if !currentOK {
		t.Fatal("declared state unavailable")
	}
	for index := 0; index < fixture.values.MountedCallArgumentCount(); index++ {
		candidate, candidateOK := fixture.values.MountedCallArgumentAt(index)
		if !candidateOK {
			t.Fatalf("mounted call actual %d unavailable", index)
		}
		_, _, outcome := judgment.decide(candidate, fixture.values.Top(), opaqueCallValue(t, fixture.calls), uint64(protocol), current)
		if outcome == structure.Refuse {
			t.Fatalf("actual %d was refused, so an unfollowable callee removed it from the population", index)
		}
	}
}

// A tag that names no declared protocol is refused rather than judged under a
// neighbouring one. The tag is the only thing that says which of a resource's
// cells a returned state belongs to, so a fold that guessed would answer one
// protocol's question with another's state machine.
func TestUndeclaredProtocolTagIsRefused(t *testing.T) {
	fixture := buildJudgmentFixture(t, judgmentSource)
	judgment, ok := Derive(fixture.values, fixture.calls, fixture.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	candidate, candidateOK := fixture.values.MountedCallArgumentAt(0)
	if !candidateOK {
		t.Fatal("the fixture publishes no mounted call actual")
	}
	for _, tag := range []uint64{0, 1 << 40} {
		_, _, outcome := judgment.decide(candidate, fixture.values.Top(), opaqueCallValue(t, fixture.calls), tag, typestate.Unknown())
		if outcome != structure.Refuse {
			t.Fatalf("tag %d was judged with outcome %d, want a refusal", tag, outcome)
		}
	}
}

// An actual another Value schema issued is refused. The judgment answers for
// the rows its own owner sealed, and an equal-shaped row from a second seal
// carries no coordinate this one may address.
func TestForeignActualIsRefused(t *testing.T) {
	own := buildJudgmentFixture(t, judgmentSource)
	foreign := buildJudgmentFixture(t, "local function g(a) return a end\ng(3)\n")
	judgment, ok := Derive(own.values, own.calls, own.packs)
	if !ok {
		t.Fatal("judgment not derived")
	}
	candidate, candidateOK := foreign.values.MountedCallArgumentAt(0)
	if !candidateOK {
		t.Fatal("the foreign fixture publishes no mounted call actual")
	}
	protocol := firstProtocol(t, judgment)
	_, _, outcome := judgment.decide(candidate, own.values.Top(), opaqueCallValue(t, own.calls), uint64(protocol), typestate.Unknown())
	if outcome != structure.Refuse {
		t.Fatalf("a foreign actual was judged with outcome %d, want a refusal", outcome)
	}
}

// opaqueCallValue is one call fact whose callee this analysis cannot follow:
// an open value with no known alternative, which is what an unresolved
// dispatch delivers.
func opaqueCallValue(t *testing.T, calls *calldomain.Algebra) calldomain.Value {
	t.Helper()
	coordinate, coordinateOK := calls.CallCoordinateAt(0)
	if !coordinateOK {
		t.Fatal("the fixture publishes no call coordinate")
	}
	key, keyOK := coordinate.Key()
	if !keyOK {
		t.Fatal("call coordinate carries no key")
	}
	value, valueOK := calls.DispatchValue(key, nil, true)
	if !valueOK || !value.HasOpaqueAlternative() {
		t.Fatal("opaque call fact unavailable")
	}
	return value
}

func firstProtocol(t *testing.T, judgment Judgment) vocabulary.Protocol {
	t.Helper()
	for handle := vocabulary.Protocol(1); handle < 64; handle++ {
		if _, ok := judgment.sealed.definitionFor(handle); ok {
			return handle
		}
	}
	t.Fatal("the judgment holds no declared protocol")
	return 0
}

func firstState(t *testing.T, judgment Judgment, protocol vocabulary.Protocol) typestate.State {
	t.Helper()
	definition, ok := judgment.sealed.definitionFor(protocol)
	if !ok || len(definition.States) == 0 {
		t.Fatal("the declared protocol holds no state")
	}
	return definition.States[0]
}
