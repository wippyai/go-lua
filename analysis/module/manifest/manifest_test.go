package manifest

import (
	"bytes"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/control"
	"github.com/wippyai/go-lua/analysis/domain/effect/dispatch"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
	"github.com/wippyai/go-lua/analysis/domain/effect/ownership"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	"github.com/wippyai/go-lua/analysis/type/annotation"
	"github.com/wippyai/go-lua/analysis/type/projection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestManifestDefineTypeAndSetExport(t *testing.T) {
	m := New("example/module")
	user := typetable.NewRecord().
		Field("id", typ.Integer).
		OptField("name", typ.String).
		StaticStringIndex("role", typ.LiteralString("admin")).
		MapComponent(typ.String, typ.Number).
		Build()
	export := typ.Func().
		Param("user", user).
		Returns(typ.NewArray(user)).
		Build()

	m.DefineType("User", user)
	m.SetExport(export)

	if m.Path != "example/module" {
		t.Fatalf("path = %q", m.Path)
	}
	if got := m.Types["User"]; !typ.TypeEquals(got, user) {
		t.Fatalf("User type = %v, want %v", got, user)
	}
	if !typ.TypeEquals(m.Export, export) {
		t.Fatalf("export = %v, want %v", m.Export, export)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := New("example/module")
	m.Version = "v1"
	m.DefineType("Status", typ.NewUnion(
		typ.LiteralString("ready"),
		typ.LiteralString("pending"),
	))
	m.DefineType("User", typ.NewAnnotated(
		typetable.NewRecord().
			ReadonlyField("id", typ.Integer).
			OptField("name", typ.String).
			Build(),
		[]annotation.Annotation{{Name: "sealed", Arg: true}},
	))
	m.SetExport(typ.Func().
		TypeParam("T", typ.NewRef("", "User")).
		Param("value", typ.NewTypeParam("T", typ.NewRef("", "User"))).
		Returns(typ.NewReadonlyMap(typ.String, typ.NewRef("", "Status"))).
		Build())

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if got.Path != m.Path || got.Version != m.Version {
		t.Fatalf("metadata = %q/%q, want %q/%q", got.Path, got.Version, m.Path, m.Version)
	}
	if len(got.Types) != len(m.Types) {
		t.Fatalf("types = %d, want %d", len(got.Types), len(m.Types))
	}
	for name, want := range m.Types {
		if !typ.TypeEquals(got.Types[name], want) {
			t.Fatalf("type %s = %v, want %v", name, got.Types[name], want)
		}
	}
	if !typ.TypeEquals(got.Export, m.Export) {
		t.Fatalf("export = %v, want %v", got.Export, m.Export)
	}
}

func TestManifestRoundTripNormalizesMapKeysOnDecode(t *testing.T) {
	mapKey := typ.NewUnion(typ.String, typ.Nil)

	m := New("example/keys")
	m.DefineType("Writable", typ.NewMap(mapKey, typ.Number))
	m.DefineType("Readonly", typ.NewReadonlyMap(mapKey, typ.Number))

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	writable, ok := got.Types["Writable"].(*typ.Map)
	if !ok {
		t.Fatalf("Writable = %T, want *typ.Map", got.Types["Writable"])
	}
	if !typ.TypeEquals(writable.Key, typ.String) {
		t.Fatalf("Writable key = %v, want string", writable.Key)
	}

	readonly, ok := got.Types["Readonly"].(*typ.ReadonlyMap)
	if !ok {
		t.Fatalf("Readonly = %T, want *typ.ReadonlyMap", got.Types["Readonly"])
	}
	if !typ.TypeEquals(readonly.Key, typ.String) {
		t.Fatalf("Readonly key = %v, want string", readonly.Key)
	}
}

func TestManifestRoundTripNamedFunctionSignatureEffects(t *testing.T) {
	row := effect.Open("rho",
		control.IO{},
		ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 1}},
		mutation.LengthChange{Target: effect.ParamRef{Index: 1}, Delta: -1},
		postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 0}, Refinement: postcondition.Present{}},
		returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: effect.ParamRef{Index: 1}}},
		returns.ReturnLength{ReturnIndex: 0, Length: expr.Add(expr.PL(1), expr.C(1))},
	)
	export := typ.Func().
		Param("input", typ.String).
		Param("out", typ.NewArray(typ.String)).
		Returns(typ.String).
		Build()
	m := New("example/effects")
	m.SetExport(export)
	m.DefineFunctionSignature("transform", signature.Function{Type: export, Effect: row})

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(string(data), `"functionSignatures"`) || !strings.Contains(string(data), `"effect"`) {
		t.Fatalf("encoded manifest missing function signature effect row:\n%s", data)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	gotFn, ok := got.Export.(*typ.Function)
	if !ok {
		t.Fatalf("export = %T, want function", got.Export)
	}
	if !typ.TypeEquals(got.Export, export) {
		t.Fatalf("export = %v, want %v", got.Export, export)
	}
	gotSig, ok := got.FunctionSignatures["transform"]
	if !ok {
		t.Fatalf("missing transform function signature")
	}
	if !gotSig.Effect.Equals(row) {
		t.Fatalf("effect = %v, want %v", gotSig.Effect, row)
	}
	if !typ.TypeEquals(gotSig.Type, gotFn) {
		t.Fatalf("signature type = %v, want %v", gotSig.Type, gotFn)
	}
	if !(signature.Function{Type: export, Effect: row}).Equals(gotSig) {
		t.Fatalf("signature = %v, want %v", gotSig, signature.Function{Type: export, Effect: row})
	}
}

func TestManifestEffectLabelRoundTripPreservesRowsAndSelectors(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	cases := []struct {
		name  string
		label effect.Label
	}{
		{"control throw", control.Throw{}},
		{"control diverge", control.Diverge{}},
		{"control io", control.IO{}},
		{"dispatch module load", dispatch.ModuleLoad{}},
		{"dispatch variadic transform", dispatch.VariadicTransform{}},
		{"dispatch type predicate", dispatch.TypePredicate{}},
		{"dispatch type value method", dispatch.TypeValueMethod{}},
		{"dispatch callable type", dispatch.CallableType{}},
		{"iteration iterator", iteration.Iterator{Source: p0, Kind: iteration.IterateIndexed}},
		{"mutation mutate", mutation.Mutate{Target: p0, Transform: mutation.ElementUnion{Source: p1}, LengthDelta: expr.Add(expr.PL(0), expr.C(1))}},
		{"mutation length change", mutation.LengthChange{Target: p1, Delta: -2}},
		{"mutation table mutator", mutation.TableMutator{Target: p0, Value: p1}},
		{"ownership borrow", ownership.Borrow{Param: p0}},
		{"ownership store", ownership.Store{Param: p0, Into: p1}},
		{"ownership borrow all", ownership.BorrowAll{}},
		{"ownership send", ownership.Send{FromParam: 1}},
		{"ownership freeze", ownership.Freeze{Param: p2}},
		{"postcondition normal return present", postcondition.NormalReturnRefinement{Target: p0, Refinement: postcondition.Present{}}},
		{"returns return", returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: p0}}},
		{"returns error return", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}},
		{"returns return length", returns.ReturnLength{ReturnIndex: 0, Length: expr.MinExpr(expr.PL(0), expr.C(3))}},
		{"returns correlated return", returns.CorrelatedReturn{Indices: []int{0, 2}}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			row := effect.Open("rho", tt.label)
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
			if got.Hash() != row.Hash() {
				t.Fatalf("roundtrip hash = %d, want %d", got.Hash(), row.Hash())
			}
			if !rowHasLabel(got, tt.label) {
				t.Fatalf("roundtrip row missing %T in %v", tt.label, got)
			}
		})
	}
}

func rowHasLabel(row effect.Row, want effect.Label) bool {
	want = effect.NormalizeLabel(want)
	return row.Has(func(got effect.Label) bool {
		return got != nil && got.Equals(want)
	})
}

func TestManifestEffectLabelRoundTripCoversNestedKinds(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	rows := []effect.Row{
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.Unchanged{}}),
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.ContainerElementUnion{Container: p1, Value: p2}}),
		effect.Empty.With(mutation.Mutate{Target: p0, Transform: mutation.ToArray{Element: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.DeepElementOf{Source: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.StringUnpackValue{Format: p2}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SelectCaseOfParam{Source: p0}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.SelectResultOfCases{Cases: p0, Default: p1}}),
		effect.Empty.With(returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
			Source: p0,
			Projection: projection.Projection{Steps: []projection.Step{
				projection.Field("payload"),
				projection.CallableReturn(),
				projection.GenericArg(0),
				projection.InstantiateGeneric(typ.String),
			}},
		}}),
	}

	for _, row := range rows {
		t.Run(row.String(), func(t *testing.T) {
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
		})
	}
}

func TestManifestEffectLabelRoundTripCoversReturnStatusMatrix(t *testing.T) {
	p0 := effect.ParamRef{Index: 0}
	p1 := effect.ParamRef{Index: 1}
	p2 := effect.ParamRef{Index: 2}
	tests := []struct {
		name   string
		status string
		label  effect.Label
	}{
		{"actively lowered", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.ElementOf{Source: p0}}},
		{"actively lowered optional", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.OptionalElementOf{Source: p0}}},
		{"actively lowered callback", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.CallbackReturn{CallbackParam: p1}}},
		{"actively lowered array callback", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.ArrayOfCallbackReturn{CallbackParam: p1}}},
		{"actively lowered same as", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: p0}}},
		{"actively lowered projection", "actively lowered by effectlowering", returns.Return{ReturnIndex: 0, Transform: returns.TypeProjection{
			Source: p0,
			Projection: projection.Projection{Steps: []projection.Step{
				projection.Field("payload"),
				projection.CallableReturn(),
			}},
		}}},
		{"reserved deep element", "reserved/falls back to declared returns", returns.Return{ReturnIndex: 0, Transform: returns.DeepElementOf{Source: p0}}},
		{"reserved string unpack", "reserved/falls back to declared returns", returns.Return{ReturnIndex: 0, Transform: returns.StringUnpackValue{Format: p2}}},
		{"reserved select case", "reserved/falls back to declared returns", returns.Return{ReturnIndex: 0, Transform: returns.SelectCaseOfParam{Source: p0}}},
		{"reserved select result", "reserved/falls back to declared returns", returns.Return{ReturnIndex: 0, Transform: returns.SelectResultOfCases{Cases: p0, Default: p1}}},
		{"data-only error return", "actively lowered by effectlowering", returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}},
		{"data-only return length", "data-only", returns.ReturnLength{ReturnIndex: 0, Length: expr.MinExpr(expr.PL(0), expr.C(3))}},
		{"data-only correlated return", "data-only", returns.CorrelatedReturn{Indices: []int{0, 2}}},
	}

	for _, tt := range tests {
		t.Run(tt.status+" / "+tt.name, func(t *testing.T) {
			row := effect.Open("rho", tt.label)
			got := mustRoundTripEffectRow(t, row)
			if !got.Equals(row) {
				t.Fatalf("roundtrip row = %v, want %v", got, row)
			}
			if !rowHasLabel(got, tt.label) {
				t.Fatalf("roundtrip row missing %T in %v", tt.label, got)
			}
		})
	}
}

func TestManifestEffectPointerLabelsNormalizeToValues(t *testing.T) {
	row := effect.Row{Labels: []effect.Label{
		&control.IO{},
		&iteration.Iterator{Source: effect.ParamRef{Index: 0}, Kind: iteration.IterateKeyed},
		&mutation.Mutate{Target: effect.ParamRef{Index: 0}, Transform: &mutation.ToArray{Element: effect.ParamRef{Index: 1}}},
		&ownership.Borrow{Param: effect.ParamRef{Index: 2}},
		&returns.Return{ReturnIndex: 0, Transform: &returns.ElementOf{Source: effect.ParamRef{Index: 0}}},
	}}

	got := mustRoundTripEffectRow(t, row)
	if !got.Equals(row) {
		t.Fatalf("roundtrip pointer row = %v, want %v", got, row)
	}
	if got.Hash() != row.Hash() {
		t.Fatalf("roundtrip pointer hash = %d, want %d", got.Hash(), row.Hash())
	}
	for _, want := range row.Labels {
		if !rowHasLabel(got, want) {
			t.Fatalf("roundtrip pointer row missing %T in %v", want, got)
		}
	}
	for _, label := range got.Labels {
		if effect.NormalizeLabel(label) != label {
			t.Fatalf("decoded label %T was not value-owned", label)
		}
	}
}

func mustRoundTripEffectRow(t *testing.T, row effect.Row) effect.Row {
	t.Helper()
	wire, err := encodeEffectRow(row)
	if err != nil {
		t.Fatalf("encodeEffectRow: %v", err)
	}
	got, err := decodeEffectRow(wire)
	if err != nil {
		t.Fatalf("decodeEffectRow: %v", err)
	}
	return got
}

func TestManifestEncodeUnknownIteratorKindErrors(t *testing.T) {
	m := New("example/bad-iterator")
	m.DefineFunctionSignature("iter", signature.Function{
		Type: typ.Func().
			Param("input", typ.NewArray(typ.String)).
			Build(),
		Effect: effect.Empty.With(iteration.Iterator{
			Source: effect.ParamRef{Index: 0},
			Kind:   iteration.IteratorKind(99),
		}),
	})

	_, err := Encode(m)
	if err == nil || !strings.Contains(err.Error(), "unknown iterator kind 99") {
		t.Fatalf("Encode error = %v, want unknown iterator kind", err)
	}
}

func TestManifestDecodeIteratorRequiresExplicitKind(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{Kind: "iteration.iterator"})
	if err == nil || !strings.Contains(err.Error(), `unknown iterator kind ""`) {
		t.Fatalf("decodeEffectLabel error = %v, want unknown iterator kind", err)
	}
}

func TestManifestDecodePostconditionRefinementRequiresKnownKind(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{
		Kind:       postcondition.NormalReturnRefinementKind,
		Target:     &paramRefWire{Index: 0},
		Refinement: &effectRefinementWire{Kind: "future"},
	})
	if err == nil || !strings.Contains(err.Error(), `unknown effect refinement kind "future"`) {
		t.Fatalf("decodeEffectLabel error = %v, want unknown effect refinement kind", err)
	}
}

func TestManifestDecodePostconditionRefinementRequiresRefinement(t *testing.T) {
	_, err := decodeEffectLabel(effectLabelWire{
		Kind:   postcondition.NormalReturnRefinementKind,
		Target: &paramRefWire{Index: 0},
	})
	if err == nil || !strings.Contains(err.Error(), "missing effect refinement") {
		t.Fatalf("decodeEffectLabel error = %v, want missing effect refinement", err)
	}
}

func TestManifestEncodePostconditionRefinementRequiresRefinement(t *testing.T) {
	m := New("example/bad-postcondition")
	m.DefineFunctionSignature("assertLike", signature.Function{
		Type: typ.Func().
			Param("input", typ.Any).
			Build(),
		Effect: effect.Empty.With(postcondition.NormalReturnRefinement{
			Target: effect.ParamRef{Index: 0},
		}),
	})

	_, err := Encode(m)
	if err == nil || !strings.Contains(err.Error(), "missing effect refinement") {
		t.Fatalf("Encode error = %v, want missing effect refinement", err)
	}
}

func TestManifestNilHandling(t *testing.T) {
	if _, err := Encode(nil); err == nil {
		t.Fatalf("Encode(nil) succeeded")
	}
	if _, err := Decode(nil); err == nil {
		t.Fatalf("Decode(nil) succeeded")
	}
	if _, err := Decode([]byte("   \n\t")); err == nil {
		t.Fatalf("Decode(blank) succeeded")
	}
}

func TestManifestEncodeOrdersNamedTypesDeterministically(t *testing.T) {
	left := New("example/module")
	left.DefineType("Zed", typ.String)
	left.DefineType("Alpha", typ.Number)
	left.DefineType("Middle", typ.Boolean)

	right := New("example/module")
	right.DefineType("Middle", typ.Boolean)
	right.DefineType("Alpha", typ.Number)
	right.DefineType("Zed", typ.String)

	leftData, err := Encode(left)
	if err != nil {
		t.Fatalf("Encode(left): %v", err)
	}
	rightData, err := Encode(right)
	if err != nil {
		t.Fatalf("Encode(right): %v", err)
	}
	if !bytes.Equal(leftData, rightData) {
		t.Fatalf("encoding is not stable:\nleft:\n%s\nright:\n%s", leftData, rightData)
	}

	text := string(leftData)
	alpha := strings.Index(text, `"name": "Alpha"`)
	middle := strings.Index(text, `"name": "Middle"`)
	zed := strings.Index(text, `"name": "Zed"`)
	if alpha < 0 || middle < 0 || zed < 0 || !(alpha < middle && middle < zed) {
		t.Fatalf("named types are not sorted:\n%s", text)
	}
}
