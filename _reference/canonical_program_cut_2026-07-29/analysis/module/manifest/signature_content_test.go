package manifest

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestCanonicalFunctionSignatureContentCoversEveryEqualityLane(t *testing.T) {
	base := signature.Function{Type: typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build()}
	baseBytes := canonicalSignatureBytes(t, base)

	mutations := map[string]signature.Function{
		"Type":                       {Type: typ.Func().Param("value", typ.Number).Returns(typ.Boolean).Build()},
		"Effect":                     {Type: base.Type, Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})},
		"OperationalEffectsPresence": {Type: base.Type, OperationalEffects: &signature.OperationalEffects{}},
	}
	for name, mutated := range mutations {
		if bytes.Equal(baseBytes, canonicalSignatureBytes(t, mutated)) {
			t.Fatalf("%s mutation did not change canonical signature content", name)
		}
	}

	rich := oracleRichOperationalEffects()
	cases := oracleSingleLaneCases(rich)
	if got, want := len(cases), reflect.TypeOf(signature.OperationalEffects{}).NumField(); got != want {
		t.Fatalf("single-lane mutation census = %d, want %d OperationalEffects fields", got, want)
	}
	emptyPresent := canonicalSignatureBytes(t, signature.Function{Type: base.Type, OperationalEffects: &signature.OperationalEffects{}})
	for _, mutation := range cases {
		t.Run(mutation.name, func(t *testing.T) {
			if bytes.Equal(emptyPresent, canonicalSignatureBytes(t, signature.Function{Type: base.Type, OperationalEffects: &mutation.e})) {
				t.Fatalf("OperationalEffects.%s mutation did not change canonical signature content", mutation.name)
			}
		})
	}
}

func TestCanonicalFunctionSignatureContentStableAcrossCloneAndOrder(t *testing.T) {
	leftEffects := oracleRichOperationalEffects()
	rightEffects := leftEffects.Clone()
	reverseOperationalEffectSlices(&rightEffects)
	left := signature.Function{
		Type:               typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build(),
		Effect:             effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
		OperationalEffects: &leftEffects,
	}
	right := left.Clone()
	right.OperationalEffects = &rightEffects
	if got, want := canonicalSignatureBytes(t, left), canonicalSignatureBytes(t, right); !bytes.Equal(got, want) {
		t.Fatalf("canonical content differs across clone/order\nleft:  %s\nright: %s", got, want)
	}
}

func TestCanonicalFunctionSignatureContentUsesSemanticFunctionPresentation(t *testing.T) {
	left := signature.Function{Type: typ.RebuildFunction(typ.FunctionParts{Params: []typ.Param{{Name: "value", Type: typ.String}}})}
	right := signature.Function{Type: typ.RebuildFunction(typ.FunctionParts{Params: []typ.Param{{Name: "renamed", Type: typ.String}}})}
	if !left.Equals(right) {
		t.Fatal("fixture labels unexpectedly participate in type equality")
	}
	if got, want := canonicalSignatureBytes(t, left), canonicalSignatureBytes(t, right); !bytes.Equal(got, want) {
		t.Fatalf("presentation-only rename changed semantic content\nleft: %s\nright: %s", got, want)
	}

	receiver := signature.Function{Type: typ.RebuildFunction(typ.FunctionParts{Params: []typ.Param{{Name: "ctx", Type: typ.String, Receiver: true}}})}
	if left.Equals(receiver) {
		t.Fatal("receiver fixture unexpectedly equals ordinary parameter")
	}
	if bytes.Equal(canonicalSignatureBytes(t, left), canonicalSignatureBytes(t, receiver)) {
		t.Fatal("receiver convention did not change semantic content")
	}
}

func TestCanonicalFunctionSignatureContentIsAliasCongruent(t *testing.T) {
	plain := signature.Function{Type: typ.Func().Param("value", typ.Number).Returns(typ.Number).Build()}
	aliasA := typ.NewAlias("A", typ.Number)
	aliasB := typ.NewAlias("B", typ.Number)
	for name, candidate := range map[string]signature.Function{
		"alias A": {Type: typ.Func().Param("value", aliasA).Returns(aliasA).Build()},
		"alias B": {Type: typ.Func().Param("value", aliasB).Returns(aliasB).Build()},
	} {
		t.Run(name, func(t *testing.T) {
			if !plain.Equals(candidate) {
				t.Fatal("alias fixture is not TypeEquals-congruent")
			}
			if got, want := canonicalSignatureBytes(t, candidate), canonicalSignatureBytes(t, plain); !bytes.Equal(got, want) {
				t.Fatalf("alias changed semantic content\naliased: %s\nplain:   %s", got, want)
			}
		})
	}
}

func TestCanonicalFunctionSignatureContentNormalizesEveryEmbeddedType(t *testing.T) {
	functionType := func(label string) typ.Type {
		return typ.Func().Param(label, typ.String).Returns(typ.Boolean).Build()
	}
	baseType := typ.Func().Param("value", typ.Any).Returns(typ.Any).Build()
	tests := []struct {
		name  string
		build func(typ.Type) *signature.OperationalEffects
	}{
		{
			name: "operational refinement",
			build: func(value typ.Type) *signature.OperationalEffects {
				return &signature.OperationalEffects{NormalReturnTypeRefinements: []signature.PathTypeRefinement{{
					Path: pathdom.NewPlaceholder(0), Type: value,
				}}}
			},
		},
		{
			name: "allocation object and key",
			build: func(value typ.Type) *signature.OperationalEffects {
				return &signature.OperationalEffects{ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
					ReturnIndex: 0,
					Root:        "root",
					Objects: []signature.AllocationObjectTemplate{{
						ID:   "root",
						Type: value,
						DynamicEntries: []signature.AllocationDynamicEntryTemplate{{
							KeyType: value,
							Value:   "root",
						}},
					}},
				}}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left := signature.Function{Type: baseType, OperationalEffects: test.build(functionType("left"))}
			right := signature.Function{Type: baseType, OperationalEffects: test.build(functionType("right"))}
			if got, want := canonicalSignatureBytes(t, left), canonicalSignatureBytes(t, right); !bytes.Equal(got, want) {
				t.Fatalf("embedded presentation changed content\nleft:  %s\nright: %s", got, want)
			}

			aliased := signature.Function{Type: baseType, OperationalEffects: test.build(typ.NewAlias("Alias", typ.Number))}
			plain := signature.Function{Type: baseType, OperationalEffects: test.build(typ.Number)}
			if got, want := canonicalSignatureBytes(t, aliased), canonicalSignatureBytes(t, plain); !bytes.Equal(got, want) {
				t.Fatalf("embedded alias changed content\naliased: %s\nplain:   %s", got, want)
			}

			receiverType := typ.RebuildFunction(typ.FunctionParts{Params: []typ.Param{{Name: "ctx", Type: typ.String, Receiver: true}}})
			ordinaryType := typ.RebuildFunction(typ.FunctionParts{Params: []typ.Param{{Name: "ctx", Type: typ.String}}})
			receiver := signature.Function{Type: baseType, OperationalEffects: test.build(receiverType)}
			ordinary := signature.Function{Type: baseType, OperationalEffects: test.build(ordinaryType)}
			if bytes.Equal(canonicalSignatureBytes(t, receiver), canonicalSignatureBytes(t, ordinary)) {
				t.Fatal("embedded receiver convention did not change content")
			}
		})
	}
}

func TestCanonicalFunctionSignatureContentEmbeddedRecursiveTypeSurvivesReparse(t *testing.T) {
	node := typ.NewRecursive("EmbeddedNode", func(self typ.Type) typ.Type {
		return &typ.Record{Fields: []typ.Field{{Name: "next", Type: self}}}
	})
	original := signature.Function{
		Type: typ.Func().Param("value", typ.Any).Returns(typ.Any).Build(),
		OperationalEffects: &signature.OperationalEffects{
			NormalReturnTypeRefinements: []signature.PathTypeRefinement{{Path: pathdom.NewPlaceholder(0), Type: node}},
			ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
				ReturnIndex: 0, Root: "root", Objects: []signature.AllocationObjectTemplate{{ID: "root", Type: node}},
			}},
		},
	}
	wire, err := encodeFunctionSignature(original)
	if err != nil {
		t.Fatal(err)
	}
	serialized, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	var decodedWire functionSignatureWire
	if err := json.Unmarshal(serialized, &decodedWire); err != nil {
		t.Fatal(err)
	}
	reparsed, err := decodeFunctionSignature(decodedWire)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := canonicalSignatureBytes(t, original), canonicalSignatureBytes(t, reparsed); !bytes.Equal(got, want) {
		t.Fatalf("embedded recursive content changed across reparse\noriginal: %s\nreparsed: %s", got, want)
	}
}

func TestCanonicalFunctionSignatureContentRejectsEmbeddedUnboundCycles(t *testing.T) {
	iface := &typ.Interface{Name: "cycle"}
	cycle := &typ.Function{Returns: []typ.Type{iface}}
	iface.Methods = []typ.Method{{Name: "next", Type: cycle}}
	base := typ.Func().Returns(typ.Any).Build()
	for name, effects := range map[string]*signature.OperationalEffects{
		"operational": {NormalReturnTypeRefinements: []signature.PathTypeRefinement{{Path: pathdom.NewPlaceholder(0), Type: cycle}}},
		"allocation": {ReturnAllocationTemplates: []signature.ReturnAllocationTemplate{{
			ReturnIndex: 0, Root: "root", Objects: []signature.AllocationObjectTemplate{{ID: "root", Type: cycle}},
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := CanonicalFunctionSignatureBytes(signature.Function{Type: base, OperationalEffects: effects}); err == nil {
				t.Fatal("embedded unbound cycle unexpectedly acquired canonical content")
			}
		})
	}
}

func TestCanonicalFunctionSignatureContentStableAcrossManifestReparse(t *testing.T) {
	effects := oracleRichOperationalEffects()
	original := signature.Function{
		Type: typ.Func().
			Param("value", typ.String).
			Param("other", typ.String).
			Returns(typ.Boolean, typ.String, typ.Number).
			Build(),
		Effect:             effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
		OperationalEffects: &effects,
	}
	wire, err := encodeFunctionSignature(original)
	if err != nil {
		t.Fatalf("encodeFunctionSignature: %v", err)
	}
	serialized, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("marshal signature wire: %v", err)
	}
	var reparsedWire functionSignatureWire
	if err := json.Unmarshal(serialized, &reparsedWire); err != nil {
		t.Fatalf("unmarshal signature wire: %v", err)
	}
	reparsed, err := decodeFunctionSignature(reparsedWire)
	if err != nil {
		t.Fatalf("decodeFunctionSignature: %v", err)
	}
	if got, want := canonicalSignatureBytes(t, original), canonicalSignatureBytes(t, reparsed); !bytes.Equal(got, want) {
		t.Fatalf("canonical content differs across manifest reparse\noriginal: %s\nreparsed: %s", got, want)
	}
}

func TestCanonicalFunctionSignatureContentHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CanonicalFunctionSignatureBytesContext(ctx, signature.Function{Type: typ.Func().Build()}); err == nil {
		t.Fatal("canceled canonical encoding unexpectedly succeeded")
	}
}

func TestCanonicalFunctionSignatureContentRejectsUnboundStructuralCycle(t *testing.T) {
	iface := &typ.Interface{Name: "cycle"}
	fn := &typ.Function{Returns: []typ.Type{iface}}
	iface.Methods = []typ.Method{{Name: "next", Type: fn}}
	if _, err := CanonicalFunctionSignatureBytes(signature.Function{Type: fn}); err == nil {
		t.Fatal("unbound object cycle unexpectedly acquired canonical content")
	}
}

func TestCanonicalFunctionSignatureContentSupportsRecursiveBinder(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return &typ.Record{Fields: []typ.Field{{Name: "next", Type: self}}}
	})
	sig := signature.Function{Type: typ.Func().Param("node", node).Returns(node).Build()}
	want := []byte(`{"schema":"go-lua.signature.content/v1","hasType":true,"type":{"kind":"function","params":[{"type":{"kind":"recursive","name":"Node","binder":1,"body":{"kind":"record","fields":[{"name":"next","type":{"kind":"recursiveRef","binder":1}}]}}}],"returns":[{"kind":"recursiveRef","binder":1}]},"hasOperationalEffects":false}`)
	if got := canonicalSignatureBytes(t, sig); !bytes.Equal(got, want) {
		t.Fatalf("recursive binder canonical content = %s, want %s", got, want)
	}
}

func canonicalSignatureBytes(t *testing.T, sig signature.Function) []byte {
	t.Helper()
	encoded, err := CanonicalFunctionSignatureBytes(sig)
	if err != nil {
		t.Fatalf("CanonicalFunctionSignatureBytes: %v", err)
	}
	return encoded
}
