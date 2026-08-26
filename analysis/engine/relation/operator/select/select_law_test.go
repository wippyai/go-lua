package selectop

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/internal/relationoracle"
)

func TestSelectionConsumesConcreteBatch(t *testing.T) {
	fn := reflect.TypeOf(Execute)
	if fn.NumIn() != 4 || fn.In(2) != reflect.TypeOf(geometry.Geometry{}) || fn.In(3) != reflect.TypeOf(tuple.Batch{}) || fn.NumOut() != 2 || fn.Out(0) != reflect.TypeOf([]tuple.Batch{}) {
		t.Fatalf("selection ABI = %v", fn)
	}
}

func TestSelectionRefusesUnavailableSealedBinding(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	outputs, ok := Execute(arrangement.SelectBinding{}, fixture.Mounted(), fixture.Geometry(), tuple.Batch{})
	if ok || outputs != nil {
		t.Fatal("Select accepted an unavailable binding or batch")
	}
}

func selectBinding(t testing.TB, fixture testfixture.Fixture) arrangement.SelectBinding {
	t.Helper()
	node, ok := fixture.SelectNode()
	if !ok || !node.Available() {
		t.Fatal("sealed Select node")
	}
	binding, ok := node.Select()
	if !ok || !binding.Available() {
		t.Fatal("sealed Select binding")
	}
	return binding
}

// selectInput redeems the exact Input range and row vector for one mounted
// relation. It never creates a range, scope, row, or value identity.
func selectInput(t testing.TB, fixture testfixture.Fixture, left, reverse, empty bool) tuple.Batch {
	t.Helper()
	var node arrangement.Node
	var reader read.Reader
	var scope witness.Scope
	var ok bool
	if left {
		node, ok = fixture.LeftInputNode()
		reader, _ = fixture.ReaderLeftPayload(fixture.BothRoot())
		scope, _ = fixture.OverlapScopes()
	} else {
		node, ok = fixture.RightInputNode()
		reader, _ = fixture.ReaderRightPayload(fixture.BothRoot())
		_, scope = fixture.OverlapScopes()
	}
	if !ok || !node.Available() {
		t.Fatal("sealed Select input node")
	}
	input, inputOK := node.Input()
	if !inputOK || !input.Available() {
		t.Fatal("sealed Select input binding")
	}
	rangeAuthority, rangeOK := input.Range()
	if !rangeOK || !rangeAuthority.Available() {
		t.Fatal("sealed Select input range")
	}
	values := make([]tuple.Tuple, 0, 2)
	if !empty {
		if !reader.Available() {
			t.Fatal("Select input reader")
		}
		completed, valid := reader.Scan(func(row read.Row) bool {
			value, valueOK := tuple.Input(fixture.Mounted(), reader, row)
			if !valueOK {
				t.Fatal("Select input tuple")
			}
			values = append(values, value)
			return true
		})
		if !completed || !valid || len(values) == 0 {
			t.Fatal("Select input scan")
		}
		scope = values[0].Scope()
		if reverse {
			for leftIndex, rightIndex := 0, len(values)-1; leftIndex < rightIndex; leftIndex, rightIndex = leftIndex+1, rightIndex-1 {
				values[leftIndex], values[rightIndex] = values[rightIndex], values[leftIndex]
			}
		}
	}
	batch, batchOK := tuple.NewRangeBatch(fixture.Mounted(), rangeAuthority, scope, values, bindingpkg.DenominatorWitness{})
	if !batchOK || !batch.ValidFor(fixture.Mounted()) {
		t.Fatal("Select input batch")
	}
	return batch
}

func selectScopeIdentity(t testing.TB, mounted witness.Mounted, scope witness.Scope) relationoracle.Scope {
	t.Helper()
	token, ok := mounted.ScopeToken(scope)
	if !ok {
		t.Fatal("Select scope token")
	}
	region, ok := mounted.RegionForToken(token)
	if !ok {
		t.Fatal("Select scope region")
	}
	content := region.Identity()
	if !content.Available() {
		t.Fatal("Select scope identity")
	}
	value, ok := relationoracle.NewScope(content)
	if !ok {
		t.Fatal("oracle Select scope")
	}
	return value
}

func oracleSelectInput(t testing.TB, fixture testfixture.Fixture, input tuple.Batch, relation model.RelationID) relationoracle.Relation {
	t.Helper()
	if input.Len() == 0 {
		value, ok := relationoracle.EmptyRelation(relation)
		if !ok {
			t.Fatal("oracle empty Select relation")
		}
		return value
	}
	scope := selectScopeIdentity(t, fixture.Mounted(), input.Scope())
	rows := make([]relationoracle.Row, 0, input.Len())
	for index := 0; index < input.Len(); index++ {
		value, ok := input.At(index)
		if !ok {
			t.Fatal("oracle Select tuple")
		}
		rowID, ok := value.SourceAt(0)
		if !ok || rowID.Relation() != relation {
			t.Fatal("oracle Select source row")
		}
		cells := make([]relationoracle.Cell, 0, value.Len())
		for cellIndex := 0; cellIndex < value.Len(); cellIndex++ {
			cell, cellOK := value.At(cellIndex)
			if !cellOK {
				t.Fatal("oracle Select cell")
			}
			var oracleValue relationoracle.ValueToken
			if cell.Value().Available() {
				oracleValue, ok = relationoracle.NewValueToken(cell.Type(), cell.Value().Opaque())
				if !ok {
					t.Fatal("oracle Select value")
				}
			}
			oracleCell, ok := relationoracle.NewCell(cell.Column(), cell.Type(), oracleValue, cell.Presence())
			if !ok {
				t.Fatal("oracle Select cell authority")
			}
			cells = append(cells, oracleCell)
		}
		row, ok := relationoracle.NewRow(rowID, scope, cells, value.Lineage())
		if !ok {
			t.Fatal("oracle Select row")
		}
		rows = append(rows, row)
	}
	value, ok := relationoracle.NewRelation(relation, rows)
	if !ok {
		t.Fatal("oracle Select relation")
	}
	return value
}

func oracleSelectExpected(t testing.TB, fixture testfixture.Fixture, binding arrangement.SelectBinding, input tuple.Batch, relation model.RelationID) relationoracle.Relation {
	t.Helper()
	logicalScope, ok := fixture.Mounted().Scope(binding.Scope())
	if !ok {
		t.Fatal("mounted Select scope")
	}
	inputRelation := oracleSelectInput(t, fixture, input, relation)
	requested := selectScopeIdentity(t, fixture.Mounted(), logicalScope)
	// The physical operator's only semantic predicate is the sealed Geometry
	// entailment. The oracle receives that same owner-supplied witness rather
	// than reimplementing physical masks or declaring a second scope algebra.
	scopes := relationoracle.ScopeEntailmentFunc(func(relationoracle.Scope, relationoracle.Scope) bool {
		return fixture.Geometry().Entails(input.Scope(), logicalScope)
	})
	return relationoracle.SelectByScope(inputRelation, requested, scopes)
}

func assertSelectOracle(t testing.TB, fixture testfixture.Fixture, input tuple.Batch, outputs []tuple.Batch, expected relationoracle.Relation) {
	t.Helper()
	if len(outputs) != 1 || !outputs[0].ValidFor(fixture.Mounted()) || outputs[0].Range().Producer() != input.Range().Producer() {
		t.Fatalf("Select output envelope=(%d,%v)", len(outputs), len(outputs) == 1 && outputs[0].Available())
	}
	output := outputs[0]
	if output.Len() != len(expected.Rows()) || !output.Scope().Same(input.Scope()) {
		t.Fatalf("Select oracle/physical shape=%d/%d scope=%v", output.Len(), len(expected.Rows()), output.Scope().Same(input.Scope()))
	}
	got := make(map[model.RowID]tuple.Tuple, output.Len())
	for index := 0; index < output.Len(); index++ {
		value, ok := output.At(index)
		if !ok {
			t.Fatal("Select output tuple")
		}
		row, ok := value.SourceAt(0)
		if !ok {
			t.Fatal("Select output source")
		}
		got[row] = value
	}
	for _, expectedRow := range expected.Rows() {
		value, ok := got[expectedRow.ID()]
		if !ok {
			t.Fatalf("Select omitted row %v", expectedRow.ID())
		}
		if value.Lineage() != expectedRow.Lineage() {
			t.Fatalf("Select changed lineage for row %v", expectedRow.ID())
		}
		for _, expectedCell := range expectedRow.Cells() {
			gotCell, gotOK := value.CellFor(expectedCell.Column())
			if !gotOK || gotCell.Type() != expectedCell.Type() || gotCell.Presence().Kind() != expectedCell.Presence().Kind() {
				t.Fatalf("Select cell row=%v column=%v diverged", expectedRow.ID(), expectedCell.Column())
			}
			wantValue, wantOK := expectedCell.Value()
			gotValue := gotCell.Value()
			if wantOK != gotValue.Available() || wantOK && wantValue.Content() != gotValue.Opaque() {
				t.Fatalf("Select value row=%v column=%v diverged", expectedRow.ID(), expectedCell.Column())
			}
		}
	}
}

func TestSelectionMatchesOracleForPositiveScope(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	binding := selectBinding(t, fixture)
	input := selectInput(t, fixture, true, false, false)
	outputs, ok := Execute(binding, fixture.Mounted(), fixture.Geometry(), input)
	if !ok {
		t.Fatal("positive Select refused")
	}
	assertSelectOracle(t, fixture, input, outputs, oracleSelectExpected(t, fixture, binding, input, fixture.RelationLeft()))
}

func TestSelectionMatchesOracleForDisjointEmptyScope(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	binding := selectBinding(t, fixture)
	node, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left input node")
	}
	inputBinding, ok := node.Input()
	if !ok {
		t.Fatal("left input binding")
	}
	rangeAuthority, ok := inputBinding.Range()
	if !ok {
		t.Fatal("left input range")
	}
	_, disjoint := fixture.DisjointScopes()
	input, ok := tuple.NewRangeBatch(fixture.Mounted(), rangeAuthority, disjoint, []tuple.Tuple{}, bindingpkg.DenominatorWitness{})
	if !ok {
		t.Fatal("disjoint empty Select input")
	}
	outputs, accepted := Execute(binding, fixture.Mounted(), fixture.Geometry(), input)
	if !accepted {
		t.Fatal("disjoint empty Select refused")
	}
	assertSelectOracle(t, fixture, input, outputs, oracleSelectExpected(t, fixture, binding, input, fixture.RelationLeft()))
}

func TestSelectionMatchesOracleForNonMatchingScope(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	binding := selectBinding(t, fixture)
	input := selectInput(t, fixture, false, false, false)
	expected := oracleSelectExpected(t, fixture, binding, input, fixture.RelationRight())
	if len(expected.Rows()) != 0 {
		t.Fatalf("fixture right scope unexpectedly entails Select scope: oracle rows=%d", len(expected.Rows()))
	}
	outputs, ok := Execute(binding, fixture.Mounted(), fixture.Geometry(), input)
	if !ok {
		t.Fatal("nonmatching Select refused")
	}
	assertSelectOracle(t, fixture, input, outputs, expected)
}

func TestSelectionMatchesOracleUnderPermutation(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	binding := selectBinding(t, fixture)
	firstInput := selectInput(t, fixture, true, false, false)
	secondInput := selectInput(t, fixture, true, true, false)
	first, firstOK := Execute(binding, fixture.Mounted(), fixture.Geometry(), firstInput)
	second, secondOK := Execute(binding, fixture.Mounted(), fixture.Geometry(), secondInput)
	if !firstOK || !secondOK {
		t.Fatal("permuted Select refused")
	}
	assertSelectOracle(t, fixture, firstInput, first, oracleSelectExpected(t, fixture, binding, firstInput, fixture.RelationLeft()))
	assertSelectOracle(t, fixture, secondInput, second, oracleSelectExpected(t, fixture, binding, secondInput, fixture.RelationLeft()))
	if first[0].Len() != second[0].Len() {
		t.Fatal("Select changed cardinality under permutation")
	}
}

func TestSelectionRefusesForeignMountAndMalformedSource(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	foreign := testfixture.New(t, 0x72)
	binding := selectBinding(t, fixture)
	foreignBinding := selectBinding(t, foreign)
	input := selectInput(t, fixture, true, false, false)
	if outputs, ok := Execute(foreignBinding, fixture.Mounted(), fixture.Geometry(), input); ok || outputs != nil {
		t.Fatal("Select accepted a binding sealed by a foreign mount")
	}
	foreignInput := selectInput(t, foreign, true, false, false)
	if outputs, ok := Execute(binding, foreign.Mounted(), fixture.Geometry(), foreignInput); ok || outputs != nil {
		t.Fatal("Select accepted a geometry from a foreign mount")
	}
	if outputs, ok := Execute(binding, fixture.Mounted(), fixture.Geometry(), tuple.Batch{}); ok || outputs != nil {
		t.Fatal("Select accepted malformed source")
	}
}
