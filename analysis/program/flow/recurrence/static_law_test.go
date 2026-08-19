package recurrence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticoperands "github.com/wippyai/go-lua/analysis/program/static/operands"
	staticoperators "github.com/wippyai/go-lua/analysis/program/static/operators"
	statictypes "github.com/wippyai/go-lua/analysis/program/static/types"
)

// TestSealExcludesDecisionsInsideStaticTypeOfAndAnnotationClosure proves the
// static boundary through the real owner transaction.  The TypeOf operand and
// Annotation Values both refer to the Function expression whose authored
// body contains a Loop and Branch.  Containment must mark the complete
// expression closure static; recurrence must not manufacture a dynamic stream
// for either decision, even though the source/Flow families and graph still
// contain their authored rows.
func TestSealExcludesDecisionsInsideStaticTypeOfAndAnnotationClosure(t *testing.T) {
	entry := term(keyspace.FamilyBody, 1)
	functionBody := term(keyspace.FamilyBody, 2)
	loopBody := term(keyspace.FamilyBody, 3)
	whenTrue := term(keyspace.FamilyBody, 4)
	whenFalse := term(keyspace.FamilyBody, 5)
	function := term(keyspace.FamilyFunction, 1)
	cell := term(keyspace.FamilyCell, 1)
	loopCell := term(keyspace.FamilyCell, 2)
	bind := term(keyspace.FamilyBind, 1)
	functionValues := term(keyspace.FamilyValues, 1)
	loopControl := term(keyspace.FamilyValues, 2)
	annotationValues := term(keyspace.FamilyValues, 3)
	loop := term(keyspace.FamilyLoop, 1)
	branch := term(keyspace.FamilyBranch, 1)
	nilEntry := term(keyspace.FamilyNil, 1)
	nilControl := term(keyspace.FamilyNil, 2)
	nilControlSecond := term(keyspace.FamilyNil, 3)
	nilCondition := term(keyspace.FamilyNil, 4)
	nilAnnotation := term(keyspace.FamilyNil, 5)
	primitive := term(keyspace.FamilyTypePrimitive, 1)

	fixture := openOwnerFixture(t, ownerSpec{
		counts: countsWith(
			familyCount(keyspace.FamilyBody, 5),
			familyCount(keyspace.FamilyFunction, 1),
			familyCount(keyspace.FamilyCell, 2),
			familyCount(keyspace.FamilyBind, 1),
			familyCount(keyspace.FamilyValues, 3),
			familyCount(keyspace.FamilyNil, 5),
			familyCount(keyspace.FamilyLoop, 1),
			familyCount(keyspace.FamilyBranch, 1),
			familyCount(keyspace.FamilyTypePrimitive, 1),
			familyCount(keyspace.FamilyDeclaredType, 1),
			familyCount(keyspace.FamilyTypeOf, 1),
			familyCount(keyspace.FamilyAnnotation, 1),
		),
		rows: [][]keyspace.Term{
			{bind},
			{loop, branch},
			nil,
			nil,
			nil,
		},
		binds:     []source.BindCells{{Bind: bind, Cells: []keyspace.Term{cell}}},
		nilOwners: []keyspace.Term{entry, functionBody, functionBody, functionBody, entry},
		flow: authored.Input{
			Values: authored.ValuesInput{
				Rows: []authored.Value{
					{Owner: entry, Fixed: authored.Range{End: 1}},
					{Owner: functionBody, Fixed: authored.Range{Start: 1, End: 3}},
					{Owner: entry, Fixed: authored.Range{Start: 3, End: 4}},
				},
				// The Function is intentionally not an ordinary Values member: the
				// TypeOf row is its sole authored expression root. This prevents an
				// ordinary Values parent from competing with the static fallback.
				Terms: []keyspace.Term{nilEntry, nilControl, nilControlSecond, nilAnnotation},
			},
			Storage: authored.StorageInput{
				Cells: []authored.Cell{
					{Kind: authored.CellLocal, Body: entry},
					{Kind: authored.CellLocal, Body: loopBody},
				},
				Binds: []authored.Bind{{Owner: entry, Values: functionValues}},
			},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: entry, Body: functionBody}}},
			Control: authored.ControlInput{
				Cells: []keyspace.Term{loopCell},
				Branches: []authored.Branch{{
					Owner: functionBody, Condition: nilCondition,
					WhenTrue: whenTrue, WhenFalse: whenFalse,
				}},
				Loops: []authored.Loop{{
					Owner: functionBody, Body: loopBody,
					Kind: kind.LoopNumericFor, Control: loopControl,
					Cells: authored.Range{End: 1},
				}},
			},
		},
		static: static.Input{
			Types: statictypes.Input{Primitive: []statictypes.Primitive{{Kind: statictypes.PrimitiveNumber}}},
			Declarations: staticdecl.Input{DeclaredType: []staticdecl.DeclaredType{{
				Cell: cell, Target: primitive,
			}}},
			Operators: staticoperators.Input{TypeOf: []staticoperators.TypeOf{{Scope: cell, Operand: function}}},
			Operands: staticoperands.Input{Annotation: []staticoperands.Annotation{{
				Scope: cell, Target: primitive, Name: 1, Values: annotationValues,
			}}},
		},
	})

	if !fixture.forest.Static(function) || !fixture.forest.Static(functionBody) ||
		!fixture.forest.Static(loop) || !fixture.forest.Static(branch) {
		t.Fatalf("static closure = function=%v body=%v loop=%v branch=%v, want all true",
			fixture.forest.Static(function), fixture.forest.Static(functionBody),
			fixture.forest.Static(loop), fixture.forest.Static(branch))
	}
	recurrence, err := Seal(fixture.sourceView, fixture.flow, fixture.bodies, fixture.forest, fixture.graph,
		fixture.staticFinalize.View().ContentID(), fixture.moduleFinalize.View().ContentID())
	if err != nil {
		t.Fatalf("recurrence.Seal: %v", err)
	}
	for _, decision := range []keyspace.Term{loop, branch} {
		if count, ok := recurrence.DecisionCount(decision); ok || count != 0 {
			t.Fatalf("static decision %v stream = %d/%v, want 0/false", decision, count, ok)
		}
	}
	for index := 0; index < recurrence.ArcCount(); index++ {
		annotation, ok := recurrence.ArcAt(index)
		if !ok {
			continue
		}
		if annotation.Head == loop || annotation.Head == branch {
			t.Fatalf("static decision acquired recurrence Arc %d: %#v", index, annotation)
		}
	}
}
