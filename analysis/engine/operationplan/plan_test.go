package operationplan

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestFactsInputCatalogIsExhaustive(t *testing.T) {
	typ := reflect.TypeOf(factflow.FactsInput{})
	if got, want := len(descriptors), typ.NumField(); got != want {
		t.Fatalf("descriptor count %d != FactsInput field count %d", got, want)
	}
	seenFields := make(map[string]bool, len(descriptors))
	seenKinds := make(map[Kind]bool, len(descriptors))
	pointType := reflect.TypeOf(cfg.Point(0))
	exprType := reflect.TypeOf(factflow.ExprRef(0))
	for i, d := range descriptors {
		field, ok := typ.FieldByName(d.field)
		if !ok {
			t.Fatalf("descriptor %q has no FactsInput field", d.field)
		}
		if seenFields[d.field] || seenKinds[d.kind] {
			t.Fatalf("duplicate descriptor field=%q kind=%v", d.field, d.kind)
		}
		seenFields[d.field], seenKinds[d.kind] = true, true
		if d.kind != Kind(i+1) {
			t.Fatalf("kind catalog is not dense at %s: got %d want %d", d.field, d.kind, i+1)
		}
		if field.Type.Kind() != reflect.Map {
			t.Fatalf("FactsInput.%s is no longer a map; classify it explicitly", d.field)
		}
		switch d.class {
		case Executable, CompositeSidecar:
			if field.Type.Key() != pointType {
				t.Fatalf("point-local %s uses key %v", d.field, field.Type.Key())
			}
			if d.at == nil {
				t.Fatalf("point-local %s has no row accessor", d.field)
			}
			inputValue := reflect.New(typ).Elem()
			fieldValue := inputValue.FieldByIndex(field.Index)
			factMap := reflect.MakeMap(field.Type)
			factValue := reflect.Zero(field.Type.Elem())
			if field.Type.Elem().Kind() == reflect.Slice {
				factValue = reflect.MakeSlice(field.Type.Elem(), 1, 1)
			}
			factMap.SetMapIndex(reflect.ValueOf(cfg.Point(1)), factValue)
			fieldValue.Set(factMap)
			if !d.at(inputValue.Interface().(factflow.FactsInput), cfg.Point(1)) {
				t.Fatalf("row accessor for %s does not observe its field", d.field)
			}
		case Dependency:
			if field.Type.Key() != exprType {
				t.Fatalf("dependency %s uses key %v", d.field, field.Type.Key())
			}
			if d.at != nil {
				t.Fatalf("expression dependency %s has a point-row accessor", d.field)
			}
		default:
			t.Fatalf("%s has unspecified class %d", d.field, d.class)
		}
	}
	for i := 0; i < typ.NumField(); i++ {
		if !seenFields[typ.Field(i).Name] {
			t.Fatalf("FactsInput.%s is unclassified", typ.Field(i).Name)
		}
	}
}

func TestPlanDenseRowsAreCanonicalAndSnapshotIsOwned(t *testing.T) {
	graph := cfg.New()
	middle := graph.AddNode(cfg.NodeAssign)
	input := factflow.FactsInput{
		NoNormalReturns:        map[cfg.Point]struct{}{middle: {}},
		BranchConditionSources: map[cfg.Point]factflow.ValueSource{middle: {}},
		RootAssignments:        map[cfg.Point]factflow.RootAssignment{middle: {}},
	}
	plan := New(graph, input)
	if got, want := plan.PointCount(), graph.Size(); got != want {
		t.Fatalf("PointCount=%d want %d", got, want)
	}
	var got []Cell
	cur := plan.Cursor(middle)
	for cell, ok := cur.Next(); ok; cell, ok = cur.Next() {
		got = append(got, cell)
	}
	want := []Cell{
		{kind: RootAssignment, class: Executable},
		{kind: NoNormalReturn, class: Executable},
		{kind: BranchConditionSource, class: CompositeSidecar},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cells=%v want %v", got, want)
	}
	delete(input.NoNormalReturns, middle)
	if !plan.Facts().NoNormalReturn(middle) {
		t.Fatal("mutating FactsInput changed the plan-owned snapshot")
	}
	if cur := plan.Cursor(cfg.Point(graph.Size() + 10)); cursorLen(cur) != 0 {
		t.Fatal("out-of-range cursor is not empty")
	}
}

func TestPlanOrderDoesNotDependOnMapInsertion(t *testing.T) {
	graph := cfg.New()
	for i := 0; i < 30; i++ {
		graph.AddNode(cfg.NodeNoop)
	}
	points := make([]cfg.Point, graph.Size())
	for i := range points {
		points[i] = cfg.Point(i)
	}
	var baseline []Kind
	for seed := int64(0); seed < 100; seed++ {
		rng := rand.New(rand.NewSource(seed))
		rng.Shuffle(len(points), func(i, j int) { points[i], points[j] = points[j], points[i] })
		input := factflow.FactsInput{
			NoNormalReturns:        make(map[cfg.Point]struct{}),
			BranchConditionSources: make(map[cfg.Point]factflow.ValueSource),
		}
		for _, point := range points {
			input.NoNormalReturns[point] = struct{}{}
			input.BranchConditionSources[point] = factflow.ValueSource{}
		}
		got := planKinds(New(graph, input))
		if seed == 0 {
			baseline = got
		} else if !reflect.DeepEqual(got, baseline) {
			t.Fatalf("seed %d changed packed plan order", seed)
		}
	}
}

func TestCursorHasZeroAllocations(t *testing.T) {
	graph := cfg.New()
	input := factflow.FactsInput{NoNormalReturns: map[cfg.Point]struct{}{graph.Entry(): {}}}
	plan := New(graph, input)
	if got := testing.AllocsPerRun(1000, func() {
		cur := plan.Cursor(graph.Entry())
		for {
			if _, ok := cur.Next(); !ok {
				break
			}
		}
	}); got != 0 {
		t.Fatalf("cursor allocated %v times", got)
	}
}

func planKinds(plan *Plan) []Kind {
	var out []Kind
	for point := 0; point < plan.PointCount(); point++ {
		cur := plan.Cursor(cfg.Point(point))
		for cell, ok := cur.Next(); ok; cell, ok = cur.Next() {
			out = append(out, cell.Kind())
		}
	}
	return out
}

func cursorLen(cur Cursor) int {
	n := 0
	for _, ok := cur.Next(); ok; _, ok = cur.Next() {
		n++
	}
	return n
}
