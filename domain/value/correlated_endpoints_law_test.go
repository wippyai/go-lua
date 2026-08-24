package value_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// endpointSubject exercises every operand family that reaches the sealed
// endpoint projection through ordinary Lua: binary arithmetic, relational
// order, equality, a nil-presence refinement arm and a runtime-kind call with
// its guarded predicate refinement.
const endpointSubject = "local function classify(a, b, tag)\n" +
	"\tlocal sum = a + b\n" +
	"\tlocal bigger = a < b\n" +
	"\tlocal same = a == b\n" +
	"\tif tag ~= nil then\n" +
	"\t\tsum = sum + 1\n" +
	"\tend\n" +
	"\tif type(tag) == \"string\" then\n" +
	"\t\tsum = sum - 1\n" +
	"\tend\n" +
	"\treturn sum, bigger, same\n" +
	"end\n" +
	"return classify(1, 2, \"x\")\n"

type endpointFixture struct {
	linked      *link.Link
	values      *valuedomain.Schema
	module      identity.ContentID
	occurrences []identity.ContentID
	valueMount  programmount.MountedArtifact
	heapMount   programmount.MountedArtifact
	structural  structure.Table
}

func newEndpointFixture(t testing.TB, name string) *endpointFixture {
	t.Helper()
	target, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatalf("standard target: %v", err)
	}
	linked, err := testfixture.SealSource(target, name+".lua", []byte(endpointSubject))
	if err != nil {
		t.Fatalf("seal source: %v", err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	if !grammarOK {
		t.Fatal("grammar identity")
	}
	issuance := testfixture.EmptyProgramIssuancePlan(t)
	mounts := linked.Project().Mounts()
	shard, shardOK := mounts.At(0)
	program, programOK := mounts.Program(shard)
	module, moduleOK := linked.Project().ModuleKey(shard)
	if mounts.Count() != 1 || !shardOK || !programOK || program == nil || !moduleOK {
		t.Fatal("source mount")
	}
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile artifact: %s", failure.Error())
	}
	structural := endpointStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	if !lowered || snapshot == nil || !snapshot.Available() {
		t.Fatal("ingress snapshot")
	}
	heapMount, heapMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !heapMountOK || !valueMountOK {
		t.Fatal("artifact mounts")
	}
	fixture := &endpointFixture{linked: linked, module: module, valueMount: valueMount, heapMount: heapMount, structural: structural}
	fixture.values = fixture.seal(t)

	count, countOK := artifact.Program().OccurrenceCount()
	if !countOK {
		t.Fatal("occurrence plane")
	}
	// One occurrence identity can be published at more than one plane index;
	// the operand directories are keyed by identity, so walk each one once.
	seen := make(map[identity.ContentID]struct{}, count)
	for index := 0; index < count; index++ {
		row, rowOK := artifact.Program().OccurrenceAt(index)
		if !rowOK {
			t.Fatalf("occurrence %d", index)
		}
		if _, duplicate := seen[row.ID()]; duplicate {
			continue
		}
		seen[row.ID()] = struct{}{}
		fixture.occurrences = append(fixture.occurrences, row.ID())
	}
	return fixture
}

// seal builds one further Value Schema over the same sealed Link. The
// projection must be a function of that Link alone.
func (fixture *endpointFixture) seal(t testing.TB) *valuedomain.Schema {
	t.Helper()
	heaps, heapFailure := heapdomain.SealWithArtifacts(fixture.linked, []programmount.MountedArtifact{fixture.heapMount})
	if heapFailure != heapdomain.SealFailureNone {
		t.Fatalf("seal heap schema: %s", heapFailure)
	}
	values, valueFailure := valuedomain.SealWithFailure(fixture.linked, heaps, []programmount.MountedArtifact{fixture.valueMount}, fixture.structural)
	if valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("seal value schema: %s", valueFailure)
	}
	return values
}

// endpointWalk is one operand's endpoint set recovered the way the consumer
// families used to recover it: authenticate the row, read its coordinates,
// then ask CoordinateIndex for each one.
type endpointWalk struct {
	family string
	roles  map[valuedomain.EndpointRole]uint64
	vector valuedomain.Endpoints
}

func (fixture *endpointFixture) walkOperands(t testing.TB, values *valuedomain.Schema) []endpointWalk {
	t.Helper()
	index := func(coordinate valuedomain.Coordinate) uint64 {
		dense, ok := values.CoordinateIndex(coordinate)
		if !ok {
			t.Fatal("operand names a coordinate this Schema does not own")
		}
		return uint64(dense)
	}
	var walks []endpointWalk
	for _, occurrence := range fixture.occurrences {
		if row, ok := values.BinaryArithmetic(fixture.module, occurrence); ok && values.OwnsBinaryArithmetic(row) {
			result, left, right, _, endpointsOK := row.Endpoints()
			vector, vectorOK := row.EndpointVector()
			if !endpointsOK || !vectorOK {
				t.Fatal("arithmetic operand has no sealed endpoint vector")
			}
			walks = append(walks, endpointWalk{family: "arithmetic", vector: vector, roles: map[valuedomain.EndpointRole]uint64{
				valuedomain.EndpointWrite: index(result), valuedomain.EndpointLeft: index(left), valuedomain.EndpointRight: index(right),
			}})
		}
		if row, ok := values.BinaryOrder(fixture.module, occurrence); ok && values.OwnsBinaryOrder(row) {
			result, left, right, _, endpointsOK := row.Endpoints()
			vector, vectorOK := row.EndpointVector()
			if !endpointsOK || !vectorOK {
				t.Fatal("order operand has no sealed endpoint vector")
			}
			walks = append(walks, endpointWalk{family: "order", vector: vector, roles: map[valuedomain.EndpointRole]uint64{
				valuedomain.EndpointWrite: index(result), valuedomain.EndpointLeft: index(left), valuedomain.EndpointRight: index(right),
			}})
		}
		if row, ok := values.BinaryEquality(fixture.module, occurrence); ok && values.OwnsBinaryEquality(row) {
			result, left, right, _, endpointsOK := row.Endpoints()
			vector, vectorOK := row.EndpointVector()
			if !endpointsOK || !vectorOK {
				t.Fatal("equality operand has no sealed endpoint vector")
			}
			walks = append(walks, endpointWalk{family: "equality", vector: vector, roles: map[valuedomain.EndpointRole]uint64{
				valuedomain.EndpointWrite: index(result), valuedomain.EndpointLeft: index(left), valuedomain.EndpointRight: index(right),
			}})
		}
		if row, ok := values.PresenceRefinement(fixture.module, occurrence); ok && values.OwnsPresenceRefinement(row) {
			target, _, targetOK := row.Target()
			vector, vectorOK := row.EndpointVector()
			if !targetOK || !vectorOK {
				t.Fatal("presence refinement has no sealed endpoint vector")
			}
			walks = append(walks, endpointWalk{family: "refinement", vector: vector, roles: map[valuedomain.EndpointRole]uint64{
				valuedomain.EndpointWrite: index(target), valuedomain.EndpointLeft: index(target),
			}})
		}
		if row, ok := values.RuntimeKindCall(fixture.module, occurrence); ok && values.OwnsRuntimeKindCall(row) {
			result, input, endpointsOK := row.Endpoints()
			write, writeOK := row.WriteTarget()
			vector, vectorOK := row.EndpointVector()
			if !endpointsOK || !writeOK || !vectorOK {
				t.Fatal("runtime-kind operand has no sealed endpoint vector")
			}
			roles := map[valuedomain.EndpointRole]uint64{
				valuedomain.EndpointWrite: index(write), valuedomain.EndpointResult: index(result), valuedomain.EndpointLeft: index(input),
			}
			if comparison, _, _, refinementOK := row.Refinement(); refinementOK {
				roles[valuedomain.EndpointCompared] = index(comparison)
			}
			walks = append(walks, endpointWalk{family: "runtimekind", vector: vector, roles: roles})
		}
		if row, ok := values.ModuleLoadCall(fixture.module, occurrence); ok && values.OwnsModuleLoadCall(row) {
			result, argument, endpointsOK := row.Endpoints()
			vector, vectorOK := row.EndpointVector()
			if !endpointsOK || !vectorOK {
				t.Fatal("module-load operand has no sealed endpoint vector")
			}
			walks = append(walks, endpointWalk{family: "moduleload", vector: vector, roles: map[valuedomain.EndpointRole]uint64{
				valuedomain.EndpointWrite: index(result), valuedomain.EndpointLeft: index(argument),
			}})
		}
	}
	return walks
}

// TestEndpointProjectionAnswersTheDeletedConsumerWalk is law (b): for every
// admitted operand the sealed vector answers exactly what the per-family
// hotEndpoints walk answered, role for role, and declares no role that walk
// did not produce.
func TestEndpointProjectionAnswersTheDeletedConsumerWalk(t *testing.T) {
	fixture := newEndpointFixture(t, "endpoint_walk")
	walks := fixture.walkOperands(t, fixture.values)
	seenFamilies := make(map[string]int, 6)
	for _, walk := range walks {
		seenFamilies[walk.family]++
		if !fixture.values.OwnsEndpoints(walk.vector) {
			t.Fatalf("%s vector is not owned by the sealing Schema", walk.family)
		}
		for role := valuedomain.EndpointWrite; role.Available(); role++ {
			expected, declared := walk.roles[role]
			actual, published := walk.vector.Index(role)
			if declared != published {
				t.Fatalf("%s role %d declared=%t published=%t", walk.family, role, declared, published)
			}
			if declared && expected != actual {
				t.Fatalf("%s role %d projected %d, the walk produced %d", walk.family, role, actual, expected)
			}
			coordinate, coordinateOK := walk.vector.Coordinate(role)
			if declared != coordinateOK {
				t.Fatalf("%s role %d coordinate availability disagrees with its index", walk.family, role)
			}
			if declared {
				dense, denseOK := fixture.values.CoordinateIndex(coordinate)
				if !denseOK || uint64(dense) != actual {
					t.Fatalf("%s role %d coordinate does not round-trip to its dense index", walk.family, role)
				}
			}
		}
	}
	for _, family := range []string{"arithmetic", "order", "equality", "refinement", "runtimekind"} {
		if seenFamilies[family] == 0 {
			t.Fatalf("fixture admitted no %s operand; the law is not exercised for it", family)
		}
	}
}

// TestEndpointProjectionIsAFunctionOfTheSealedProgram is law (a): two Value
// Schemas sealed over one Link publish one identical table with identical
// ordinals.
func TestEndpointProjectionIsAFunctionOfTheSealedProgram(t *testing.T) {
	fixture := newEndpointFixture(t, "endpoint_function")
	first := fixture.values
	second := fixture.seal(t)

	firstTable, firstTableOK := first.EndpointTableID()
	secondTable, secondTableOK := second.EndpointTableID()
	if !firstTableOK || !secondTableOK {
		t.Fatalf("table identity first=%t second=%t", firstTableOK, secondTableOK)
	}
	if firstTable != secondTable {
		t.Fatal("two seals of one program published different endpoint tables")
	}
	if first.EndpointCount() == 0 || first.EndpointCount() != second.EndpointCount() {
		t.Fatalf("projection extent first=%d second=%d", first.EndpointCount(), second.EndpointCount())
	}
	for ordinal := 0; ordinal < first.EndpointCount(); ordinal++ {
		left, leftOK := first.EndpointsAt(ordinal)
		right, rightOK := second.EndpointsAt(ordinal)
		if !leftOK || !rightOK {
			t.Fatalf("row %d left=%t right=%t", ordinal, leftOK, rightOK)
		}
		for role := valuedomain.EndpointWrite; role.Available(); role++ {
			leftIndex, leftDeclared := left.Index(role)
			rightIndex, rightDeclared := right.Index(role)
			if leftDeclared != rightDeclared || leftIndex != rightIndex {
				t.Fatalf("row %d role %d differs across seals", ordinal, role)
			}
		}
	}

	// The ordinals a Rule Program join carries must select the same operands
	// in both seals, so every operand's ordinal is stable too.
	firstWalks := fixture.walkOperands(t, first)
	secondWalks := fixture.walkOperands(t, second)
	if len(firstWalks) == 0 || len(firstWalks) != len(secondWalks) {
		t.Fatalf("operand count first=%d second=%d", len(firstWalks), len(secondWalks))
	}
	for index := range firstWalks {
		leftOrdinal, leftOK := firstWalks[index].vector.Ordinal()
		rightOrdinal, rightOK := secondWalks[index].vector.Ordinal()
		if !leftOK || !rightOK || leftOrdinal != rightOrdinal {
			t.Fatalf("operand %d ordinal first=%d second=%d", index, leftOrdinal, rightOrdinal)
		}
	}
}

// TestEndpointVectorsAreDistinctPerOperand keeps ordinals injective: two
// operands never share one row, so a join that references a row by ordinal
// names exactly one operand.
func TestEndpointVectorsAreDistinctPerOperand(t *testing.T) {
	fixture := newEndpointFixture(t, "endpoint_distinct")
	walks := fixture.walkOperands(t, fixture.values)
	seen := make(map[int]string, len(walks))
	for _, walk := range walks {
		ordinal, ordinalOK := walk.vector.Ordinal()
		if !ordinalOK {
			t.Fatalf("%s vector has no ordinal", walk.family)
		}
		if prior, duplicate := seen[ordinal]; duplicate {
			t.Fatalf("%s and %s share endpoint ordinal %d", prior, walk.family, ordinal)
		}
		seen[ordinal] = walk.family
	}
	if len(seen) != len(walks) {
		t.Fatalf("distinct ordinals %d for %d operands", len(seen), len(walks))
	}
}

// TestBinaryArithmeticUsesTheSharedEndpointOrdinal pins the arithmetic
// candidate cut: arithmetic rows round-trip through the one endpoint table,
// while their write/left/right projections are owner-issued no-argument
// accessors over that same vector.
func TestBinaryArithmeticUsesTheSharedEndpointOrdinal(t *testing.T) {
	fixture := newEndpointFixture(t, "endpoint_arithmetic_roundtrip")
	seen := 0
	for _, occurrence := range fixture.occurrences {
		row, rowOK := fixture.values.BinaryArithmetic(fixture.module, occurrence)
		if !rowOK || !fixture.values.OwnsBinaryArithmetic(row) {
			continue
		}
		seen++
		ordinal, ordinalOK := fixture.values.BinaryArithmeticOrdinal(row)
		if !ordinalOK {
			t.Fatal("arithmetic row has no endpoint ordinal")
		}
		vector, vectorOK := row.EndpointVector()
		if !vectorOK {
			t.Fatal("arithmetic row has no endpoint vector")
		}
		vectorOrdinal, vectorOrdinalOK := vector.Ordinal()
		if !vectorOrdinalOK || uint32(vectorOrdinal) != ordinal {
			t.Fatalf("arithmetic ordinal=%d, vector ordinal=%d/%t", ordinal, vectorOrdinal, vectorOrdinalOK)
		}
		at, atOK := fixture.values.BinaryArithmeticAt(int(ordinal))
		atID, atIDOK := at.ID()
		rowID, rowIDOK := row.ID()
		if !atOK || !atIDOK || !rowIDOK || atID != rowID {
			t.Fatalf("arithmetic endpoint roundtrip at=%+v/%t id=%v/%t row=%v/%t", at, atOK, atID, atIDOK, rowID, rowIDOK)
		}
		result, left, right, _, endpointsOK := row.Endpoints()
		write, writeOK := row.Write()
		leftProjection, leftOK := row.Left()
		rightProjection, rightOK := row.Right()
		if !endpointsOK || !writeOK || !leftOK || !rightOK || write != result || leftProjection != left || rightProjection != right {
			t.Fatalf("arithmetic projections write=%+v/%t left=%+v/%t right=%+v/%t endpoints=%+v/%+v/%+v/%t", write, writeOK, leftProjection, leftOK, rightProjection, rightOK, result, left, right, endpointsOK)
		}
	}
	if seen == 0 {
		t.Fatal("fixture admitted no arithmetic operand")
	}
}

// TestEndpointVectorRefusesForeignRows keeps the owner fence: a vector issued
// by one Schema is not admitted by another over the same program.
func TestEndpointVectorRefusesForeignRows(t *testing.T) {
	fixture := newEndpointFixture(t, "endpoint_fence")
	first := fixture.values
	second := fixture.seal(t)
	vector, vectorOK := first.EndpointsAt(0)
	if !vectorOK {
		t.Fatal("first vector")
	}
	if !first.OwnsEndpoints(vector) {
		t.Fatal("issuing Schema refused its own vector")
	}
	if second.OwnsEndpoints(vector) {
		t.Fatal("a foreign Schema admitted a vector it did not issue")
	}
}

// endpointStructuralVocabulary supplies the structural declarations the
// ingress lowering requires to publish a snapshot for this fixture.
func endpointStructuralVocabulary(t testing.TB) structure.Table {
	t.Helper()
	counts := func(category structure.Category) int {
		switch category {
		case structure.CategoryArm:
			return 8
		case structure.CategoryEvent:
			return 3
		case structure.CategoryOutcome:
			return 7
		case structure.CategoryRuntimeKind:
			return int(runtimekind.Count) - 1
		case structure.CategoryOccurrenceKind:
			return 32
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("value-endpoints/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal), Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("value-endpoint structural declarations")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("value-endpoint structure surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(endpointEmptySurface{kind: kind}) {
			t.Fatalf("value-endpoint structural surface %d", kind)
		}
	}
	sealed, sealFailure := builder.Seal()
	if sealFailure.Available() || sealed == nil {
		t.Fatalf("value-endpoint structural schema: %v", sealFailure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("value-endpoint structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("value-endpoint structural table")
	}
	return table
}

type endpointEmptySurface struct{ kind schema.SurfaceKind }

func (surface endpointEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface endpointEmptySurface) Entries() []schema.Entry  { return nil }
func (surface endpointEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
