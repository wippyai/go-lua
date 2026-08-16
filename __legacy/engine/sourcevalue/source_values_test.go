package sourcevalue

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	path "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	. "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

func TestSourceValuesPanicsWithoutRegistry(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "SourceValuesConfig.Registry is required") {
			t.Fatal("NewSourceValues did not panic")
		}
	}()

	_ = NewSourceValues(SourceValuesConfig{})
}

func TestSourceValuesExpressionMapResolvesAndDoesNotMutateState(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(10)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	value := presentValue(reg)
	values := map[ExprRef]product.Value{expr: value}
	resolver := NewSourceValues(SourceValuesConfig{
		Registry:         reg,
		ExpressionValues: values,
	})
	values[expr] = absentValue(reg)

	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(1)), absentValue(reg))
	wantState := in.Snapshot()

	got, ok := resolver.ValueOfSource(cfg.Point(1), source, in, nil)
	if !ok {
		t.Fatal("expression source did not resolve")
	}
	if !product.Equal(reg, got, value) {
		t.Fatalf("expression value = %s, want %s", formatValue(reg, got), formatValue(reg, value))
	}
	assertStateEqual(t, reg, in, wantState)
}

func TestWithExpressionValueRebindsExpressionProvider(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(11)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	want := typevalue.FromType(reg, typ.String)
	base := NewSourceValues(SourceValuesConfig{Registry: reg})
	rebound := WithExpressionValue(base, func(point cfg.Point, gotExpr ExprRef, gotSource ValueSource, _ state.State) (product.Value, bool) {
		if point != 3 {
			t.Fatalf("point = %d, want 3", point)
		}
		if gotExpr != expr {
			t.Fatalf("expr = %d, want %d", gotExpr, expr)
		}
		if gotSource != source {
			t.Fatalf("source = %#v, want %#v", gotSource, source)
		}
		return want, true
	})

	got, ok := rebound.ValueOfSource(cfg.Point(3), source, state.State{}, nil)
	if !ok {
		t.Fatal("rebound expression source did not resolve")
	}
	if !product.Equal(reg, got, want) {
		t.Fatalf("rebound expression value = %s, want %s", formatValue(reg, got), formatValue(reg, want))
	}
}

func TestSourceValuesNilReturnsAbsentPresence(t *testing.T) {
	reg := standard.Registry()
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})

	got, ok := resolver.ValueOfSource(cfg.Point(2), NewNilValueSource(0), state.State{}, nil)
	if !ok {
		t.Fatal("nil source did not resolve")
	}
	if !presence.Equal(product.PresenceOf(got), presence.Absent()) {
		t.Fatalf("nil source presence = %s, want absent", product.PresenceOf(got))
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Nil) {
		t.Fatalf("nil source type = %v/%v, want nil witness", gotType, ok)
	}
}

func TestSourceValuesPathSourceReadsRootSymbolValue(t *testing.T) {
	reg := standard.Registry()
	ks := keyspace.New()
	pathKey := path.NewPath(symbol.ID(7), "value").Key()
	source, ok := NewPathValueSource(pathKey, 0, 0, 0, ValueSourceShape{})
	if !ok {
		t.Fatalf("NewPathValueSource returned false")
	}
	want := product.Set(reg, presentValue(reg), runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(7)), want)
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg, KeySpace: ks})

	got, ok := resolver.ValueOfSource(cfg.Point(1), source, in, nil)
	if !ok {
		t.Fatal("path source did not resolve")
	}
	if kind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(kind, runtimekind.Singleton(runtimekind.String)) {
		t.Fatalf("path source runtime kind = %s, want string", kind)
	}
}

func TestSourceValuesPathSourceUsesPointVisibleVersionedPathProof(t *testing.T) {
	reg := standard.Registry()
	target := symbol.ID(7)
	point := cfg.Point(42)
	p := path.NewPath(target, "bindings").Field("checkpoint")
	source, ok := NewPathValueSource(p.Key(), 0, 0, 0, ValueSourceShape{})
	if !ok {
		t.Fatalf("NewPathValueSource returned false")
	}
	builder := visibility.NewBuilder()
	version := builder.Define(point, target, "bindings")
	resolver := visibility.NewResolver(builder.Build())
	pathKey := resolver.KeyForVersion(target, version.ID, p.Segments)
	want := typevalue.WithWitness(reg, product.Top(), typetable.BuiltinTopMarker())
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(target), product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())).
		WritePathKey(reg, resolver.KeySpace(), pathKey, want)
	sources := NewSourceValues(SourceValuesConfig{
		Registry:   reg,
		KeySpace:   resolver.KeySpace(),
		Visibility: resolver,
	})

	got, ok := sources.ValueOfSource(point, source, in, nil)
	if !ok {
		t.Fatal("path source did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok {
		t.Fatalf("path source type missing, want point-visible table proof")
	}
	gotKinds, ok := typevalue.RuntimeKindFromType(gotType)
	if !ok || !gotKinds.Contains(runtimekind.Table) {
		t.Fatalf("path source type = %v, want point-visible table proof", gotType)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); gotEvidence.IsExplicitTop() {
		t.Fatalf("path source evidence = %s, want visible path proof not root any evidence", gotEvidence)
	}
}

func TestSourceValuesPathSourceProjectsSegmentedPathFromRootType(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	ks := keyspace.New()
	p := path.NewPath(symbol.ID(7), "self").Field("node_id")
	source, ok := NewPathValueSource(p.Key(), 0, 0, 0, ValueSourceShape{})
	if !ok {
		t.Fatalf("NewPathValueSource returned false")
	}
	rootType := typetable.NewRecord().Field("node_id", typ.String).Build()
	rootValue := typevalue.WithWitness(reg, typeValues.FromType(reg, rootType), rootType)
	in := state.State{}.WriteValue(reg, key.SymbolValue(symbol.ID(7)), rootValue)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry:         reg,
		TypeValues:       typeValues,
		KeySpace:         ks,
		ProjectPathValue: testSegmentValueProjector(reg, typeValues),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), source, in, nil)
	if !ok {
		t.Fatal("segmented path source did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("segmented path source type = %v/%v, want string", gotType, ok)
	}
}

func TestSourceValuesDynamicIndexReadUsesStaticPathValue(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	point := cfg.Point(21)
	root := symbol.ID(21)
	rootPath := path.NewPath(root, "bindings")
	memberPath := rootPath.Field("checkpoint")
	keySource, ok := NewStringLiteralValueSource("checkpoint", 0, 0, 0, ValueSourceShape{})
	if !ok {
		t.Fatal("NewStringLiteralValueSource returned false")
	}
	dyn, ok := NewDynamicIndexExpression(rootPath, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpression returned false")
	}
	expr := ExprRef(21)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	tableValue := typevalue.WithWitness(reg, typeValues.FromType(reg, typetable.BuiltinTopMarker()), typetable.BuiltinTopMarker())
	visibilityBuilder := visibility.NewBuilder()
	visibilityBuilder.Define(point, root, "bindings")
	resolver := visibility.NewResolver(visibilityBuilder.Build())
	memberKey, ok := visibility.AddressAt(resolver, point, memberPath).VisibleLocalKeyspaceKey()
	if !ok {
		t.Fatal("missing member key")
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(root), product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())).
		WritePathKey(reg, resolver.KeySpace(), resolver.KeySpace().Format(memberKey), tableValue)
	values := NewSourceValues(SourceValuesConfig{
		Registry:          reg,
		TypeValues:        typeValues,
		KeySpace:          resolver.KeySpace(),
		Visibility:        resolver,
		DynamicIndexExprs: map[ExprRef]DynamicIndexExpression{expr: dyn},
	})

	got, ok := values.ValueOfSource(point, source, in, nil)
	if !ok {
		t.Fatal("dynamic index source did not resolve")
	}
	gotType, ok := typeValues.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typetable.BuiltinTopMarker()) {
		t.Fatalf("dynamic index type = %v/%v, want table marker", gotType, ok)
	}
}

func TestSourceValuesLogicalOperationProjectsPathOperandsFromRootTypes(t *testing.T) {
	reg := standard.Registry()
	typeValues := typevalue.NewCache()
	ks := keyspace.New()
	target := symbol.ID(7)
	self := symbol.ID(8)
	targetPath := path.NewPath(target, "target").Field("node_id")
	selfPath := path.NewPath(self, "self").Field("node_id")
	left, ok := NewPathValueSource(targetPath.Key(), 0, 0, 0, ValueSourceShape{})
	if !ok {
		t.Fatalf("left path source")
	}
	right, ok := NewPathValueSource(selfPath.Key(), 0, 0, 0, ValueSourceShape{})
	if !ok {
		t.Fatalf("right path source")
	}
	op, ok := NewBinaryExpressionOperation("or", left, right)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	expr := ExprRef(22)
	targetType := typetable.NewRecord().Field("node_id", typ.MaterializeOptional(typ.String)).Build()
	selfType := typetable.NewRecord().Field("node_id", typ.String).Build()
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(target), typevalue.WithWitness(reg, typeValues.FromType(reg, targetType), targetType)).
		WriteValue(reg, key.SymbolValue(self), typevalue.WithWitness(reg, typeValues.FromType(reg, selfType), selfType))
	resolver := NewSourceValues(SourceValuesConfig{
		Registry:      reg,
		TypeValues:    typeValues,
		KeySpace:      ks,
		ExpressionOps: map[ExprRef]ExpressionOperation{expr: op},
		ExpressionOp: func(op ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			leftType, leftOK := typeValues.TypeOf(reg, left)
			rightType, rightOK := typeValues.TypeOf(reg, right)
			if !leftOK || !typ.TypeEquals(leftType, typ.MaterializeOptional(typ.String)) {
				t.Fatalf("left operand type = %v/%v, want string?", leftType, leftOK)
			}
			if !rightOK || !typ.TypeEquals(rightType, typ.String) {
				t.Fatalf("right operand type = %v/%v, want string", rightType, rightOK)
			}
			return typevalue.WithWitness(reg, typeValues.FromType(reg, typ.String), typ.String), true
		},
		ProjectPathValue: testSegmentValueProjector(reg, typeValues),
	})
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}

	got, ok := resolver.ValueOfSource(cfg.Point(1), source, in, nil)
	if !ok {
		t.Fatal("logical expression source did not resolve")
	}
	gotType, ok := typeValues.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("logical expression type = %v/%v, want string", gotType, ok)
	}
}

func testSegmentProjector(root typ.Type, segments []segment.Segment) (typ.Type, bool) {
	if len(segments) != 1 || segments[0].Kind != segment.SegmentField || segments[0].Name != "node_id" {
		return nil, false
	}
	record, ok := root.(*typ.Record)
	if !ok {
		return nil, false
	}
	field := record.GetField("node_id")
	if field == nil {
		return nil, false
	}
	return field.Type, true
}

func testSegmentValueProjector(reg *axis.Registry, typeValues *typevalue.Cache) func(product.Value, []segment.Segment) (product.Value, bool) {
	return func(root product.Value, segments []segment.Segment) (product.Value, bool) {
		rootType, ok := typeValues.TypeOf(reg, root)
		if !ok {
			return product.Value{}, false
		}
		projected, ok := testSegmentProjector(rootType, segments)
		if !ok {
			return product.Value{}, false
		}
		return typevalue.WithWitness(reg, typeValues.FromType(reg, projected), projected), true
	}
}

func TestSourceValuesLiteralSourcesResolveWitnesses(t *testing.T) {
	reg := standard.Registry()
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	shape := ValueSourceShape{}
	cases := []struct {
		name string
		make func() (ValueSource, bool)
		want typ.Type
	}{
		{name: "bool", make: func() (ValueSource, bool) { return NewBoolLiteralValueSource(false, 0, 0, 0, shape) }, want: typ.False},
		{name: "integer", make: func() (ValueSource, bool) { return NewIntegerLiteralValueSource(42, 0, 0, 0, shape) }, want: typ.LiteralInt(42)},
		{name: "number", make: func() (ValueSource, bool) { return NewNumberLiteralValueSource(3.5, 0, 0, 0, shape) }, want: typ.LiteralNumber(3.5)},
		{name: "string", make: func() (ValueSource, bool) { return NewStringLiteralValueSource("ready", 0, 0, 0, shape) }, want: typ.LiteralString("ready")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, ok := tc.make()
			if !ok {
				t.Fatal("constructor returned false")
			}
			got, ok := resolver.ValueOfSource(cfg.Point(1), source, state.State{}, nil)
			if !ok {
				t.Fatal("literal source did not resolve")
			}
			gotType, ok := typevalue.TypeOf(reg, got)
			if !ok || !typ.TypeEquals(gotType, tc.want) {
				t.Fatalf("literal type = %v/%v, want %v", gotType, ok, tc.want)
			}
		})
	}
}

func TestSourceValuesCallReadsPointOwnedResult(t *testing.T) {
	reg := standard.Registry()
	callPoint := cfg.Point(33)
	source := ValueSource{
		Kind:         ValueSourceCall,
		CallPoint:    callPoint,
		HasCallPoint: true,
		ResultIndex:  1,
	}
	value := presentValue(reg)
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	var readPoint cfg.Point

	got, ok := resolver.ValueOfSource(cfg.Point(3), source, state.State{}, func(point cfg.Point) state.State {
		readPoint = point
		return state.State{}.WriteValue(reg, key.CallResult(uint32(callPoint), 1), value)
	})
	if !ok {
		t.Fatal("call source did not resolve")
	}
	if readPoint != callPoint {
		t.Fatalf("read point = %d, want %d", readPoint, callPoint)
	}
	if !product.Equal(reg, got, value) {
		t.Fatalf("call source value = %s, want %s", formatValue(reg, got), formatValue(reg, value))
	}
}

func TestSourceValuesMissingMetadataAndUnknownSourcesReturnFalse(t *testing.T) {
	reg := standard.Registry()
	resolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	read := func(point cfg.Point) state.State {
		return state.State{}.WriteValue(reg, key.CallResult(uint32(point), 0), presentValue(reg))
	}

	cases := []struct {
		name   string
		source ValueSource
		read   func(cfg.Point) state.State
	}{
		{
			name:   "expression without expr ref",
			source: ValueSource{Kind: ValueSourceExpression},
		},
		{
			name:   "expression missing from table",
			source: ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(100), HasExpr: true},
		},
		{
			name:   "call without call point",
			source: ValueSource{Kind: ValueSourceCall, ResultIndex: 0},
			read:   read,
		},
		{
			name:   "call negative result index",
			source: ValueSource{Kind: ValueSourceCall, HasCallPoint: true, CallPoint: cfg.Point(4), ResultIndex: -1},
			read:   read,
		},
		{
			name:   "call nil read",
			source: ValueSource{Kind: ValueSourceCall, HasCallPoint: true, CallPoint: cfg.Point(4), ResultIndex: 0},
		},
		{
			name:   "unknown",
			source: ValueSource{Kind: ValueSourceUnknown},
		},
		{
			name:   "vararg without provider",
			source: ValueSource{Kind: ValueSourceVararg},
		},
		{
			name: "malformed nil with expression evidence",
			source: ValueSource{
				Kind:        ValueSourceNil,
				ExprRef:     ExprRef(1),
				HasExpr:     true,
				ResultIndex: 0,
				Final:       true,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, ok := resolver.ValueOfSource(cfg.Point(4), tc.source, state.State{}, tc.read); ok {
				t.Fatalf("source resolved to %s, want false", formatValue(reg, got))
			}
		})
	}
}

func TestSourceValuesPathBackedOperationOperandUsesFlowValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6)
	pathExpr := ExprRef(601)
	oneExpr := ExprRef(602)
	sumExpr := ExprRef(603)
	pathSource := ValueSource{Kind: ValueSourceExpression, ExprRef: pathExpr, HasExpr: true}
	oneSource := ValueSource{Kind: ValueSourceExpression, ExprRef: oneExpr, HasExpr: true}
	sumSource := ValueSource{Kind: ValueSourceExpression, ExprRef: sumExpr, HasExpr: true}
	op, ok := NewBinaryExpressionOperation("+", pathSource, oneSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	flowType := typ.LiteralInt(3)
	flowValue := typevalue.WithWitness(reg, typevalue.FromType(reg, flowType), flowType)
	oneType := typ.LiteralInt(1)
	oneValue := typevalue.WithWitness(reg, typevalue.FromType(reg, oneType), oneType)
	want := presentValue(reg)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			pathExpr: cached,
			oneExpr:  oneValue,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			pathExpr: {},
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			sumExpr: op,
		},
		ExpressionValue: func(gotPoint cfg.Point, expr ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if gotPoint != point || expr != pathExpr || source.ExprRef != pathExpr {
				return product.Value{}, false
			}
			return flowValue, true
		},
		ExpressionOp: func(got ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			if floor, ok := typevalue.IntegerLiteralValue(reg, left); !ok || floor != 3 {
				t.Fatalf("left operand = %s/%v, want literal 3 flow value", formatValue(reg, left), ok)
			}
			if floor, ok := typevalue.IntegerLiteralValue(reg, right); !ok || floor != 1 {
				t.Fatalf("right operand = %s/%v, want literal 1", formatValue(reg, right), ok)
			}
			return want, true
		},
	})

	got, ok := resolver.ValueOfSource(point, sumSource, state.State{}, nil)
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("path-backed operation source = %s/%v, want %s/true", formatValue(reg, got), ok, formatValue(reg, want))
	}
}

func TestSourceValuesOperationOperandPrefersObjectLiteralOverCachedRuntimeKind(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(651)
	opExpr := ExprRef(6510)
	tableExpr := ExprRef(6511)
	tableSource := ValueSource{Kind: ValueSourceExpression, ExprRef: tableExpr, HasExpr: true}
	opSource := ValueSource{Kind: ValueSourceExpression, ExprRef: opExpr, HasExpr: true}
	op, ok := NewBinaryExpressionOperation("or", NewNilValueSource(0), tableSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	wantType := typ.NewArray(typ.Any)
	want := typevalue.WithWitness(reg, typevalue.FromType(reg, wantType), wantType)
	cachedRuntimeTable := product.Set(
		reg,
		product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		runtimekind.Key,
		runtimekind.Singleton(runtimekind.Table),
	)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			tableExpr: cachedRuntimeTable,
		},
		ObjectLiteralView: func(expr ExprRef) (ObjectLiteralView, bool) {
			if expr != tableExpr {
				return ObjectLiteralView{}, false
			}
			return NewObjectLiteral(nil).View(), true
		},
		ObjectLiteralFromView: func(ObjectLiteralView, ValueSourceResolver) (product.Value, bool) {
			return want, true
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			opExpr: op,
		},
		ExpressionOp: func(got ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			gotType, ok := typevalue.TypeOf(reg, right)
			if !ok || !typ.TypeEquals(gotType, wantType) {
				t.Fatalf("right operand = %s/%v, want object-literal type %v", formatValue(reg, right), ok, wantType)
			}
			return right, true
		},
	})

	got, ok := resolver.ValueOfSource(point, opSource, state.State{}, nil)
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("operation source = %s/%v, want object literal value %s/true", formatValue(reg, got), ok, formatValue(reg, want))
	}
}

func TestSourceValuesExpressionOperationWinsOverCachedStaticValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(653)
	opExpr := ExprRef(6530)
	leftExpr := ExprRef(6531)
	rightExpr := ExprRef(6532)
	opSource := ValueSource{Kind: ValueSourceExpression, ExprRef: opExpr, HasExpr: true}
	leftSource := ValueSource{Kind: ValueSourceExpression, ExprRef: leftExpr, HasExpr: true}
	rightSource := ValueSource{Kind: ValueSourceExpression, ExprRef: rightExpr, HasExpr: true}
	op, ok := NewBinaryExpressionOperation("or", leftSource, rightSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	cachedNil := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Nil), typ.Nil)
	stringValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	want := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	var calls int
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			opExpr:    cachedNil,
			leftExpr:  cachedNil,
			rightExpr: stringValue,
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			opExpr: op,
		},
		ExpressionOp: func(got ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			calls++
			return want, true
		},
	})

	got, ok := resolver.ValueOfSource(point, opSource, state.State{}, nil)
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("operation source = %s/%v, want operation value %s/true", formatValue(reg, got), ok, formatValue(reg, want))
	}
	if calls != 1 {
		t.Fatalf("expression operation calls = %d, want 1", calls)
	}
}

func TestSourceValuesOperationOperandOperationWinsOverCachedStaticValue(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(654)
	outerExpr := ExprRef(6540)
	innerExpr := ExprRef(6541)
	fallbackExpr := ExprRef(6542)
	outerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: outerExpr, HasExpr: true}
	innerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: innerExpr, HasExpr: true}
	fallbackSource := ValueSource{Kind: ValueSourceExpression, ExprRef: fallbackExpr, HasExpr: true}
	innerOp, ok := NewBinaryExpressionOperation("or", NewNilValueSource(0), fallbackSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(inner) returned false")
	}
	outerOp, ok := NewBinaryExpressionOperation("..", innerSource, fallbackSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(outer) returned false")
	}
	cachedNil := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Nil), typ.Nil)
	stringValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			innerExpr:    cachedNil,
			fallbackExpr: stringValue,
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			innerExpr: innerOp,
			outerExpr: outerOp,
		},
		ExpressionOp: func(got ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			if got.Op() == "or" {
				return stringValue, true
			}
			if got.Op() == ".." {
				leftType, ok := typevalue.TypeOf(reg, left)
				if !ok || !typ.TypeEquals(leftType, typ.String) {
					t.Fatalf("nested operation operand = %s/%v, want string from inner operation", formatValue(reg, left), ok)
				}
				return stringValue, true
			}
			return product.Value{}, false
		},
	})

	got, ok := resolver.ValueOfSource(point, outerSource, state.State{}, nil)
	if !ok {
		t.Fatal("outer operation source did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("outer operation type = %v/%v, want string", gotType, ok)
	}
}

func TestSourceValuesSelfReferentialOperationTerminates(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6541)
	expr := ExprRef(65410)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	op, ok := NewBinaryExpressionOperation("or", source, NewNilValueSource(0))
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}

	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionOps: map[ExprRef]ExpressionOperation{
			expr: op,
		},
		ExpressionOp: func(ExpressionOperation, product.Value, product.Value) (product.Value, bool) {
			t.Fatal("self-referential operation must not reach the evaluator")
			return product.Value{}, false
		},
	})

	if got, ok := resolver.ValueOfSource(point, source, state.State{}, nil); ok {
		t.Fatalf("self-referential operation resolved to %s, want unresolved", formatValue(reg, got))
	}
}

func TestExpressionRefinementsApplyToOperationOperands(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(655)
	leftInner := ExprRef(6550)
	leftOuter := ExprRef(6551)
	rightExpr := ExprRef(6552)
	opExpr := ExprRef(6553)
	leftSource := ValueSource{Kind: ValueSourceExpression, ExprRef: leftOuter, HasExpr: true}
	rightSource := ValueSource{Kind: ValueSourceExpression, ExprRef: rightExpr, HasExpr: true}
	opSource := ValueSource{Kind: ValueSourceExpression, ExprRef: opExpr, HasExpr: true}
	op, ok := NewBinaryExpressionOperation("*", leftSource, rightSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	anyValue := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	anyValue = product.Set(reg, anyValue, evidence.Key, evidence.ExplicitTop())
	anyValue = product.Set(reg, anyValue, assertion.Key, assertion.Any())
	anyValue = typevalue.WithWitness(reg, anyValue, typ.Any)
	numberValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	base := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			leftInner: anyValue,
			rightExpr: numberValue,
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			opExpr: op,
		},
		ExpressionOp: func(got ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			if got.Op() != "*" {
				return product.Value{}, false
			}
			leftClaim := product.Get(reg, left, assertion.Key)
			if !leftClaim.Has(assertion.RuntimeClaim) || leftClaim.Has(assertion.AnyClaim) {
				t.Fatalf("left operand assertion = %s, want runtime proof without stale any", leftClaim)
			}
			leftType, ok := typevalue.WitnessOf(reg, left)
			if !ok || !typ.TypeEquals(leftType, typ.Number) {
				t.Fatalf("left operand type = %v/%v, want number", leftType, ok)
			}
			return numberValue, true
		},
	})
	refinement := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	refinement = product.Set(reg, refinement, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	resolver := WithExpressionRefinements(reg, base, map[ExprRef]ExpressionRefinement{
		leftOuter: NewExpressionRuntimeValidation(
			ValueSource{Kind: ValueSourceExpression, ExprRef: leftInner, HasExpr: true},
			refinement,
		),
	})

	got, ok := resolver.ValueOfSource(point, opSource, state.State{}, nil)
	if !ok {
		t.Fatal("operation source did not resolve")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("operation result type = %v/%v, want number", gotType, ok)
	}
}

func TestExpressionConditionRefinesShortCircuitRightOperandState(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(660)
	guardExpr := ExprRef(6600)
	valueExpr := ExprRef(6601)
	opExpr := ExprRef(6602)
	valueSym := symbol.ID(660)
	valuePath := path.Path{Symbol: valueSym}
	guardSource := ValueSource{Kind: ValueSourceExpression, ExprRef: guardExpr, HasExpr: true}
	valueSource := ValueSource{Kind: ValueSourceExpression, ExprRef: valueExpr, HasExpr: true}
	opSource := ValueSource{Kind: ValueSourceExpression, ExprRef: opExpr, HasExpr: true}
	op, ok := NewBinaryExpressionOperation("and", guardSource, valueSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	anyValue := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	anyValue = product.Set(reg, anyValue, evidence.Key, evidence.ExplicitTop())
	anyValue = product.Set(reg, anyValue, assertion.Key, assertion.Any())
	anyValue = typevalue.WithWitness(reg, anyValue, typ.Any)
	boolValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)
	stringValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			guardExpr: boolValue,
			valueExpr: anyValue,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			valueExpr: {},
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			opExpr: op,
		},
		ExpressionConditions: map[ExprRef]ExpressionCondition{
			guardExpr: NewExpressionCondition(
				[]PostconditionRefinement{
					NewPostconditionRefinement(valuePath, NewValueConstraint(stringValue)),
				},
				nil,
				nil,
				nil,
			),
		},
		ExpressionCondition: func(gotPoint cfg.Point, in state.State, facts ExpressionConditionFacts) state.State {
			if gotPoint != point {
				t.Fatalf("condition point = %d, want %d", gotPoint, point)
			}
			refinements := facts.Refinements()
			if len(refinements) != 1 || !refinements[0].TargetPathRef().Equal(valuePath) {
				t.Fatalf("condition refinements = %#v, want value path refinement", refinements)
			}
			return in.WriteValue(reg, key.SymbolValue(valueSym), stringValue)
		},
		ExpressionValue: func(_ cfg.Point, expr ExprRef, _ ValueSource, in state.State) (product.Value, bool) {
			if expr == valueExpr {
				return in.ReadValue(reg, key.SymbolValue(valueSym)), true
			}
			return product.Value{}, false
		},
		ExpressionOp: func(got ExpressionOperation, _ product.Value, right product.Value) (product.Value, bool) {
			if got.Op() != "and" {
				return product.Value{}, false
			}
			rightType, ok := typevalue.WitnessOf(reg, right)
			if !ok || !typ.TypeEquals(rightType, typ.String) {
				t.Fatalf("right operand type = %v/%v, want string", rightType, ok)
			}
			return right, true
		},
	})
	got, ok := resolver.ValueOfSource(point, opSource, state.State{}.WriteValue(reg, key.SymbolValue(valueSym), anyValue), nil)
	if !ok {
		t.Fatal("operation source did not resolve")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("operation result type = %v/%v, want string", gotType, ok)
	}
}

func TestExpressionConditionForShortCircuitRightOperandSkipsDescendantRefinements(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6606)
	guardExpr := ExprRef(66060)
	valueExpr := ExprRef(66061)
	opExpr := ExprRef(66062)
	valueSym := symbol.ID(6606)
	rootPath := path.Path{Symbol: valueSym}
	descendantPath := rootPath.Field("child")
	guardSource := ValueSource{Kind: ValueSourceExpression, ExprRef: guardExpr, HasExpr: true}
	valueSource := ValueSource{Kind: ValueSourceExpression, ExprRef: valueExpr, HasExpr: true}
	opSource := ValueSource{Kind: ValueSourceExpression, ExprRef: opExpr, HasExpr: true}
	op, ok := NewBinaryExpressionOperation("and", guardSource, valueSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	anyValue := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	anyValue = product.Set(reg, anyValue, evidence.Key, evidence.ExplicitTop())
	anyValue = product.Set(reg, anyValue, assertion.Key, assertion.Any())
	anyValue = typevalue.WithWitness(reg, anyValue, typ.Any)
	boolValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)
	stringValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			guardExpr: boolValue,
			valueExpr: anyValue,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			valueExpr: {},
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			opExpr: op,
		},
		ExpressionConditions: map[ExprRef]ExpressionCondition{
			guardExpr: NewExpressionCondition(
				[]PostconditionRefinement{
					NewPostconditionRefinement(rootPath, NewValueConstraint(stringValue)),
					NewPostconditionRefinement(descendantPath, NewValueConstraint(stringValue)),
				},
				nil,
				nil,
				nil,
			),
		},
		ExpressionCondition: func(_ cfg.Point, in state.State, facts ExpressionConditionFacts) state.State {
			refinements := facts.Refinements()
			if len(refinements) != 1 || !refinements[0].TargetPathRef().Equal(rootPath) {
				t.Fatalf("RHS condition refinements = %#v, want root-only refinement", refinements)
			}
			return in.WriteValue(reg, key.SymbolValue(valueSym), stringValue)
		},
		ExpressionValue: func(_ cfg.Point, expr ExprRef, _ ValueSource, in state.State) (product.Value, bool) {
			if expr == valueExpr {
				return in.ReadValue(reg, key.SymbolValue(valueSym)), true
			}
			return product.Value{}, false
		},
		ExpressionOp: func(got ExpressionOperation, _ product.Value, right product.Value) (product.Value, bool) {
			if got.Op() != "and" {
				return product.Value{}, false
			}
			rightType, ok := typevalue.WitnessOf(reg, right)
			if !ok || !typ.TypeEquals(rightType, typ.String) {
				t.Fatalf("right operand type = %v/%v, want string", rightType, ok)
			}
			return right, true
		},
	})
	got, ok := resolver.ValueOfSource(point, opSource, state.State{}.WriteValue(reg, key.SymbolValue(valueSym), anyValue), nil)
	if !ok {
		t.Fatal("operation source did not resolve")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("operation result type = %v/%v, want string", gotType, ok)
	}
}

func TestFlowExpressionRecoveryPreservesActiveExpressionContext(t *testing.T) {
	reg := standard.Registry()
	outerExpr := ExprRef(66070)
	pathBackedExpr := ExprRef(66071)
	outerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: outerExpr, HasExpr: true}
	pathBackedSource := ValueSource{Kind: ValueSourceExpression, ExprRef: pathBackedExpr, HasExpr: true}
	literalSource, ok := NewIntegerLiteralValueSource(1, 0, 0, 0, ValueSourceShape{Final: true})
	if !ok {
		t.Fatal("NewIntegerLiteralValueSource returned false")
	}
	outerOp, ok := NewBinaryExpressionOperation("+", pathBackedSource, literalSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(outer) returned false")
	}
	pathBackedOp, ok := NewBinaryExpressionOperation("+", outerSource, literalSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(path-backed) returned false")
	}
	cachedTop := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	want := presentValue(reg)
	var evaluated int
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			pathBackedExpr: cachedTop,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			pathBackedExpr: {},
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			outerExpr:      outerOp,
			pathBackedExpr: pathBackedOp,
		},
		ExpressionOp: func(got ExpressionOperation, _ product.Value, _ product.Value) (product.Value, bool) {
			evaluated++
			if got.Left().ExprRef != pathBackedExpr {
				t.Fatalf("evaluated cyclic path-backed op; active expression context was not preserved")
			}
			return want, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(6607), outerSource, state.State{}, nil)
	if !ok {
		t.Fatal("outer operation source did not resolve")
	}
	if evaluated != 1 {
		t.Fatalf("expression operations evaluated %d times, want 1", evaluated)
	}
	if !product.Equal(reg, got, want) {
		t.Fatalf("value = %s, want %s", formatValue(reg, got), formatValue(reg, want))
	}
}

func TestNestedLogicalConditionDerivesFallbackStateWhenInnerRHSIsTruthy(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(661)
	guardExpr := ExprRef(6610)
	memberExpr := ExprRef(6611)
	innerExpr := ExprRef(6612)
	fallbackExpr := ExprRef(6613)
	outerExpr := ExprRef(6614)
	memberSym := symbol.ID(661)
	fallbackSym := symbol.ID(662)
	memberPath := path.Path{Symbol: memberSym}
	fallbackPath := path.Path{Symbol: fallbackSym}
	guardSource := ValueSource{Kind: ValueSourceExpression, ExprRef: guardExpr, HasExpr: true}
	memberSource := ValueSource{Kind: ValueSourceExpression, ExprRef: memberExpr, HasExpr: true}
	innerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: innerExpr, HasExpr: true}
	fallbackSource := ValueSource{Kind: ValueSourceExpression, ExprRef: fallbackExpr, HasExpr: true}
	innerOp, ok := NewBinaryExpressionOperation("and", guardSource, memberSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(inner) returned false")
	}
	outerOp, ok := NewBinaryExpressionOperation("or", innerSource, fallbackSource)
	if !ok {
		t.Fatal("NewBinaryExpressionOperation(outer) returned false")
	}
	anyValue := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	anyValue = product.Set(reg, anyValue, evidence.Key, evidence.ExplicitTop())
	anyValue = product.Set(reg, anyValue, assertion.Key, assertion.Any())
	anyValue = typevalue.WithWitness(reg, anyValue, typ.Any)
	boolValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)
	stringValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			guardExpr:    boolValue,
			memberExpr:   anyValue,
			fallbackExpr: anyValue,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			memberExpr:   {},
			fallbackExpr: {},
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			innerExpr: innerOp,
			outerExpr: outerOp,
		},
		ExpressionConditions: map[ExprRef]ExpressionCondition{
			guardExpr: NewExpressionCondition(
				[]PostconditionRefinement{
					NewPostconditionRefinement(memberPath, NewValueConstraint(stringValue)),
				},
				[]PostconditionRefinement{
					NewPostconditionRefinement(fallbackPath, NewValueConstraint(stringValue)),
				},
				nil,
				nil,
			),
		},
		ExpressionCondition: func(gotPoint cfg.Point, in state.State, facts ExpressionConditionFacts) state.State {
			if gotPoint != point {
				t.Fatalf("condition point = %d, want %d", gotPoint, point)
			}
			out := in
			for _, refinement := range facts.Refinements() {
				switch {
				case refinement.TargetPathRef().Equal(memberPath):
					out = out.WriteValue(reg, key.SymbolValue(memberSym), stringValue)
				case refinement.TargetPathRef().Equal(fallbackPath):
					out = out.WriteValue(reg, key.SymbolValue(fallbackSym), stringValue)
				default:
					t.Fatalf("unexpected refinement target: %#v", refinement.TargetPathRef())
				}
			}
			return out
		},
		ExpressionValue: func(_ cfg.Point, expr ExprRef, _ ValueSource, in state.State) (product.Value, bool) {
			switch expr {
			case memberExpr:
				return in.ReadValue(reg, key.SymbolValue(memberSym)), true
			case fallbackExpr:
				return in.ReadValue(reg, key.SymbolValue(fallbackSym)), true
			default:
				return product.Value{}, false
			}
		},
		ExpressionOp: func(got ExpressionOperation, _ product.Value, right product.Value) (product.Value, bool) {
			rightType, ok := typevalue.WitnessOf(reg, right)
			if !ok || !typ.TypeEquals(rightType, typ.String) {
				t.Fatalf("%s right operand type = %v/%v, want string", got.Op(), rightType, ok)
			}
			return right, true
		},
	})
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: outerExpr, HasExpr: true}
	initial := state.State{}.
		WriteValue(reg, key.SymbolValue(memberSym), anyValue).
		WriteValue(reg, key.SymbolValue(fallbackSym), anyValue)
	got, ok := resolver.ValueOfSource(point, source, initial, nil)
	if !ok {
		t.Fatal("outer operation source did not resolve")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("operation result type = %v/%v, want string", gotType, ok)
	}
}

func TestExpressionRuntimeValidationUsesRefinementWhenOperandDisjoint(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(812)
	innerRef := ExprRef(8120)
	outerRef := ExprRef(8121)
	shape, ok := NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := NewExpressionValueSource(outerRef, 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(outer) returned false")
	}
	innerSource, ok := NewExpressionValueSource(innerRef, 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(inner) returned false")
	}
	base := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			innerRef: typevalue.Nil(reg),
		},
	})
	refinement := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	refinement = product.Set(reg, refinement, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	resolver := WithExpressionRefinements(reg, base, map[ExprRef]ExpressionRefinement{
		outerRef: NewExpressionRuntimeValidation(
			innerSource,
			refinement,
		),
	})

	got, ok := resolver.ValueOfSource(point, source, state.State{}, nil)
	if !ok {
		t.Fatal("runtime validation source did not resolve")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("runtime validation type = %v/%v, want string", gotType, ok)
	}
	if gotClaim := product.Get(reg, got, assertion.Key); !gotClaim.Has(assertion.RuntimeClaim) || !gotClaim.Has(assertion.TypeClaim) {
		t.Fatalf("runtime validation assertion = %s, want type+runtime", gotClaim)
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("runtime validation presence = %s, want present", gotPresence)
	}
}

func TestExpressionRuntimeValidationUsesRecordRefinementWhenOperandScalar(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(813)
	innerRef := ExprRef(8130)
	outerRef := ExprRef(8131)
	shape, ok := NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := NewExpressionValueSource(outerRef, 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(outer) returned false")
	}
	innerSource, ok := NewExpressionValueSource(innerRef, 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(inner) returned false")
	}
	base := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			innerRef: typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number),
		},
	})
	recordType := typetable.NewRecord().Field("name", typ.String).Build()
	refinement := typevalue.WithWitness(reg, typevalue.FromType(reg, recordType), recordType)
	refinement = product.Set(reg, refinement, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	resolver := WithExpressionRefinements(reg, base, map[ExprRef]ExpressionRefinement{
		outerRef: NewExpressionRuntimeValidation(
			innerSource,
			refinement,
		),
	})

	got, ok := resolver.ValueOfSource(point, source, state.State{}, nil)
	if !ok {
		t.Fatal("runtime validation source did not resolve")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, recordType) {
		t.Fatalf("runtime validation type = %v/%v, want %v", gotType, ok, recordType)
	}
	if gotClaim := product.Get(reg, got, assertion.Key); !gotClaim.Has(assertion.RuntimeClaim) || !gotClaim.Has(assertion.TypeClaim) {
		t.Fatalf("runtime validation assertion = %s, want type+runtime", gotClaim)
	}
}

func TestExpressionRuntimeValidationUsesRefinementWhenOperandSourceUnavailable(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(814)
	innerRef := ExprRef(8140)
	outerRef := ExprRef(8141)
	shape, ok := NewValueSourceShape(true, false, false, false)
	if !ok {
		t.Fatal("NewValueSourceShape returned false")
	}
	source, ok := NewExpressionValueSource(outerRef, 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(outer) returned false")
	}
	innerSource, ok := NewExpressionValueSource(innerRef, 0, 0, 0, shape)
	if !ok {
		t.Fatal("NewExpressionValueSource(inner) returned false")
	}
	recordType := typetable.NewRecord().Field("name", typ.String).Build()
	refinement := typevalue.WithWitness(reg, typevalue.FromType(reg, recordType), recordType)
	refinement = product.Set(reg, refinement, assertion.Key, assertion.Of(assertion.TypeClaim, assertion.RuntimeClaim))
	resolver := WithExpressionRefinements(reg, NewSourceValues(SourceValuesConfig{Registry: reg}), map[ExprRef]ExpressionRefinement{
		outerRef: NewExpressionRuntimeValidation(
			innerSource,
			refinement,
		),
	})

	got, ok := resolver.ValueOfSource(point, source, state.State{}, nil)
	if !ok {
		t.Fatal("runtime validation source did not resolve")
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, recordType) {
		t.Fatalf("runtime validation type = %v/%v, want %v", gotType, ok, recordType)
	}
	if gotClaim := product.Get(reg, got, assertion.Key); !gotClaim.Has(assertion.RuntimeClaim) || !gotClaim.Has(assertion.TypeClaim) {
		t.Fatalf("runtime validation assertion = %s, want type+runtime", gotClaim)
	}
}

func TestSourceValuesObjectLiteralPrefersViewOverCachedTopOrigin(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(652)
	litExpr := ExprRef(6520)
	fieldExpr := ExprRef(6521)
	litSource := ValueSource{Kind: ValueSourceExpression, ExprRef: litExpr, HasExpr: true}
	fieldSource := ValueSource{Kind: ValueSourceExpression, ExprRef: fieldExpr, HasExpr: true}
	lit := NewObjectLiteral([]ObjectEntry{
		NewObjectEntryWithMetadata(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "id"}}}, fieldSource, SourceSpan{}, ""),
	})
	cachedTop := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	fieldValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	wantType := typetable.NewRecord().Field("id", typ.String).Build()
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			litExpr:   cachedTop,
			fieldExpr: fieldValue,
		},
		ObjectLiteralView: func(expr ExprRef) (ObjectLiteralView, bool) {
			if expr != litExpr {
				return ObjectLiteralView{}, false
			}
			return lit.View(), true
		},
		ObjectLiteralFromView: func(got ObjectLiteralView, resolver ValueSourceResolver) (product.Value, bool) {
			entryValue, ok := resolver.ResolveValueSource(fieldSource)
			if !ok {
				t.Fatal("object-literal entry source did not resolve")
			}
			entryType, ok := typevalue.TypeOf(reg, entryValue)
			if !ok || !typ.TypeEquals(entryType, typ.String) {
				t.Fatalf("entry type = %v/%v, want string", entryType, ok)
			}
			return typevalue.WithWitness(reg, typevalue.FromType(reg, wantType), wantType), true
		},
	})

	got, ok := resolver.ValueOfSource(point, litSource, state.State{}, nil)
	if !ok {
		t.Fatal("object literal source did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, wantType) {
		t.Fatalf("object literal type = %v/%v, want %v", gotType, ok, wantType)
	}
}

func TestSourceValuesPathBackedExpressionOverlaysFlowPresenceOnCachedType(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(66)
	expr := ExprRef(6601)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	declared := typeexpr.Optional(typ.String)
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, declared), declared)
	flow := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(gotPoint cfg.Point, gotExpr ExprRef, gotSource ValueSource, in state.State) (product.Value, bool) {
			if gotPoint != point || gotExpr != expr || gotSource.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(point, source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present overlay from flow", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("type = %v/%v, want present cached type string", gotType, ok)
	}
}

func TestSourceValuesPathBackedExpressionNarrowsMaybeCachedPresenceByFlow(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6602)
	expr := ExprRef(6602)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	declared := typeexpr.Optional(typ.String)
	cached := product.WithPresence(reg, typevalue.WithWitness(reg, typevalue.FromType(reg, declared), declared), presence.Maybe())
	flow := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(gotPoint cfg.Point, gotExpr ExprRef, gotSource ValueSource, in state.State) (product.Value, bool) {
			if gotPoint != point || gotExpr != expr || gotSource.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(point, source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present from flow proof", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("type = %v/%v, want string", gotType, ok)
	}
}

func TestSourceValuesPathBackedExpressionPrefersFlowOverStaleOperation(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(6603)
	expr := ExprRef(6603)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	literalSource, ok := NewIntegerLiteralValueSource(1, 0, 0, 0, ValueSourceShape{Final: true})
	if !ok {
		t.Fatal("NewIntegerLiteralValueSource returned false")
	}
	op, ok := NewUnaryExpressionOperation("stale-member-read", literalSource)
	if !ok {
		t.Fatal("NewUnaryExpressionOperation returned false")
	}
	record := typetable.NewRecord().
		Field("id", typ.String).
		Field("email", typ.String).
		Build()
	cachedType := typeexpr.Optional(record)
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, cachedType), cachedType)
	flow := typevalue.WithWitness(reg, typevalue.FromType(reg, record), record)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionOps: map[ExprRef]ExpressionOperation{
			expr: op,
		},
		ExpressionValue: func(gotPoint cfg.Point, gotExpr ExprRef, gotSource ValueSource, in state.State) (product.Value, bool) {
			if gotPoint != point || gotExpr != expr || gotSource.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
		ExpressionOp: func(got ExpressionOperation, left product.Value, right product.Value) (product.Value, bool) {
			return cached, true
		},
	})

	got, ok := resolver.ValueOfSource(point, source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, record) {
		t.Fatalf("path-backed type = %v/%v, want flow type %v", gotType, ok, record)
	}
}

func TestSourceValuesExpressionAndVarargProvidersAreGenericHooks(t *testing.T) {
	reg := standard.Registry()
	exprValue := presentValue(reg)
	varargValue := absentValue(reg)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValue: func(point cfg.Point, expr ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(5) || expr != ExprRef(55) || source.Kind != ValueSourceExpression {
				return product.Value{}, false
			}
			return exprValue, true
		},
		VarargValue: func(point cfg.Point, source ValueSource, in state.State, read func(cfg.Point) state.State) (product.Value, bool) {
			if point != cfg.Point(5) || source.Kind != ValueSourceVararg {
				return product.Value{}, false
			}
			return varargValue, true
		},
	})

	gotExpr, ok := resolver.ValueOfSource(cfg.Point(5), ValueSource{
		Kind:    ValueSourceExpression,
		ExprRef: ExprRef(55),
		HasExpr: true,
	}, state.State{}, nil)
	if !ok || !product.Equal(reg, gotExpr, exprValue) {
		t.Fatalf("expression provider = %s/%v, want %s/true", formatValue(reg, gotExpr), ok, formatValue(reg, exprValue))
	}

	gotVararg, ok := resolver.ValueOfSource(cfg.Point(5), ValueSource{Kind: ValueSourceVararg}, state.State{}, nil)
	if !ok || !product.Equal(reg, gotVararg, varargValue) {
		t.Fatalf("vararg provider = %s/%v, want %s/true", formatValue(reg, gotVararg), ok, formatValue(reg, varargValue))
	}
}

func TestSourceValuesProviderRecoversFromNonAssertedCachedTop(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(56)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	cached := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	recovered := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(56) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return recovered, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(56), source, state.State{}, nil)
	if !ok {
		t.Fatal("expression provider did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("recovered type = %v/%v, want string", gotType, ok)
	}
}

func TestSourceValuesPathBackedAnyPrefersFlowTableWitness(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(5601)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	cached := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	flow := typevalue.WithWitness(reg, typevalue.FromType(reg, typetable.BuiltinTopMarker()), typetable.BuiltinTopMarker())
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(5601) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(5601), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typetable.BuiltinTopMarker()) {
		t.Fatalf("path-backed flow type = %v/%v, want builtin table marker", gotType, ok)
	}
}

func TestSourceValuesPathBackedAnyPrefersFlowRuntimeKindEvidence(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(5602)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	cached := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	flow := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	flow = product.Set(reg, flow, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(5602) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(5602), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	if gotKinds := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKinds, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("path-backed flow runtime kind = %s, want table", gotKinds)
	}
}

func TestSourceValuesPathBackedRuntimeKindDoesNotReplacePreciseCachedType(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(5603)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	flow := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	flow = product.Set(reg, flow, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(5603) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(5603), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("path-backed cached type = %v/%v, want string", gotType, ok)
	}
}

func TestSourceValuesPathBackedAnyArrayKeepsCachedContractOverFlowTableWitness(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(56030)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	cachedType := typ.NewArray(typ.Any)
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, cachedType), cachedType)
	recordType := typetable.NewRecord().Field("id", typ.String).Build()
	flowType := typ.NewArray(recordType)
	flow := typevalue.WithWitness(reg, typevalue.FromType(reg, flowType), flowType)
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(56030) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(56030), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, cachedType) {
		t.Fatalf("path-backed cached type = %v/%v, want declared %v instead of flow %v", gotType, ok, cachedType, flowType)
	}
}

func TestSourceValuesPathBackedTableTypeKeepsFlowIdentity(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(56031)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	recordType := typetable.NewRecord().Field("id", typ.String).Build()
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, recordType), recordType)
	wantID := identity.ID{Kind: "lua.table", Site: "source-value", Index: 1}
	flow := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(wantID))
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(56031) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(56031), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, recordType) {
		t.Fatalf("path-backed cached type = %v/%v, want %v", gotType, ok, recordType)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); !ok || gotID != wantID {
		t.Fatalf("path-backed identity = %v/%v, want %v", gotID, ok, wantID)
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present", gotPresence)
	}
}

func TestSourceValuesPathBackedNonTableTypeRejectsFlowIdentity(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(56032)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	flow := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), identity.Key, identity.Singleton(identity.ID{Kind: "lua.table", Site: "source-value", Index: 2}))
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(56032) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(56032), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("path-backed cached type = %v/%v, want string", gotType, ok)
	}
	if gotID, ok := product.Get(reg, got, identity.Key).ID(); ok {
		t.Fatalf("path-backed identity = %v, want none", gotID)
	}
}

func TestSourceValuesPathBackedConcreteTypeRejectsExplicitTopFlow(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(56033)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	flow := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	flow = product.Set(reg, flow, evidence.Key, evidence.ExplicitTop())
	flow = product.Set(reg, flow, assertion.Key, assertion.Any())
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(56033) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(56033), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("path-backed cached type = %v/%v, want number", gotType, ok)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); gotEvidence.IsExplicitTop() {
		t.Fatalf("path-backed evidence = %s, want cached concrete evidence", gotEvidence)
	}
}

func TestSourceValuesPathBackedRuntimeKindNarrowsCachedUnionType(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(5604)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	mapType := typetable.NewMap(typ.String, typ.String)
	declared := typeexpr.Union(typ.Nil, typ.String, mapType)
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, declared), declared)
	flow := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	flow = product.Set(reg, flow, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(5604) || got != expr || source.ExprRef != expr {
				return product.Value{}, false
			}
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(5604), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present runtime-kind proof", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, mapType) {
		t.Fatalf("path-backed narrowed type = %v/%v, want %v", gotType, ok, mapType)
	}
}

func TestSourceValuesPreservesExplicitAnyRefinementOverRecoveredProviderValue(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(57)
	outer := ExprRef(58)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}
	cached := product.Set(reg, product.Top(), evidence.Key, evidence.ExplicitTop())
	cached = product.Set(reg, cached, assertion.Key, assertion.Any())
	recovered := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	base := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: cached,
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			if point != cfg.Point(57) || got != inner || source.ExprRef != inner {
				return product.Value{}, false
			}
			return recovered, true
		},
	})
	resolver := WithExpressionRefinements(reg, base, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			product.Set(reg, typevalue.FromType(reg, typ.Any), assertion.Key, assertion.Any()),
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(57), source, state.State{}, nil)
	if !ok {
		t.Fatal("expression source did not resolve")
	}
	if claims := product.Get(reg, got, assertion.Key); !claims.Has(assertion.AnyClaim) {
		t.Fatalf("assertion = %s, want explicit any claim preserved", claims)
	}
	if ev := product.Get(reg, got, evidence.Key); !ev.IsExplicitTop() {
		t.Fatalf("evidence = %s, want explicit top preserved", ev)
	}
}

func TestExpressionRefinementSourceValuesAnyClaimPreservesPresentInnerValue(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(5901)
	outer := ExprRef(5902)
	innerValue := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	refinement := product.Set(reg, typevalue.FromType(reg, typ.Any), assertion.Key, assertion.Any())
	base := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: innerValue,
		},
	})
	resolver := WithExpressionRefinements(reg, base, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			refinement,
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(59), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("any-claim source did not resolve")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want explicit any claim to preserve present inner value", gotPresence)
	}
	if gotAssertion := product.Get(reg, got, assertion.Key); !gotAssertion.Has(assertion.AnyClaim) {
		t.Fatalf("assertion = %s, want any claim", gotAssertion)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !gotEvidence.IsExplicitTop() {
		t.Fatalf("evidence = %s, want explicit top", gotEvidence)
	}
}

func TestSourceValuesDynamicIndexExpressionProjectsCallResultTableSource(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(61)
	callPoint := cfg.Point(60)
	tableExpr := ExprRef(6010)
	keyExpr := ExprRef(6011)
	indexExpr := ExprRef(6012)
	tableSource := ValueSource{Kind: ValueSourceCall, ExprRef: tableExpr, HasExpr: true, CallPoint: callPoint, HasCallPoint: true, ResultIndex: 0}
	keySource := ValueSource{Kind: ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	indexSource := ValueSource{Kind: ValueSourceExpression, ExprRef: indexExpr, HasExpr: true}
	dyn, ok := NewDynamicIndexExpressionFromSource(tableSource, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpressionFromSource returned false")
	}
	memberType := typetable.NewRecord().Field("id", typ.String).Build()
	tableType := typetable.NewRecord().StaticStringIndex("root", memberType).Build()
	keyType := typ.LiteralString("root")
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			keyExpr: typevalue.WithWitness(reg, typevalue.FromType(reg, keyType), keyType),
		},
		DynamicIndexExprs: map[ExprRef]DynamicIndexExpression{
			indexExpr: dyn,
		},
	})
	read := func(got cfg.Point) state.State {
		if got != callPoint {
			return state.State{}
		}
		return state.State{}.WriteValue(reg, key.CallResult(uint32(callPoint), 0), typevalue.WithWitness(reg, typevalue.FromType(reg, tableType), tableType))
	}

	got, ok := resolver.ValueOfSource(point, indexSource, state.State{}, read)
	if !ok {
		t.Fatal("dynamic index source did not resolve")
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, memberType) {
		t.Fatalf("dynamic index type = %v/%v, want %v", gotType, ok, memberType)
	}
}

func TestSourceValuesDynamicIndexExpressionGivesExpressionProviderFirstRefusal(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(62)
	tableExpr := ExprRef(6210)
	keyExpr := ExprRef(6211)
	indexExpr := ExprRef(6212)
	tableSource := ValueSource{Kind: ValueSourceExpression, ExprRef: tableExpr, HasExpr: true}
	keySource := ValueSource{Kind: ValueSourceExpression, ExprRef: keyExpr, HasExpr: true}
	indexSource := ValueSource{Kind: ValueSourceExpression, ExprRef: indexExpr, HasExpr: true}
	dyn, ok := NewDynamicIndexExpressionFromSource(tableSource, keySource)
	if !ok {
		t.Fatal("NewDynamicIndexExpressionFromSource returned false")
	}
	tableType := typ.NewMap(typ.String, typ.Number)
	provenValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	var providerCalls int
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			tableExpr: typevalue.WithWitness(reg, typevalue.FromType(reg, tableType), tableType),
			keyExpr:   typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String),
		},
		DynamicIndexExprs: map[ExprRef]DynamicIndexExpression{
			indexExpr: dyn,
		},
		ExpressionValue: func(gotPoint cfg.Point, gotExpr ExprRef, gotSource ValueSource, in state.State) (product.Value, bool) {
			if gotPoint != point || gotExpr != indexExpr || gotSource.ExprRef != indexExpr {
				return product.Value{}, false
			}
			providerCalls++
			return provenValue, true
		},
	})

	got, ok := resolver.ValueOfSource(point, indexSource, state.State{}, nil)
	if !ok {
		t.Fatal("dynamic index source did not resolve")
	}
	if providerCalls != 1 {
		t.Fatalf("expression provider calls = %d, want one dynamic-index first-refusal call", providerCalls)
	}
	if !product.Equal(reg, got, provenValue) {
		t.Fatalf("dynamic index value = %s, want proof-aware provider value %s", formatValue(reg, got), formatValue(reg, provenValue))
	}
}

func TestSourceValuesObjectLiteralViewResolver(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(61)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	want := presentValue(reg)
	nilSource := NewNilValueSource(0)
	lit := NewObjectLiteral([]ObjectEntry{
		NewObjectEntryWithMetadata(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}}, nilSource, SourceSpan{}, ""),
	})
	viewCalls := 0
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ObjectLiteralView: func(got ExprRef) (ObjectLiteralView, bool) {
			if got != expr {
				return ObjectLiteralView{}, false
			}
			return lit.View(), true
		},
		ObjectLiteralFromView: func(got ObjectLiteralView, resolver ValueSourceResolver) (product.Value, bool) {
			viewCalls++
			if got.EntryCount() != 1 {
				t.Fatalf("view entry count = %d, want 1", got.EntryCount())
			}
			value, ok := resolver.ResolveValueSource(nilSource)
			if !ok {
				t.Fatal("view evaluator did not resolve entry source")
			}
			if gotPresence := product.PresenceOf(value); !presence.Equal(gotPresence, presence.Absent()) {
				t.Fatalf("resolved nil presence = %s, want absent", gotPresence)
			}
			return want, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), source, state.State{}, nil)
	if !ok || !product.Equal(reg, got, want) {
		t.Fatalf("object literal view value = %s/%v, want %s/true", formatValue(reg, got), ok, formatValue(reg, want))
	}
	if viewCalls != 1 {
		t.Fatalf("view calls = %d, want one view resolution", viewCalls)
	}
}

func TestExpressionRefinementsApplyInsideObjectLiteralViewResolver(t *testing.T) {
	reg := standard.Registry()
	root := ExprRef(62)
	inner := ExprRef(63)
	outer := ExprRef(64)
	rootSource := ValueSource{Kind: ValueSourceExpression, ExprRef: root, HasExpr: true}
	innerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true}
	outerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}
	lit := NewObjectLiteral([]ObjectEntry{
		NewObjectEntryWithMetadata(path.Path{Segments: []segment.Segment{{Kind: segment.SegmentField, Name: "value"}}}, outerSource, SourceSpan{}, ""),
	})
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: presentValue(reg),
		},
		ObjectLiteralView: func(got ExprRef) (ObjectLiteralView, bool) {
			if got != root {
				return ObjectLiteralView{}, false
			}
			return lit.View(), true
		},
		ObjectLiteralFromView: func(got ObjectLiteralView, resolver ValueSourceResolver) (product.Value, bool) {
			var resolved bool
			got.ForEachEntry(func(entry ObjectEntryView) bool {
				value, ok := resolver.ResolveValueSource(entry.Source())
				if !ok {
					t.Fatal("object literal entry source did not resolve through expression refinement")
				}
				kind := product.Get(reg, value, runtimekind.Key)
				if !runtimekind.Equal(kind, runtimekind.Singleton(runtimekind.Table)) {
					t.Fatalf("entry runtime kind = %s, want table refinement", kind)
				}
				resolved = true
				return true
			})
			if !resolved {
				t.Fatal("object literal entry was not visited")
			}
			return presentValue(reg), true
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(innerSource, runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Table))),
	})

	if _, ok := resolver.ValueOfSource(cfg.Point(1), rootSource, state.State{}, nil); !ok {
		t.Fatal("object literal root source did not resolve")
	}
}

func TestExpressionRefinementOnObjectLiteralPreservesLiteralIdentity(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(65)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	litID := identity.ID{Kind: "test.table", Site: "literal", Index: 1}
	litValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(litID))
	refinement := runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Table))
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ObjectLiteralView: func(got ExprRef) (ObjectLiteralView, bool) {
			if got != expr {
				return ObjectLiteralView{}, false
			}
			return NewObjectLiteral(nil).View(), true
		},
		ObjectLiteralFromView: func(ObjectLiteralView, ValueSourceResolver) (product.Value, bool) {
			return litValue, true
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		expr: NewExpressionRefinement(source, refinement),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), source, state.State{}, nil)
	if !ok {
		t.Fatal("refined object literal source did not resolve")
	}
	if gotID := product.Get(reg, got, identity.Key); !identity.Equal(gotID, identity.Singleton(litID)) {
		t.Fatalf("identity = %s, want literal identity", gotID)
	}
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("runtimekind = %s, want table refinement", gotKind)
	}
}

func TestSourceValuesPathBackedExpressionPrefersVariantOriginFlow(t *testing.T) {
	reg := standard.Registry()
	expr := ExprRef(77)
	source := ValueSource{Kind: ValueSourceExpression, ExprRef: expr, HasExpr: true}
	text := typetable.NewRecord().
		Field("kind", typ.LiteralString("text")).
		Field("value", typ.String).
		Build()
	group := typetable.NewRecord().
		Field("kind", typ.LiteralString("group")).
		Field("children", typ.NewArray(typ.Unknown)).
		Build()
	union := typeexpr.Union(text, group)
	family, cases, ok := variant.OriginByPathLiteral(union, []segment.Segment{{Kind: segment.SegmentField, Name: "kind"}}, typ.LiteralString("text"))
	if !ok {
		t.Fatal("test union did not expose text origin")
	}
	cached := typevalue.WithWitness(reg, typevalue.FromType(reg, union), union)
	flow := product.Set(reg, product.Top(), variantorigin.Key, variantorigin.Of(family, cases))
	resolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			expr: cached,
		},
		ExpressionPaths: map[ExprRef]struct{}{
			expr: {},
		},
		ExpressionValue: func(point cfg.Point, got ExprRef, source ValueSource, in state.State) (product.Value, bool) {
			return flow, true
		},
	})

	got, ok := resolver.ValueOfSource(cfg.Point(9), source, state.State{}, nil)
	if !ok {
		t.Fatal("path-backed expression did not resolve")
	}
	gotOrigin := product.Get(reg, got, variantorigin.Key)
	if gotOrigin.IsTop() || gotOrigin.IsBottom() || gotOrigin.Family() != family {
		t.Fatalf("origin = %#v, want narrowed family %d", gotOrigin, family)
	}
}

func TestExpressionRefinementSourceValuesMeetsRefinementAndDoesNotMutateBase(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(10)
	outer := ExprRef(11)
	innerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true}
	outerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}
	base := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	refinement := runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Table))
	refinements := map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(innerSource, refinement),
	}
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: base,
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, refinements)

	got, ok := resolver.ValueOfSource(cfg.Point(1), outerSource, state.State{}, nil)
	if !ok {
		t.Fatal("expression refinement source did not resolve through inner expression")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want original present", gotPresence)
	}
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("runtimekind = %s, want table", gotKind)
	}
	if baseKind := product.Get(reg, base, runtimekind.Key); !runtimekind.Equal(baseKind, runtimekind.Top()) {
		t.Fatalf("base expression value mutated with runtime kind = %s", baseKind)
	}
}

func TestExpressionRefinementsOwnInputAndBindRepeatedly(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(40)
	outer := ExprRef(41)
	replacement := ExprRef(42)
	innerSource := ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true}
	replacementSource := ValueSource{Kind: ValueSourceExpression, ExprRef: replacement, HasExpr: true}
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner:       presentValue(reg),
			replacement: presentValue(reg),
		},
	})
	input := map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(innerSource, runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Table))),
	}
	owned := NewExpressionRefinements(input)

	input[outer] = NewExpressionRefinement(replacementSource, runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Function)))

	for i := 0; i < 2; i++ {
		resolver := owned.Bind(reg, baseResolver)
		got, ok := resolver.ValueOfSource(cfg.Point(1), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
		if !ok {
			t.Fatalf("bind %d did not resolve owned refinement", i)
		}
		if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Table)) {
			t.Fatalf("bind %d runtime kind = %s, want original table refinement", i, gotKind)
		}
	}
}

func TestExpressionRefinementSourceValuesAppliesRefinementToCallSource(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(12)
	outer := ExprRef(13)
	callPoint := cfg.Point(44)
	callValue := presentValue(reg)
	outerSource := ValueSource{
		Kind:         ValueSourceCall,
		ExprRef:      outer,
		HasExpr:      true,
		CallPoint:    callPoint,
		HasCallPoint: true,
		ResultIndex:  0,
	}
	innerSource := outerSource
	innerSource.ExprRef = inner
	baseResolver := NewSourceValues(SourceValuesConfig{Registry: reg})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(innerSource, runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Function))),
	})

	var readPoint cfg.Point
	got, ok := resolver.ValueOfSource(cfg.Point(1), outerSource, state.State{}, func(point cfg.Point) state.State {
		readPoint = point
		return state.State{}.WriteValue(reg, key.CallResult(uint32(callPoint), 0), callValue)
	})
	if !ok {
		t.Fatal("asserted call source did not resolve")
	}
	if readPoint != callPoint {
		t.Fatalf("read point = %d, want call point %d", readPoint, callPoint)
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("overlaid call presence = %s, want original present", gotPresence)
	}
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Function)) {
		t.Fatalf("overlaid call runtime kind = %s, want function", gotKind)
	}
	if baseKind := product.Get(reg, callValue, runtimekind.Key); !runtimekind.Equal(baseKind, runtimekind.Top()) {
		t.Fatalf("call return value mutated with runtime kind = %s", baseKind)
	}
}

func TestExpressionRefinementSourceValuesPanicsWithoutRegistry(t *testing.T) {
	reg := standard.Registry()
	base := NewSourceValues(SourceValuesConfig{Registry: reg})

	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "expression refinement source values require a registry") {
			t.Fatal("WithExpressionRefinements did not panic")
		}
	}()

	_ = WithExpressionRefinements(nil, base, map[ExprRef]ExpressionRefinement{
		ExprRef(1): NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(2), HasExpr: true},
			runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Table)),
		),
	})
}

func TestExpressionRefinementSourceValuesCanMeetCorePresenceRefinement(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(14)
	outer := ExprRef(15)
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: absentValue(reg),
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			product.NewWithPresence(reg, product.ShapeTop, presence.Absent()),
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("presence refinement source did not resolve")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Absent()) {
		t.Fatalf("presence refinement = %s, want absent", gotPresence)
	}
}

func TestExpressionRefinementSourceValuesNonNilClaimMarksOptionalWitnessPresent(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(18)
	outer := ExprRef(19)
	optionalString := typeexpr.Optional(typ.String)
	base := typevalue.WithWitness(reg, typevalue.FromType(reg, optionalString), optionalString)
	refinement := product.Set(reg, product.Top(), assertion.Key, assertion.NonNil())
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: base,
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			refinement,
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("non-nil refinement source did not resolve")
	}
	if gotPresence := product.PresenceOf(got); !presence.Equal(gotPresence, presence.Present()) {
		t.Fatalf("presence = %s, want present", gotPresence)
	}
	gotType, ok := typevalue.TypeOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("type = %v/%v, want string after non-nil projection", gotType, ok)
	}
}

func TestExpressionRefinementSourceValuesAppliesDeclaredContractWithoutErasingAnyEvidence(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(16)
	outer := ExprRef(17)
	base := product.Set(reg, product.NewWithPresence(reg, product.ShapeTop, presence.Present()), assertion.Key, assertion.Any())
	base = product.Set(reg, base, evidence.Key, evidence.ExplicitTop())
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Number), typ.Number)
	declared = product.Set(reg, declared, assertion.Key, assertion.Type())
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: base,
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionDeclaredContract(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			declared,
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("declared-contract refinement source did not resolve")
	}
	if gotAssertion := product.Get(reg, got, assertion.Key); !assertion.Equal(gotAssertion, assertion.Of(assertion.TypeClaim, assertion.AnyClaim)) {
		t.Fatalf("assertion = %s, want type+any", gotAssertion)
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.ExplicitTop()) {
		t.Fatalf("evidence = %s, want explicit-top from inner any", gotEvidence)
	}
	gotWitness := product.Get(reg, got, typewitness.Key)
	gotType, ok := gotWitness.Type()
	if !ok || !typ.TypeEquals(gotType, typ.Number) {
		t.Fatalf("type witness = %v/%v, want number", gotWitness, ok)
	}
}

func TestExpressionRefinementDeclaredScalarContractClearsStaleAnyEvidenceAfterRuntimeProof(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(18)
	outer := ExprRef(19)
	base := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	base = product.Set(reg, base, runtimekind.Key, runtimekind.Singleton(runtimekind.String))
	base = product.Set(reg, base, evidence.Key, evidence.ExplicitTop())
	declared := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.String), typ.String)
	declared = product.Set(reg, declared, assertion.Key, assertion.Type())
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: base,
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		outer: NewExpressionDeclaredContract(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			declared,
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(1), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("declared-contract refinement source did not resolve")
	}
	if gotEvidence := product.Get(reg, got, evidence.Key); !evidence.Equal(gotEvidence, evidence.Top()) {
		t.Fatalf("evidence = %s, want trusted top after scalar runtime proof satisfies declared contract", gotEvidence)
	}
	gotType, ok := typevalue.WitnessOf(reg, got)
	if !ok || !typ.TypeEquals(gotType, typ.String) {
		t.Fatalf("type witness = %v/%v, want string", gotType, ok)
	}
}

func TestExpressionRefinementSourceValuesNestedRefinementsMeet(t *testing.T) {
	reg := standard.Registry()
	inner := ExprRef(20)
	middle := ExprRef(21)
	outer := ExprRef(22)
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
		ExpressionValues: map[ExprRef]product.Value{
			inner: presentValue(reg),
		},
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		middle: NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: inner, HasExpr: true},
			runtimeKindRefinement(reg, runtimekind.Top().Without(runtimekind.Nil)),
		),
		outer: NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: middle, HasExpr: true},
			runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Table)),
		),
	})

	got, ok := resolver.ValueOfSource(cfg.Point(2), ValueSource{Kind: ValueSourceExpression, ExprRef: outer, HasExpr: true}, state.State{}, nil)
	if !ok {
		t.Fatal("nested refinement source did not resolve")
	}
	if gotKind := product.Get(reg, got, runtimekind.Key); !runtimekind.Equal(gotKind, runtimekind.Singleton(runtimekind.Table)) {
		t.Fatalf("nested runtime kind = %s, want table", gotKind)
	}
}

func TestExpressionRefinementSourceValuesMissingInnerSourceReturnsFalse(t *testing.T) {
	reg := standard.Registry()
	baseResolver := NewSourceValues(SourceValuesConfig{
		Registry: reg,
	})
	resolver := WithExpressionRefinements(reg, baseResolver, map[ExprRef]ExpressionRefinement{
		ExprRef(30): NewExpressionRefinement(
			ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(31), HasExpr: true},
			runtimeKindRefinement(reg, runtimekind.Singleton(runtimekind.Table)),
		),
	})

	if got, ok := resolver.ValueOfSource(cfg.Point(3), ValueSource{Kind: ValueSourceExpression, ExprRef: ExprRef(30), HasExpr: true}, state.State{}, nil); ok {
		t.Fatalf("missing refinement inner source resolved to %s, want false", formatValue(reg, got))
	}
}

func TestNumFloorForSourceDerivesExactIntegerPathAndBinaryFloors(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(91)
	exactExpr := ExprRef(910)
	pathExpr := ExprRef(911)
	sumExpr := ExprRef(912)
	missingExpr := ExprRef(913)
	oneExpr := ExprRef(914)
	pathSymbol := symbol.ID(991)
	pathValue := path.NewPath(pathSymbol, "i")
	pathKey := pathValue.Key()
	exactSource := ValueSource{Kind: ValueSourceExpression, ExprRef: exactExpr, HasExpr: true}
	pathSource := ValueSource{Kind: ValueSourceExpression, ExprRef: pathExpr, HasExpr: true}
	directPathSource := ValueSource{Kind: ValueSourcePath, PathKey: pathKey}
	sumSource := ValueSource{Kind: ValueSourceExpression, ExprRef: sumExpr, HasExpr: true}
	missingSource := ValueSource{Kind: ValueSourceExpression, ExprRef: missingExpr, HasExpr: true}
	literalSource, ok := NewIntegerLiteralValueSource(11, 0, 0, 0, ValueSourceShape{Final: true})
	if !ok {
		t.Fatal("NewIntegerLiteralValueSource returned false")
	}
	oneType := typ.LiteralInt(1)
	oneValue := typevalue.WithWitness(reg, typevalue.FromType(reg, oneType), oneType)
	exactType := typ.LiteralInt(7)
	exactValue := typevalue.WithWitness(reg, typevalue.FromType(reg, exactType), exactType)
	resolver := fixedPathKeyResolver{key: pathKey, ks: keyspace.New()}
	sumOp, ok := NewBinaryExpressionOperation("+", pathSource, ValueSource{Kind: ValueSourceExpression, ExprRef: oneExpr, HasExpr: true})
	if !ok {
		t.Fatal("NewBinaryExpressionOperation returned false")
	}
	facts := NewFacts(FactsInput{
		ExpressionValues: map[ExprRef]product.Value{
			exactExpr: exactValue,
			oneExpr:   oneValue,
		},
		ExpressionPaths: map[ExprRef]path.Path{
			pathExpr: pathValue,
		},
		ExpressionOperations: map[ExprRef]ExpressionOperation{
			sumExpr: sumOp,
		},
	})
	stateKey, stateKeyOK := pathaddr.StateKeyFromPathKey(pathKey)
	if !stateKeyOK {
		t.Fatal("StateKeyFromPathKey(pathKey) failed")
	}
	in := state.State{}.WriteNumFloor(resolver.ks, stateKey, 3)

	tests := []struct {
		name   string
		source ValueSource
		want   int64
		ok     bool
	}{
		{name: "exact integer", source: exactSource, want: 7, ok: true},
		{name: "literal integer source", source: literalSource, want: 11, ok: true},
		{name: "path floor", source: pathSource, want: 3, ok: true},
		{name: "direct path floor", source: directPathSource, want: 3, ok: true},
		{name: "binary plus constant", source: sumSource, want: 4, ok: true},
		{name: "missing unresolved", source: missingSource, ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NumFloorForSource(reg, resolver, point, facts, in, tc.source)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("floor = %d, want %d", got, tc.want)
			}
		})
	}
}

type fixedPathKeyResolver struct {
	key path.PathKey
	ks  *keyspace.KeySpace
}

func (r fixedPathKeyResolver) KeyAt(point cfg.Point, p path.Path) path.PathKey {
	return r.key
}

func (r fixedPathKeyResolver) KeySpace() *keyspace.KeySpace {
	return r.ks
}

func runtimeKindRefinement(reg *axis.Registry, value runtimekind.Value) product.Value {
	return product.Set(reg, product.Top(), runtimekind.Key, value)
}

func assertStateEqual(t *testing.T, reg *axis.Registry, got state.State, want state.State) {
	t.Helper()
	if !state.Domain(reg).Equal(got, want) {
		t.Fatalf("state changed")
	}
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	default:
		return product.PresenceOf(v).String()
	}
}
