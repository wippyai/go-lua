package composite

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	typedomain "github.com/wippyai/go-lua/domain/type"
)

// The declared identities every diagnostic row is named against: the
// observation populations the rows are measured over, the publication families
// the query boundary gates them by, and the severities they default to. They
// are members of the structural vocabulary, so a row names them by reference
// and the sealed table resolves them.
const (
	diagnosticFamilyAdvice = schema.Key("family/advice")
	diagnosticFamilyType   = schema.Key("family/type")
	diagnosticFamilyValue  = schema.Key("family/value")
	diagnosticFamilyLint   = schema.Key("family/lint")
)

// diagnosticVocabulary is the diagnostic surface's contribution to the
// structural vocabulary. The families a code is published under, the
// observation populations a row is measured over, and the severities a row
// defaults to are declared here, beside the rows that name them.
//
// The observation rows come from the neutral structure vocabulary, whose
// ordinals are the identities an artifact carries. The severity ordinals are
// authored for the same reason - the severity a policy carries is that
// position. Families are numbered by declaration order: no foreign spelling
// numbers them, and a row resolves one by key.
func diagnosticVocabulary() []structure.Spec {
	specs := structure.DiagnosticObservationSpecs()
	specs = append(specs,
		structure.Spec{Key: diagnosticFamilyAdvice, Category: structure.CategoryDiagnosticFamily, Spelling: "advice", Accepted: true},
		structure.Spec{Key: diagnosticFamilyType, Category: structure.CategoryDiagnosticFamily, Spelling: "type", Accepted: true},
		structure.Spec{Key: diagnosticFamilyValue, Category: structure.CategoryDiagnosticFamily, Spelling: "value", Accepted: true},
		structure.Spec{Key: diagnosticFamilyLint, Category: structure.CategoryDiagnosticFamily, Spelling: "lint", Accepted: true},

		structure.Spec{Key: "severity/error", Category: structure.CategoryDiagnosticSeverity, Ordinal: diagnostic.SeverityError.Ordinal(), Spelling: "error", Accepted: true},
		structure.Spec{Key: "severity/warning", Category: structure.CategoryDiagnosticSeverity, Ordinal: diagnostic.SeverityWarning.Ordinal(), Spelling: "warning", Accepted: true},
		structure.Spec{Key: "severity/hint", Category: structure.CategoryDiagnosticSeverity, Ordinal: diagnostic.SeverityHint.Ordinal(), Spelling: "hint", Accepted: true},
	)
	return specs
}

// declaredMember names one member of the structural vocabulary from a
// diagnostic row.
func declaredMember(key schema.Key) diagnostic.Reference {
	return diagnostic.Reference{Surface: schema.SurfaceKindStructure, Key: key}
}

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
//
// A row a domain declares for itself is contributed rather than authored: the
// domain that owns the judgment owns the row, and this table states its
// membership and its position. The identities such a row names resolve in the
// same sealed table as every other row's, so a contributed row is admitted on
// exactly the terms the authored ones are.
func diagnosticSpecs() []diagnostic.Spec {
	return append(analyzerDiagnosticSpecs(), typedomain.DiagnosticSpec(), typedomain.DiagnosticCallArgumentSpec())
}

func analyzerDiagnosticSpecs() []diagnostic.Spec {
	return []diagnostic.Spec{
		{
			Code: DiagnosticCodeAlwaysTrueGuard, Family: declaredMember(diagnosticFamilyAdvice),
			DefaultSeverity: diagnostic.SeverityHint,
			Lane:            diagnostic.LaneBranch,
			Observation:     declaredMember(structure.DiagnosticObservationBranchCondition.Key()),
			Collection:      diagnostic.Reference{Surface: schema.SurfaceKindObservation, Key: ObservationBranchValueSummary},
			Fact:            diagnostic.Reference{Surface: schema.SurfaceKindAxis, Key: valueAxis},
			Message:         "condition is proven always true",
			Help:            "Remove the guard or move the guarded code out of the branch.",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "condition is proven to be true on every reachable path")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "constant guard"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeAlwaysFalseGuard, Family: declaredMember(diagnosticFamilyAdvice),
			DefaultSeverity: diagnostic.SeverityHint,
			Lane:            diagnostic.LaneBranch,
			Observation:     declaredMember(structure.DiagnosticObservationBranchCondition.Key()),
			Collection:      diagnostic.Reference{Surface: schema.SurfaceKindObservation, Key: ObservationBranchValueSummary},
			Fact:            diagnostic.Reference{Surface: schema.SurfaceKindAxis, Key: valueAxis},
			Message:         "condition is proven always false",
			Help:            "Remove the unreachable branch or invert the guard.",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "condition is proven to be false on every reachable path")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "constant guard"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeRedundantClaim, Family: declaredMember(diagnosticFamilyAdvice),
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
			Code: DiagnosticCodeUnresolvedTypeReference, Family: declaredMember(diagnosticFamilyType),
			DefaultSeverity: diagnostic.SeverityError,
			Lane:            diagnostic.LaneStatic,
			Observation:     declaredMember(structure.DiagnosticObservationTypeReferenceUnresolved.Key()),
			Requirements:    diagnostic.RequiresSubject,
			Message:         "unknown type {subject}",
			Help:            "Declare the type in scope",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "no type named {subject} is declared in this scope")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "unknown type"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeUnresolvedValueReference, Family: declaredMember(diagnosticFamilyValue),
			DefaultSeverity: diagnostic.SeverityError,
			Lane:            diagnostic.LaneStatic,
			Observation:     declaredMember(structure.DiagnosticObservationValueReferenceUnresolved.Key()),
			Requirements:    diagnostic.RequiresSubject,
			Message:         "unknown value {subject}",
			Help:            "Declare the value",
			Evidence:        []diagnostic.Evidence{diagnosticEvidence(diagnostic.AnchorPrimary, "no value named {subject} is declared, predeclared, imported, or configured global in this scope")},
			Labels:          []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "unknown value"}},
			Render:          diagnosticRender,
		},
		{
			Code: DiagnosticCodeUnusedLocal, Family: declaredMember(diagnosticFamilyLint),
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

// DiagnosticCollection is one LaneBranch row's post-seal collection handle:
// the query or observation family the row named, and the issued producer,
// projection, geometry, and anchor that family carries.
type DiagnosticCollection struct {
	Code       diagnostic.Code
	Collection diagnostic.Reference
	Population schema.Key
	Site       diagnostic.Site
	Producer   schema.Key
	Projection schema.Key
	Codec      schema.Key
	Geometry   schema.Key
	Anchor     schema.Key
}

// DiagnosticCollectionDirectory joins every sealed LaneBranch row to the
// issued query or observation inventory it named as Collection.
func DiagnosticCollectionDirectory() ([]DiagnosticCollection, bool) {
	table, tableOK := Diagnostics()
	if !tableOK {
		return nil, false
	}
	observations := make(map[schema.Key]IssuedObservation)
	for _, issued := range ObservationIssuance() {
		observations[issued.Key] = issued
	}
	queries := make(map[schema.Key]IssuedQuery)
	for _, issued := range QueryIssuance() {
		queries[issued.Family] = issued
	}
	rows := make([]DiagnosticCollection, 0, table.Count())
	for position := 0; position < table.Count(); position++ {
		entry, entryOK := table.At(position)
		if !entryOK || entry.Lane() != diagnostic.LaneBranch {
			continue
		}
		collection := entry.Collection()
		row := DiagnosticCollection{
			Code:       entry.Code(),
			Collection: collection,
			Population: entry.Observation().Key,
			Site:       entry.Site(),
		}
		switch collection.Surface {
		case schema.SurfaceKindObservation:
			issued, found := observations[collection.Key]
			if !found || !issued.Producer.Available() || !issued.Codec.Available() {
				return nil, false
			}
			row.Producer = issued.Producer
			row.Codec = issued.Codec
			row.Geometry = issued.Geometry
			row.Anchor = issued.Anchor
		case schema.SurfaceKindQuery:
			issued, found := queries[collection.Key]
			if !found || !issued.Family.Available() || !issued.Projection.Available() {
				return nil, false
			}
			row.Producer = issued.Family
			row.Projection = issued.Projection
		default:
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, len(rows) > 0
}
