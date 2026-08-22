package manifest_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/types/signature"
)

// The catalogue is the signature lookup a consumer seals its providers into,
// so it is where a module's declared error type turns into the correlation its
// members are written around. These laws read the sealed law back off the
// catalogue Function.

func correlationProvider(declare func(*moduleio.Manifest)) manifest.Provider {
	return manifest.Provider{
		Identity: "correlation",
		Mount:    manifest.MountModule,
		Declaration: func() *moduleio.Manifest {
			declaration := moduleio.New("host")
			errorType := typ.NewInterface("Error", []typ.Method{
				{Name: "message", Type: typ.Func().Param("self", typ.Any).Returns(typ.String).Build()},
			})
			declaration.ErrorType = errorType
			declaration.DefineType("Error", errorType)
			declare(declaration)
			return declaration
		},
	}
}

func correlationOptionalError() typ.Type {
	return typeexpr.Optional(typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Any).Returns(typ.String).Build()},
	}))
}

// TestSealDerivesTheValueErrorCorrelation states that a member answering the
// module's value/error pair leaves the catalogue carrying the two correlated
// arms, even though its provider tagged nothing.
func TestSealDerivesTheValueErrorCorrelation(t *testing.T) {
	catalogue, err := manifest.Seal(correlationProvider(func(declaration *moduleio.Manifest) {
		declaration.DefineFunctionSignature("open", signature.Function{
			Type: typ.Func().Param("name", typ.String).Returns(typ.String, correlationOptionalError()).Build(),
		})
	}))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	function, ok := catalogue.Function("host.open")
	if !ok {
		t.Fatal("catalogue has no host.open")
	}
	law, ok := function.Operation()
	if !ok {
		t.Fatal("host.open carries no operational law; its declared value/error correlation reaches no consumer")
	}
	if !law.ReplaceNormalSet || len(law.ReplaceNormal) != 2 {
		t.Fatalf("host.open declares %d normal arms with replacement %v, want the two correlated arms",
			len(law.ReplaceNormal), law.ReplaceNormalSet)
	}
	value, failure := law.ReplaceNormal[0], law.ReplaceNormal[1]
	if len(value.Fixed) != 2 || len(failure.Fixed) != 2 {
		t.Fatalf("correlated arms carry %d and %d values, want two each", len(value.Fixed), len(failure.Fixed))
	}
	if !typ.TypeEquals(value.Fixed[0], typ.String) || !typ.TypeEquals(value.Fixed[1], typ.Nil) {
		t.Fatalf("value arm = (%s, %s), want the declared value with no error", value.Fixed[0], value.Fixed[1])
	}
	if !typ.TypeEquals(failure.Fixed[0], typ.Nil) {
		t.Fatalf("error arm answers %s for its value, want nil", failure.Fixed[0])
	}
	if typ.TypeEquals(failure.Fixed[1], typ.Nil) {
		t.Fatal("error arm answers nil for its error")
	}
}

// TestSealLeavesAuthoredArmsAlone keeps one authority per boundary: a provider
// that stated its arms is not restated.
func TestSealLeavesAuthoredArmsAlone(t *testing.T) {
	authored := []moduleio.Values{{Fixed: []typ.Type{typ.String, typ.Nil}, Tail: moduleio.ValuesClosed}}
	catalogue, err := manifest.Seal(correlationProvider(func(declaration *moduleio.Manifest) {
		declaration.DefineFunctionSignature("open", signature.Function{
			Type: typ.Func().Param("name", typ.String).Returns(typ.String, correlationOptionalError()).Build(),
		})
		declaration.DefineFunctionOperation("open", moduleio.Operation{
			ReplaceNormalSet: true, ReplaceNormal: authored,
		})
	}))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	function, ok := catalogue.Function("host.open")
	if !ok {
		t.Fatal("catalogue has no host.open")
	}
	law, _ := function.Operation()
	if len(law.ReplaceNormal) != 1 {
		t.Fatalf("host.open declares %d normal arms, want the one its provider authored", len(law.ReplaceNormal))
	}
}

// TestSealLeavesUnpairedMembersAlone fences the derivation to the pair the
// module declared.
func TestSealLeavesUnpairedMembersAlone(t *testing.T) {
	catalogue, err := manifest.Seal(correlationProvider(func(declaration *moduleio.Manifest) {
		declaration.DefineFunctionSignature("lookup", signature.Function{
			Type: typ.Func().Param("key", typ.String).Returns(typ.String, typeexpr.Optional(typ.Integer)).Build(),
		})
		declaration.DefineFunctionSignature("flush", signature.Function{
			Type: typ.Func().Returns(correlationOptionalError()).Build(),
		})
	}))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	for _, member := range []string{"host.lookup", "host.flush"} {
		function, ok := catalogue.Function(member)
		if !ok {
			t.Fatalf("catalogue has no %s", member)
		}
		if _, ok := function.Operation(); ok {
			t.Fatalf("%s carries a derived law; it declares no value/error pair", member)
		}
	}
}

// TestSealRefusesAnUnstatableCorrelation keeps the boundary honest where it
// cannot serve both authorities. A law that addresses outcomes by ordinal was
// written against the outcome set the signature implies; deriving arms would
// renumber that set under it, and omitting them would drop a correlation the
// module declared. Neither may happen silently.
func TestSealRefusesAnUnstatableCorrelation(t *testing.T) {
	_, err := manifest.Seal(correlationProvider(func(declaration *moduleio.Manifest) {
		declaration.DefineFunctionSignature("open", signature.Function{
			Type: typ.Func().Param("name", typ.String).Returns(typ.String, correlationOptionalError()).Build(),
		})
		declaration.DefineFunctionOperation("open", moduleio.Operation{
			OutcomeTailTypes: []moduleio.OutcomeTailType{{Outcome: 0, Type: typ.Any}},
		})
	}))
	if err == nil {
		t.Fatal("seal accepted a correlated member whose law addresses outcomes by ordinal and states no arms")
	}
	if !strings.Contains(err.Error(), "host.open") {
		t.Fatalf("seal error = %v, want the refusal to name the member", err)
	}
}
