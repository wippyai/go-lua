package transformer

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEffectArenaKeepsEffectsStructuredAndHashConsed(t *testing.T) {
	reg := standard.Registry()
	terms := NewArena(reg)
	effects := NewEffectArena(terms)
	shape := Shape{Params: 2}
	tableRoot := Root{Kind: RootParam, Index: 0}
	keyRoot := Root{Kind: RootParam, Index: 1}
	tablePath := terms.Path(tableRoot, segment.Segment{Kind: segment.SegmentField, Name: "items"})
	key := terms.Root(keyRoot)
	value := terms.Constant(typevalue.LiteralBool(reg, true))
	suffix := []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}
	precise := &PreciseDynamicTarget{Table: tablePath, Key: key, Suffix: suffix}
	invalidation := InvalidatePathConfig{
		Target: PathEffectTarget(tablePath), Scope: InvalidationScopeDescendants,
		PreserveStructuralWitness: true, PreserveDynamicValueMemberships: true, Precise: precise,
	}
	config := IndexMutationConfig{
		Invalidation: invalidation, Table: PathEffectTarget(tablePath), Key: key, Value: value,
		Admission: dynamicindex.AdmissionAdmitted, Readback: factflow.DynamicIndexReadbackKeyAndValue,
		Site: EffectSite{Owner: 41, Ordinal: 9},
	}
	first, err := effects.IndexMutation(config)
	if err != nil {
		t.Fatal(err)
	}
	if effects.Kind(first) != EffectIndexMutation || !effects.Valid(first, shape) {
		t.Fatalf("index mutation term = %d kind=%d valid=%t", first, effects.Kind(first), effects.Valid(first, shape))
	}
	// The effect owns suffix storage. Mutating caller input cannot change its
	// identity; rebuilding the original payload must find the first node.
	suffix[0].Name = "mutated"
	config.Invalidation.Precise.Suffix = []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}
	second, err := effects.IndexMutation(config)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("structured effect was not hash-consed: %d != %d", second, first)
	}
}

func TestEffectArenaIndexMutationRequiresAtomicInvalidation(t *testing.T) {
	reg := standard.Registry()
	terms := NewArena(reg)
	effects := NewEffectArena(terms)
	root := Root{Kind: RootParam, Index: 0}
	path := terms.Path(root)
	value := terms.Root(root)
	_, err := effects.IndexMutation(IndexMutationConfig{
		Table: PathEffectTarget(path), Key: value, Value: value,
		Admission: dynamicindex.AdmissionAdmitted, Site: EffectSite{Owner: 1},
	})
	if err == nil || !strings.Contains(err.Error(), "invalidation") {
		t.Fatalf("unpaired index mutation error = %v", err)
	}
	if _, err := effects.InvalidatePath(InvalidatePathConfig{Target: PathEffectTarget(path)}); err == nil {
		t.Fatal("invalidation without explicit scope was admitted")
	}
}

func TestDefaultEffectCatalogCoversExactlyAllSeventeenStateLanes(t *testing.T) {
	catalog := DefaultEffectCatalog()
	lanes := state.DefaultLaneCatalog().LaneSet().IDs()
	if len(lanes) != 17 {
		t.Fatalf("test assumption: got %d State lanes", len(lanes))
	}
	if got := catalog.Lanes(); len(got) != len(lanes) {
		t.Fatalf("effect lanes = %d, want %d", len(got), len(lanes))
	}
	for kind := EffectInvalidatePath; kind < effectKindCount; kind++ {
		descriptor, ok := catalog.Descriptor(kind)
		if !ok {
			t.Fatalf("effect descriptor %d missing", kind)
		}
		for _, lane := range lanes {
			if use := descriptor.LaneUse(lane); use == LaneUseInvalid {
				t.Fatalf("effect %d lane %q has no verdict", kind, lane)
			}
		}
	}
	expected := map[EffectKind]map[state.LaneID]LaneUse{
		EffectInvalidatePath: {
			state.LaneValues: LaneUseReadWrite, state.LanePathEvidence: LaneUseReadWrite,
			state.LaneDynamicIndex: LaneUseReadWrite, state.LaneHeapTableIdentity: LaneUseReadWrite,
			state.LaneKeyMemberships: LaneUseReadWrite, state.LaneLenFloors: LaneUseReadWrite,
			state.LaneDiffRelations: LaneUseReadWrite,
		},
		EffectIndexMutation: {
			state.LaneValues: LaneUseReadWrite, state.LanePathEvidence: LaneUseReadWrite,
			state.LaneDynamicIndex: LaneUseReadWrite, state.LaneHeapTableIdentity: LaneUseReadWrite,
			state.LaneEffectDeltas: LaneUseReadWrite, state.LaneKeyMemberships: LaneUseReadWrite,
			state.LaneTypestates: LaneUseReadWrite, state.LanePlacement: LaneUseReadWrite,
			state.LaneLenFloors: LaneUseReadWrite, state.LaneDiffRelations: LaneUseReadWrite,
		},
		EffectAllocationTemplate: {
			state.LaneHeapTableIdentity: LaneUseReadWrite, state.LanePlacement: LaneUseReadWrite,
		},
		EffectObjectMaterialization: {
			state.LaneValues: LaneUseRead, state.LaneHeapTableIdentity: LaneUseWrite,
			state.LanePlacement: LaneUseReadWrite,
		},
		EffectPathStore: {
			state.LaneValues: LaneUseReadWrite, state.LanePathEvidence: LaneUseReadWrite,
			state.LaneDynamicIndex: LaneUseReadWrite, state.LaneHeapTableIdentity: LaneUseReadWrite,
			state.LaneKeyMemberships: LaneUseReadWrite, state.LaneTypestates: LaneUseReadWrite,
			state.LanePlacement: LaneUseReadWrite, state.LaneLenFloors: LaneUseReadWrite,
			state.LaneUserLattices: LaneUseReadWrite,
		},
	}
	for kind := EffectInvalidatePath; kind < effectKindCount; kind++ {
		descriptor, _ := catalog.Descriptor(kind)
		for _, lane := range lanes {
			want := LaneUseUnaffected
			if use, ok := expected[kind][lane]; ok {
				want = use
			}
			if got := descriptor.LaneUse(lane); got != want {
				t.Errorf("effect %d lane %q = %d, want %d", kind, lane, got, want)
			}
		}
	}
}

func TestEffectCatalogFailsClosedOnLaneCatalogGrowthMissingAndOrphanCells(t *testing.T) {
	descriptors := defaultEffectDescriptors()
	grown := append(state.DefaultLaneCatalog().LaneSet().IDs(), state.LaneID("future-axis"))
	if _, err := bindEffectCatalog(grown, descriptors); err == nil || !strings.Contains(err.Error(), "future-axis") {
		t.Fatalf("catalog growth error = %v", err)
	}

	missing := defaultEffectDescriptors()
	delete(missing[0].lanes, state.LaneValues)
	if _, err := NewEffectCatalog(state.DefaultLaneCatalog(), missing); err == nil || !strings.Contains(err.Error(), "values") {
		t.Fatalf("missing lane error = %v", err)
	}

	orphan := defaultEffectDescriptors()
	orphan[0].lanes[state.LaneID("orphan")] = LaneUseUnaffected
	if _, err := NewEffectCatalog(state.DefaultLaneCatalog(), orphan); err == nil || !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("orphan lane error = %v", err)
	}
}

func TestEffectCatalogAdmitsIndexMutationPairAtomically(t *testing.T) {
	catalog := DefaultEffectCatalog()
	if _, ok, err := catalog.AdmitPoint([]operationplan.Kind{operationplan.DynamicIndexWrite}); err == nil || ok {
		t.Fatalf("unpaired write admitted ok=%t err=%v", ok, err)
	}
	admission, ok, err := catalog.AdmitPoint([]operationplan.Kind{
		operationplan.PathDescendantInvalidation,
		operationplan.DynamicIndexWrite,
	})
	if err != nil || !ok || admission.Kind != EffectIndexMutation || len(admission.Consumes) != 2 {
		t.Fatalf("paired admission = %#v ok=%t err=%v", admission, ok, err)
	}
	admission.Consumes[0] = operationplan.Return
	again, _, _ := catalog.AdmitPoint([]operationplan.Kind{
		operationplan.PathDescendantInvalidation,
		operationplan.DynamicIndexWrite,
	})
	if again.Consumes[0] == operationplan.Return {
		t.Fatal("admission exposed catalog source storage")
	}

	invalidation, ok, err := catalog.AdmitPoint([]operationplan.Kind{operationplan.PathDescendantInvalidation})
	if err != nil || !ok || invalidation.Kind != EffectInvalidatePath || len(invalidation.Consumes) != 1 {
		t.Fatalf("invalidation admission = %#v ok=%t err=%v", invalidation, ok, err)
	}
	if _, ok, err := catalog.AdmitPoint([]operationplan.Kind{operationplan.Return}); err != nil || ok {
		t.Fatalf("unrelated operation admission ok=%t err=%v", ok, err)
	}
}

func TestEffectCatalogRejectsNonAtomicDescriptorSources(t *testing.T) {
	descriptors := defaultEffectDescriptors()
	descriptors[1].sources = []operationplan.Kind{operationplan.DynamicIndexWrite}
	if _, err := NewEffectCatalog(state.DefaultLaneCatalog(), descriptors); err == nil || !strings.Contains(err.Error(), "atomic") {
		t.Fatalf("non-atomic descriptor error = %v", err)
	}
}
