package relationadmission_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/relationadmission"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

const registryLawDomain = "analysis/program/relationadmission/registry-law/v1"

func TestRegistrySealsMixedAscendingAndEquatableOnlyAuthorities(t *testing.T) {
	ascending := registryLawType(t, "ascending")
	equatable := registryLawType(t, "equatable-only")
	unknown := registryLawType(t, "unknown")
	algebra := &lawAlgebra{typeID: ascending}
	equality := &lawEquality{typeID: equatable}

	registry, ok := relationadmission.NewRegistry(
		[]binding.ValueAlgebra{algebra},
		[]binding.ValueEquality{equality},
	)
	if !ok || !registry.Available() {
		t.Fatal("mixed owner authorities were not sealed")
	}
	if got, ok := registry.Resolve(ascending); !ok || got != algebra {
		t.Fatalf("Resolve(ascending) = (%T, %v), want exact algebra", got, ok)
	}
	if got, ok := registry.ResolveEquality(equatable); !ok || got != equality {
		t.Fatalf("ResolveEquality(equatable-only) = (%T, %v), want exact equality", got, ok)
	}

	// The equatable-only owner must not acquire ascent authority, and the
	// ascending owner must not acquire equality merely because an algebra is
	// present. Both are nominal TypeID lookups with no fallback.
	if _, ok := registry.Resolve(equatable); ok {
		t.Fatal("equatable-only TypeID unexpectedly resolved an algebra")
	}
	if _, ok := registry.ResolveEquality(ascending); ok {
		t.Fatal("ascending TypeID unexpectedly received fabricated equality")
	}
	if _, ok := registry.Resolve(unknown); ok {
		t.Fatal("unknown TypeID unexpectedly resolved an algebra")
	}
	if _, ok := registry.ResolveEquality(unknown); ok {
		t.Fatal("unknown TypeID unexpectedly resolved equality")
	}
	if _, ok := registry.Resolve(model.TypeID{}); ok {
		t.Fatal("unavailable TypeID unexpectedly resolved an algebra")
	}
	if _, ok := registry.ResolveEquality(model.TypeID{}); ok {
		t.Fatal("unavailable TypeID unexpectedly resolved equality")
	}
}

func TestRegistryAcceptsExplicitEqualityForAscendingType(t *testing.T) {
	typeID := registryLawType(t, "ascending-with-explicit-equality")
	algebra := &lawAlgebra{typeID: typeID}
	equality := &lawEquality{typeID: typeID}
	registry, ok := relationadmission.NewRegistry(
		[]binding.ValueAlgebra{algebra},
		[]binding.ValueEquality{equality},
	)
	if !ok || !registry.Available() {
		t.Fatal("explicit equality for ascending TypeID was refused")
	}
	if got, ok := registry.Resolve(typeID); !ok || got != algebra {
		t.Fatal("registry did not retain the exact ascending algebra")
	}
	if got, ok := registry.ResolveEquality(typeID); !ok || got != equality {
		t.Fatal("registry did not retain the exact explicit equality")
	}
}

func TestRegistryRefusesDuplicateAuthorities(t *testing.T) {
	typeID := registryLawType(t, "duplicate")
	firstAlgebra := &lawAlgebra{typeID: typeID}
	secondAlgebra := &lawAlgebra{typeID: typeID}
	if _, ok := relationadmission.NewRegistry([]binding.ValueAlgebra{firstAlgebra, secondAlgebra}, nil); ok {
		t.Fatal("duplicate algebra TypeID was accepted")
	}

	firstEquality := &lawEquality{typeID: typeID}
	secondEquality := &lawEquality{typeID: typeID}
	if _, ok := relationadmission.NewRegistry(nil, []binding.ValueEquality{firstEquality, secondEquality}); ok {
		t.Fatal("duplicate equality TypeID was accepted")
	}
}

func TestRegistryRefusesNilAndUnavailableAuthorities(t *testing.T) {
	typeID := registryLawType(t, "valid")
	var nilAlgebra *lawAlgebra
	if _, ok := relationadmission.NewRegistry([]binding.ValueAlgebra{nilAlgebra}, nil); ok {
		t.Fatal("typed-nil algebra was accepted")
	}
	var nilEquality *lawEquality
	if _, ok := relationadmission.NewRegistry(nil, []binding.ValueEquality{nilEquality}); ok {
		t.Fatal("typed-nil equality was accepted")
	}
	if _, ok := relationadmission.NewRegistry([]binding.ValueAlgebra{nil}, nil); ok {
		t.Fatal("nil algebra was accepted")
	}
	if _, ok := relationadmission.NewRegistry(nil, []binding.ValueEquality{nil}); ok {
		t.Fatal("nil equality was accepted")
	}
	if _, ok := relationadmission.NewRegistry([]binding.ValueAlgebra{&lawAlgebra{}}, nil); ok {
		t.Fatal("unavailable algebra TypeID was accepted")
	}
	if _, ok := relationadmission.NewRegistry(nil, []binding.ValueEquality{&lawEquality{}}); ok {
		t.Fatal("unavailable equality TypeID was accepted")
	}
	if _, ok := relationadmission.NewRegistry([]binding.ValueAlgebra{&lawAlgebra{typeID: typeID}, &lawAlgebra{typeID: model.TypeID{}}}, nil); ok {
		t.Fatal("mixed valid/unavailable algebra entries were accepted")
	}
}

func TestRegistrySnapshotsOwnerSlicesAndReturnsExactValues(t *testing.T) {
	typeID := registryLawType(t, "snapshot")
	algebra := &lawAlgebra{typeID: typeID}
	equality := &lawEquality{typeID: typeID}
	algebras := []binding.ValueAlgebra{algebra}
	equalities := []binding.ValueEquality{equality}
	registry, ok := relationadmission.NewRegistry(algebras, equalities)
	if !ok {
		t.Fatal("valid authorities were not sealed")
	}
	algebras[0] = &lawAlgebra{typeID: registryLawType(t, "replaced-algebra")}
	equalities[0] = &lawEquality{typeID: registryLawType(t, "replaced-equality")}
	if got, ok := registry.Resolve(typeID); !ok || got != algebra {
		t.Fatal("registry retained caller slice storage for algebra")
	}
	if got, ok := registry.ResolveEquality(typeID); !ok || got != equality {
		t.Fatal("registry retained caller slice storage for equality")
	}
}

type lawAlgebra struct{ typeID model.TypeID }

func (algebra *lawAlgebra) Type() model.TypeID {
	if algebra == nil {
		return model.TypeID{}
	}
	return algebra.typeID
}

func (algebra *lawAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return binding.ValueToken{}, false
}

func (algebra *lawAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return binding.ValueToken{}, false
}

func (algebra *lawAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	return false
}

type lawEquality struct{ typeID model.TypeID }

func (equality *lawEquality) Type() model.TypeID {
	if equality == nil {
		return model.TypeID{}
	}
	return equality.typeID
}

func (equality *lawEquality) Equal(left, right binding.ValueToken) bool {
	return false
}

func registryLawType(t *testing.T, label string) model.TypeID {
	t.Helper()
	content, ok := identity.DeriveContentID(registryLawDomain, []byte(label))
	if !ok {
		t.Fatal("registry law content")
	}
	ownerContent, ok := identity.DeriveContentID(registryLawDomain, []byte("owner"))
	if !ok {
		t.Fatal("registry law owner content")
	}
	owner, ok := model.IssueOwnerID(ownerContent)
	if !ok {
		t.Fatal("registry law owner")
	}
	typeID, ok := model.IssueTypeID(owner, content)
	if !ok {
		t.Fatal("registry law TypeID")
	}
	return typeID
}
