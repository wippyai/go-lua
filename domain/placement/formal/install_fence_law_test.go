package formal

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

// formalOwnerSemantic seals one distinct owner semantic key per fixture
// principal, which is all a declaration needs to issue an owner here.
func formalOwnerSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0x66, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("formal owner semantic")
	}
	return key
}

// formalAuthorities is the composition record this rule's family install arm
// reads: the three mounted owners the declaration names, and the Pack schema
// its route derivation is sealed against.
type formalAuthorities struct {
	placement *placementowner.HotOwner
	values    *valueowner.HotOwner
	calls     *callowner.HotOwner
	packs     *packdomain.Schema
}

func (authorities formalAuthorities) PlacementAuthority() *placementowner.HotOwner {
	return authorities.placement
}

func (authorities formalAuthorities) ValueAuthority() *valueowner.HotOwner {
	return authorities.values
}

func (authorities formalAuthorities) CallAuthority() *callowner.HotOwner {
	return authorities.calls
}

func (authorities formalAuthorities) PackSchema() *packdomain.Schema { return authorities.packs }

// formalAuthorityJoin seals one composition's three mounted owners over one
// fixture's schemas and answers the binding that issued them.
func formalAuthorityJoin(t testing.TB, fixture opaqueDispatchLawFixture) (formalAuthorities, *engine.SchemaBinding) {
	t.Helper()
	builder := engine.NewSchema()
	callFragment, callOK := callowner.DeclareSchema(builder, formalOwnerSemantic(31))
	valueFragment, valueOK := valueowner.DeclareSchema(builder, formalOwnerSemantic(32), formalOwnerSemantic(33), formalOwnerSemantic(34))
	placementFragment, placementOK := placementowner.DeclareSchema(builder, formalOwnerSemantic(35), formalOwnerSemantic(36))
	cold, coldOK := builder.Seal()
	if !callOK || !valueOK || !placementOK || !coldOK || cold == nil {
		t.Fatalf("formal owner declaration call=%t value=%t placement=%t cold=%t", callOK, valueOK, placementOK, coldOK)
	}
	binding := engine.NewSchemaBinding(cold)
	callHot, callHotOK := callowner.BindHot(binding, callFragment, fixture.calls)
	valueHot, valueHotOK := valueowner.BindHot(binding, valueFragment, fixture.values)
	placementHot, placementHotOK := placementowner.BindHot(binding, placementFragment, fixture.placement)
	if !callHotOK || !valueHotOK || !placementHotOK {
		t.Fatalf("formal owner bind call=%t value=%t placement=%t", callHotOK, valueHotOK, placementHotOK)
	}
	return formalAuthorities{placement: placementHot, values: valueHot, calls: callHot, packs: fixture.packs}, binding
}

// TestFormalFamilyInstallsOnlyAgainstTheLinkAuthorityJoinItWasIssuedBy is the
// install-time fence the deleted hot binder used to carry, and which the
// composite Placement binder law proved while this rule was still on that
// protocol. Two structurally equal compositions issue two owner sets, and the
// family is sealed against the schemas of exactly one of them; the Pack schema
// must belong to the same Link owner Call's algebra names, because that owner
// is the join every formal ownership row this rule reads is authenticated
// under.
func TestFormalFamilyInstallsOnlyAgainstTheLinkAuthorityJoinItWasIssuedBy(t *testing.T) {
	fixture := newOpaqueDispatchLawFixture(t, "formal-family-install")
	local, localBinding := formalAuthorityJoin(t, fixture)
	foreignFixture := newOpaqueDispatchLawFixture(t, "formal-family-install-foreign")
	foreign, _ := formalAuthorityJoin(t, foreignFixture)

	if !local.placement.MatchesBinding(localBinding) || local.placement.MatchesBinding(nil) {
		t.Fatal("a Placement owner did not answer for the binding that issued it")
	}
	for _, law := range []struct {
		name        string
		binding     *engine.SchemaBinding
		authorities formalAuthorities
	}{
		{name: "foreign-placement", binding: localBinding, authorities: formalAuthorities{placement: foreign.placement, values: local.values, calls: local.calls, packs: local.packs}},
		{name: "foreign-value", binding: localBinding, authorities: formalAuthorities{placement: local.placement, values: foreign.values, calls: local.calls, packs: local.packs}},
		{name: "foreign-call", binding: localBinding, authorities: formalAuthorities{placement: local.placement, values: local.values, calls: foreign.calls, packs: local.packs}},
		{name: "foreign-pack", binding: localBinding, authorities: formalAuthorities{placement: local.placement, values: local.values, calls: local.calls, packs: foreign.packs}},
		{name: "absent-pack", binding: localBinding, authorities: formalAuthorities{placement: local.placement, values: local.values, calls: local.calls}},
		{name: "absent-binding", authorities: local},
	} {
		if InstallFamily(law.binding, nil, law.authorities) {
			t.Fatalf("the family installed under %s", law.name)
		}
	}
}
