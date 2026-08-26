package issuance

import (
	"reflect"
	"testing"

	seal "github.com/wippyai/go-lua/analysis/schema/seal"

	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
)

type emptySurface struct{ kind schema.SurfaceKind }

func (surface emptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (emptySurface) Entries() []schema.Entry          { return nil }
func (emptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func TestProgramCallStagesOwnTheirExactPredecessorAndTransportGraph(t *testing.T) {
	table := programIssuanceTable(t)
	stage := func(key schema.Key) *schemaissuance.Entry {
		t.Helper()
		entry, ok := table.Entry(key, schemaissuance.KindStage)
		if !ok {
			t.Fatalf("stage %q unavailable", key)
		}
		return entry
	}
	dispatch, summary, effect := stage(StageCallDispatch), stage(StageCallSummary), stage(StageCallEffect)
	if dispatch.BaseParameter() != 1 || !dispatch.Native() || dispatch.InputCount() != 1 || !reflect.DeepEqual(dispatch.Predecessors(), []schema.Key{StageBase}) {
		t.Fatal("call dispatch lost its explicit Base predecessor")
	}
	if summary.BaseParameter() != 1 || !summary.Native() || summary.InputCount() != 1 || !reflect.DeepEqual(summary.Predecessors(), []schema.Key{StageCallDispatch}) {
		t.Fatal("call summary lost its explicit Dispatch predecessor")
	}
	if effect.BaseParameter() != 1 || !effect.Native() || effect.InputCount() != 1 || !reflect.DeepEqual(effect.Predecessors(), []schema.Key{StageCallSummary}) {
		t.Fatal("call effect lost its explicit Summary predecessor")
	}
	wantSummary := []schemaissuance.StageEdge{
		{Source: schemaissuance.StageEdgeSourceBeforeStage, Stage: StageCallDispatch, Transport: schemaissuance.StageTransportAllExceptWritesOfStages, WriterStages: []schema.Key{StageCallDispatch}, Framing: "analysis/program-artifact/call-base-summary-transfer"},
		{Source: schemaissuance.StageEdgeSourceStage, Stage: StageCallDispatch, Transport: schemaissuance.StageTransportWritesOfStages, WriterStages: []schema.Key{StageCallDispatch}, Framing: "analysis/program-artifact/call-dispatch-summary-transfer"},
	}
	wantEffect := []schemaissuance.StageEdge{
		{Source: schemaissuance.StageEdgeSourceStage, Stage: StageCallSummary, Transport: schemaissuance.StageTransportAllExceptTargetWrites, Framing: "analysis/program-artifact/call-summary-effect-transfer"},
	}
	if !reflect.DeepEqual(summary.Edges(), wantSummary) || !reflect.DeepEqual(effect.Edges(), wantEffect) {
		t.Fatal("call stage transport graph drifted from its sealed declaration")
	}
}

func TestProgramVocabularySealsAsOneIssuanceSurface(t *testing.T) {
	table := programIssuanceTable(t)
	field, fieldOK := table.Entry(FieldResultSlotValueID, schemaissuance.KindField)
	if !fieldOK || field.Space() != RowCallResultSlot ||
		field.Type() != schemaissuance.IdentityType(TypeContentID) ||
		field.Cardinality() != schemaissuance.CardinalityOptional {
		t.Fatal("sealed table lost the optional result-slot Value identity declaration")
	}
}

func programIssuanceTable(t testing.TB) schemaissuance.Table {
	t.Helper()
	entries, entriesOK := Entries()
	if !entriesOK {
		t.Fatal("Program issuance vocabulary refused construction")
	}
	builder := seal.NewBuilder()
	builder.Register(emptySurface{schema.SurfaceKindStructure})
	builder.Register(emptySurface{schema.SurfaceKindAxis})
	builder.Register(schemaissuance.NewSurface(entries))
	for kind := schema.SurfaceKindRule; kind <= schema.SurfaceKindObservation; kind++ {
		builder.Register(emptySurface{kind})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil || !sealed.Available() {
		t.Fatalf("Program issuance vocabulary refused sealing: %+v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindIssuance)
	table, tableOK := schemaissuance.NewTable(view)
	if !viewOK || !tableOK {
		t.Fatal("sealed Program issuance table unavailable")
	}
	return table
}
