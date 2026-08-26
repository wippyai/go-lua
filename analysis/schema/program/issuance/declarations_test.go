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
		{Source: schemaissuance.StageEdgeSourceStage, Stage: StageCallSummary, Transport: schemaissuance.StageTransportAll, Framing: "analysis/program-artifact/call-summary-effect-transfer"},
	}
	if !reflect.DeepEqual(summary.Edges(), wantSummary) || !reflect.DeepEqual(effect.Edges(), wantEffect) {
		t.Fatal("call stage transport graph drifted from its sealed declaration")
	}

	// The chain's transport invariant. A stage that drops what its own point
	// writes is only admissible when a later stage re-supplies exactly those
	// writes: dispatch drops the axes it writes and summary restores them from
	// the dispatch point. The terminal stage has no successor to restore
	// anything, so it must drop nothing. An axis dropped there does not resume
	// at its predecessor state, it restarts at the empty initial root, and
	// every point the occurrence hands its environment to inherits that.
	dispatchResupplied := false
	for _, edge := range summary.Edges() {
		if edge.Transport == schemaissuance.StageTransportWritesOfStages &&
			len(edge.WriterStages) == 1 && edge.WriterStages[0] == StageCallDispatch {
			dispatchResupplied = true
		}
	}
	if !dispatchResupplied {
		t.Fatal("call dispatch drops the axes it writes and no later stage re-supplies them")
	}
	for index, edge := range effect.Edges() {
		if edge.Transport != schemaissuance.StageTransportAll {
			t.Fatalf("terminal call stage edge %d transports %d, want the complete predecessor state", index, edge.Transport)
		}
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
