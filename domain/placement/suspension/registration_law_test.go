package suspension

import (
	"testing"

	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	callowner "github.com/wippyai/go-lua/domain/call/owner"
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
func (registrationLawPrincipals) CallPrincipal() *callowner.SchemaFragment   { return nil }
func (registrationLawPrincipals) EvidencePrincipal() *EvidenceFactorFragment { return nil }

func (registrationLawAuthorities) ValueAuthority() *valueowner.HotOwner { return nil }
func (registrationLawAuthorities) PlacementAuthority() *placementowner.HotOwner {
	return nil
}
func (registrationLawAuthorities) CallAuthority() *callowner.HotOwner { return nil }
func (registrationLawAuthorities) EvidenceAuthority() *EvidenceOwner  { return nil }
func (registrationLawAuthorities) ValueSchema() *valuedomain.Schema   { return nil }
func (registrationLawAuthorities) PlacementSchema() placementdomain.Schema {
	return placementdomain.Schema{}
}

func TestEvidenceRuleDeclaresItsActualFactorAuthority(t *testing.T) {
	spec := EvidenceRuleEntry[registrationLawPrincipals, registrationLawAuthorities]()
	if spec.Writes != "placement-suspension-evidence" || spec.Owner != "placement-suspension-evidence" {
		t.Fatalf("evidence rule declares writes=%q owner=%q, want its dedicated factor", spec.Writes, spec.Owner)
	}
	// The producer answers over one Program-issued subject-liveness span and
	// reads the Call fact solved at that span's boundary, so it is issued at a
	// mounted point rather than at the Link bootstrap.
	if spec.Lane != rule.LaneMounted || len(spec.Issues) != 1 {
		t.Fatalf("evidence rule lane=%v issues=%d, want one mounted subject-liveness issuance", spec.Lane, len(spec.Issues))
	}
	if issue := spec.Issues[0]; issue.Occurrence != "occurrence/subject-liveness" ||
		issue.Requirement != programissuance.RequirementUnrestricted || issue.Form != programissuance.FormCallSummary {
		t.Fatalf("evidence rule issuance=%+v, want the subject-liveness call summary", spec.Issues[0])
	}
}
