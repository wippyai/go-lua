package manifest

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/iteration"
	"github.com/wippyai/go-lua/analysis/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/analysis/domain/effect/postcondition"
	"github.com/wippyai/go-lua/analysis/domain/type/annotation"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typecall"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func TestEncodeCompactMatchesCanonicalManifestContent(t *testing.T) {
	m := New("example/module")
	m.DefineType("User", typetable.NewRecord().Field("id", typ.Integer).Build())
	m.SetExport(typ.NewArray(typ.NewRef("", "User")))

	pretty, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	compact, err := EncodeCompact(m)
	if err != nil {
		t.Fatalf("EncodeCompact: %v", err)
	}
	var want bytes.Buffer
	if err := json.Compact(&want, pretty); err != nil {
		t.Fatalf("json.Compact(Encode): %v", err)
	}
	if !bytes.Equal(compact, want.Bytes()) {
		t.Fatalf("EncodeCompact content differs from Encode\nwant: %s\n got: %s", want.Bytes(), compact)
	}
}

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
	m.DefineType("Status", typeexpr.Union(
		typ.LiteralString("ready"),
		typ.LiteralString("pending"),
	))
	m.DefineType("User", typ.NewAnnotated(
		typetable.NewRecord().
			ReadonlyField("id", typ.Integer).
			OptField("name", typ.String).
			Build(),
		[]annotation.Annotation{{Name: "sealed", Arg: annotation.BoolArg(true)}},
	))
	formal := typ.NewTypeParam("T", typ.NewRef("", "User"))
	m.SetExport(typ.Func().
		TypeParamRef(formal).
		Param("value", formal).
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

func TestManifestRoundTripPreservesDerivedReceiverWithoutChangingWireShape(t *testing.T) {
	m := New("receiver/roundtrip")
	m.SetExport(typ.Func().Param("self", typ.Self).Param("value", typ.String).Returns(typ.Boolean).Build())
	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if strings.Contains(string(data), `"receiver":`) {
		t.Fatalf("receiver metadata leaked into existing manifest wire: %s", data)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	fn, ok := decoded.Export.(*typ.Function)
	if !ok || len(fn.Params) != 2 || !fn.Params[0].Receiver || fn.Params[1].Receiver || fn.Params[0].Name != "self" {
		t.Fatalf("decoded receiver params = %#v", decoded.Export)
	}
	reencoded, err := Encode(decoded)
	if err != nil {
		t.Fatalf("re-Encode: %v", err)
	}
	if string(data) != string(reencoded) {
		t.Fatalf("manifest bytes changed across receiver round-trip:\n%s\n%s", data, reencoded)
	}
}

func TestManifestRoundTripCallbackPhaseProtocol(t *testing.T) {
	m := New("example/callbacks")
	m.DefineCallbackPhaseRegistration("before_each", 0, "setup")
	m.DefineCallbackPhaseRegistration("before_each", 0, "setup")
	m.DefineCallbackPhaseRegistration("after_each", 0, "teardown")
	m.DefineCallbackPhaseInvocation("it", 1, []string{"setup", "setup"}, []string{"teardown"})
	m.DefineCallbackPhaseInvocation("it", 1, []string{"setup"}, []string{"teardown"})
	m.DefineCallbackPhaseInvocation("case", 1, []string{"z_outer", "a_inner", "z_outer"}, []string{"z_cleanup", "a_final"})

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if len(got.CallbackPhaseRegistrations) != 2 {
		t.Fatalf("registrations = %#v, want before_each/setup and after_each/teardown", got.CallbackPhaseRegistrations)
	}
	if len(got.CallbackPhaseInvocations) != 2 {
		t.Fatalf("invocations = %#v, want normalized it and case invocations", got.CallbackPhaseInvocations)
	}
	invocation := got.CallbackPhaseInvocations[1]
	if invocation.Function != "it" || invocation.CallbackParam != 1 ||
		len(invocation.Before) != 1 || invocation.Before[0] != "setup" ||
		len(invocation.After) != 1 || invocation.After[0] != "teardown" {
		t.Fatalf("invocation = %#v, want normalized setup -> teardown protocol", invocation)
	}
	ordered := got.CallbackPhaseInvocations[0]
	if ordered.Function != "case" || ordered.CallbackParam != 1 ||
		len(ordered.Before) != 2 || ordered.Before[0] != "z_outer" || ordered.Before[1] != "a_inner" ||
		len(ordered.After) != 2 || ordered.After[0] != "z_cleanup" || ordered.After[1] != "a_final" {
		t.Fatalf("ordered invocation = %#v, want declared phase order with duplicates removed", ordered)
	}
}

func TestManifestRoundTripNormalizesMapKeysOnDecode(t *testing.T) {
	mapKey := typ.MaterializeUnion([]typ.Type{typ.String, typ.Nil})

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

func TestManifestRoundTripPreservesGenericCallbackAliasInference(t *testing.T) {
	aliasParam := typ.NewTypeParam("T", nil)
	predicate := typ.NewGeneric("Predicate", []*typ.TypeParam{aliasParam},
		typ.Func().Param("item", aliasParam).Returns(typ.Boolean).Build())
	fnParam := typ.NewTypeParam("T", nil)
	fn := typ.Func().
		TypeParamRef(fnParam).
		Param("items", typ.NewArray(fnParam)).
		Param("pred", typ.Instantiate(predicate, fnParam)).
		Returns(typeexpr.Optional(fnParam)).
		Build()
	m := New("iter")
	m.FunctionSignatures["iter.find"] = signature.Function{Type: fn}

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sig, ok := got.FunctionSignatures["iter.find"]
	if !ok {
		t.Fatalf("missing iter.find signature: %#v", got.FunctionSignatures)
	}
	user := typetable.NewRecord().
		Field("name", typ.String).
		Field("age", typ.Number).
		Build()
	callback := typ.Func().
		Param("user", user).
		Returns(typ.Boolean).
		Build()
	instantiated, violations, bindings := typecall.InstantiateGenericCallWithBindings(sig.Type, []typ.Type{typ.NewArray(user), callback})
	if len(violations) != 0 {
		t.Fatalf("violations = %#v, want none", violations)
	}
	if len(bindings) != 1 || !typ.TypeEquals(bindings[0].Type, user) {
		t.Fatalf("bindings = %#v, want T=%v", bindings, user)
	}
	if len(instantiated.Returns) != 1 || !typ.TypeEquals(instantiated.Returns[0], typeexpr.Optional(user)) {
		t.Fatalf("returns = %v, want %v", instantiated.Returns, typeexpr.Optional(user))
	}
}

func TestManifestRejectsLifecycleEffectsWithoutDeclaredFSM(t *testing.T) {
	fn := typ.Func().Param("tx", typ.Any).Build()
	m := New("example/lifecycle-missing-fsm")
	m.DefineFunctionSignature("begin", signature.Function{
		Type: fn,
		Effect: effect.Empty.With(lifecycle.Acquire{
			Target:   effect.ParamRef{Index: 0},
			Protocol: "transaction",
			State:    "active",
			Obligation: typestate.Obligation{
				Final: "finished",
			},
		}),
	})

	_, err := Encode(m)
	if err == nil || !strings.Contains(err.Error(), `lifecycle protocol "transaction" is not declared as a typestate FSM`) {
		t.Fatalf("Encode error = %v, want undeclared typestate FSM", err)
	}
}

func TestManifestRoundTripsLifecycleAcquireFinalStateSet(t *testing.T) {
	fn := typ.Func().Param("tx", typ.Any).Build()
	m := New("example/lifecycle-final-set")
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    "transaction",
		States:      []typestate.State{"active", "committed", "rolled_back"},
		FinalStates: []typestate.State{"committed", "rolled_back"},
		Transitions: []typestate.TransitionDecl{
			{From: "active", To: "committed"},
			{From: "active", To: "rolled_back"},
		},
	}); err != nil {
		t.Fatalf("DefineTypestateProtocol: %v", err)
	}
	obligation := typestate.Obligation{Finals: typestate.NewFinalStates("rolled_back", "committed")}
	m.DefineFunctionSignature("begin", signature.Function{
		Type: fn,
		Effect: effect.Empty.With(lifecycle.Acquire{
			Target:     effect.ParamRef{Index: 0},
			Protocol:   "transaction",
			State:      "active",
			Obligation: obligation,
		}),
	})

	data, err := Encode(m)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !bytes.Contains(data, []byte(`"finals"`)) ||
		!bytes.Contains(data, []byte(`"committed"`)) ||
		!bytes.Contains(data, []byte(`"rolled_back"`)) {
		t.Fatalf("encoded manifest missing canonical finals set:\n%s", data)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	sig, ok := got.FunctionSignatures["begin"]
	if !ok {
		t.Fatalf("decoded signatures = %#v, want begin", got.FunctionSignatures)
	}
	if !rowHasLabel(sig.Effect, lifecycle.Acquire{
		Target:     effect.ParamRef{Index: 0},
		Protocol:   "transaction",
		State:      "active",
		Obligation: obligation,
	}) {
		t.Fatalf("decoded effect = %v, want lifecycle acquire with finals set %q", sig.Effect, obligation.Finals)
	}
}

func TestManifestRejectsMalformedTypestateProtocolDeclarations(t *testing.T) {
	m := New("example/bad-fsm")
	err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    "cursor",
		States:      []typestate.State{"open"},
		FinalStates: []typestate.State{"closed"},
	})
	if err == nil || !strings.Contains(err.Error(), `final state "closed" is not declared`) {
		t.Fatalf("DefineTypestateProtocol error = %v, want unknown final state", err)
	}
	err = m.DefineTypestateProtocol(typestate.Definition{
		Protocol: "cursor",
		States:   []typestate.State{"open", "open"},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicates state "open"`) {
		t.Fatalf("DefineTypestateProtocol duplicate error = %v, want duplicate state", err)
	}

	_, err = Decode([]byte(`{
  "path": "example/bad-fsm",
  "typestateProtocols": [
    {
      "name": "cursor",
      "states": ["open"],
      "transitions": [{ "from": "open", "to": "closed" }]
    }
  ]
}`))
	if err == nil || !strings.Contains(err.Error(), `transition target "closed" is not declared`) {
		t.Fatalf("Decode error = %v, want unknown transition target", err)
	}
}

func defineTestTypestateProtocol(t *testing.T, m *Manifest, protocol typestate.Protocol, states, finals []typestate.State, transitions []typestate.TransitionDecl) {
	t.Helper()
	if err := m.DefineTypestateProtocol(typestate.Definition{
		Protocol:    protocol,
		States:      states,
		FinalStates: finals,
		Transitions: transitions,
	}); err != nil {
		t.Fatalf("DefineTypestateProtocol(%s): %v", protocol, err)
	}
}

func rowHasLabel(row effect.Row, want effect.Label) bool {
	want = effect.NormalizeLabel(want)
	return row.Has(func(got effect.Label) bool {
		return got != nil && got.Equals(want)
	})
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
