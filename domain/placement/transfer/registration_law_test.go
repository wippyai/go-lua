package transfer

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
	packdomain "github.com/wippyai/go-lua/domain/pack"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type transferRegistrationPrincipals struct {
	value     *valueowner.SchemaFragment
	call      *callowner.SchemaFragment
	placement *placementowner.SchemaFragment
}

func (set transferRegistrationPrincipals) ValuePrincipal() *valueowner.SchemaFragment {
	return set.value
}
func (set transferRegistrationPrincipals) CallPrincipal() *callowner.SchemaFragment {
	return set.call
}
func (set transferRegistrationPrincipals) PlacementPrincipal() *placementowner.SchemaFragment {
	return set.placement
}

type transferRegistrationAuthorities struct{}

func (transferRegistrationAuthorities) ValueAuthority() *valueowner.HotOwner { return nil }
func (transferRegistrationAuthorities) CallAuthority() *callowner.HotOwner   { return nil }
func (transferRegistrationAuthorities) PlacementAuthority() *placementowner.HotOwner {
	return nil
}
func (transferRegistrationAuthorities) TargetContract() *contract.Contract { return nil }
func (transferRegistrationAuthorities) PackSchema() *packdomain.Schema     { return nil }

func transferRegistrationSemantic(seed byte) identity.SemanticKey {
	digest := sha256.Sum256([]byte{0x74, seed})
	key, ok := identity.NewSemanticKey(digest, 1)
	if !ok {
		panic("transfer registration semantic")
	}
	return key
}

func TestPlacementTransferRuleDeclaresMountedCallEffectAndNoEffectPrincipal(t *testing.T) {
	spec := RuleEntry[transferRegistrationPrincipals, transferRegistrationAuthorities]()
	if spec.Key != "placement-transfer" || spec.Writes != "placement" || spec.Owner != "placement" || spec.Lane != rule.LaneMounted {
		t.Fatalf("transfer rule key/writes/owner/lane = %q/%q/%q/%v", spec.Key, spec.Writes, spec.Owner, spec.Lane)
	}
	if len(spec.Issues) != 1 || spec.Issues[0].Occurrence != "occurrence/call" || spec.Issues[0].Form != "program-form/call-effect" || spec.Issues[0].Requirement != "program-requirement/unrestricted" {
		t.Fatalf("transfer rule issues = %#v, want exact unrestricted call-effect issuance", spec.Issues)
	}
	if len(spec.Roles) != 1 || spec.Roles[0] != "semantic/operand/placement/transfer" {
		t.Fatalf("transfer rule roles = %#v", spec.Roles)
	}
}

func TestPlacementTransferSchemaDeclaresCallValuePlacementRoute(t *testing.T) {
	builder := engine.NewSchema()
	calls, callsOK := callowner.DeclareSchema(builder, transferRegistrationSemantic(1))
	values, valuesOK := valueowner.DeclareSchema(builder, transferRegistrationSemantic(2), transferRegistrationSemantic(3), transferRegistrationSemantic(4))
	placement, placementOK := placementowner.DeclareSchema(builder, transferRegistrationSemantic(5), transferRegistrationSemantic(6))
	fragment, fragmentOK := DeclareSchema(builder, transferRegistrationSemantic(7), transferRegistrationSemantic(8), values, calls, placement)
	sealed, sealedOK := builder.Seal()
	if !callsOK || !valuesOK || !placementOK || !fragmentOK || !sealedOK || sealed == nil || fragment == nil || fragment.RuleSlot() == nil {
		t.Fatalf("transfer schema declaration calls=%t values=%t placement=%t fragment=%t sealed=%t", callsOK, valuesOK, placementOK, fragmentOK, sealedOK)
	}
}
