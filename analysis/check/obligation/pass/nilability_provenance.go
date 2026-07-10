package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// appendNilabilityProvenance projects solved nilability causes into the common
// judgment vocabulary. Renderers can therefore use the same causal chain for
// operators, arguments, member receivers, and returns without inspecting AST
// or rebuilding flow facts.
func appendNilabilityProvenance(
	evidence judgment.EvidenceChain,
	point cfg.Point,
	file string,
	subject string,
	primary readmodel.SourceSpan,
	provenance readmodel.NilabilityProvenance,
) judgment.EvidenceChain {
	if provenance.CallResult.Present {
		detail := judgment.CallResultAssignmentEvidenceDetail(provenance.CallResult.CallableName, provenance.CallResult.ResultIndex)
		if provenance.CallResult.UnderSupplied {
			detail = judgment.UnderSuppliedCallResultAssignmentEvidenceDetail(provenance.CallResult.CallableName, provenance.CallResult.ResultIndex)
		}
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: detail,
			Origin: judgment.OriginRef{Point: point, Key: "nilability:call-result"},
			Span:   spanFromReadModel(file, primary),
		})
		if provenance.CallResult.ReturnSpan.Valid() {
			evidence = append(evidence, judgment.Evidence{
				Kind:   judgment.EvidenceUserAssertion,
				Trust:  judgment.EvidenceTrustClaimed,
				Detail: detail,
				Origin: judgment.OriginRef{Point: point, Key: "nilability:return-contract"},
				Span:   spanFromReadModel(file, provenance.CallResult.ReturnSpan),
			})
		}
	}
	if provenance.OptionalField {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailMayBeNil,
				Cause:        judgment.EvidenceCause{Kind: judgment.EvidenceCauseBirth},
				SubjectLabel: subject,
			},
			Origin: judgment.OriginRef{Point: point, Key: "nilability:optional-field"},
			Span:   spanFromReadModel(file, primary),
		})
	}
	for i, access := range provenance.NilableAccesses {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailMayBeNil,
				Cause:        judgment.EvidenceCause{Kind: judgment.EvidenceCauseFlowAssign},
				SubjectLabel: access.Label,
				Field:        access.Access,
			},
			Origin: judgment.OriginRef{Point: point, Key: fmt.Sprintf("nilability:access:%d", i)},
			Span:   spanFromReadModel(file, access.Span),
		})
	}
	if provenance.ExplicitTopOrigin || provenance.UntrustedTopOrigin {
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceUserAssertion,
			Trust:  judgment.EvidenceTrustClaimed,
			Detail: judgment.UserAssertedAnyEvidenceDetail(subject),
			Origin: judgment.OriginRef{Point: point, Key: "nilability:any"},
			Span:   spanFromReadModel(file, primary),
		})
	}
	evidence = append(evidence, judgment.Evidence{
		Kind:  judgment.EvidenceMissingProof,
		Trust: judgment.EvidenceTrustUnknown,
		Detail: judgment.EvidenceDetail{
			Kind:         judgment.EvidenceDetailMayBeNil,
			Cause:        judgment.EvidenceCause{Kind: judgment.EvidenceCauseMissingProof},
			SubjectLabel: subject,
		},
		Origin: judgment.OriginRef{Point: point, Key: "nilability:guard"},
		Span:   spanFromReadModel(file, primary),
	})
	return evidence
}
