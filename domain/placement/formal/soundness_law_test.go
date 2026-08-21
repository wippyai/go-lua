package formal

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

func TestFormalActualTagsAreCanonicalAndUnique(t *testing.T) {
	for ordinal := 0; ordinal < 8; ordinal++ {
		got, ok := canonicalActualTag(ordinal)
		if !ok || got != actualTag(ordinal+1) {
			t.Fatalf("actual ordinal %d tag = %d/%t, want %d/true", ordinal, got, ok, ordinal+1)
		}
	}
	if _, ok := canonicalActualTag(-1); ok {
		t.Fatal("negative actual ordinal received a canonical tag")
	}

	// The staged evaluator and frame checker both consume selection rows in
	// ordinal order. A duplicate or permutation must fail the same canonical
	// tag check before it can be mapped to an authored actual.
	for name, tags := range map[string][]actualTag{
		"duplicate":    {actualTag(1), actualTag(1)},
		"permuted":     {actualTag(2), actualTag(1)},
		"zero":         {actualTag(0), actualTag(2)},
		"out-of-range": {actualTag(1), actualTag(3)},
	} {
		t.Run(name, func(t *testing.T) {
			canonical := true
			for ordinal, tag := range tags {
				expected, expectedOK := canonicalActualTag(ordinal)
				if !expectedOK || tag != expected {
					canonical = false
					break
				}
			}
			if canonical {
				t.Fatalf("malformed tag sequence %v passed canonical ordinal checks", tags)
			}
		})
	}
}

func TestUnknownOpenTailUnavailableActualRefuses(t *testing.T) {
	schema, values := formalSoundnessSchemas(t)
	for name, observation := range map[string]actualObservation{
		"invalid":     {},
		"unavailable": {valid: true, present: false},
	} {
		t.Run(name, func(t *testing.T) {
			demands := make(map[heap.Key]routeDemand)
			if addUnknownOpenTailObservationDemand(schema, values, observation, demands) {
				t.Fatalf("missing unknown open-tail observation was accepted: %#v", demands)
			}
			if len(demands) != 0 {
				t.Fatalf("rejected unknown open-tail observation mutated demands: %#v", demands)
			}
		})
	}

	// An authenticated Value Top remains a lawful widening witness under an
	// authenticated unknown formal tail. The distinction is the present,
	// owner-fenced Value fact, not the absence of a selected cell.
	demands := make(map[heap.Key]routeDemand)
	top := values.Top()
	if !addUnknownOpenTailObservationDemand(schema, values, actualObservation{fact: top, present: true, valid: true}, demands) {
		t.Fatal("authenticated Value Top was rejected under unknown open tail")
	}
	if _, widened := demands[heap.Key{}]; !widened {
		t.Fatalf("authenticated Value Top demands = %#v, want all-root widening sentinel", demands)
	}
}

func formalSoundnessSchemas(t testing.TB) (placement.Schema, *valuedomain.Schema) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "formal-soundness.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	target, err := compiler.Seal(&declaration.Spec{
		Semantics: typecontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "formal-soundness", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, issuance.Directory{})
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	programID, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural := formalSoundnessStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := heap.NewArtifactMount(snapshot, module, programID)
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []heap.ArtifactMount{mount})
	placementSchema, placementOK := placement.NewSchema(heapSchema)
	valueMount, valueMountOK := valuedomain.NewArtifactMount(snapshot, module, programID)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, []valuedomain.ArtifactMount{valueMount}, structural)
	if !grammarOK || failure.Available() || artifact == nil || !lowered || !shardOK || !moduleOK || !programIDOK || !mountOK || !valueMountOK || heapFailure != heap.SealFailureNone || !placementOK || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("formal soundness fixture grammar=%t artifact=%v ingress=%t shard=%t module=%t program=%t mount=%t valueMount=%t heap=%v placement=%t value=%v", grammarOK, failure, lowered, shardOK, moduleOK, programIDOK, mountOK, valueMountOK, heapFailure, placementOK, valueFailure)
	}
	return placementSchema, values
}

func formalSoundnessStructuralVocabulary(t testing.TB) structure.Table {
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
		case structure.CategoryIssuanceForm:
			return 5
		case structure.CategoryIssuanceInput:
			return 4
		case structure.CategoryIssuanceStage:
			return 5
		case structure.CategoryIssuanceRequirement:
			return 2
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("formal-soundness/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal),
				Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("formal soundness structural declarations")
	}
	builder := schema.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("formal soundness structural surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(formalSoundnessEmptySurface{kind: kind}) {
			t.Fatalf("formal soundness surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("formal soundness schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("formal soundness structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("formal soundness structural table")
	}
	return table
}

type formalSoundnessEmptySurface struct{ kind schema.SurfaceKind }

func (surface formalSoundnessEmptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface formalSoundnessEmptySurface) Entries() []schema.Entry  { return nil }
func (surface formalSoundnessEmptySurface) Seal(schema.View, schema.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
