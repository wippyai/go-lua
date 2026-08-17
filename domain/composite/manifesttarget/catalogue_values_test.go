package manifesttarget_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
)

func TestNewProviderValuesMountWithoutTargetRegistration(t *testing.T) {
	providers := append(stdlib.Providers(), manifest.Provider{
		Identity: "custom.constants", Mount: manifest.MountModule, Immutable: true,
		Declaration: func() *manifestwire.Manifest {
			declaration := manifestwire.New("custom")
			declaration.SetExport(typetable.NewRecord().
				ReadonlyField("answer", typ.LiteralInt(42)).
				ReadonlyField("state", typ.NewMap(typ.String, typ.Any)).Build())
			return declaration
		},
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		t.Fatal(err)
	}
	contract, err := manifesttarget.SealCatalogue(catalogue)
	if err != nil {
		t.Fatal(err)
	}

	root := initialRootByIdentity(t, contract, "ModuleRoot:custom.constants")
	answer, mutability, ok := contract.InitialEntry(root, exactStringKey(t, contract, "answer"))
	if value, valueOK := contract.InitialValueInteger(answer); !ok || !valueOK || value != 42 || mutability != target.InitialFrozen {
		t.Fatalf("custom.answer = value:%d mutability:%d present:%t typed:%t", value, mutability, ok, valueOK)
	}
	state, mutability, ok := contract.InitialEntry(root, exactStringKey(t, contract, "state"))
	if kind, kindOK := contract.InitialValueKind(state); !ok || !kindOK || kind != target.InitialValueAbsent || mutability != target.InitialFrozen {
		t.Fatalf("custom.state = kind:%d mutability:%d present:%t typed:%t", kind, mutability, ok, kindOK)
	}
}

func initialRootByIdentity(t *testing.T, contract *target.Contract, identity string) target.InitialRoot {
	t.Helper()
	for index := 0; index < contract.InitialRootCount(); index++ {
		root, ok := contract.InitialRootAt(index)
		got, identityOK := contract.InitialRootIdentity(root)
		if ok && identityOK && got == identity {
			return root
		}
	}
	t.Fatalf("missing initial root %q", identity)
	return 0
}

func exactStringKey(t *testing.T, contract *target.Contract, value string) target.ExactKey {
	t.Helper()
	for index := 0; index < contract.ExactKeyCount(); index++ {
		key, ok := contract.ExactKeyAt(index)
		literal, literalOK := contract.ExactKeyValue(key)
		if ok && literalOK && literal == (keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value}) {
			return key
		}
	}
	t.Fatalf("missing exact key %q", value)
	return 0
}
