package formalfreeze

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
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	"github.com/wippyai/go-lua/domain/runtimekind"
	"github.com/wippyai/go-lua/domain/type/typecontract"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/internal/testfixture"
)

// TestFreezeReadsAnActualThroughTheOwnersCellAdmission states how this
// derivation reads one cell of the actuals vector. The cells are Value's, so
// what one means is Value's judgment: AuthenticateFactorCell admits a present
// owner-fenced fact or the owner's own sparse Bottom, and refuses everything
// else. An absent cell is therefore answered as Bottom - no exact root, and no
// route - and never read as though the value beside its presence bit were the
// fact the cell holds.
func TestFreezeReadsAnActualThroughTheOwnersCellAdmission(t *testing.T) {
	values, allocation, recent := freezeActualCellFixture(t)
	foreignValues, _, foreignRecent := freezeActualCellFixture(t)
	if foreignValues == values {
		t.Fatal("the fixture sealed one owner where the law needs two")
	}

	tests := []struct {
		name     string
		cell     operand.MemberCell[valuedomain.Value]
		admitted bool
		rooted   bool
	}{
		{name: "a present exact Recent allocation is the root it names", cell: operand.MemberCell[valuedomain.Value]{Value: recent, Present: true}, admitted: true, rooted: true},
		{name: "an absent cell is the owner's exact Bottom", cell: operand.MemberCell[valuedomain.Value]{Value: values.Bottom(), Present: false}, admitted: true},
		{name: "an absent cell beside a rooted value is malformed", cell: operand.MemberCell[valuedomain.Value]{Value: recent, Present: false}},
		{name: "a fact this owner never issued is malformed", cell: operand.MemberCell[valuedomain.Value]{Value: foreignRecent, Present: true}},
		{name: "a present cell with no fact is malformed", cell: operand.MemberCell[valuedomain.Value]{Present: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actuals, actualsOK := operand.NewMemberVector([]operand.MemberCell[valuedomain.Value]{test.cell})
			if !actualsOK {
				t.Fatal("member vector")
			}
			root, rooted, admitted := freezeActualRoot(values, actuals, 0)
			if admitted != test.admitted {
				t.Fatalf("admitted = %t, want %t", admitted, test.admitted)
			}
			if rooted != test.rooted {
				t.Fatalf("rooted = %t, want %t", rooted, test.rooted)
			}
			if test.rooted && root != allocation {
				t.Fatalf("root = %#v, want %#v", root, allocation)
			}
			if !test.rooted && root.Valid() {
				t.Fatalf("an unrooted cell answered with key %#v", root)
			}
		})
	}

	actuals, actualsOK := operand.NewMemberVector([]operand.MemberCell[valuedomain.Value]{{Value: recent, Present: true}})
	if !actualsOK {
		t.Fatal("member vector")
	}
	if _, _, admitted := freezeActualRoot(values, actuals, 1); admitted {
		t.Fatal("an ordinal past the vector's width named a cell")
	}
	if _, _, admitted := freezeActualRoot(nil, actuals, 0); admitted {
		t.Fatal("a cell was admitted with no owner to admit it")
	}
}

// freezeActualCellFixture seals one program to the altitude Value's allocation
// directory is published at, without reaching the composition this package is
// a member of, and answers with an exact Recent allocation fact of that owner.
func freezeActualCellFixture(t testing.TB) (*valuedomain.Schema, heapdomain.Key, valuedomain.Value) {
	t.Helper()
	program, err := lower.Lower(lower.Source{Name: "formal-freeze-actual-cell.lua", Text: []byte("return {}")})
	if err != nil {
		t.Fatal(err)
	}
	anyType, anyOK := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !anyOK {
		t.Fatal("portable any type")
	}
	target, err := compiler.Seal(&declaration.Spec{
		Semantics: typecontract.NewSemantics(),
		Operations: []vocabulary.OperationSpec{{
			Bindings: []vocabulary.BindingSpec{{Namespace: vocabulary.BindingBuiltin, Member: []string{"require"}}},
			Input:    vocabulary.ValuesSpec{Tail: vocabulary.ValuesClosed},
			Outcomes: []vocabulary.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: vocabulary.ValuesSpec{Fixed: []schematype.Type{anyType}, Tail: vocabulary.ValuesClosed}}},
			Effects:  vocabulary.RowSpec{Tail: vocabulary.RowClosed},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	linked, err := link.Seal(&link.Spec{Target: target, Modules: []linkproject.Module{{Name: "formal-freeze-actual-cell", Program: program}}})
	if err != nil {
		t.Fatal(err)
	}
	grammar, grammarOK := programartifact.NewExecutionSchemaID(identity.ContentID{1}, identity.ContentID{2}, programartifact.GrammarABIVersion)
	artifact, failure := artifactcompiler.CompileDetailed(program, grammar, testfixture.EmptyProgramIssuancePlan(t))
	shard, shardOK := linked.Project().Mounts().At(0)
	module, moduleOK := linked.Project().ModuleKey(shard)
	structural := freezeActualCellVocabulary(t)
	snapshot, lowered := ingress.Lower(artifact, structural)
	mount, mountOK := programmount.MountedArtifactFromSnapshot(snapshot, module)
	if !grammarOK || failure.Available() || artifact == nil || !lowered || !shardOK || !moduleOK || !mountOK {
		t.Fatalf("actual-cell fixture grammar=%t artifact=%v ingress=%t shard=%t module=%t mount=%t", grammarOK, failure, lowered, shardOK, moduleOK, mountOK)
	}
	heaps, heapFailure := heapdomain.SealWithArtifacts(linked, []programmount.MountedArtifact{mount})
	values, valueFailure := valuedomain.SealWithFailure(linked, heaps, calltest.MustSeal(t, linked, []programmount.MountedArtifact{mount}), []programmount.MountedArtifact{mount}, structural)
	if heapFailure != heapdomain.SealFailureNone || valueFailure != valuedomain.SealFailureNone || values == nil {
		t.Fatalf("actual-cell seal heap=%v value=%v", heapFailure, valueFailure)
	}
	for index := 0; index < heaps.KeyCount(); index++ {
		key, keyOK := heaps.KeyAt(index)
		if !keyOK || key.Kind() != heapdomain.RootAllocation {
			continue
		}
		atom, atomOK := values.Allocation(key, materialization.Recent)
		if !atomOK {
			continue
		}
		if fact, factOK := values.Singleton(atom); factOK {
			return values, key, fact
		}
	}
	t.Fatal("the fixture sealed no exact Recent allocation fact")
	return nil, heapdomain.Key{}, valuedomain.Value{}
}

// freezeActualCellVocabulary is the structural table the seal above needs and
// the composition would otherwise supply.
func freezeActualCellVocabulary(t testing.TB) structure.Table {
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
			spelling := fmt.Sprintf("formal-freeze-actual-cell/%d/%d", category, ordinal)
			specs = append(specs, structure.Spec{
				Key: schema.Key(spelling), Category: category, Ordinal: uint16(ordinal),
				Spelling: spelling, Accepted: true,
			})
		}
	}
	entries, entriesOK := structure.Collect(specs)
	if !entriesOK {
		t.Fatal("actual-cell structural declarations")
	}
	builder := seal.NewBuilder()
	if !builder.Register(structure.NewSurface(entries)) {
		t.Fatal("actual-cell structural surface")
	}
	for kind := schema.SurfaceKindAxis; kind <= schema.SurfaceKindObservation; kind++ {
		if !builder.Register(freezeActualCellSurface{kind: kind}) {
			t.Fatalf("actual-cell surface %d", kind)
		}
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("actual-cell schema: %v", failure)
	}
	view, viewOK := sealed.Surface(schema.SurfaceKindStructure)
	if !viewOK {
		t.Fatal("actual-cell structural view")
	}
	table, tableOK := structure.NewTable(view)
	if !tableOK {
		t.Fatal("actual-cell structural table")
	}
	return table
}

type freezeActualCellSurface struct{ kind schema.SurfaceKind }

func (surface freezeActualCellSurface) Kind() schema.SurfaceKind { return surface.kind }
func (surface freezeActualCellSurface) Entries() []schema.Entry  { return nil }
func (surface freezeActualCellSurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}
