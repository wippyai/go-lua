package transformer

import (
	"bytes"
	"crypto/sha256"
	"reflect"
	"strings"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestGlobalBoundarySealsImmutableClassesAndExactDependencies(t *testing.T) {
	stdlib := globalID("stdlib-v1")
	runtime := globalID("runtime-v2")
	module := globalID("module-json-v7")
	boundary, err := SealGlobalBoundary(GlobalBoundaryComplete, []GlobalRootDescriptor{
		{Symbol: 30, Class: GlobalRootImportedAlias, StableName: "json", ContentID: module},
		{Symbol: 10, Class: GlobalRootImmutableStdlib, StableName: "pairs", ContentID: stdlib},
		{Symbol: 20, Class: GlobalRootRuntimeModule, StableName: "process", ContentID: runtime},
	})
	if err != nil {
		t.Fatal(err)
	}
	if index, ok := boundary.RootIndex(20); !ok || index != 1 {
		t.Fatalf("runtime root index = %d/%v", index, ok)
	}
	if got := boundary.Dependencies(); !reflect.DeepEqual(got, []GlobalContentID{module, runtime, stdlib}) {
		// Dependency order is byte order, not the labels above.
		want := append([]GlobalContentID(nil), got...)
		if len(want) != 3 || !containsGlobalID(want, module) || !containsGlobalID(want, runtime) || !containsGlobalID(want, stdlib) {
			t.Fatalf("dependencies = %x", got)
		}
	}
	if boundary.ContentID().zero() || len(boundary.CanonicalBytes()) == 0 {
		t.Fatal("boundary was not content sealed")
	}
	bindings, err := InstantiateGlobalBoundary(boundary, []GlobalRootBinding{
		{Symbol: 20, ContentID: runtime, Value: product.Top(), Path: pathdom.NewPath(20, "process"), HasPath: true},
		{Symbol: 10, ContentID: stdlib, Value: product.Top()},
		{Symbol: 30, ContentID: module, Value: product.Top()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Values()) != 3 || bindings.Paths()[1].Symbol != 20 {
		t.Fatalf("dense bindings = %#v/%#v", bindings.Values(), bindings.Paths())
	}
}

func TestGlobalBoundaryMutableUnknownAndIncompleteFailClosed(t *testing.T) {
	immutable := GlobalRootDescriptor{Symbol: 10, Class: GlobalRootImmutableStdlib, StableName: "pairs", ContentID: globalID("stdlib")}
	mutable := GlobalRootDescriptor{Symbol: 11, Class: GlobalRootMutableUnknown, StableName: "application-global"}
	immutableOnly, err := SealGlobalBoundary(GlobalBoundaryComplete, []GlobalRootDescriptor{immutable})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InstantiateGlobalBoundary(immutableOnly, []GlobalRootBinding{{Symbol: 10, ContentID: immutable.ContentID}}); err != nil {
		t.Fatalf("immutable-only boundary: %v", err)
	}
	boundary, err := SealGlobalBoundary(GlobalBoundaryComplete, []GlobalRootDescriptor{immutable, mutable})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InstantiateGlobalBoundary(boundary, []GlobalRootBinding{{Symbol: 10, ContentID: immutable.ContentID}, {Symbol: 11}}); err == nil || !strings.Contains(err.Error(), "mutable or unknown") {
		t.Fatalf("mutable instantiation error = %v", err)
	}
	incomplete, err := SealGlobalBoundary(GlobalBoundaryIncomplete, []GlobalRootDescriptor{immutable})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InstantiateGlobalBoundary(incomplete, []GlobalRootBinding{{Symbol: 10, ContentID: immutable.ContentID}}); err == nil || !strings.Contains(err.Error(), "not complete") {
		t.Fatalf("incomplete instantiation error = %v", err)
	}
}

func TestGlobalBoundaryRejectsMissingOrStaleArtifactIdentity(t *testing.T) {
	if _, err := SealGlobalBoundary(GlobalBoundaryComplete, []GlobalRootDescriptor{{Symbol: 1, Class: GlobalRootImportedAlias, StableName: "json"}}); err == nil || !strings.Contains(err.Error(), "no dependency content identity") {
		t.Fatalf("missing content identity error = %v", err)
	}
	content := globalID("json-v1")
	boundary, err := SealGlobalBoundary(GlobalBoundaryComplete, []GlobalRootDescriptor{{Symbol: 1, Class: GlobalRootImportedAlias, StableName: "json", ContentID: content}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InstantiateGlobalBoundary(boundary, []GlobalRootBinding{{Symbol: 1, ContentID: globalID("json-v2")}}); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("stale content identity error = %v", err)
	}
}

func TestGlobalBoundaryCanonicalIdentityIgnoresInputOrderAndBindingsDetach(t *testing.T) {
	first := GlobalRootDescriptor{Symbol: 1, Class: GlobalRootImmutableStdlib, StableName: "pairs", ContentID: globalID("stdlib")}
	second := GlobalRootDescriptor{Symbol: 2, Class: GlobalRootImportedAlias, StableName: "json", ContentID: globalID("json")}
	left, err := SealGlobalBoundary(GlobalBoundaryComplete, []GlobalRootDescriptor{first, second})
	if err != nil {
		t.Fatal(err)
	}
	right, err := SealGlobalBoundary(GlobalBoundaryComplete, []GlobalRootDescriptor{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left.CanonicalBytes(), right.CanonicalBytes()) || left.ContentID() != right.ContentID() {
		t.Fatal("input order changed global boundary artifact")
	}
	path := pathdom.NewPath(symbol.ID(1), "pairs")
	bindings, err := InstantiateGlobalBoundary(left, []GlobalRootBinding{{Symbol: 1, ContentID: first.ContentID, Path: path, HasPath: true}, {Symbol: 2, ContentID: second.ContentID}})
	if err != nil {
		t.Fatal(err)
	}
	returned := bindings.Paths()
	returned[0] = pathdom.Path{}
	if bindings.Paths()[0].Symbol != 1 {
		t.Fatal("returned paths alias sealed dense bindings")
	}
}

func globalID(value string) GlobalContentID { return sha256.Sum256([]byte(value)) }

func containsGlobalID(values []GlobalContentID, target GlobalContentID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
