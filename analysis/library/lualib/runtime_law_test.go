package lualib

import (
	"encoding/hex"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
	"github.com/wippyai/go-lua/analysis/domain/type/ambient"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/library/contract"
	"github.com/wippyai/go-lua/analysis/module/signature/wire"
	"github.com/wippyai/go-lua/analysis/schema/library"
)

// The runtime type instance publishes the named types a type annotation
// resolves. These laws state what it publishes, that it publishes nothing else,
// and - while the ambient package still holds the same two declarations - that
// what THAT package holds is derived from this instance and held to it.

func runtimeInstance(t *testing.T) *contract.Instance {
	t.Helper()
	instance, ok := RuntimeContract(declaredKind(t, composite.LibraryContractKind))
	if !ok {
		t.Fatal("the runtime type contract was rejected by the declared library kind")
	}
	return instance
}

// publishedType decodes the type one runtime name is published as.
func publishedType(t *testing.T, name string) typ.Type {
	t.Helper()
	member, found := runtimeInstance(t).Resolve(library.FormExportType, contract.Export(name))
	if !found {
		t.Fatalf("the contract publishes no type at the address of %q", name)
	}
	if member.Encoding != contract.EncodingResolved {
		t.Fatalf("the published type of %q is deferred", name)
	}
	decoded, err := wire.DecodeExportType(member.Body)
	if err != nil {
		t.Fatalf("the published type of %q did not decode: %v", name, err)
	}
	return decoded
}

// TestRuntimeTypesAreAdmittedByTheDeclaredLibraryKind is the base case: the
// instance is admitted by the kind the analyzer declares, over that kind's own
// codec and payload format, and it carries one member per published name.
func TestRuntimeTypesAreAdmittedByTheDeclaredLibraryKind(t *testing.T) {
	kind := declaredKind(t, composite.LibraryContractKind)
	instance := runtimeInstance(t)
	if instance.Kind() != kind.Key() || instance.Codec() != kind.Codec() {
		t.Fatal("the instance is not published under the kind it was admitted against")
	}
	if instance.Class() != library.ClassLibrary {
		t.Fatal("the runtime types are not published as a library contract")
	}
	if instance.Root() != RuntimeRoot {
		t.Fatalf("mount selector is %q, want %q", instance.Root(), RuntimeRoot)
	}
	if instance.Count() != len(runtimeTypeExports) {
		t.Fatalf("rows=%d want %d", instance.Count(), len(runtimeTypeExports))
	}
	if instance.Deferred() != 0 {
		t.Fatalf("deferred rows=%d, and a published type is serializable today", instance.Deferred())
	}
	// A named type is the whole of what this contract says. It publishes no
	// value, so it claims no aggregate a mount would have to produce.
	for _, member := range instance.Members() {
		if member.Form != library.FormExportType {
			t.Fatalf("the runtime type contract carries the form %d", member.Form)
		}
	}
	if _, found := instance.Resolve(library.FormExportValue, contract.Root()); found {
		t.Fatal("the runtime type contract claims a root export value")
	}
	if _, ok := RuntimeContract(declaredKind(t, composite.EnvironmentContractKind)); ok {
		t.Fatal("the runtime types were authored as an environment contract")
	}
	if _, ok := RuntimeContract(nil); ok {
		t.Fatal("the runtime types were authored against no kind at all")
	}
}

// TestAmbientTypesAreDerivedFromTheAuthoredContract is the drift law, in the
// direction the retirement establishes. The published member is the statement of
// what a name resolves to; the ambient lookup that still answers the same names
// is derived from it here and held to it.
func TestAmbientTypesAreDerivedFromTheAuthoredContract(t *testing.T) {
	for name, spelling := range map[string]string{
		"Channel": ambient.Channel,
		"table":   ambient.Table,
	} {
		t.Run(name, func(t *testing.T) {
			if spelling != name {
				t.Fatalf("the ambient package spells the published name %q as %q", name, spelling)
			}
			published := publishedType(t, name)
			resolved, ok := ambient.Lookup(spelling)
			if !ok {
				t.Fatalf("the ambient lookup answers nothing for the published name %q", name)
			}
			if !typ.TypeEquals(resolved, published) {
				t.Fatalf("the ambient lookup answers %s and the contract publishes %s", resolved, published)
			}
		})
	}
	if !typ.IsBuiltinTableTopMarker(publishedType(t, "table")) {
		t.Fatal("the published table type is not the top of the table lattice")
	}
}

// TestPublishedChannelCarriesItsPayload is the content law of the channel
// declaration. The consumer that reads a channel reads the payload out of an
// instantiation of this generic, so an instantiation of the PUBLISHED generic
// must be the one it recognizes: a declaration that published a different arity,
// a different name, or a different body would leave that consumer answering
// nothing.
func TestPublishedChannelCarriesItsPayload(t *testing.T) {
	published := publishedType(t, "Channel")
	generic, isGeneric := published.(*typ.Generic)
	if !isGeneric {
		t.Fatalf("the published channel is %T, want a generic declaration", published)
	}
	if generic.Name != ambient.Channel || len(generic.TypeParams) != 1 {
		t.Fatalf("the published channel is %s, want one payload parameter", published)
	}
	if !ambient.IsRuntimeChannelName(generic.Name) {
		t.Fatalf("the published channel name %q is not the runtime channel ABI", generic.Name)
	}
	payload, ok := ambient.ChannelPayloadType(typ.Instantiate(generic, typ.String))
	if !ok || !typ.TypeEquals(payload, typ.String) {
		t.Fatalf("an instantiation of the published channel carries %v, want string", payload)
	}
}

// TestRuntimeTypeContractWireIsPinned holds the shipped instance's serialized
// bytes still. A published type is a data artifact: a name added, moved or
// readdressed, or a declaration whose shape changed, is a different contract, and
// this is where that shows.
func TestRuntimeTypeContractWireIsPinned(t *testing.T) {
	const pinned = "0f6d15027ced81445d7c13ec4c64c4fcf999739ee6272e5d7ba57a73ea4ec893"
	const pinnedSize = 440
	instance := runtimeInstance(t)
	data, err := contract.Encode(instance)
	if err != nil {
		t.Fatalf("the runtime type contract did not encode: %v", err)
	}
	if len(data) != pinnedSize {
		t.Errorf("contract wire is %d bytes, pinned %d", len(data), pinnedSize)
	}
	id := contract.ContentID(instance)
	if got := hex.EncodeToString(id[:]); got != pinned {
		t.Errorf("contract identity is %s, pinned %s", got, pinned)
	}
	decoded, err := contract.Decode(data, declaredTable(t))
	if err != nil {
		t.Fatalf("the runtime type contract did not decode: %v", err)
	}
	if contract.ContentID(decoded) != id {
		t.Fatal("the decoded contract is not the contract that was written")
	}
}
