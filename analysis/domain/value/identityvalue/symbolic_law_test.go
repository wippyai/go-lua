package identityvalue

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/formal"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
)

func symbolicRoot(seed string, ordinal uint64, vocabulary formal.Vocabulary) formal.Root {
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte(seed)), 1)
	return formal.NewRoot(body, ordinal, vocabulary)
}

func symbolicTemplate(seed string, allocation, object uint32) identity.AllocationTemplate {
	body := lexicalidentity.FunctionBody(lexicalidentity.UnitNamespaceFromContent([]byte(seed)), 1)
	return identity.ManifestAllocationTemplate(body, allocation, object)
}

func mustSymbolicRegistry(t testing.TB) *Registry {
	t.Helper()
	registry, err := NewRegistry()
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return registry
}

func mustFormalValue(t testing.TB, registry *Registry, seed string, ordinal uint64) Value {
	t.Helper()
	value, ok := registry.Formalize(symbolicRoot(seed, ordinal, formal.Input))
	if !ok {
		t.Fatalf("Formalize(%q, %d) rejected", seed, ordinal)
	}
	return value
}

func TestSymbolicSubstitutionFencesOwnerAndNeverInstantiatesAllocation(t *testing.T) {
	registry := mustSymbolicRegistry(t)
	foreign := mustSymbolicRegistry(t)
	variable := symbolicRoot("symbolic-substitution", 1, formal.Input)
	formalValue, ok := registry.Formalize(variable)
	if !ok {
		t.Fatal("Formalize rejected valid variable")
	}
	image := identity.Singleton(identity.ID{Kind: "test.object", Site: "symbolic-substitution", Index: 1})
	substitution, ok := NewSubstitution(registry, []Binding{{Variable: variable, Image: image}})
	if !ok {
		t.Fatal("NewSubstitution rejected valid binding")
	}
	resolved, ok := substitution.Apply(formalValue)
	if !ok {
		t.Fatal("Apply rejected bound formal")
	}
	term, ok := resolved.Term()
	if !ok || term.Kind() != identity.TermConcrete {
		t.Fatalf("resolved term = %v/%t, want concrete/true", term, ok)
	}

	otherFormal := mustFormalValue(t, registry, "symbolic-unbound", 2)
	if _, ok := substitution.Apply(otherFormal); ok {
		t.Fatal("Apply instantiated an unbound formal")
	}
	allocation, ok := registry.Allocation(symbolicTemplate("symbolic-allocation", 1, 1))
	if !ok {
		t.Fatal("Allocation rejected valid template")
	}
	if _, ok := substitution.Apply(allocation); ok {
		t.Fatal("Apply instantiated an allocation template")
	}
	foreignFormal := mustFormalValue(t, foreign, "symbolic-substitution", 1)
	if _, ok := substitution.Apply(foreignFormal); ok {
		t.Fatal("Apply accepted a value from a foreign registry")
	}
	if _, ok := NewSubstitution(nil, nil); ok {
		t.Fatal("NewSubstitution accepted a nil registry")
	}
}

func TestSymbolicCanonicalReplayFreshAuthorityAndBytes(t *testing.T) {
	ctx := context.Background()
	leftRegistry := mustSymbolicRegistry(t)
	rightRegistry := mustSymbolicRegistry(t)
	left := mustFormalValue(t, leftRegistry, "symbolic-canonical", 1)
	right := mustFormalValue(t, rightRegistry, "symbolic-canonical", 1)
	if Equal(left, right) {
		t.Fatal("Equal accepted values from distinct fresh authorities")
	}
	equal, err := CanonicalEqual(ctx, left, right)
	if err != nil || !equal {
		t.Fatalf("CanonicalEqual = %t, %v; want true", equal, err)
	}
	artifact, err := left.Canonical(ctx)
	if err != nil {
		t.Fatalf("Canonical: %v", err)
	}
	replayed, err := artifact.Replay(ctx, rightRegistry)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if !Equal(right, replayed) {
		t.Fatal("replayed value is not hot-equal under its receiving authority")
	}
	if Equal(left, replayed) {
		t.Fatal("replayed value bypassed the source authority fence")
	}
	var malformed Value
	if malformed.Valid() {
		t.Fatal("zero Value reported valid")
	}
	if _, err := malformed.Canonical(ctx); err == nil {
		t.Fatal("zero Value produced a canonical artifact")
	}
	if _, ok := malformed.WidenRank(); ok {
		t.Fatal("zero Value produced a widening rank")
	}
	if _, ok := Join(malformed, right); ok {
		t.Fatal("Join accepted a zero Value")
	}
	if _, err := (CanonicalArtifact{}).Replay(ctx, rightRegistry); err == nil {
		t.Fatal("zero canonical artifact replayed")
	}
	if len(artifact.Bytes()) == 0 || artifact.SchemaIdentity() == (axis.SchemaIdentity{}) {
		t.Fatal("canonical artifact did not publish bytes and schema identity")
	}
	thirdRegistry, fresh, err := Freshen(ctx, left)
	if err != nil {
		t.Fatalf("Freshen: %v", err)
	}
	if Equal(left, fresh) || !Equal(fresh, mustFormalValue(t, thirdRegistry, "symbolic-canonical", 1)) {
		t.Fatal("Freshen did not replace authority while preserving value")
	}
	if bytesEqual, err := CanonicalEqual(ctx, left, fresh); err != nil || !bytesEqual {
		t.Fatalf("fresh canonical bytes = %t, %v; want equal", bytesEqual, err)
	}
}

func TestSymbolicAlgebraAndRankParityAfterCanonicalReplay(t *testing.T) {
	ctx := context.Background()
	first := mustSymbolicRegistry(t)
	second := mustSymbolicRegistry(t)
	formalLeft := mustFormalValue(t, first, "symbolic-algebra-left", 1)
	concrete, ok := first.Concrete(identity.ID{Kind: "test.object", Site: "symbolic-algebra", Index: 3})
	if !ok {
		t.Fatal("Concrete rejected valid identity")
	}
	leftArtifact, err := formalLeft.Canonical(ctx)
	if err != nil {
		t.Fatalf("left Canonical: %v", err)
	}
	rightArtifact, err := concrete.Canonical(ctx)
	if err != nil {
		t.Fatalf("right Canonical: %v", err)
	}
	left, err := leftArtifact.Replay(ctx, second)
	if err != nil {
		t.Fatalf("left Replay: %v", err)
	}
	right, err := rightArtifact.Replay(ctx, second)
	if err != nil {
		t.Fatalf("right Replay: %v", err)
	}
	wantJoin, ok := Join(formalLeft, concrete)
	if !ok {
		t.Fatal("Join rejected same-authority values")
	}
	gotJoin, ok := Join(left, right)
	if !ok {
		t.Fatal("replayed Join rejected same-authority values")
	}
	if parity, err := CanonicalEqual(ctx, wantJoin, gotJoin); err != nil || !parity {
		t.Fatalf("Join replay parity = %t, %v; want true", parity, err)
	}
	wantOrder := LessOrEq(formalLeft, wantJoin)
	gotOrder := LessOrEq(left, gotJoin)
	if !wantOrder || wantOrder != gotOrder {
		t.Fatalf("LessOrEq replay parity = %t/%t, want true/true", wantOrder, gotOrder)
	}
	wantWiden, ok := Widen(formalLeft, concrete)
	if !ok {
		t.Fatal("Widen rejected same-authority values")
	}
	gotWiden, ok := Widen(left, right)
	if !ok {
		t.Fatal("replayed Widen rejected same-authority values")
	}
	if parity, err := CanonicalEqual(ctx, wantWiden, gotWiden); err != nil || !parity {
		t.Fatalf("Widen replay parity = %t, %v; want true", parity, err)
	}
	wantRank, ok := formalLeft.WidenRank()
	if !ok {
		t.Fatal("WidenRank rejected valid value")
	}
	gotRank, ok := left.WidenRank()
	if !ok || wantRank != gotRank {
		t.Fatalf("WidenRank replay = %d/%t, want %d/true", gotRank, ok, wantRank)
	}
	if _, ok := Join(formalLeft, left); ok {
		t.Fatal("Join accepted cross-authority operands")
	}
	if _, ok := Widen(formalLeft, left); ok {
		t.Fatal("Widen accepted cross-authority operands")
	}
	if LessOrEq(formalLeft, left) {
		t.Fatal("LessOrEq accepted cross-authority operands")
	}
}
