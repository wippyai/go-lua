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
		case Executable, Composite, CompositeSidecar:
			if d.phase == 0 || d.barrier == 0 {
				t.Fatalf("point-local %s has no phase/barrier", d.field)
			}
			metadata, ok := Describe(d.kind)
			if !ok || !metadata.Stages.Has(d.barrier) {
				t.Fatalf("point-local %s stages omit primary barrier", d.field)
			}
			if d.class == CompositeSidecar && d.owners == 0 {
				t.Fatalf("sidecar %s has no composite owner", d.field)
			}
			if d.kind > 32 {
				t.Fatalf("point-local %s exceeds the packed kind mask capacity", d.field)
			}
			if field.Type.Key() != pointType {
				t.Fatalf("point-local %s uses key %v", d.field, field.Type.Key())
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
			plan := New(cfg.New(), inputValue.Interface().(factflow.FactsInput))
			observed := false
			cur := plan.Cursor(cfg.Point(1))
			for cell, ok := cur.Next(); ok; cell, ok = cur.Next() {
				observed = observed || cell.Kind() == d.kind
			}
			if !observed {
				t.Fatalf("plan compiler does not observe FactsInput.%s", d.field)
			}
		case Dependency:
			if d.phase != 0 || d.barrier != 0 || d.stages != 0 {
				t.Fatalf("dependency %s claims point execution metadata", d.field)
			}
			if d.owners == 0 {
				t.Fatalf("dependency %s has no consuming composite owner", d.field)
			}
			if field.Type.Key() != exprType {
				t.Fatalf("dependency %s uses key %v", d.field, field.Type.Key())
			}
		default:
			t.Fatalf("%s has unspecified class %d", d.field, d.class)
		}
	}
	for _, d := range descriptors {
		for _, owner := range descriptors {
			if !d.owners.Has(owner.kind) {
				continue
			}
			if owner.class != Composite {
				t.Fatalf("%s owner %s is class %d, not Composite", d.field, owner.field, owner.class)
			}
		}
	}
	for i := 0; i < typ.NumField(); i++ {
		if !seenFields[typ.Field(i).Name] {
			t.Fatalf("FactsInput.%s is unclassified", typ.Field(i).Name)
		}
	}
}

func TestOccurrenceIndexMatchesPointFamilyProbes(t *testing.T) {
	const points = 257
	input := sparseBenchmarkFacts(points)
	// Facts outside the graph were ignored by the old point-probe compiler and
	// must not manufacture an unreachable dense row in the occurrence compiler.
	input.NoNormalReturns[cfg.Point(points+100)] = struct{}{}
	gotRows, gotCells := compileIndex(points, input)
	wantRows, wantCells := compileIndexByProbing(points, input)
	if !reflect.DeepEqual(gotRows, wantRows) {
		t.Fatal("occurrence index rows differ from point-family probe rows")
	}
	if !reflect.DeepEqual(gotCells, wantCells) {
		t.Fatal("occurrence index cells differ from point-family probe cells")
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
		{kind: NoNormalReturn},
		{kind: RootAssignment},
		{kind: BranchConditionSource},
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

func TestCursorAndOwnersFollowConcreteSemanticBarriers(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	input := factflow.FactsInput{
		RootAssignments:               map[cfg.Point]factflow.RootAssignment{point: {}},
		NoNormalReturns:               map[cfg.Point]struct{}{point: {}},
		BranchConditionSources:        map[cfg.Point]factflow.ValueSource{point: {}},
		BranchRefinements:             map[cfg.Point]factflow.BranchRefinementSet{point: {}},
		PathValuePresenceImplications: map[cfg.Point]factflow.PathValuePresenceImplicationSet{point: {}},
		CallResultValues:              map[cfg.Point]factflow.CallResultValueSet{point: {}},
		CallSites:                     map[cfg.Point]factflow.CallSite{point: {}},
	}
	plan := New(graph, input)
	var got []Kind
	cursor := plan.Cursor(point)
	last := -1
	for cell, ok := cursor.Next(); ok; cell, ok = cursor.Next() {
		got = append(got, cell.Kind())
		rank := cursorBarrierRank(cell.Barrier())
		if rank < last {
			t.Fatalf("cursor moved backward from barrier rank %d to %d at %s", last, rank, cell.Kind())
		}
		last = rank
	}
	want := []Kind{CallSite, CallResultValue, NoNormalReturn, PathValuePresenceImplication, RootAssignment, BranchConditionSource, BranchRefinement}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cursor kinds=%v want %v", got, want)
	}
	call, _ := Describe(CallSite)
	if call.Class != Composite || !call.Stages.Has(N0Materialize) || !call.Stages.Has(E5CallEffects) || call.Stages.Has(N1NoReturn) {
		t.Fatalf("call metadata does not describe exact N0/E5 composite: %+v", call)
	}
	channel, _ := Describe(ChannelSelect)
	if !channel.Stages.Has(N0Materialize) || !channel.Stages.Has(N3Postconditions) || channel.Stages.Has(N2ImplicationClosure) {
		t.Fatalf("channel metadata does not describe exact N0/N3 composite: %+v", channel)
	}
	fixedResult, _ := Describe(CallResultValue)
	if fixedResult.Class != CompositeSidecar || !fixedResult.Owners.Has(CallSite) {
		t.Fatalf("fixed call result is not owned by call transaction: %+v", fixedResult)
	}
	expression, _ := Describe(ExpressionValue)
	if expression.Class != Dependency || expression.Phase != 0 || expression.Barrier != 0 || !expression.Owners.Has(RootAssignment) {
		t.Fatalf("expression dependency metadata is incomplete: %+v", expression)
	}
}

func TestHigherLayerExtensionIsPackedWithoutEnteringFactCursor(t *testing.T) {
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeAssign)
	plan := New(graph, factflow.FactsInput{}).WithExtensions([]ExtensionInput{
		{Point: point, Kind: BodyGenericFor},
		{Point: point, Kind: BodyGenericFor},
	})
	if !plan.HasExtensions() {
		t.Fatal("generic-for extension not registered")
	}
	factCursor := plan.Cursor(point)
	if _, ok := factCursor.Next(); ok {
		t.Fatal("higher-layer extension leaked into generic fact cursor")
	}
	cursor := plan.ExtensionCursor(point)
	cell, ok := cursor.Next()
	if !ok || cell.Kind() != BodyGenericFor {
		t.Fatalf("extension = %#v/%v, want BodyGenericFor", cell, ok)
	}
	if _, ok := cursor.Next(); ok {
		t.Fatal("duplicate extension was not canonicalized")
	}
	meta := cell.Metadata()
	if meta.Class != Composite || meta.Phase != Node || meta.Barrier != N7BodySemantics {
		t.Fatalf("metadata = %#v", meta)
	}
}

func TestExtensionCatalogIsExhaustiveAndFailClosed(t *testing.T) {
	for kind := ExtensionKind(1); kind <= BodyGenericFor; kind++ {
		meta := (ExtensionCell{kind: kind}).Metadata()
		if meta.Class == 0 || meta.Phase == 0 || meta.Barrier == 0 || meta.Stages == 0 {
			t.Fatalf("extension kind %d is not fully classified: %#v", kind, meta)
		}
	}
	if meta := (ExtensionCell{kind: BodyGenericFor + 1}).Metadata(); meta != (Metadata{}) {
		t.Fatalf("unknown extension classified as %#v", meta)
	}
}

func cursorBarrierRank(barrier Barrier) int {
	order := [...]Barrier{
		N0Materialize, N1NoReturn, N2ImplicationClosure, N3Postconditions, N4Writes, N5Return, N6CovariantFinalizer,
		E0Reachability, E1Refinements, E2ImplicationClosure, E3Relations, E4Evidence, E5CallEffects,
	}
	for i, candidate := range order {
		if candidate == barrier {
			return i
		}
	}
	return len(order)
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
