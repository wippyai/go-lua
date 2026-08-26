package suspension

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
	placementowner "github.com/wippyai/go-lua/domain/placement/owner"
	valuedomain "github.com/wippyai/go-lua/domain/value"
	valueowner "github.com/wippyai/go-lua/domain/value/owner"
)

type registrationLawPrincipals struct{}
type registrationLawAuthorities struct{}

func (registrationLawPrincipals) ValuePrincipal() *valueowner.SchemaFragment { return nil }
func (registrationLawPrincipals) PlacementPrincipal() *placementowner.SchemaFragment {
	return nil
}
func (registrationLawPrincipals) EvidencePrincipal() *EvidenceFactorFragment { return nil }

func (registrationLawAuthorities) ValueAuthority() *valueowner.HotOwner { return nil }
func (registrationLawAuthorities) PlacementAuthority() *placementowner.HotOwner {
	return nil
}
func (registrationLawAuthorities) EvidenceAuthority() *EvidenceOwner { return nil }
func (registrationLawAuthorities) ValueSchema() *valuedomain.Schema  { return nil }
func (registrationLawAuthorities) PlacementSchema() placementdomain.Schema {
	return placementdomain.Schema{}
}

func TestEvidenceRuleDeclaresItsActualFactorAuthority(t *testing.T) {
	spec := EvidenceRuleEntry[registrationLawPrincipals, registrationLawAuthorities]()
	if spec.Writes != "placement-suspension-evidence" || spec.Owner != "placement-suspension-evidence" {
		t.Fatalf("evidence rule declares writes=%q owner=%q, want its dedicated factor", spec.Writes, spec.Owner)
	}
	if spec.Lane != rule.LaneLink || len(spec.Issues) != 0 {
		t.Fatalf("evidence rule lane=%v issues=%d, want Link-owned producer", spec.Lane, len(spec.Issues))
	}
}
