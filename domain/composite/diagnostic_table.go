package composite

import (
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/sendsafety"
	typedomain "github.com/wippyai/go-lua/domain/type"
)

// The declared identities every diagnostic row is named against: the
// observation populations the rows are measured over, the publication families
// the query boundary gates them by, and the severities they default to. They
// are members of the structural vocabulary, so a row names them by reference
// and the sealed table resolves them.
const (
	diagnosticFamilyAdvice  = schema.Key("family/advice")
	diagnosticFamilyType    = schema.Key("family/type")
	diagnosticFamilyValue   = schema.Key("family/value")
	diagnosticFamilyLint    = schema.Key("family/lint")
	diagnosticFamilyChannel = schema.Key("family/channel")
	diagnosticFamilySend    = schema.Key("family/send")
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
	specs = append(specs, typedomain.ConformanceVerdictStructureSpecs()...)
	specs = append(specs,
		structure.Spec{Key: diagnosticFamilyAdvice, Category: structure.CategoryDiagnosticFamily, Spelling: "advice", Accepted: true},
		structure.Spec{Key: diagnosticFamilyType, Category: structure.CategoryDiagnosticFamily, Spelling: "type", Accepted: true},
		structure.Spec{Key: diagnosticFamilyValue, Category: structure.CategoryDiagnosticFamily, Spelling: "value", Accepted: true},
		structure.Spec{Key: diagnosticFamilyLint, Category: structure.CategoryDiagnosticFamily, Spelling: "lint", Accepted: true},
		structure.Spec{Key: diagnosticFamilyChannel, Category: structure.CategoryDiagnosticFamily, Spelling: "channel", Accepted: true},
		structure.Spec{Key: diagnosticFamilySend, Category: structure.CategoryDiagnosticFamily, Spelling: "send", Accepted: true},

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
	DiagnosticCodeSendIsolation            diagnostic.Code = "send.isolation"
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

// diagnosticClaimProofWitness is the redundant-claim row's one located
// witness: the place the value was already proven to be the claimed type.
const diagnosticClaimProofWitness uint8 = 1

func diagnosticEvidence(anchor diagnostic.Anchor, detail diagnostic.Text) diagnostic.Evidence {
	return diagnostic.Evidence{
		Anchor: anchor,
		Kind:   diagnosticEvidenceKind,
		Trust:  diagnosticEvidenceTrust,
		Reason: diagnosticEvidenceReason,
		Detail: detail,
	}
}

// diagnosticWitnessEvidence is one proof line established at a located witness
// the producer supplies rather than at the finding's own site.
func diagnosticWitnessEvidence(witness uint8, detail diagnostic.Text) diagnostic.Evidence {
	return diagnostic.Evidence{
		Anchor:  diagnostic.AnchorWitness,
		Witness: witness,
		Kind:    diagnosticEvidenceKind,
		Trust:   diagnosticEvidenceTrust,
		Reason:  diagnosticEvidenceReason,
		Detail:  detail,
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
	return append(analyzerDiagnosticSpecs(), typedomain.DiagnosticSpec(), typedomain.DiagnosticCallArgumentSpec(), typedomain.DiagnosticChannelSelectExhaustivenessSpec())
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
			Witnesses:       1,
			Requirements:    diagnostic.RequiresSubject | diagnostic.RequiresTarget | diagnostic.RequiresClaimForm | diagnostic.RequiresWitness,
			Message:         "{claim} is redundant; value is already {target}",
			Help:            "Remove the runtime type claim when the proven source type is sufficient.",
			Evidence: []diagnostic.Evidence{
				diagnosticWitnessEvidence(diagnosticClaimProofWitness, "{subject} is proven to be {target} before the claim"),
				diagnosticEvidence(diagnostic.AnchorPrimary, "claim checks {target} at this site"),
			},
			Labels: []diagnostic.Label{
				{Anchor: diagnostic.AnchorPrimary, Text: "claim site"},
				{Anchor: diagnostic.AnchorWitness, Witness: diagnosticClaimProofWitness, Text: "proven value"},
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
		{
			Code: DiagnosticCodeSendIsolation, Family: declaredMember(diagnosticFamilySend),
			DefaultSeverity: diagnostic.SeverityHint,
			Lane:            diagnostic.LaneResult,
			VerdictCategory: structure.CategoryNativeSendSafety,
			Variants: []diagnostic.Variant{
				{
					Verdict:  sendsafety.VerdictImmutable.Ordinal(),
					Message:  "send payload is proven immutable for direct sharing",
					Help:     "The runtime may share this exact identity without copying it.",
					Evidence: []diagnostic.Evidence{{Anchor: diagnostic.AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "deep freeze", Detail: "the complete sent allocation graph is deeply frozen"}},
					Labels:   []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "immutable send payload"}},
				},
				{
					Verdict:  sendsafety.VerdictIsolated.Ordinal(),
					Message:  "send payload is proven isolated for direct transfer",
					Help:     "The runtime may transfer this exact identity without copying it.",
					Evidence: []diagnostic.Evidence{{Anchor: diagnostic.AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "unique ownership", Detail: "the sent allocation has no retaining alias before this send"}},
					Labels:   []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "isolated send payload"}},
				},
				{
					Verdict:  sendsafety.VerdictCopyRequired.Ordinal(),
					Message:  "send payload has a proven retaining alias; copy-on-write is required",
					Help:     "Remove the retaining alias or freeze the complete payload graph before sending it.",
					Evidence: []diagnostic.Evidence{{Anchor: diagnostic.AnchorPrimary, Kind: "abstract fact", Trust: "proven", Reason: "retaining escape", Detail: "a retaining boundary precedes this send"}},
					Labels:   []diagnostic.Label{{Anchor: diagnostic.AnchorPrimary, Text: "retained send payload"}},
				},
			},
			Render: diagnosticRender,
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
func Diagnostics(compilation Compilation) (diagnostic.Table, bool) {
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		return diagnostic.Table{}, false
	}
	return state.diagnostics, state.diagnostics.Available()
}

// DiagnosticCollection is one LaneBranch row's post-seal collection handle:
// the query or observation family the row named, and the issued producer,
// projection, geometry, and anchor that family carries.
type DiagnosticCollection struct {
	Code       diagnostic.Code
	Collection diagnostic.Reference
	Population schema.Key
	Producer   schema.Key
	Projection schema.Key
	Codec      schema.Key
	Geometry   schema.Key
	Anchor     schema.Key
	sites      []diagnostic.Site
}

func (row DiagnosticCollection) SiteCount() int { return len(row.sites) }

func (row DiagnosticCollection) SiteAt(position int) (diagnostic.Site, bool) {
	if position < 0 || position >= len(row.sites) {
		return diagnostic.SiteNone, false
	}
	return row.sites[position], true
}

// DiagnosticCollections is the immutable collection directory sealed into a
// Compilation. Its rows and site slices are private, so carrying this value
// through a Workspace or report requires no defensive copy.
type DiagnosticCollections struct{ rows []DiagnosticCollection }

func (directory DiagnosticCollections) Available() bool { return len(directory.rows) > 0 }
func (directory DiagnosticCollections) Count() int      { return len(directory.rows) }
func (directory DiagnosticCollections) At(position int) (DiagnosticCollection, bool) {
	if position < 0 || position >= len(directory.rows) {
		return DiagnosticCollection{}, false
	}
	return directory.rows[position], true
}

// entrySites copies one row's declared geometries so a directory row holds its
// own list rather than aliasing the sealed entry.
func entrySites(entry *diagnostic.Entry) []diagnostic.Site {
	sites := make([]diagnostic.Site, 0, entry.SiteCount())
	for index := 0; index < entry.SiteCount(); index++ {
		site, siteOK := entry.SiteAt(index)
		if !siteOK {
			return nil
		}
		sites = append(sites, site)
	}
	return sites
}

// diagnosticCollectionDirectory joins every sealed LaneBranch row to the
// issued query or observation inventory it named as Collection. Construction
// calls it once and retains the result in Compilation.
func diagnosticCollectionDirectory(table diagnostic.Table, issuedObservations []IssuedObservation, issuedQueries []IssuedQuery, state *catalog) (DiagnosticCollections, bool) {
	if !table.Available() || state == nil {
		return DiagnosticCollections{}, false
	}
	observations := make(map[schema.Key]IssuedObservation)
	for _, issued := range issuedObservations {
		observations[issued.Key] = issued
	}
	queries := make(map[schema.Key]IssuedQuery)
	for _, issued := range issuedQueries {
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
			sites:      entrySites(entry),
		}
		switch collection.Surface {
		case schema.SurfaceKindObservation:
			issued, found := observations[collection.Key]
			if !found || !issued.Producer.Available() || !issued.Codec.Available() ||
				!issued.Geometry.Available() || !issued.Anchor.Available() {
				return DiagnosticCollections{}, false
			}
			if _, _, producerOK := observationProducerRegistration(state, issued); !producerOK {
				return DiagnosticCollections{}, false
			}
			row.Producer = issued.Producer
			row.Codec = issued.Codec
			row.Geometry = issued.Geometry
			row.Anchor = issued.Anchor
		case schema.SurfaceKindQuery:
			issued, found := queries[collection.Key]
			if !found || !issued.Family.Available() || !issued.Projection.Available() {
				return DiagnosticCollections{}, false
			}
			row.Producer = issued.Family
			row.Projection = issued.Projection
		default:
			return DiagnosticCollections{}, false
		}
		rows = append(rows, row)
	}
	directory := DiagnosticCollections{rows: rows}
	return directory, directory.Available()
}
