package issuance

import (
	"testing"

	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/schema"
)

type scratchSurface struct{ kind schema.SurfaceKind }

func (surface scratchSurface) Kind() schema.SurfaceKind { return surface.kind }
func (scratchSurface) Entries() []schema.Entry          { return nil }
func (scratchSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func mustEntry(t *testing.T, spec Spec) *Entry {
	t.Helper()
	entry, ok := New(spec)
	if !ok {
		t.Fatalf("declaration %q was refused before sealing", spec.Key)
	}
	return entry
}

func sealTable(t *testing.T, entries ...*Entry) (*seal.Schema, schema.SealFailure) {
	t.Helper()
	builder := seal.NewBuilder()
	builder.Register(scratchSurface{schema.SurfaceKindStructure})
	builder.Register(scratchSurface{schema.SurfaceKindAxis})
	builder.Register(NewSurface(entries))
	for kind := schema.SurfaceKindRule; kind <= schema.SurfaceKindObservation; kind++ {
		builder.Register(scratchSurface{kind})
	}
	return builder.Seal()
}

const (
	typeContent schema.Key = "type/content"
	typeForm    schema.Key = "type/form"
)

func canonicalEntries(t *testing.T) []*Entry {
	t.Helper()
	types := []schema.Key{
		typeContent, typeForm, TypeRelationIndex, TypeRelationCount,
		TypePoint, TypePointIdentity, TypeRoute, TypeRouteIdentity,
		TypeEmission, TypeRuleKey, TypeAxisKey,
	}
	var entries []*Entry
	for index, key := range types {
		entries = append(entries, mustEntry(t, Spec{Key: key, Kind: KindType, Ordinal: uint16(index + 1)}))
	}
	entries = append(entries,
		mustEntry(t, Spec{Key: "row/occurrence", Kind: KindRowSpace, Ordinal: 1}),
		mustEntry(t, Spec{Key: "row/call", Kind: KindRowSpace, Ordinal: 2}),
		mustEntry(t, Spec{Key: "row/geometry", Kind: KindRowSpace, Ordinal: 3}),
		mustEntry(t, Spec{Key: "field/occurrence-id", Kind: KindField, Ordinal: 1, Space: "row/occurrence", Type: IdentityType(typeContent), Cardinality: CardinalityOne}),
		mustEntry(t, Spec{Key: "field/call-id", Kind: KindField, Ordinal: 2, Space: "row/call", Type: IdentityType(typeContent), Cardinality: CardinalityOne}),
		mustEntry(t, Spec{Key: "field/call-form", Kind: KindField, Ordinal: 3, Space: "row/call", Type: UintType(typeForm), Cardinality: CardinalityOne}),
		mustEntry(t, Spec{Key: "field/geometry-occurrence-id", Kind: KindField, Ordinal: 4, Space: "row/geometry", Type: IdentityType(typeContent), Cardinality: CardinalityOne}),
		mustEntry(t, Spec{Key: "field/geometry-position", Kind: KindField, Ordinal: 5, Space: "row/geometry", Type: UintType(TypeRelationIndex), Cardinality: CardinalityOne}),
		mustEntry(t, Spec{Key: "field/geometry-point", Kind: KindField, Ordinal: 6, Space: "row/geometry", Type: IdentityType(TypePointIdentity), Cardinality: CardinalityOne}),
		mustEntry(t, Spec{Key: "relation/occurrence-call", Kind: KindRelation, Ordinal: 1, Space: "row/occurrence", Target: "row/call", Cardinality: CardinalityOptional,
			Joins:   []JoinField{{Source: "field/occurrence-id", Target: "field/call-id", Missing: JoinMissingNoEdge}},
			Program: Program{{Op: OpLiteral, Out: 1, Type: BoolType(), Literal: 1}}, Result: 1}),
		mustEntry(t, Spec{Key: "relation/occurrence-geometry", Kind: KindRelation, Ordinal: 2, Space: "row/occurrence", Target: "row/geometry", Cardinality: CardinalityMany,
			Joins:   []JoinField{{Source: "field/occurrence-id", Target: "field/geometry-occurrence-id", Missing: JoinMissingNoEdge}},
			Program: Program{{Op: OpLiteral, Out: 1, Type: BoolType(), Literal: 1}}, Result: 1}),
		mustEntry(t, Spec{Key: "family/occurrence", Kind: KindFamily, Ordinal: 1, Space: "row/occurrence",
			Program: Program{{Op: OpLiteral, Out: 1, Type: BoolType(), Literal: 1}}, Result: 1}),
		mustEntry(t, Spec{Key: "output/occurrence", Kind: KindOutput, Ordinal: 1, Type: DataType{Value: ValueRow, Space: "row/occurrence", Cardinality: CardinalityOne}}),
		mustEntry(t, Spec{Key: "requirement/unrestricted", Kind: KindRequirement, Ordinal: 1, Space: "row/occurrence", Program: Program{
			{Op: OpCurrent, Out: 1},
			{Op: OpLiteral, Out: 2, Type: BoolType(), Literal: 1},
		}, Result: 2, Outputs: []OutputBinding{{Output: "output/occurrence", Register: 1, Proof: 2}}}),
		mustEntry(t, Spec{Key: "input/finish", Kind: KindInput, Ordinal: 1, Input: InputFinish, InputSource: InputSourceRelation, Selection: InputSelectionDriver, Source: "relation/occurrence-geometry"}),
		mustEntry(t, Spec{Key: "stage/base", Kind: KindStage, Ordinal: 1, Constructor: StageConstructorPassthrough, Parameters: []DataType{{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityMany}}, Base: 1, Identity: []uint16{1}, Order: 1}),
		mustEntry(t, Spec{Key: "stage/local", Kind: KindStage, Ordinal: 2, Constructor: StageConstructorFramed, Parameters: []DataType{{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityMany}}, Base: 1, Identity: []uint16{1}, Order: 2, Predecessors: []schema.Key{"stage/base"}, Edges: []StageEdge{{Source: StageEdgeSourcePrevious, Transport: StageTransportAll, Framing: "issuance/local-transfer/v1"}}, Framing: "issuance/local/v1", InputCount: 1}),
		mustEntry(t, Spec{Key: "form/local", Kind: KindForm, Ordinal: 1, Empty: EmptyRefuse, Subject: "output/occurrence", Requires: []schema.Key{"output/occurrence"}, Program: Program{
			{Op: OpSelection, Out: 1, Ref: "output/occurrence"},
			{Op: OpFollow, Out: 2, Args: [6]uint16{1}, Ref: "relation/occurrence-geometry"},
			{Op: OpProjectPoints, Out: 3, Args: [6]uint16{2}, Ref: "field/geometry-point", Aux: "field/geometry-position"},
			{Op: OpInput, Out: 4, Args: [6]uint16{3}, Ref: "input/finish"},
			{Op: OpRequestStage, Out: 5, Args: [6]uint16{3, 4}, Ref: "stage/local"},
			{Op: OpEmit, Out: 6, Args: [6]uint16{5}},
		}, Emissions: []uint16{6}}),
	)
	return entries
}

func TestSurfaceSealsNominalMachine(t *testing.T) {
	sealed, failure := sealTable(t, canonicalEntries(t)...)
	if failure.Available() || sealed == nil || !sealed.Available() {
		t.Fatalf("canonical issuance schema refused: %+v", failure)
	}
	view, ok := sealed.Surface(schema.SurfaceKindIssuance)
	table, tableOK := NewTable(view)
	entry, entryOK := table.Entry("requirement/unrestricted", KindRequirement)
	if !ok || !tableOK || !entryOK || len(entry.Outputs()) != 1 {
		t.Fatal("sealed issuance table lost its authenticated output")
	}
}

func TestSurfaceRefusesCrossNominalComparison(t *testing.T) {
	entries := canonicalEntries(t)
	entries = append(entries, mustEntry(t, Spec{Key: "requirement/bad", Kind: KindRequirement, Ordinal: 2, Space: "row/occurrence", Program: Program{
		{Op: OpLiteral, Out: 1, Type: UintType(typeForm)},
		{Op: OpLiteral, Out: 2, Type: UintType(TypeRelationIndex)},
		{Op: OpEqual, Out: 3, Args: [6]uint16{1, 2}},
	}, Result: 3}))
	if _, failure := sealTable(t, entries...); !failure.Available() || failure.Law != LawProgramShape {
		t.Fatalf("cross-nominal comparison produced %+v", failure)
	}
}

func TestSurfaceRefusesUnauthenticatedOutputAndWrongPhase(t *testing.T) {
	for name, bad := range map[string]*Entry{
		"wrong proof": mustEntry(t, Spec{Key: "requirement/bad-proof", Kind: KindRequirement, Ordinal: 2, Space: "row/occurrence", Program: Program{
			{Op: OpCurrent, Out: 1}, {Op: OpLiteral, Out: 2, Type: BoolType(), Literal: 1}, {Op: OpLiteral, Out: 3, Type: BoolType(), Literal: 1},
		}, Result: 2, Outputs: []OutputBinding{{Output: "output/occurrence", Register: 1, Proof: 3}}}),
		"projection in requirement": mustEntry(t, Spec{Key: "requirement/geometry", Kind: KindRequirement, Ordinal: 2, Space: "row/occurrence", Program: Program{{Op: OpProjectPoints, Out: 1}}, Result: 1}),
	} {
		t.Run(name, func(t *testing.T) {
			if _, failure := sealTable(t, append(canonicalEntries(t), bad)...); !failure.Available() || failure.Law != LawProgramShape {
				t.Fatalf("defect produced %+v", failure)
			}
		})
	}
}

func TestSurfaceRefusesRelationAndStageCycles(t *testing.T) {
	relationCycle := canonicalEntries(t)
	relationCycle = append(relationCycle,
		mustEntry(t, Spec{Key: "relation/call-self", Kind: KindRelation, Ordinal: 3, Space: "row/call", Target: "row/call", Cardinality: CardinalityMany,
			Joins:   []JoinField{{Source: "field/call-id", Target: "field/call-id", Missing: JoinMissingNoEdge}},
			Program: Program{{Op: OpCurrent, Out: 1}, {Op: OpFollow, Out: 2, Args: [6]uint16{1}, Ref: "relation/call-self"}, {Op: OpExactlyOne, Out: 3, Args: [6]uint16{2}}}, Result: 3}),
	)
	if _, failure := sealTable(t, relationCycle...); !failure.Available() || failure.Law != LawRelationAcyclic {
		t.Fatalf("relation cycle produced %+v", failure)
	}

	entries := canonicalEntries(t)
	for _, entry := range entries {
		if entry.key == "stage/base" {
			entry.edges = []StageEdge{{Source: StageEdgeSourceStage, Stage: "stage/local", Transport: StageTransportAll, Framing: "issuance/cycle/v1"}}
		}
	}
	if _, failure := sealTable(t, entries...); !failure.Available() || failure.Law != LawStageAcyclic {
		t.Fatalf("stage cycle produced %+v", failure)
	}
}

func TestSurfaceRefusesSparseOrdinals(t *testing.T) {
	entries := canonicalEntries(t)
	for _, entry := range entries {
		if entry.key == "row/call" {
			entry.ordinal = 4
		}
	}
	if _, failure := sealTable(t, entries...); !failure.Available() || failure.Law != LawOrdinalDense {
		t.Fatalf("sparse ordinal produced %+v", failure)
	}
}

func TestSurfaceRefusesDuplicateStageAndTransportFraming(t *testing.T) {
	entries := canonicalEntries(t)
	for _, entry := range entries {
		if entry.key == "stage/local" {
			entry.edges[0].Framing = entry.framing
		}
	}
	if _, failure := sealTable(t, entries...); !failure.Available() || failure.Law != LawFramingUnique || failure.Disposition != schema.DispositionDuplicate {
		t.Fatalf("duplicate stage/transport framing produced %+v", failure)
	}
}

func TestAllExceptStageWritersRequiresAndCarriesWriterStages(t *testing.T) {
	entries := canonicalEntries(t)
	for _, entry := range entries {
		if entry.key != "stage/local" {
			continue
		}
		entry.edges[0].Transport = StageTransportAllExceptWritesOfStages
		entry.edges[0].WriterStages = []schema.Key{"stage/base"}
	}
	if _, failure := sealTable(t, entries...); failure.Available() {
		t.Fatalf("all-except stage-writers declaration refused: %+v", failure)
	}

	entries = canonicalEntries(t)
	for _, entry := range entries {
		if entry.key != "stage/local" {
			continue
		}
		entry.edges[0].Transport = StageTransportAllExceptWritesOfStages
		entry.edges[0].WriterStages = nil
	}
	if _, failure := sealTable(t, entries...); !failure.Available() {
		t.Fatal("all-except stage-writers edge without exclusions was admitted")
	}
}

func TestStageDeclarationOwnsBaseAndInputContract(t *testing.T) {
	pointMany := DataType{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityMany}
	content := IdentityType(typeContent)
	for name, spec := range map[string]Spec{
		"missing base": {
			Key: "stage/missing-base", Kind: KindStage, Ordinal: 1,
			Constructor: StageConstructorFramed, Parameters: []DataType{pointMany}, Identity: []uint16{1}, Order: 1, Framing: "stage/missing-base",
		},
		"non-point base": {
			Key: "stage/non-point-base", Kind: KindStage, Ordinal: 1,
			Constructor: StageConstructorFramed, Parameters: []DataType{content}, Base: 1, Identity: []uint16{1}, Order: 1, Framing: "stage/non-point-base",
		},
		"identity omits base": {
			Key: "stage/identity-omits-base", Kind: KindStage, Ordinal: 1,
			Constructor: StageConstructorFramed, Parameters: []DataType{content, pointMany}, Base: 2, Identity: []uint16{1}, Order: 1, Framing: "stage/identity-omits-base",
		},
		"unsupported identity carrier": {
			Key: "stage/unsupported-identity", Kind: KindStage, Ordinal: 1,
			Constructor: StageConstructorFramed, Parameters: []DataType{pointMany, {Value: ValueRoute, Name: TypeRoute, Cardinality: CardinalityOne}}, Base: 1, Identity: []uint16{1, 2}, Order: 1, Framing: "stage/unsupported-identity",
		},
		"repeating node omitted from identity": {
			Key: "stage/node-omitted", Kind: KindStage, Ordinal: 1,
			Constructor: StageConstructorFramed, Parameters: []DataType{pointMany, content, content}, Base: 1, Identity: []uint16{1}, Node: 2, Dependencies: []uint16{3}, Order: 1, Framing: "stage/node-omitted",
		},
		"native without input": {
			Key: "stage/native-without-input", Kind: KindStage, Ordinal: 1,
			Constructor: StageConstructorFramed, Parameters: []DataType{pointMany}, Base: 1, Identity: []uint16{1}, Order: 1, Framing: "stage/native-without-input", Native: true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, ok := New(spec); ok {
				t.Fatal("malformed stage base contract was admitted")
			}
		})
	}

	entries := canonicalEntries(t)
	for _, entry := range entries {
		if entry.key == "stage/local" {
			entry.inputCount = 7
		}
	}
	if _, failure := sealTable(t, entries...); !failure.Available() {
		t.Fatalf("stage input width mismatch produced %+v", failure)
	}
}

func TestProgramBytesChangeSchemaDigest(t *testing.T) {
	left := canonicalEntries(t)
	right := canonicalEntries(t)
	for _, entry := range right {
		if entry.key == "requirement/unrestricted" {
			entry.program[1].Literal = 0
		}
	}
	leftSchema, leftFailure := sealTable(t, left...)
	rightSchema, rightFailure := sealTable(t, right...)
	if leftFailure.Available() || rightFailure.Available() || leftSchema.Digest() == rightSchema.Digest() {
		t.Fatal("changing a machine instruction did not change the sealed schema digest")
	}
}

var _ seal.Surface = scratchSurface{}
