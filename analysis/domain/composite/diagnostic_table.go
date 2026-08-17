package composite

import (
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
)

// The published diagnostic codes. A code is the analyzer's stable external
// identity for one finding family and is the authored key of its declaration
// row, so there is exactly one spelling of a diagnostic in the analyzer.
const (
	DiagnosticCodeAlwaysTrueGuard          diagnostic.Code = "advice.always_true_guard"
	DiagnosticCodeAlwaysFalseGuard         diagnostic.Code = "advice.always_false_guard"
	DiagnosticCodeRedundantClaim           diagnostic.Code = "advice.redundant_claim"
	DiagnosticCodeUnresolvedTypeReference  diagnostic.Code = "type.reference.unresolved"
	DiagnosticCodeUnresolvedValueReference diagnostic.Code = "value.reference.unresolved"
	DiagnosticCodeUnusedLocal              diagnostic.Code = "lint.unused.local"
)

// valueAxis is the coordinate space the solver-observed diagnostics are
// decided by. It is the axis surface's own authored key, resolved by the
// declaration root when the diagnostic surface is sealed.
const valueAxis = schema.Key("value")

// diagnosticEvidenceKind, diagnosticEvidenceTrust, and diagnosticEvidenceReason
// are the classification every current proof line carries: an abstract fact the
// solver proved, without a narrower reason vocabulary yet.
const (
	diagnosticEvidenceKind   = "abstract fact"
	diagnosticEvidenceTrust  = "proven"
	diagnosticEvidenceReason = "unspecified"
)

// diagnosticRender is the section order every current row publishes.
var diagnosticRender = []diagnostic.Section{
	diagnostic.SectionSummary,
	diagnostic.SectionLocation,
	diagnostic.SectionSource,
	diagnostic.SectionEvidence,
	diagnostic.SectionHelp,
}

func diagnosticEvidence(anchor diagnostic.Anchor, detail diagnostic.Text) diagnostic.Evidence {
	return diagnostic.Evidence{
		Anchor: anchor,
		Kind:   diagnosticEvidenceKind,
		Trust:  diagnosticEvidenceTrust,
		Reason: diagnosticEvidenceReason,
		Detail: detail,
	}
}

// diagnosticSpecs is the authored analyzer diagnostic inventory. Each row is
// one published finding family: the code it publishes under, the family the
// query boundary gates it by, the severity it defaults to, the lane its
// subjects arrive on, and the exact presentation it renders from.
//
// A row is data end to end. Adding a diagnostic over facts the analyzer
// already produces is one entry here; nothing else in the analyzer holds a
// per-code table to keep in step.
func diagnosticSpecs() []diagnostic.Spec {
	return []diagnostic.Spec{
		{
			Code: DiagnosticCodeAlwaysTrueGuard, Family: diagnostic.FamilyAdvice,
			DefaultSeverity: diagnostic.SeverityHint,
			Lane:            diagnostic.LaneBranch,
			Observation:     programartifact.DiagnosticObservationBranchCondition,
			Fact:            diagnostic.Reference{Surface: schema.SurfaceKindAxis, Key: valueAxis},
			Message:         "condition is proven always true",
			Help:            "Remove the guard or move the guarded code out of the branch.",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "condition is proven to be true on every reachable path")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "constant guard"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeAlwaysFalseGuard, Family: diagnostic.FamilyAdvice,
			DefaultSeverity: diagnostic.SeverityHint,
			Lane:            diagnostic.LaneBranch,
			Observation:     programartifact.DiagnosticObservationBranchCondition,
			Fact:            diagnostic.Reference{Surface: schema.SurfaceKindAxis, Key: valueAxis},
			Message:         "condition is proven always false",
			Help:            "Remove the unreachable branch or invert the guard.",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "condition is proven to be false on every reachable path")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "constant guard"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeRedundantClaim, Family: diagnostic.FamilyAdvice,
			DefaultSeverity: diagnostic.SeverityHint,
			Lane:            diagnostic.LaneDeclared,
			Requirements:    diagnostic.RequiresSubject | diagnostic.RequiresTarget | diagnostic.RequiresClaimForm | diagnostic.RequiresProofLocation,
			Message:         "{claim} is redundant; value is already {target}",
			Help:            "Remove the runtime type claim when the proven source type is sufficient.",
			Evidence: []diagnostic.Evidence{
				diagnosticEvidence(diagnostic.AnchorProof, "{subject} is proven to be {target} before the claim"),
				diagnosticEvidence(diagnostic.AnchorPrimary, "claim checks {target} at this site"),
			},
			Labels: []diagnostic.Label{
				{Anchor: diagnostic.AnchorPrimary, Text: "claim site"},
				{Anchor: diagnostic.AnchorProof, Text: "proven value"},
			},
			Render: diagnosticRender,
		},
		{
			Code: DiagnosticCodeUnresolvedTypeReference, Family: diagnostic.FamilyType,
			DefaultSeverity: diagnostic.SeverityError,
			Lane:            diagnostic.LaneStatic,
			Observation:     programartifact.DiagnosticObservationTypeReferenceUnresolved,
			Requirements:    diagnostic.RequiresSubject,
			Message:         "unknown type {subject}",
			Help:            "Declare the type in scope",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "no type named {subject} is declared in this scope")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "unknown type"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeUnresolvedValueReference, Family: diagnostic.FamilyValue,
			DefaultSeverity: diagnostic.SeverityError,
			Lane:            diagnostic.LaneStatic,
			Observation:     programartifact.DiagnosticObservationValueReferenceUnresolved,
			Requirements:    diagnostic.RequiresSubject,
			Message:         "unknown value {subject}",
			Help:            "Declare the value",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "no value named {subject} is declared, predeclared, imported, or configured global in this scope")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "unknown value"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeUnusedLocal, Family: diagnostic.FamilyLint,
			DefaultSeverity: diagnostic.SeverityHint,
			Lane:            diagnostic.LaneDeclared,
			Requirements:    diagnostic.RequiresSubject,
			Message:         "local {subject.quoted} is never read",
			Help:            "Remove it, use it, or rename it with a leading _ when intentionally unused.",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "no read of local {subject.quoted} was found in this scope")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "unused local"}},
			Render:          diagnosticRender,
		},
	}
}

// diagnosticEntries admits the authored inventory. A rejected row leaves the
// table unavailable rather than half declared.
func diagnosticEntries() ([]*diagnostic.Entry, bool) {
	specs := diagnosticSpecs()
	entries := make([]*diagnostic.Entry, 0, len(specs))
	for _, spec := range specs {
		entry, ok := diagnostic.New(spec)
		if !ok {
			return nil, false
		}
		entries = append(entries, entry)
	}
	return entries, true
}

// Diagnostics is the derived read model of the sealed diagnostic surface. It
// is the single lookup authority for every published diagnostic declaration.
func Diagnostics() (diagnostic.Table, bool) {
	sealRegistry()
	if registry.sealed == nil {
		return diagnostic.Table{}, false
	}
	return registry.diagnostics, registry.diagnostics.Available()
}
