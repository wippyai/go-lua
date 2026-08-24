package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	calldomain "github.com/wippyai/go-lua/domain/call"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/composite"
	"github.com/wippyai/go-lua/domain/composite/snapshottest"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	staticdomain "github.com/wippyai/go-lua/domain/static"
	typeauthority "github.com/wippyai/go-lua/domain/type/authority"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// mountedCallArgumentFixture seals the same Pack/Value stack the existing
// mounted-call-shape laws build (see domain/pack/mounted_input_fixed_shape_law_test.go
// and domain/pack/input_selector_law_test.go), so the published directory is
// checked against the exact Pack/Value derivation, never a second model of it.
type mountedCallArgumentFixture struct {
	pack    *packdomain.Schema
	values  *valuedomain.Schema
	calls   *calldomain.Algebra
	module  identity.ContentID
	program programschema.Program
}

func buildMountedCallArgumentFixture(t *testing.T, source string) mountedCallArgumentFixture {
	t.Helper()
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked, err := testfixture.SealSource(contract, "mounted_call_argument.lua", []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Build()
	grammar := compilation.ExecutionSchemaID()
	issuance, issuanceOK := composite.ArtifactIssuanceDirectory(compilation)
	if !compilationOK || !grammar.Available() || !issuanceOK {
		t.Fatal("program schema")
	}
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := mounts.ProgramID(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK || !programIDOK {
		t.Fatal("mount")
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
	statics, _, err := staticdomain.SealMountedPrograms(staticdomain.MountContext{LinkID: linked.ContentID(), Target: contract}, types, []staticdomain.MountedProgram{{Program: snapshottest.MustMount(t, artifact, module).Program, ModuleID: module, NamespaceID: module}})
	if err != nil || statics == nil {
		t.Fatalf("seal static: %v", err)
	}
	structural, structuralOK := composite.StructureVocabulary(compilation)
	if !structuralOK {
		t.Fatal("structure vocabulary")
	}

	packMount, packMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !packMountOK || !heapMountOK || !valueMountOK {
		t.Fatal("artifact mounts")
	}
	packSchema, packOK := packdomain.SealMountedArtifacts(linked, statics, []programmount.MountedArtifact{packMount})
	if !packOK || packSchema == nil {
		t.Fatal("seal pack")
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{heapMount})
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("seal heap: %s", heapFailure)
	}
	calls := calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calls, []programmount.MountedArtifact{valueMount}, structural)
	if valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal value: %s", valueFailure)
	}
	return mountedCallArgumentFixture{pack: packSchema, values: values, calls: calls, module: module, program: artifact.Program()}
}

// TestPublishedMountedCallArgumentsEqualTheDerivedActuals is the two-way
// equality law for domain/value's sealed mounted-call-argument directory: for
// every mounted call and every actual index, the published row must exist and
// its Coordinate must equal CoordinateForMountedSemantic(ActualAt(i).Module(),
// ActualAt(i).ID()); and the published dense directory must contain no row the
// Pack/Value derivation does not produce, in the same order.
//
// Before MountedCallArgument existed, this test failed at the very first
// assertion: values.MountedCallArgumentFor(module, callID, 0) had no method to
// call, because no Rule Program could name a mounted call argument as a
// candidate at all - a lossy parent column between Pack's derivation and
// Value's published vocabulary.
func TestPublishedMountedCallArgumentsEqualTheDerivedActuals(t *testing.T) {
	const source = "local receiver = {}\n" +
		"function receiver:method(a, b)\n" +
		"\treturn a\n" +
		"end\n" +
		"local function two(a, b)\n" +
		"\treturn a\n" +
		"end\n" +
		"local function noop()\n" +
		"end\n" +
		"noop()\n" +
		"two(1, 2)\n" +
		"receiver:method(3, 4)\n"

	fixture := buildMountedCallArgumentFixture(t, source)

	callCount, callsOK := fixture.program.CallCount()
	if !callsOK || callCount == 0 {
		t.Fatalf("call count = %d/%t", callCount, callsOK)
	}

	exercisedMultiActual := false
	exercisedReceiver := false
	dense := uint32(0)
	for callIndex := 0; callIndex < callCount; callIndex++ {
		call, callOK := fixture.program.CallAt(callIndex)
		if !callOK || !call.Available() {
			t.Fatalf("call %d unavailable", callIndex)
		}
		actual, actualOK := fixture.pack.MountedActualProjection(fixture.module, call.ID())
		if !actualOK {
			t.Fatalf("call %d has no mounted actual projection", callIndex)
		}
		if actual.ActualCount() > 1 {
			exercisedMultiActual = true
		}
		if call.Form() == programschema.CallFormMethod {
			exercisedReceiver = true
		}
		for actualIndex := 0; actualIndex < actual.ActualCount(); actualIndex++ {
			actualSource, actualSourceOK := actual.ActualAt(actualIndex)
			if !actualSourceOK {
				t.Fatalf("call %d actual %d unavailable", callIndex, actualIndex)
			}
			expected, expectedOK := fixture.values.CoordinateForMountedSemantic(actualSource.Module(), actualSource.ID())
			if !expectedOK {
				t.Fatalf("call %d actual %d has no derived coordinate", callIndex, actualIndex)
			}

			row, rowOK := fixture.values.MountedCallArgumentFor(fixture.module, call.ID(), uint32(actualIndex))
			if !rowOK {
				t.Fatalf("call %d actual %d has no published MountedCallArgument row", callIndex, actualIndex)
			}
			if !fixture.values.OwnsMountedCallArgument(row) {
				t.Fatalf("call %d actual %d published row fails the owner fence", callIndex, actualIndex)
			}
			coordinate, coordinateOK := row.Coordinate()
			if !coordinateOK || coordinate != expected {
				t.Fatalf("call %d actual %d published coordinate = %#v/%t, want %#v", callIndex, actualIndex, coordinate, coordinateOK, expected)
			}
			if valueID, valueOK := row.ValueID(); !valueOK || valueID != actualSource.ID() {
				t.Fatalf("call %d actual %d published ValueID = %#v/%t, want %#v", callIndex, actualIndex, valueID, valueOK, actualSource.ID())
			}

			byOccurrence, occurrenceOK := row.ID()
			if !occurrenceOK {
				t.Fatalf("call %d actual %d row has no owner-issued identity", callIndex, actualIndex)
			}
			resolved, resolvedOK := fixture.values.MountedCallArgumentForMountedOccurrence(fixture.module, byOccurrence)
			if !resolvedOK || resolved != row {
				t.Fatalf("call %d actual %d occurrence resolver did not invert the row's own identity", callIndex, actualIndex)
			}

			// The dense directory order must be exactly the Pack/Value walk
			// order: mount order, then call order, then per-call actual order.
			atDense, atOK := fixture.values.MountedCallArgumentAt(int(dense))
			if !atOK || atDense != row {
				t.Fatalf("call %d actual %d dense position %d = %#v/%t, want the same published row", callIndex, actualIndex, dense, atDense, atOK)
			}
			ordinal, ordinalOK := fixture.values.MountedCallArgumentOrdinal(row)
			if !ordinalOK || ordinal != dense {
				t.Fatalf("call %d actual %d ordinal = %d/%t, want %d", callIndex, actualIndex, ordinal, ordinalOK, dense)
			}
			dense++
		}
	}
	if !exercisedMultiActual {
		t.Fatal("fixture exercises no multi-actual call; the per-call ordering law is not exercised")
	}
	if !exercisedReceiver {
		t.Fatal("fixture exercises no method-form call; the receiver-first law is not exercised")
	}

	if count := fixture.values.MountedCallArgumentCount(); count != int(dense) {
		t.Fatalf("published directory count = %d, want the exact derived total %d", count, dense)
	}
	if _, ok := fixture.values.MountedCallArgumentAt(int(dense)); ok {
		t.Fatal("published directory holds a row beyond the derived total")
	}
}

// TestMountedCallActualTagRanksInAuthoredOrder is the direct restatement of
// the ordering guarantee 0e5fab95e7 left implicit when it deleted
// domain/heap/formalfreeze's canonicalActualTag and its law
// (TestFormalFreezeActualTagsRankInAuthoredOrder): a rule that reads a
// dependent selection ranked by tag reads it in authored order only because
// the tag strictly increases with the actual's own ordinal.
//
// The mounted-call axis no longer mints that tag itself; Value's
// MountedCallArgument.ActualTag publishes it as the one-based form of
// ActualIndex, so the guarantee is now a property of the published directory
// rather than of any one rule's decode. This states it directly over that
// directory instead of leaving it as a corollary of the parent-row span law
// in mounted_call_actuals_law_test.go.
func TestMountedCallActualTagRanksInAuthoredOrder(t *testing.T) {
	const source = "local receiver = {}\n" +
		"function receiver:method(a, b)\n" +
		"\treturn a\n" +
		"end\n" +
		"local function two(a, b)\n" +
		"\treturn a\n" +
		"end\n" +
		"receiver:method(1, 2)\n"

	fixture := buildMountedCallArgumentFixture(t, source)

	callCount, callsOK := fixture.program.CallCount()
	if !callsOK || callCount == 0 {
		t.Fatalf("call count = %d/%t", callCount, callsOK)
	}

	exercisedMultiActual := false
	for callIndex := 0; callIndex < callCount; callIndex++ {
		call, callOK := fixture.program.CallAt(callIndex)
		if !callOK || !call.Available() {
			t.Fatalf("call %d unavailable", callIndex)
		}
		actual, actualOK := fixture.pack.MountedActualProjection(fixture.module, call.ID())
		if !actualOK {
			t.Fatalf("call %d has no mounted actual projection", callIndex)
		}
		if actual.ActualCount() > 1 {
			exercisedMultiActual = true
		}

		previousTag := uint64(0)
		seenTags := make(map[uint64]int, actual.ActualCount())
		for ordinal := 0; ordinal < actual.ActualCount(); ordinal++ {
			row, rowOK := fixture.values.MountedCallArgumentFor(fixture.module, call.ID(), uint32(ordinal))
			if !rowOK {
				t.Fatalf("call %d actual %d has no published row", callIndex, ordinal)
			}
			index, indexOK := row.ActualIndex()
			tag, tagOK := row.ActualTag()
			if !indexOK || index != uint32(ordinal) || !tagOK || tag != uint64(ordinal)+1 {
				t.Fatalf("call %d actual %d carries index %d/%t tag %d/%t, want %d/true %d/true", callIndex, ordinal, index, indexOK, tag, tagOK, ordinal, ordinal+1)
			}
			if tag == 0 {
				t.Fatalf("call %d actual %d carries the reserved zero tag", callIndex, ordinal)
			}
			if ordinal > 0 && tag <= previousTag {
				t.Fatalf("call %d actual %d tag %d does not rank after %d", callIndex, ordinal, tag, previousTag)
			}
			if earlier, repeated := seenTags[tag]; repeated {
				t.Fatalf("call %d tag %d names both actual %d and %d", callIndex, tag, earlier, ordinal)
			}
			seenTags[tag], previousTag = ordinal, tag
		}
		if _, beyond := fixture.values.MountedCallArgumentFor(fixture.module, call.ID(), uint32(actual.ActualCount())); beyond {
			t.Fatalf("call %d minted a tag for an actual beyond its own count", callIndex)
		}
	}
	if !exercisedMultiActual {
		t.Fatal("fixture exercises no multi-actual call; the tag-ranking law is not exercised")
	}
}
