package proof

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestValueProofAdmissibleRejectsAnyClaimWithoutRuntimeProof(t *testing.T) {
	reg := registry()
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.AnyClaim))

	if New(reg, typevalue.NewCache()).ValueProofAdmissible(value, typ.String) {
		t.Fatalf("any claim without runtime proof satisfied string contract")
	}
}

func TestValueProofAdmissibleAcceptsAnyClaimForTopLikeContract(t *testing.T) {
	reg := registry()
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.Any)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.AnyClaim))

	proofs := New(reg, typevalue.NewCache())
	for _, want := range []typ.Type{
		typ.Any,
		typ.Unknown,
		typ.MaterializeOptional(typ.Any),
		typ.MaterializeOptional(typ.Unknown),
	} {
		if !proofs.ValueProofAdmissible(value, want) {
			t.Fatalf("explicit any should satisfy top-like contract %s", want)
		}
	}
}

func TestValueProofAdmissibleAcceptsRuntimeProof(t *testing.T) {
	reg := registry()
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.RuntimeClaim))

	if !New(reg, typevalue.NewCache()).ValueProofAdmissible(value, typ.String) {
		t.Fatalf("runtime proof did not satisfy string contract")
	}
}

func TestValueProofAdmissibleRejectsAbsentRuntimeProofForNonNilContract(t *testing.T) {
	reg := registry()
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Of(assertion.RuntimeClaim))
	value = product.WithPresence(reg, value, presence.Absent())

	if New(reg, typevalue.NewCache()).ValueProofAdmissible(value, typ.String) {
		t.Fatalf("absent runtime proof satisfied non-nil string contract")
	}
	if !New(reg, typevalue.NewCache()).ValueProofAdmissible(value, typ.MaterializeOptional(typ.String)) {
		t.Fatalf("absent runtime proof should satisfy optional string contract through nil")
	}
}

func TestValueProofAdmissibleFreshRecordWitnessUsesConstructorWidening(t *testing.T) {
	reg := registry()
	cache := typevalue.NewCache()
	methods := typ.MaterializeUnion([]typ.Type{
		typ.LiteralString("GET"),
		typ.LiteralString("POST"),
	})
	source := typetable.NewRecord().
		Field("headers", typetable.NewRecord().Build()).
		Field("method", typ.LiteralString("GET")).
		Field("path", typ.LiteralString("/users")).
		Build()
	target := typetable.NewRecord().
		Field("body", typ.MaterializeOptional(typ.Any)).
		Field("headers", typ.NewMap(typ.String, typ.String)).
		Field("method", methods).
		Field("path", typ.String).
		Build()
	value := cache.FromTypeWithWitness(reg, source)

	if New(reg, cache).ValueProofAdmissible(value, target) {
		t.Fatalf("non-fresh witness satisfied fresh-constructor contract")
	}

	value = product.Set(reg, value, escape.Key, escape.Fresh())
	if !New(reg, cache).ValueProofAdmissible(value, target) {
		t.Fatalf("fresh record witness did not satisfy fresh-constructor contract")
	}
}

func TestExplicitTopFreshRecordWitnessUsesConstructorWidening(t *testing.T) {
	reg := registry()
	cache := typevalue.NewCache()
	source := typetable.NewRecord().
		Field("next", typ.Nil).
		Field("value", typ.LiteralInt(2)).
		Build()
	target := typetable.NewRecord().
		Field("next", typ.MaterializeOptional(typ.Any)).
		Field("value", typ.Number).
		Build()
	value := cache.FromTypeWithWitness(reg, source)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, escape.Key, escape.Fresh())

	if !New(reg, cache).ValueProofAdmissible(value, target) {
		t.Fatalf("fresh explicit-top record witness did not satisfy widened structural contract")
	}
}

func TestExplicitTopAnyClaimFreshRecordWitnessDoesNotUseConstructorWidening(t *testing.T) {
	reg := registry()
	cache := typevalue.NewCache()
	source := typetable.NewRecord().
		Field("next", typ.Nil).
		Field("value", typ.LiteralInt(2)).
		Build()
	target := typetable.NewRecord().
		Field("next", typ.MaterializeOptional(typ.Any)).
		Field("value", typ.Number).
		Build()
	value := cache.FromTypeWithWitness(reg, source)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	value = product.Set(reg, value, assertion.Key, assertion.Any())
	value = product.Set(reg, value, escape.Key, escape.Fresh())

	if New(reg, cache).ValueProofAdmissible(value, target) {
		t.Fatalf("fresh explicit-any record witness satisfied structural contract")
	}
}

func TestValueTypeWithPresenceKeepsNilInMaybeValues(t *testing.T) {
	reg := registry()
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	value = product.WithPresence(reg, value, presence.Maybe())

	got, ok := New(reg, typevalue.NewCache()).ValueTypeWithPresence(value)
	if !ok {
		t.Fatalf("ValueTypeWithPresence returned false")
	}
	if got.String() != "string?" {
		t.Fatalf("type = %s, want string?", got)
	}
}

func TestRefineDeclaredTypeOptionalByPresentEvidence(t *testing.T) {
	reg := registry()
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(runtimekind.String))

	got, ok := New(reg, typevalue.NewCache()).RefineDeclaredType(typ.MaterializeOptional(typ.String), value)
	if !ok {
		t.Fatalf("RefineDeclaredType returned false")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RefineDeclaredType = %s, want string", got)
	}
}

func TestRefineDeclaredTypeProjectsRuntimeKindForUnknownDeclaredType(t *testing.T) {
	reg := registry()
	proofs := New(reg, typevalue.NewCache())
	for _, tc := range []struct {
		name string
		tag  runtimekind.Tag
		want kind.Kind
	}{
		{name: "table", tag: runtimekind.Table, want: kind.Map},
		{name: "function", tag: runtimekind.Function, want: kind.Function},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
			value = product.Set(reg, value, runtimekind.Key, runtimekind.Singleton(tc.tag))
			got, ok := proofs.RefineDeclaredType(typ.Unknown, value)
			if !ok {
				t.Fatalf("RefineDeclaredType returned false")
			}
			if got.Kind() != tc.want {
				t.Fatalf("RefineDeclaredType kind = %s, want %s (%v)", got.Kind(), tc.want, got)
			}
		})
	}
}

func TestRuntimeKindProjectionKeepsTableAndFunctionEvidence(t *testing.T) {
	reg := registry()
	tableValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	tableValue = product.Set(reg, tableValue, runtimekind.Key, runtimekind.Singleton(runtimekind.Table))
	if got, ok := RuntimeKindType(reg, tableValue, product.PresenceOf(tableValue)); !ok || got.Kind() != kind.Map {
		t.Fatalf("RuntimeKindType(table) = %v/%v, want map", got, ok)
	}

	tableUnion := typ.MaterializeUnion([]typ.Type{typ.String, typetable.BuiltinTopMarker()})
	withoutTable := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	withoutTable = product.Set(reg, withoutTable, runtimekind.Key, runtimekind.Top().Without(runtimekind.Table))
	got, ok := New(reg, typevalue.NewCache()).RefineDeclaredType(tableUnion, withoutTable)
	if !ok {
		t.Fatalf("RefineDeclaredType returned false")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RefineDeclaredType without table = %s, want string", got)
	}
}

func TestRuntimeKindReducedTypeNarrowsByRuntimeKindExclusion(t *testing.T) {
	reg := registry()
	proofs := New(reg, typevalue.NewCache())
	declared := typ.MaterializeUnion([]typ.Type{typ.Number, typ.String})
	excluded := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	excluded = product.Set(reg, excluded, runtimekind.Key, runtimekind.Top().Without(runtimekind.Number))

	got, ok := proofs.RuntimeKindReducedType(excluded, declared)
	if !ok {
		t.Fatalf("RuntimeKindReducedType returned false")
	}
	if !typ.TypeEquals(got, typ.String) {
		t.Fatalf("RuntimeKindReducedType = %s, want string", got)
	}
	if narrowed, ok := proofs.RuntimeKindReducedType(product.Top(), declared); ok {
		t.Fatalf("RuntimeKindReducedType narrowed top runtime-kind evidence to %v", narrowed)
	}
	if narrowed, ok := proofs.RuntimeKindReducedType(excluded, typ.String); ok {
		t.Fatalf("RuntimeKindReducedType narrowed non-union string to %v", narrowed)
	}
}

func TestVariantOriginTypeAndFullFamilyProjection(t *testing.T) {
	reg := registry()
	dog := typetable.NewRecord().
		Field("kind", typ.LiteralString("dog")).
		Field("bark", typ.String).
		Build()
	cat := typetable.NewRecord().
		Field("kind", typ.LiteralString("cat")).
		Field("meow", typ.String).
		Build()
	union := typ.MaterializeUnion([]typ.Type{dog, cat})
	family, cases, ok := variant.OriginOfType(union)
	if !ok {
		t.Fatal("missing union variant origin")
	}
	value := typevalue.NewCache().FromTypeWithWitness(reg, union)
	value = product.Set(reg, value, variantorigin.Key, variantorigin.Of(family, cases))
	proofs := New(reg, typevalue.NewCache())

	projected, ok := proofs.VariantOriginType(value)
	if !ok {
		t.Fatalf("VariantOriginType returned false")
	}
	if !typ.TypeEquals(projected, union) {
		t.Fatalf("VariantOriginType = %s, want full union %s", projected, union)
	}
	full, ok := proofs.FullVariantOriginType(value)
	if !ok {
		t.Fatalf("FullVariantOriginType returned false")
	}
	if !typ.TypeEquals(full, union) {
		t.Fatalf("FullVariantOriginType = %s, want full union %s", full, union)
	}
	if _, ok := proofs.VariantOriginType(product.Bottom(reg)); ok {
		t.Fatalf("VariantOriginType(bottom) returned true")
	}
}

func TestValueWitnessProvenMismatchRejectsMaybePresenceForNonNilContract(t *testing.T) {
	reg := registry()
	value := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	value = product.WithPresence(reg, value, presence.Maybe())

	if !New(reg, typevalue.NewCache()).ValueWitnessProvenMismatch(value, typ.String) {
		t.Fatalf("maybe string witness did not refute non-optional string contract")
	}
	if New(reg, typevalue.NewCache()).ValueWitnessProvenMismatch(value, typ.MaterializeOptional(typ.String)) {
		t.Fatalf("maybe string witness refuted optional string contract")
	}
}

func TestValueTypeProjectsExplicitTopBoundaryAsAny(t *testing.T) {
	reg := registry()
	value := typevalue.NewCache().FromType(reg, typ.Any)

	got, ok := New(reg, typevalue.NewCache()).ValueType(value)
	if !ok || !typ.TypeEquals(got, typ.Any) {
		t.Fatalf("ValueType(explicit top) = %v/%v, want any", got, ok)
	}
}

func TestValueHasExactIdentityOwnsPureIdentityProof(t *testing.T) {
	reg := registry()
	proofs := New(reg, typevalue.NewCache())
	value := identityvalue.WithExact(reg, product.Top(), identity.LuaTableLiteral(2, 7))

	if !proofs.ValueHasExactIdentity(value) {
		t.Fatalf("ValueHasExactIdentity(exact identity) = false, want true")
	}
	if proofs.ValueHasExactIdentity(product.Top()) {
		t.Fatalf("ValueHasExactIdentity(top) = true, want false")
	}
	if New(nil, typevalue.NewCache()).ValueHasExactIdentity(value) {
		t.Fatalf("ValueHasExactIdentity(nil registry) = true, want false")
	}
}

func TestTypeEvidenceUsableForInferenceHonorsRuntimeClaims(t *testing.T) {
	reg := registry()
	base := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)
	proofs := New(reg, typevalue.NewCache())

	runtimeValidated := product.Set(reg, base, evidence.Key, evidence.ExplicitTop())
	runtimeValidated = product.Set(reg, runtimeValidated, assertion.Key, assertion.Runtime())
	if !proofs.TypeEvidenceUsableForInference(runtimeValidated) {
		t.Fatalf("runtime-validated explicit top rejected for inference")
	}

	explicitAny := product.Set(reg, base, evidence.Key, evidence.ExplicitTop())
	explicitAny = product.Set(reg, explicitAny, assertion.Key, assertion.Any())
	if proofs.TypeEvidenceUsableForInference(explicitAny) {
		t.Fatalf("explicit any accepted for inference")
	}

	gradual := product.Set(reg, base, evidence.Key, evidence.GradualTop())
	if proofs.TypeEvidenceUsableForInference(gradual) {
		t.Fatalf("gradual top accepted for inference")
	}
}

func registry() *axis.Registry { return standard.Registry() }
