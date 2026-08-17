package wire

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/types/signature"
)

func TestInitialEnvironmentRoundTripsWithoutLosingReferences(t *testing.T) {
	manifest := New("example")
	manifest.DefineFunctionSignature("Thing.call", signature.Function{Type: typ.Func().Returns(typ.String).Build()})
	manifest.DefineInitialRoot(InitialRoot{Identity: "ThingMetatable", Aggregate: InitialAggregateMetatable, Immutable: true})
	manifest.DefineInitialRoot(InitialRoot{Identity: "ThingMethods", Aggregate: InitialAggregateTable, Immutable: true})
	manifest.DefineInitialEntry(InitialEntry{
		Root: DeclaredInitialRoot("ThingMetatable"), Key: "__index",
		Value: InitialRootValue(DeclaredInitialRoot("ThingMethods")), Mutability: InitialFrozen,
	})
	manifest.DefineInitialEntry(InitialEntry{
		Root: DeclaredInitialRoot("ThingMethods"), Key: "call",
		Value: InitialFunctionValue("Thing.call"), Mutability: InitialFrozen,
	})
	manifest.DefineInitialMetatable(InitialMetatableAttachment{
		Primitive: InitialPrimitiveString, Metatable: DeclaredInitialRoot("ThingMetatable"),
	})

	encoded, err := Encode(manifest)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	reencoded, err := Encode(decoded)
	if err != nil {
		t.Fatalf("re-Encode: %v", err)
	}
	if string(encoded) != string(reencoded) {
		t.Fatalf("initial environment changed across round trip:\n%s\n%s", encoded, reencoded)
	}
	if len(decoded.InitialRoots) != 2 || len(decoded.InitialEntries) != 2 || len(decoded.InitialMetatables) != 1 {
		t.Fatalf("initial topology = roots:%d entries:%d metatables:%d", len(decoded.InitialRoots), len(decoded.InitialEntries), len(decoded.InitialMetatables))
	}
}

func TestInitialEnvironmentRejectsUnknownFunction(t *testing.T) {
	manifest := New("example")
	manifest.DefineInitialRoot(InitialRoot{Identity: "Methods", Aggregate: InitialAggregateTable})
	manifest.DefineInitialEntry(InitialEntry{
		Root: DeclaredInitialRoot("Methods"), Key: "missing",
		Value: InitialFunctionValue("Thing.missing"), Mutability: InitialMutable,
	})
	if _, err := Encode(manifest); err == nil {
		t.Fatal("Encode accepted an initial entry naming an undeclared function")
	}
}
