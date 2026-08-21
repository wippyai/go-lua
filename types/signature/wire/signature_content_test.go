package wire

import (
	"bytes"
	"context"
	"testing"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
)

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

func TestCanonicalFunctionSignatureContentIncludesKokaEffect(t *testing.T) {
	typing := typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build()
	pure := signature.Function{Type: typing}
	effectful := signature.Function{
		Type:   typing,
		Effect: effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}),
	}
	if pure.Equals(effectful) {
		t.Fatal("effectful signature unexpectedly equals pure signature")
	}
	if bytes.Equal(canonicalSignatureBytes(t, pure), canonicalSignatureBytes(t, effectful)) {
		t.Fatal("Koka effect changed signature semantics without changing canonical content")
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
	want := []byte(`{"schema":"go-lua.signature.content/v4","hasType":true,"type":{"kind":"function","params":[{"type":{"kind":"recursive","name":"Node","binder":1,"body":{"kind":"record","fields":[{"name":"next","type":{"kind":"recursiveRef","binder":1}}]}}}],"returns":[{"kind":"recursiveRef","binder":1}]}}`)
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
