package formal

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/operand"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	artifactcompiler "github.com/wippyai/go-lua/analysis/program/artifact/compiler"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/link"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/target/compiler"
	"github.com/wippyai/go-lua/analysis/program/target/declaration"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/ingress"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
	"github.com/wippyai/go-lua/domain/call/calltest"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/placement"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestFormalActualsAreAddressedByTheirPositionInTheDeliveredVector states the
// correlation the deleted tag machinery used to state. A member vector is
// addressed by (parent, ordinal) and a cell's POSITION is the ordinal its
// owner declared it at, so the derivation reads actual N at index N and needs
// no tag beside the cell to agree with. A vector narrower than the ordinal a
// formal selector names is not that call's member set, and it is refused
// rather than completed.
func TestFormalActualsAreAddressedByTheirPositionInTheDeliveredVector(t *testing.T) {
	schema, values := formalSoundnessSchemas(t)
	keys := routePlanAllocationKeys(t, schema)
	if len(keys) < 2 {
		t.Skip("soundness schema exposes fewer than two allocation roots")
	}
	cells := make([]operand.MemberCell[valuedomain.Value], len(keys))
	for index, key := range keys {
		atom, atomOK := values.Allocation(key, materialization.Recent)
		fact, factOK := values.Singleton(atom)
		if !atomOK || !factOK {
			t.Fatalf("allocation fact at ordinal %d", index)
		}
		cells[index] = operand.MemberCell[valuedomain.Value]{Value: fact, Present: true}
	}
	actuals := formalActuals(t, cells)
	for ordinal, key := range keys {
		var demands denseDemandScratch
		unknown, demandOK := addFactDemandDense(schema, values, actuals, ordinal, placement.Retain, &demands)
		if !demandOK || unknown || demands.count != 1 {
			t.Fatalf("ordinal %d demand = %t/%t/%d, want one exact demand", ordinal, demandOK, unknown, demands.count)
		}
		demand, demandAt := demands.at(0)
		dense, denseOK := denseDemandKey(schema, key)
		if !demandAt || !denseOK || demand.dense != dense {
			t.Fatalf("ordinal %d resolved dense %d, want the root declared at that position (%d)", ordinal, demand.dense, dense)
		}
	}
	var beyond denseDemandScratch
	if _, beyondOK := addFactDemandDense(schema, values, actuals, len(keys), placement.Retain, &beyond); beyondOK {
		t.Fatal("an ordinal past the delivered vector was completed")
	}
	if _, negativeOK := addFactDemandDense(schema, values, actuals, -1, placement.Retain, &beyond); negativeOK {
		t.Fatal("a negative ordinal was completed")
	}
}

func TestUnknownOpenTailUnavailableActualRefuses(t *testing.T) {
	schema, values := formalSoundnessSchemas(t)
	// Two shapes of missing evidence survive the move to a delivered vector:
	// an ordinal the vector does not name at all, and a cell that claims
	// absence while carrying a value the owner's Factor default is not.
	absent := formalActuals(t, []operand.MemberCell[valuedomain.Value]{{Value: values.Top()}})
	for name, ordinal := range map[string]int{"past-the-vector": 1, "negative": -1} {
		t.Run(name, func(t *testing.T) {
			var demands denseDemandScratch
			if addUnknownOpenTailActualDemandDense(schema, values, absent, ordinal, &demands) {
				t.Fatalf("ordinal %d outside the delivered vector was accepted: %#v", ordinal, demands)
			}
			if demands.count != 0 || demands.allUnknown {
				t.Fatalf("rejected ordinal %d mutated demands: %#v", ordinal, demands)
			}
		})
	}
	var forged denseDemandScratch
	if addUnknownOpenTailActualDemandDense(schema, values, absent, 0, &forged) {
		t.Fatalf("an absent cell carrying a non-default value was accepted: %#v", forged)
	}
	if forged.count != 0 || forged.allUnknown {
		t.Fatalf("rejected forged absence mutated demands: %#v", forged)
	}

	// An authenticated Value Top remains a lawful widening witness under an
	// authenticated unknown formal tail. The distinction is the present,
	// owner-fenced Value fact, not the absence of a selected cell.
	var demands denseDemandScratch
	top := values.Top()
	if !addUnknownOpenTailActualDemandDense(schema, values, formalActuals(t, []operand.MemberCell[valuedomain.Value]{{Value: top, Present: true}}), 0, &demands) {
		t.Fatal("authenticated Value Top was rejected under unknown open tail")
	}
	if !demands.allUnknown {
		t.Fatalf("authenticated Value Top demands = %#v, want all-root widening mode", demands)
	}

	// The owner-issued sparse Bottom is equally authenticated, but carries no
	// allocation alternatives and therefore adds no demand.
	demands = denseDemandScratch{}
	if !addUnknownOpenTailActualDemandDense(schema, values, formalActuals(t, []operand.MemberCell[valuedomain.Value]{{Value: values.Bottom()}}), 0, &demands) {
		t.Fatal("owner-issued sparse Bottom was rejected under unknown open tail")
	}
	if demands.count != 0 || demands.allUnknown {
		t.Fatalf("sparse Bottom open-tail demands = %#v, want empty exact demand", demands)
	}
}

func TestDenseDemandExactAllocationUsesCanonicalHeapCoordinate(t *testing.T) {
	schema, values := formalSoundnessSchemas(t)
	keys := routePlanAllocationKeys(t, schema)
	if len(keys) == 0 {
		t.Fatal("soundness fixture has no allocation root")
	}
	atom, atomOK := values.Allocation(keys[0], materialization.Recent)
	fact, factOK := values.Singleton(atom)
	if !atomOK || !factOK {
		t.Fatalf("allocation fact = %t/%t", atomOK, factOK)
	}
	var demands denseDemandScratch
	unknown, demandOK := addFactDemandDense(schema, values, formalActuals(t, []operand.MemberCell[valuedomain.Value]{{Value: fact, Present: true}}), 0, placement.Retain, &demands)
	if unknown || !demandOK || demands.count != 1 {
		t.Fatalf("exact dense demand = unknown:%t ok:%t count:%d", unknown, demandOK, demands.count)
	}
	plan, planOK := (&routePlan{}).seal(schema, &demands)
	if !planOK || plan.routeCount() != 1 {
		t.Fatalf("exact dense plan = %t/%d", planOK, plan.routeCount())
	}
	route, routeOK := plan.routeAt(0)
	if !routeOK || route.unknown || route.escape != placement.Retain || route.key != keys[0] {
		t.Fatalf("exact dense route = %#v/%t", route, routeOK)
	}
}

// formalSoundnessAnyType is the one portable result type the fixture's
// require operation answers with.
func formalSoundnessAnyType(t testing.TB) schematype.Type {
	t.Helper()
	value, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		t.Fatal("portable any type")
	}
	return value
}

func formalSoundnessSchemas(t testing.TB) (placement.Schema, *valuedomain.Schema) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "formal-soundness.lua", Text: []byte("return {}")})
	if err != nil {
		t.Fatal(err)
	}
	target, err := compiler.Seal(&declaration.Spec{
		Semantics: typecontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{{
			// Require is the module-load producer the Boundary names, so its
			// normal outcome carries the one result value a module root is
			// answered at. A require declaring no result answers no module,
			// and Value refuses the whole schema for it.
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{formalSoundnessAnyType(t)}, Tail: vocabulary.ValuesClosed}}},
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
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, testfixture.EmptyProgramIssuancePlan(t))
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	_, programIDOK := linked.Project().Mounts().ProgramID(shard)
	structural := formalSoundnessStructuralVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	heapSchema, heapFailure := heap.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	placementSchema, placementOK := placement.NewSchema(heapSchema)
	valueMount, valueMountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	values, valueFailure := valuedomain.SealWithFailure(linked, heapSchema, calltest.MustSeal(t, linked, []programmount.MountedArtifact{valueMount}), []programmount.MountedArtifact{valueMount}, structural)
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
		default:
			return 1
		}
	}
	var specs []structure.Spec
	for category := structure.CategoryArm; category.Available(); category++ {
		if category == structure.CategoryRelationGeometryScalar {
			continue
		}
		for ordinal := 1; ordinal <= counts(category); ordinal++ {
			spelling := fmt.Sprintf("formal-soundness/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal),
				Spelling: spelling, Accepted: true,
			})
		}
	}
	specs = append(specs, structure.RelationGeometrySpecs()...)
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("formal soundness structural declarations")
	}
	builder := seal.NewBuilder()
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
func (surface formalSoundnessEmptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
