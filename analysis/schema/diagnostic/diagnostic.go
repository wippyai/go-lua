// Package diagnostic owns the diagnostic surface of the analyzer declaration
// table: the row one published diagnostic is declared as, and the surface laws
// the declaration root seals it under.
//
// A diagnostic row is pure data. It carries the code it publishes under, the
// family it is gated by, the severity it defaults to, the lane its subjects
// arrive on, the observation population it is measured over, the declaration
// whose facts decide it, and the exact presentation it is rendered from. It
// carries no hook, no interface, and no function value: a diagnostic is a
// post-publication read of facts that already exist, so nothing in a row can
// run during a solve.
//
// This is what makes the diagnostic-addition law real: a diagnostic that
// consumes facts the analyzer already produces is added by writing one row
// here, and every derived lookup projects it without a second table.
//
// The identities a row is declared against are named, not enumerated. The
// family it is gated by and the observation population it is measured over are
// members of vocabularies the structural surface declares, and a row names them
// by reference; the declaration whose facts decide it is a reference too. Seal
// resolves each against the same table the row is sealed into, so a family the
// analyzer has never published before is one more declared row rather than one
// more member of a closed type here.
//
// Deferred resolutions. Two identities a row is eventually read under are
// not yet spellable:
//
//   - the denominator, which awaits an observation surface of its own. The
//     denominator surface is in the catalog and declares closed worlds, but it
//     sits above this one and a denominator names the entry that owns it, so the
//     resolution lands there rather than as a reference out of a row. The worlds
//     declared today are the axes' coordinate populations; none quantifies over
//     a diagnostic row's population.
//   - the evidence projection, which awaits the same surface. It is declared
//     today as the row's own evidence lines.
//
// The collection plan is named on the row as a reference to the query or
// observation family that supplies its subjects. That family seals above this
// surface, so Seal checks the reference's shape and a post-seal directory
// joins it to the issued inventory.
package diagnostic

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/internal/framing"
)

// Content record markers. They separate the presentation collections one row
// writes, so an evidence line can never be read as a label.
const (
	contentRecordEvidence uint64 = iota + 1
	contentRecordLabel
	contentRecordSection
	contentRecordVariant
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawEntryShape schema.LawID = schema.SurfaceLawFloor + iota
	LawCodeIdentity
	LawFamilyDeclared
	LawTierValid
	LawRequirementCovered
	LawObservationUnique
	LawFactResolves
	LawRenderComplete
	LawSurfacePopulated
	LawObservationDeclared
	LawCollectionDeclared
	LawSiteUnique
	LawVariantDeclared
)

// Code is the stable published identity of one diagnostic. It is the only part
// of a row a reader outside the analyzer sees, and it doubles as the row's
// authored key: the entry identity a verdict carries is derived from it, so a
// code and an entry cannot drift apart.
//
// A code is a dotted lowercase path whose first segment is the family it is
// published under.
type Code string

// CodeInvalid is the absent identity. No row may publish it.
const CodeInvalid Code = ""

func (code Code) String() string { return string(code) }

func (code Code) Available() bool {
	family, rest, split := strings.Cut(string(code), ".")
	return split && codeSegment(family) && codeSegments(rest)
}

// Family is the first segment of the code: the publication family the query
// boundary gates on.
func (code Code) Family() (string, bool) {
	family, _, split := strings.Cut(string(code), ".")
	return family, split && code.Available()
}

func codeSegments(path string) bool {
	if path == "" {
		return false
	}
	for {
		segment, rest, split := strings.Cut(path, ".")
		if !codeSegment(segment) {
			return false
		}
		if !split {
			return true
		}
		path = rest
	}
}

func codeSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for index := 0; index < len(segment); index++ {
		character := segment[index]
		if character >= 'a' && character <= 'z' || character == '_' || (index > 0 && character >= '0' && character <= '9') {
			continue
		}
		return false
	}
	return true
}

// Severity is the severity a row defaults to. A policy may refine a row's
// severity within this vocabulary; it can never invent one. The vocabulary
// itself is declared on the structural surface under
// structure.CategoryDiagnosticSeverity, and these ordinals are the positions of
// its members, so a renderer reads the declared spelling rather than holding a
// switch of its own.
type Severity uint8

const (
	SeverityInvalid Severity = iota
	SeverityError
	SeverityWarning
	SeverityHint
)

func (severity Severity) Available() bool {
	return severity >= SeverityError && severity <= SeverityHint
}

// Ordinal is this severity's position in the declared severity vocabulary. A
// renderer resolves the member at this ordinal and reads its spelling.
func (severity Severity) Ordinal() uint16 { return uint16(severity) }

// Tier is the publication tier a row belongs to. The error tier is the
// analyzer's own verdict on a program; the advisory tier is advice a consumer
// opts into. The tier is derived from the declared default severity and is
// never declared beside it, so a config-gated tier and the severity a reader
// sees cannot disagree.
type Tier uint8

const (
	TierInvalid Tier = iota
	TierError
	TierAdvisory
)

// Lane is the closed catalog of ways one row's subjects reach the collector.
type Lane uint8

const (
	LaneInvalid Lane = iota
	// LaneDeclared is a published code with no installed producer. Its
	// presentation is complete and its policy admission is refused, so a
	// consumer can never enable it and receive a clean empty report.
	LaneDeclared
	// LaneBranch is collected from solver-observed rows, decided by the facts
	// of the declaration the row references.
	LaneBranch
	// LaneStatic is collected from artifact-issued rows alone. The disposition
	// was proven before the program was mounted, so the lane reads no fact.
	LaneStatic
	laneLimit
)

func (lane Lane) Available() bool { return lane > LaneInvalid && lane < laneLimit }

// Site is the discriminator that chooses one code among LaneBranch rows
// that share an observation population. A polarity pair over one population
// declares none; a shared type-conformance population declares one site
// per code.
type Site uint8

const (
	SiteNone Site = iota
	SiteAssignment
	SiteCallArgument
	// SiteMember is one established constructor member measured against the
	// declared node its key, index, or map position resolves to.
	SiteMember
	// SiteMemberAbsent is one required declared field the constructor's
	// established key set does not supply.
	SiteMemberAbsent
	siteLimit
)

func (site Site) Available() bool { return site > SiteNone && site < siteLimit }

func (site Site) Declared() bool { return site != SiteNone }

// Produces reports whether the lane has an installed producer. Only a lane
// that consumes an observation population produces findings.
func (lane Lane) Produces() bool { return lane == LaneBranch || lane == LaneStatic }

// Reference is one declared identity on another surface: the surface the
// referenced entry is declared on, and its authored key there. The catalog
// order is the dependency order, so a reference resolves downward.
type Reference struct {
	Surface schema.SurfaceKind
	Key     schema.Key
}

func (reference Reference) Available() bool {
	return reference.Surface.Available() && reference.Key.Available()
}

// Declared reports whether a row named a reference at all.
func (reference Reference) Declared() bool { return reference != Reference{} }

// Requirement is the closed set of typed payload fields a row's presentation
// reads. A producer supplies exactly this set; nothing else can enter a
// rendered diagnostic.
type Requirement uint8

const (
	RequiresInvalid Requirement = 0
	RequiresSubject Requirement = 1 << iota >> 1
	RequiresTarget
	RequiresClaimForm
	// RequiresWitness is the located witness roster a row's non-primary anchors
	// read. A row declares how many witnesses its producer supplies; an anchor
	// names one of them by ordinal.
	RequiresWitness
	RequiresHandled
	RequiresMissing
	// RequiresActual is the spelling of the value that was observed: its exact
	// literal when it has one, and the runtime family it may carry otherwise.
	RequiresActual
	// RequiresMember is the declared member a finding names: the field a
	// constructor did not establish.
	RequiresMember
)

func (requirement Requirement) has(other Requirement) bool { return requirement&other == other }

// Placeholder is the closed catalog of payload reads a template may perform.
// A template can name nothing outside it, so authored prose can never reach a
// rendered diagnostic.
type Placeholder uint8

const (
	PlaceholderInvalid Placeholder = iota
	PlaceholderSubject
	PlaceholderQuotedSubject
	PlaceholderTarget
	PlaceholderClaimForm
	PlaceholderHandled
	PlaceholderMissing
	PlaceholderActual
	PlaceholderMember
)

func placeholderFor(name string) (Placeholder, bool) {
	switch name {
	case "subject":
		return PlaceholderSubject, true
	case "subject.quoted":
		return PlaceholderQuotedSubject, true
	case "target":
		return PlaceholderTarget, true
	case "claim":
		return PlaceholderClaimForm, true
	case "handled":
		return PlaceholderHandled, true
	case "missing":
		return PlaceholderMissing, true
	case "actual":
		return PlaceholderActual, true
	case "member":
		return PlaceholderMember, true
	default:
		return PlaceholderInvalid, false
	}
}

// Requires is the payload field one placeholder reads.
func (placeholder Placeholder) Requires() Requirement {
	switch placeholder {
	case PlaceholderSubject, PlaceholderQuotedSubject:
		return RequiresSubject
	case PlaceholderTarget:
		return RequiresTarget
	case PlaceholderClaimForm:
		return RequiresClaimForm
	case PlaceholderHandled:
		return RequiresHandled
	case PlaceholderMissing:
		return RequiresMissing
	case PlaceholderActual:
		return RequiresActual
	case PlaceholderMember:
		return RequiresMember
	default:
		return RequiresInvalid
	}
}

// Text is one authored template. It is a construction input: the surface
// parses it once, at admission, and publishes the parsed line.
type Text string

// Segment is one piece of a parsed template: either a literal run or one
// payload read. A renderer walks segments; it never parses.
type Segment struct {
	Literal     string
	Placeholder Placeholder
}

// Line is one parsed template. Parsing is an admission law, so a published
// line is already proven to name only declared payload reads.
type Line struct {
	text     Text
	segments []Segment
	requires Requirement
}

func newLine(text Text) (Line, bool) {
	if text == "" {
		return Line{}, false
	}
	line := Line{text: text}
	start := 0
	for index := 0; index < len(text); index++ {
		switch text[index] {
		case '}':
			return Line{}, false
		case '{':
			offset := strings.IndexByte(string(text[index:]), '}')
			if offset < 0 {
				return Line{}, false
			}
			placeholder, known := placeholderFor(string(text[index+1 : index+offset]))
			if !known {
				return Line{}, false
			}
			if index > start {
				line.segments = append(line.segments, Segment{Literal: string(text[start:index])})
			}
			line.segments = append(line.segments, Segment{Placeholder: placeholder})
			line.requires |= placeholder.Requires()
			index += offset
			start = index + 1
		}
	}
	if start < len(text) {
		line.segments = append(line.segments, Segment{Literal: string(text[start:])})
	}
	return line, len(line.segments) > 0
}

func (line Line) Available() bool { return line.text != "" && len(line.segments) > 0 }

func (line Line) Text() Text { return line.text }

func (line Line) Count() int { return len(line.segments) }

func (line Line) At(index int) (Segment, bool) {
	if index < 0 || index >= len(line.segments) {
		return Segment{}, false
	}
	return line.segments[index], true
}

// Requires is the payload this line reads.
func (line Line) Requires() Requirement { return line.requires }

// Anchor is the closed catalog of source positions a row's evidence or label
// may be attached to.
type Anchor uint8

const (
	AnchorInvalid Anchor = iota
	// AnchorPrimary is the finding's own location.
	AnchorPrimary
	// AnchorWitness is one already-authenticated location the payload carries
	// beside the finding's own: the earlier guard a proof came from, the write
	// that overwrote a value, the birth site of a table. The row declares how
	// many such locations its producer supplies and each reference names one by
	// ordinal, so a judgment whose proof spans several places states every one
	// of them rather than collapsing them into the finding's own site.
	AnchorWitness
	anchorLimit
)

func (anchor Anchor) Available() bool { return anchor > AnchorInvalid && anchor < anchorLimit }

// Requires is the payload field one anchor reads.
func (anchor Anchor) Requires() Requirement {
	if anchor == AnchorWitness {
		return RequiresWitness
	}
	return RequiresInvalid
}

// Section is the closed catalog of rendered sections, in the order a row may
// declare them.
type Section uint8

const (
	SectionInvalid Section = iota
	SectionSummary
	SectionLocation
	SectionSource
	SectionEvidence
	// SectionContext is the surrounding place a finding is decided in rather
	// than the place it is reported at: the loop a read is invariant across,
	// the construct a judgment ranges over. It renders one declared witness.
	SectionContext
	SectionHelp
	sectionLimit
)

func (section Section) Available() bool { return section > SectionInvalid && section < sectionLimit }

// Evidence is one authored proof line of a row. Witness names which declared
// witness the line is anchored at, and is read only under AnchorWitness.
type Evidence struct {
	Anchor              Anchor
	Witness             uint8
	Kind, Trust, Reason string
	Detail              Text
}

// Label is one authored source label of a row. Witness names which declared
// witness the label is attached to, and is read only under AnchorWitness.
type Label struct {
	Anchor  Anchor
	Witness uint8
	Text    Text
}

// EvidenceRow is one admitted proof line: the authored classification and the
// parsed detail line.
type EvidenceRow struct {
	Anchor              Anchor
	Witness             uint8
	Kind, Trust, Reason string
	Detail              Line
}

// LabelRow is one admitted source label.
type LabelRow struct {
	Anchor  Anchor
	Witness uint8
	Text    Line
}

// Variant is one authored presentation of a row, selected by the verdict its
// collector reached. A code is the row's authored key, so a judgment that
// answers in a vocabulary rather than a boolean cannot publish one row per
// answer: it publishes one row with one variant per answer, and the collector
// emits the verdict rather than the prose.
//
// Verdict is the member's ordinal in the declared conformance-verdict
// vocabulary. The judgment that produces it owns those ordinals, so a row
// keyed by them cannot drift from the answers the judgment gives.
type Variant struct {
	Verdict uint16
	// Requirements is the typed payload this variant's presentation reads. It
	// is stated per variant because the payload one answer names is not the
	// payload another names: an absent member names the member, a family
	// mismatch names the value that was observed.
	Requirements  Requirement
	Message, Help Text
	Evidence      []Evidence
	Labels        []Label
}

// VariantRow is one admitted variant: the verdict it answers for and the
// presentation that answer renders.
type VariantRow struct {
	verdict      uint16
	requirements Requirement
	message      Line
	help         Line
	evidence     []EvidenceRow
	labels       []LabelRow
}

func (variant VariantRow) Verdict() uint16 { return variant.verdict }

// Presentation is the rendering one finding is published from: the payload it
// reads and the exact lines it renders. A row with no verdict vocabulary has
// one; a row with variants has one per declared answer. Every consumer renders
// through this, so the base row and a variant are the same shape to a reader.
type Presentation struct {
	Requirements  Requirement
	Message, Help Line
	Evidence      []EvidenceRow
	Labels        []LabelRow
}

// reads is the payload every part of this presentation consumes.
func (presentation Presentation) reads() Requirement {
	reads := presentation.Message.Requires() | presentation.Help.Requires()
	for _, evidence := range presentation.Evidence {
		reads |= evidence.Detail.Requires() | evidence.Anchor.Requires()
	}
	for _, label := range presentation.Labels {
		reads |= label.Text.Requires() | label.Anchor.Requires()
	}
	return reads
}

func (presentation Presentation) available() bool { return presentation.Message.Available() }

// Spec is the authored declaration of one diagnostic. Every field is data.
type Spec struct {
	// Code is the row's published identity and its authored key.
	Code Code
	// Family is the publication family this row is gated by: a member of the
	// declared family vocabulary, named by reference. The family's declared
	// spelling is the first segment of the published code, so publishing under a
	// family the analyzer has never published before is declaring one more row on
	// the structural surface rather than widening an enum here.
	Family Reference
	// DefaultSeverity is the severity a policy that enables this row without an
	// override receives. The publication tier is derived from it.
	DefaultSeverity Severity
	Lane            Lane
	// Observation is the population this row is measured over: a member of the
	// declared observation vocabulary, named by reference. A producing lane
	// declares exactly one; a declared lane declares none.
	Observation Reference
	// Collection is the query or observation family that supplies this row's
	// subjects. A solver-observed row names one; other lanes name none. The
	// named family seals above this surface, so Seal checks the reference
	// shape and a post-seal directory joins the issued inventory.
	Collection Reference
	// Sites are the geometries this row is the published code for, among
	// LaneBranch codes that share Observation. A polarity pair over one
	// population declares none. A row declares every geometry whose findings
	// carry its code, so a population measured at several places inside one
	// value publishes one code per code rather than one code per place.
	Sites []Site
	// Fact names the declaration whose facts decide this row. A solver-observed
	// row names one; a static row reads no fact and names none.
	Fact Reference
	// Witnesses is how many located witnesses this row's producer supplies
	// beside the finding's own location. Every AnchorWitness reference names one
	// of them by ordinal, and every declared witness is named by at least one
	// reference, so a row can neither read a location its producer does not
	// supply nor oblige a producer to locate something nothing renders.
	Witnesses uint8
	// Context is the witness the "where" section renders: the surrounding place
	// the finding is decided in. A row declares one exactly when it renders that
	// section.
	Context uint8
	// Requirements is the typed payload a producer must supply. It is exactly
	// the set the row's own presentation reads.
	Requirements  Requirement
	Message, Help Text
	Evidence      []Evidence
	Labels        []Label
	// Variants is the per-verdict presentation of a row whose collector answers
	// in a declared vocabulary. A row declares either one presentation of its
	// own or a set of variants, never both: two presentations of one code would
	// leave a reader to decide which one a finding rendered from.
	Variants []Variant
	Render   []Section
}

// Entry is one admitted diagnostic declaration. It is immutable once built.
type Entry struct {
	code            Code
	id              schema.EntryID
	family          Reference
	defaultSeverity Severity
	lane            Lane
	observation     Reference
	collection      Reference
	sites           []Site
	fact            Reference
	witnesses       uint8
	context         uint8
	requirements    Requirement
	message, help   Line
	evidence        []EvidenceRow
	labels          []LabelRow
	variants        []VariantRow
	render          []Section
}

// New admits one authored declaration. A rejected spec returns no entry rather
// than a partially usable one. Admission states the row's own form; the
// relations between rows, and between a row and the table it is sealed into,
// are stated by Seal.
func New(spec Spec) (*Entry, bool) {
	if !spec.Code.Available() || !spec.Family.Available() || !spec.DefaultSeverity.Available() || !spec.Lane.Available() {
		return nil, false
	}
	// A producing lane is measured over exactly one declared population, and a
	// lane with no producer is measured over none. Which population the name
	// resolves to is stated at seal, against the vocabulary that declares it.
	if spec.Observation.Declared() != spec.Observation.Available() || spec.Lane.Produces() != spec.Observation.Declared() {
		return nil, false
	}
	// A solver-observed row is decided by facts, so it names the declaration
	// that produces them. A static row reads no fact and names none.
	if spec.Fact.Declared() != spec.Fact.Available() || (spec.Lane == LaneBranch) != spec.Fact.Declared() {
		return nil, false
	}
	if spec.Collection.Declared() != spec.Collection.Available() || (spec.Lane == LaneBranch) != spec.Collection.Declared() {
		return nil, false
	}
	if spec.Lane == LaneBranch && spec.Collection.Surface != schema.SurfaceKindQuery && spec.Collection.Surface != schema.SurfaceKindObservation {
		return nil, false
	}
	if !sitesAdmissible(spec.Sites, spec.Lane) {
		return nil, false
	}
	// A row declares one presentation of its own or a set of verdict variants.
	// Authoring both would publish two renderings of one code; authoring
	// neither would publish a code that renders nothing.
	if len(spec.Variants) != 0 {
		if spec.Message != "" || spec.Help != "" || spec.Requirements != RequiresInvalid ||
			len(spec.Evidence) != 0 || len(spec.Labels) != 0 {
			return nil, false
		}
		return newVariantEntry(spec)
	}
	message, messageOK := newLine(spec.Message)
	help, helpOK := newLine(spec.Help)
	if !messageOK || !helpOK {
		return nil, false
	}
	entry := &Entry{
		code:            spec.Code,
		id:              schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(spec.Code)),
		family:          spec.Family,
		defaultSeverity: spec.DefaultSeverity,
		lane:            spec.Lane,
		observation:     spec.Observation,
		collection:      spec.Collection,
		sites:           append([]Site(nil), spec.Sites...),
		fact:            spec.Fact,
		witnesses:       spec.Witnesses,
		context:         spec.Context,
		requirements:    spec.Requirements,
		message:         message,
		help:            help,
	}
	evidence, evidenceOK := admitEvidence(spec.Evidence)
	labels, labelsOK := admitLabels(spec.Labels)
	if !evidenceOK || !labelsOK {
		return nil, false
	}
	entry.evidence, entry.labels = evidence, labels
	if !renderPlanAdmissible(spec.Render) {
		return nil, false
	}
	entry.render = append([]Section(nil), spec.Render...)
	if !entry.witnessRosterAdmissible() {
		return nil, false
	}
	return entry, entry.EntryAvailable()
}

// newVariantEntry admits a row whose presentation is per verdict. The row's
// own identity, gating, and collection are admitted exactly as any other row's;
// what differs is that every rendered line belongs to one declared answer.
//
// A variant's verdict is a member of a vocabulary sealed above this surface, so
// admission states the shape - a verdict is named, named once, and its
// presentation reads exactly the payload it declares - and Seal states that the
// named member is declared.
func newVariantEntry(spec Spec) (*Entry, bool) {
	entry := &Entry{
		code:            spec.Code,
		id:              schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(spec.Code)),
		family:          spec.Family,
		defaultSeverity: spec.DefaultSeverity,
		lane:            spec.Lane,
		observation:     spec.Observation,
		collection:      spec.Collection,
		sites:           append([]Site(nil), spec.Sites...),
		fact:            spec.Fact,
		witnesses:       spec.Witnesses,
		context:         spec.Context,
	}
	declared := make(map[uint16]struct{}, len(spec.Variants))
	for _, variant := range spec.Variants {
		if variant.Verdict == 0 {
			return nil, false
		}
		if _, duplicate := declared[variant.Verdict]; duplicate {
			return nil, false
		}
		declared[variant.Verdict] = struct{}{}
		message, messageOK := newLine(variant.Message)
		help, helpOK := newLine(variant.Help)
		evidence, evidenceOK := admitEvidence(variant.Evidence)
		labels, labelsOK := admitLabels(variant.Labels)
		if !messageOK || !helpOK || !evidenceOK || !labelsOK {
			return nil, false
		}
		row := VariantRow{
			verdict: variant.Verdict, requirements: variant.Requirements,
			message: message, help: help, evidence: evidence, labels: labels,
		}
		// The payload a producer supplies for one answer is exactly the payload
		// that answer's presentation reads. The row-level law states this once
		// for a row with a single presentation; a variant states it for itself,
		// because a payload another answer names is a field this one's producer
		// would have to fabricate.
		if !requirementsCover(row.requirements, row.presentation().reads()) {
			return nil, false
		}
		entry.variants = append(entry.variants, row)
	}
	if !renderPlanAdmissible(spec.Render) {
		return nil, false
	}
	entry.render = append([]Section(nil), spec.Render...)
	if !entry.witnessRosterAdmissible() {
		return nil, false
	}
	return entry, entry.EntryAvailable()
}

// witnessRosterAdmissible states the located-witness contract of one row, in
// both directions. A reference may name only a witness the producer supplies,
// and a supplied witness must be named by something that renders it: the first
// half stops a presentation reading a location that does not exist, the second
// stops a row obliging its producer to locate a place nothing shows. The
// context witness is a reference like any other, and it exists exactly when the
// row renders the section that shows it.
func (entry *Entry) witnessRosterAdmissible() bool {
	named := make([]bool, int(entry.witnesses)+1)
	reference := func(anchor Anchor, witness uint8) bool {
		if anchor != AnchorWitness {
			return witness == 0
		}
		if witness == 0 || witness > entry.witnesses {
			return false
		}
		named[witness] = true
		return true
	}
	for _, evidence := range entry.evidence {
		if !reference(evidence.Anchor, evidence.Witness) {
			return false
		}
	}
	for _, label := range entry.labels {
		if !reference(label.Anchor, label.Witness) {
			return false
		}
	}
	for _, variant := range entry.variants {
		for _, evidence := range variant.evidence {
			if !reference(evidence.Anchor, evidence.Witness) {
				return false
			}
		}
		for _, label := range variant.labels {
			if !reference(label.Anchor, label.Witness) {
				return false
			}
		}
	}
	if (entry.context != 0) != entry.Renders(SectionContext) {
		return false
	}
	if entry.context != 0 {
		if entry.context > entry.witnesses {
			return false
		}
		named[entry.context] = true
	}
	for witness := uint8(1); witness <= entry.witnesses; witness++ {
		if !named[witness] {
			return false
		}
	}
	return true
}

// requirementsCover states the payload contract in both directions: a field a
// presentation reads is required, and a field nothing reads is not.
func requirementsCover(requirements, reads Requirement) bool {
	return requirements.has(reads) && reads.has(requirements)
}

func admitEvidence(specs []Evidence) ([]EvidenceRow, bool) {
	rows := make([]EvidenceRow, 0, len(specs))
	for _, evidence := range specs {
		detail, detailOK := newLine(evidence.Detail)
		if !detailOK || !evidence.Anchor.Available() || evidence.Kind == "" || evidence.Trust == "" || evidence.Reason == "" {
			return nil, false
		}
		rows = append(rows, EvidenceRow{Anchor: evidence.Anchor, Witness: evidence.Witness, Kind: evidence.Kind, Trust: evidence.Trust, Reason: evidence.Reason, Detail: detail})
	}
	return rows, true
}

func admitLabels(specs []Label) ([]LabelRow, bool) {
	rows := make([]LabelRow, 0, len(specs))
	for _, label := range specs {
		text, textOK := newLine(label.Text)
		if !textOK || !label.Anchor.Available() {
			return nil, false
		}
		rows = append(rows, LabelRow{Anchor: label.Anchor, Witness: label.Witness, Text: text})
	}
	return rows, true
}

// presentation is this variant's rendering.
func (variant VariantRow) presentation() Presentation {
	return Presentation{
		Requirements: variant.requirements, Message: variant.message, Help: variant.help,
		Evidence: variant.evidence, Labels: variant.labels,
	}
}

// renderPlanAdmissible states that a row publishes a declared, ordered set of
// sections and never repeats one.
func renderPlanAdmissible(sections []Section) bool {
	if len(sections) == 0 {
		return false
	}
	var seen [sectionLimit]bool
	for _, section := range sections {
		if !section.Available() || seen[section] {
			return false
		}
		seen[section] = true
	}
	return true
}

func (entry *Entry) Key() schema.Key { return schema.Key(entry.code) }

// EntryAvailable is the root's admissibility question: does this row identify
// one entry. Whether the diagnostic it identifies agrees with the table it is
// sealed into is the surface's own law.
func (entry *Entry) EntryAvailable() bool {
	return entry != nil && entry.code.Available() && entry.id.Available()
}

func (entry *Entry) Code() Code { return entry.code }

func (entry *Entry) ID() schema.EntryID { return entry.id }

// Family is the declared publication family this row is gated by. A consumer
// that renders or gates on the family resolves this reference in the sealed
// structural vocabulary and reads the member's declared spelling.
func (entry *Entry) Family() Reference { return entry.family }

func (entry *Entry) DefaultSeverity() Severity { return entry.defaultSeverity }

// Tier is the publication tier this row belongs to, derived from its declared
// default severity.
func (entry *Entry) Tier() Tier {
	switch entry.defaultSeverity {
	case SeverityError:
		return TierError
	case SeverityWarning, SeverityHint:
		return TierAdvisory
	default:
		return TierInvalid
	}
}

func (entry *Entry) Lane() Lane { return entry.lane }

// Collectable reports whether a producer is installed for this row. A policy
// that enables a row without one would receive a clean empty report for a
// family nothing has collected, so admission refuses it.
func (entry *Entry) Collectable() bool { return entry != nil && entry.lane.Produces() }

// Observation is the declared population this row is measured over.
func (entry *Entry) Observation() Reference { return entry.observation }

// Collection is the query or observation family that supplies this row's
// subjects.
func (entry *Entry) Collection() Reference { return entry.collection }

// SiteCount is how many geometries this row is the published code for.
func (entry *Entry) SiteCount() int { return len(entry.sites) }

// SiteAt returns one declared geometry in declaration order.
func (entry *Entry) SiteAt(index int) (Site, bool) {
	if index < 0 || index >= len(entry.sites) {
		return SiteNone, false
	}
	return entry.sites[index], true
}

// Sited reports whether this row is the published code for one geometry.
func (entry *Entry) Sited(site Site) bool {
	for _, declared := range entry.sites {
		if declared == site {
			return true
		}
	}
	return false
}

// Fact is the declaration whose facts decide this row.
func (entry *Entry) Fact() Reference { return entry.fact }

func (entry *Entry) Requirements() Requirement { return entry.requirements }

// Witnesses is how many located witnesses this row's producer supplies beside
// the finding's own location.
func (entry *Entry) Witnesses() uint8 { return entry.witnesses }

// Context is the witness the "where" section renders, or zero when the row
// renders no such section.
func (entry *Entry) Context() uint8 { return entry.context }

func (entry *Entry) Message() Line { return entry.message }

func (entry *Entry) Help() Line { return entry.help }

// VariantCount is how many declared answers this row renders. A row with one
// presentation of its own has none.
func (entry *Entry) VariantCount() int { return len(entry.variants) }

// VariantAt returns one declared answer in declaration order.
func (entry *Entry) VariantAt(index int) (VariantRow, bool) {
	if index < 0 || index >= len(entry.variants) {
		return VariantRow{}, false
	}
	return entry.variants[index], true
}

// Presentation is the rendering one finding publishes from. A row with no
// variants renders its own presentation and answers under any verdict, so a
// producer that has no vocabulary to answer in supplies none. A row with
// variants renders only the answer it is given, so a finding that names no
// declared verdict has nothing to render and is refused here rather than
// rendered under whichever answer happens to be first.
func (entry *Entry) Presentation(verdict uint16) (Presentation, bool) {
	if entry == nil {
		return Presentation{}, false
	}
	if len(entry.variants) == 0 {
		if verdict != 0 {
			return Presentation{}, false
		}
		return Presentation{
			Requirements: entry.requirements, Message: entry.message, Help: entry.help,
			Evidence: entry.evidence, Labels: entry.labels,
		}, entry.message.Available()
	}
	for _, variant := range entry.variants {
		if variant.verdict == verdict {
			return variant.presentation(), true
		}
	}
	return Presentation{}, false
}

func (entry *Entry) EvidenceCount() int { return len(entry.evidence) }

func (entry *Entry) EvidenceAt(index int) (EvidenceRow, bool) {
	if index < 0 || index >= len(entry.evidence) {
		return EvidenceRow{}, false
	}
	return entry.evidence[index], true
}

func (entry *Entry) LabelCount() int { return len(entry.labels) }

func (entry *Entry) LabelAt(index int) (LabelRow, bool) {
	if index < 0 || index >= len(entry.labels) {
		return LabelRow{}, false
	}
	return entry.labels[index], true
}

func (entry *Entry) RenderCount() int { return len(entry.render) }

func (entry *Entry) RenderAt(index int) (Section, bool) {
	if index < 0 || index >= len(entry.render) {
		return SectionInvalid, false
	}
	return entry.render[index], true
}

// Renders reports whether this row publishes one section.
func (entry *Entry) Renders(section Section) bool {
	for _, declared := range entry.render {
		if declared == section {
			return true
		}
	}
	return false
}

// EntryContent writes this row's declared data: the family it is gated by, the
// severity it defaults to, the lane its subjects arrive on, the population it
// is measured over, the declaration whose facts decide it, the payload a
// producer must supply, and the exact presentation it is rendered from. A row
// is pure data, so all of it is content.
//
// The publication tier is not written: it is derived from the declared default
// severity, which is, so a tier that moves moves the digest through the
// severity it is read from. A parsed line is written as its authored template
// for the same reason: the segments and the payload a line reads are derived
// from that template.
func (entry *Entry) EntryContent(content *framing.Writer) error {
	if err := referenceContent(content, entry.family); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.defaultSeverity)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.lane)); err != nil {
		return err
	}
	if err := referenceContent(content, entry.observation); err != nil {
		return err
	}
	if err := referenceContent(content, entry.collection); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.sites))); err != nil {
		return err
	}
	for _, site := range entry.sites {
		if err := content.Uint(uint64(site)); err != nil {
			return err
		}
	}
	if err := referenceContent(content, entry.fact); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.witnesses)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.context)); err != nil {
		return err
	}
	if err := content.Uint(uint64(entry.requirements)); err != nil {
		return err
	}
	if err := content.String(string(entry.message.text)); err != nil {
		return err
	}
	if err := content.String(string(entry.help.text)); err != nil {
		return err
	}
	return entry.presentationContent(content)
}

// referenceContent writes one named identity: the surface it is declared on
// and the authored key it names there. What the reference resolves to is
// derived from those two at seal, so the resolution is not written a second
// time.
func referenceContent(content *framing.Writer, reference Reference) error {
	if err := content.Uint(uint64(reference.Surface)); err != nil {
		return err
	}
	return content.String(string(reference.Key))
}

// presentationContent writes the row's evidence lines, source labels, declared
// verdict variants, and render plan, each in declaration order behind its own
// arity: the order a row declares them in is the order they are published in.
func (entry *Entry) presentationContent(content *framing.Writer) error {
	if err := evidenceContent(content, entry.evidence); err != nil {
		return err
	}
	if err := labelContent(content, entry.labels); err != nil {
		return err
	}
	if err := content.Count(uint64(len(entry.variants))); err != nil {
		return err
	}
	for _, variant := range entry.variants {
		if err := content.Record(contentRecordVariant); err != nil {
			return err
		}
		if err := content.Uint(uint64(variant.verdict)); err != nil {
			return err
		}
		if err := content.Uint(uint64(variant.requirements)); err != nil {
			return err
		}
		if err := content.String(string(variant.message.text)); err != nil {
			return err
		}
		if err := content.String(string(variant.help.text)); err != nil {
			return err
		}
		if err := evidenceContent(content, variant.evidence); err != nil {
			return err
		}
		if err := labelContent(content, variant.labels); err != nil {
			return err
		}
	}
	if err := content.Count(uint64(len(entry.render))); err != nil {
		return err
	}
	for _, section := range entry.render {
		if err := content.Record(contentRecordSection); err != nil {
			return err
		}
		if err := content.Uint(uint64(section)); err != nil {
			return err
		}
	}
	return nil
}

func evidenceContent(content *framing.Writer, rows []EvidenceRow) error {
	if err := content.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, evidence := range rows {
		if err := content.Record(contentRecordEvidence); err != nil {
			return err
		}
		if err := content.Uint(uint64(evidence.Anchor)); err != nil {
			return err
		}
		if err := content.Uint(uint64(evidence.Witness)); err != nil {
			return err
		}
		if err := content.String(evidence.Kind); err != nil {
			return err
		}
		if err := content.String(evidence.Trust); err != nil {
			return err
		}
		if err := content.String(evidence.Reason); err != nil {
			return err
		}
		if err := content.String(string(evidence.Detail.text)); err != nil {
			return err
		}
	}
	return nil
}

func labelContent(content *framing.Writer, rows []LabelRow) error {
	if err := content.Count(uint64(len(rows))); err != nil {
		return err
	}
	for _, label := range rows {
		if err := content.Record(contentRecordLabel); err != nil {
			return err
		}
		if err := content.Uint(uint64(label.Anchor)); err != nil {
			return err
		}
		if err := content.Uint(uint64(label.Witness)); err != nil {
			return err
		}
		if err := content.String(string(label.Text.text)); err != nil {
			return err
		}
	}
	return nil
}

// reads is the payload every part of this row's own presentation consumes. A
// row that renders per verdict has no presentation of its own; each variant
// states its own payload contract at admission.
func (entry *Entry) reads() Requirement {
	presentation, available := entry.Presentation(0)
	if !available {
		return RequiresInvalid
	}
	reads := presentation.reads()
	// The context section reads a located witness whether or not any evidence
	// line or label happens to name one, so the payload contract states it here
	// rather than leaving it to a coincidence of authoring.
	if entry.context != 0 {
		reads |= RequiresWitness
	}
	return reads
}

// surface is the diagnostic contribution to the analyzer declaration root.
type surface struct{ entries []*Entry }

// NewSurface hands one ordered set of diagnostic declarations to the table.
func NewSurface(entries []*Entry) schema.Surface { return surface{entries: entries} }

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindDiagnostic }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.entries))
	for index, entry := range contribution.entries {
		entries[index] = entry
	}
	return entries
}

// Seal states the diagnostic surface's own laws over the indexed view and the
// surfaces sealed below it.
func (contribution surface) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	// How many rows a surface holds is that surface's own law. This one is the
	// analyzer's whole published vocabulary and every reader of a verdict
	// resolves its row here, so an inventory of none is an unusable table rather
	// than a surface with nothing in it yet.
	if view.Count() == 0 {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, schema.EntryID{}, LawSurfacePopulated, schema.DispositionIncomplete)
	}
	observations := make(map[schema.Key]schema.EntryID, view.Count())
	branchSited := make(map[schema.Key]map[Site]schema.EntryID, view.Count())
	branchUnsited := make(map[schema.Key]schema.EntryID, view.Count())
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || entry == nil {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// Entry uniqueness is the root's law. What the surface states here is
		// that the identity a verdict carries is this surface's own derivation
		// of the published code, so a row cannot travel under another surface's
		// identity and a code cannot drift from the entry it names.
		if !entry.code.Available() || entry.id != schema.NewEntryID(schema.SurfaceKindDiagnostic, schema.Key(entry.code)) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawCodeIdentity, schema.DispositionMalformed)
		}
		if failure := sealFamily(entry, sealed); failure.Available() {
			return failure
		}
		if !entry.defaultSeverity.Available() || entry.Tier() == TierInvalid {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawTierValid, schema.DispositionIncomplete)
		}
		// The payload a producer must supply is exactly the payload the row's
		// own presentation reads. A field nothing reads is dead weight on every
		// producer; a read nothing requires is a hole a renderer would find at
		// render time.
		if entry.VariantCount() == 0 {
			reads := entry.reads()
			if !entry.requirements.has(reads) {
				return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRequirementCovered, schema.DispositionIncomplete)
			}
			if !reads.has(entry.requirements) {
				return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRequirementCovered, schema.DispositionMalformed)
			}
		}
		if failure := sealVariants(entry, sealed); failure.Available() {
			return failure
		}
		if failure := sealObservation(entry, sealed); failure.Available() {
			return failure
		}
		if failure := sealCollection(entry); failure.Available() {
			return failure
		}
		if failure := sealBranchSite(entry, branchSited, branchUnsited); failure.Available() {
			return failure
		}
		if entry.lane == LaneStatic {
			if prior, duplicate := observations[entry.observation.Key]; duplicate {
				return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, prior, LawObservationUnique, schema.DispositionDuplicate)
			}
			observations[entry.observation.Key] = entry.id
		}
		if failure := sealFact(entry, sealed); failure.Available() {
			return failure
		}
		// A section a row never renders publishes nothing, so declaring the
		// content without the section is an incomplete row. A variant's content
		// is the row's content for the answer it names, so it is held to the
		// same render plan.
		if entry.declaresEvidence() && !entry.Renders(SectionEvidence) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRenderComplete, schema.DispositionIncomplete)
		}
		if entry.declaresHelp() && !entry.Renders(SectionHelp) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRenderComplete, schema.DispositionIncomplete)
		}
		if !entry.Renders(SectionSummary) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRenderComplete, schema.DispositionIncomplete)
		}
	}
	return schema.SealFailure{}
}

// declaresEvidence reports whether this row publishes any proof line, under
// its own presentation or under one of its declared answers.
func (entry *Entry) declaresEvidence() bool {
	if len(entry.evidence) > 0 {
		return true
	}
	for _, variant := range entry.variants {
		if len(variant.evidence) > 0 {
			return true
		}
	}
	return false
}

func (entry *Entry) declaresHelp() bool {
	if entry.help.Available() {
		return true
	}
	for _, variant := range entry.variants {
		if variant.help.Available() {
			return true
		}
	}
	return false
}

// sealVariants states that every declared answer names a member of the sealed
// conformance-verdict vocabulary. The vocabulary's ordinals are owned by the
// judgment that produces them, so a row keyed by an ordinal the vocabulary does
// not declare is a presentation no collector can ever select, and a member the
// judgment adds without a variant is a finding that renders nothing.
func sealVariants(entry *Entry, sealed schema.Sealed) schema.SealFailure {
	for _, variant := range entry.variants {
		if _, disposition := structure.Member(sealed, structure.CategoryConformanceVerdict, variant.verdict); disposition != schema.DispositionAccepted {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawVariantDeclared, disposition)
		}
	}
	return schema.SealFailure{}
}

// sealFamily resolves one row's family reference and states the two halves of
// the family law: the row names a member of the declared family vocabulary, and
// that member's declared spelling is the first segment of the published code.
// A consumer gating on the family it reads off a code and the boundary gating
// on the declared family therefore reach the same decision, and a family the
// analyzer has never published before is a row on the structural surface rather
// than a member of an enum here.
func sealFamily(entry *Entry, sealed schema.Sealed) schema.SealFailure {
	if entry.family.Surface != schema.SurfaceKindStructure {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFamilyDeclared, schema.DispositionMalformed)
	}
	member, disposition := structure.Resolve(sealed, entry.family.Key, structure.CategoryDiagnosticFamily)
	if disposition != schema.DispositionAccepted {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFamilyDeclared, disposition)
	}
	family, familyOK := entry.code.Family()
	if !familyOK || family != member.Spelling() {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFamilyDeclared, schema.DispositionMalformed)
	}
	return schema.SealFailure{}
}

// sealObservation resolves one row's observation reference. The population a
// row is measured over is declared once, on the structural surface, so this is
// the only bound on it: a producing row names a member of that vocabulary, and
// a row with no producer names none.
func sealObservation(entry *Entry, sealed schema.Sealed) schema.SealFailure {
	if !entry.observation.Declared() {
		return schema.SealFailure{}
	}
	if entry.observation.Surface != schema.SurfaceKindStructure {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawObservationDeclared, schema.DispositionMalformed)
	}
	if _, disposition := structure.Resolve(sealed, entry.observation.Key, structure.CategoryDiagnosticObservation); disposition != schema.DispositionAccepted {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawObservationDeclared, disposition)
	}
	return schema.SealFailure{}
}

// sitesAdmissible states one row's own site shape: a geometry is named only by
// a solver-observed row, every named geometry is a member of the vocabulary,
// and no geometry is named twice by one row.
func sitesAdmissible(sites []Site, lane Lane) bool {
	if len(sites) == 0 {
		return true
	}
	if lane != LaneBranch {
		return false
	}
	var seen [siteLimit]bool
	for _, site := range sites {
		if !site.Available() || seen[site] {
			return false
		}
		seen[site] = true
	}
	return true
}

// sealBranchSite states that LaneBranch rows sharing an observation
// population are either a polarity pair (no site) or a sited family
// whose (population, site) pairs are unique. Mixing a sited row with an
// unsited sibling on the same population is malformed. A row naming several
// geometries claims each of them, so the pair uniqueness is stated per named
// geometry rather than per row.
func sealBranchSite(entry *Entry, sited map[schema.Key]map[Site]schema.EntryID, unsited map[schema.Key]schema.EntryID) schema.SealFailure {
	if entry.lane != LaneBranch || !entry.observation.Declared() {
		if len(entry.sites) != 0 {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawSiteUnique, schema.DispositionMalformed)
		}
		return schema.SealFailure{}
	}
	population := entry.observation.Key
	if len(entry.sites) != 0 {
		if prior, shared := unsited[population]; shared {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, prior, LawSiteUnique, schema.DispositionMalformed)
		}
		bySite := sited[population]
		if bySite == nil {
			bySite = make(map[Site]schema.EntryID)
			sited[population] = bySite
		}
		for _, site := range entry.sites {
			if prior, duplicate := bySite[site]; duplicate {
				return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, prior, LawSiteUnique, schema.DispositionDuplicate)
			}
			bySite[site] = entry.id
		}
		return schema.SealFailure{}
	}
	if _, shared := sited[population]; shared {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawSiteUnique, schema.DispositionMalformed)
	}
	if _, seen := unsited[population]; !seen {
		unsited[population] = entry.id
	}
	return schema.SealFailure{}
}

// sealCollection states the collection-plan shape. A solver-observed row names
// a query or observation family; other lanes name none. The named family
// seals above this surface, so the join to the issued inventory is the
// post-seal directory rather than a downward Resolve.
func sealCollection(entry *Entry) schema.SealFailure {
	if (entry.lane == LaneBranch) != entry.collection.Declared() {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawCollectionDeclared, schema.DispositionIncomplete)
	}
	if !entry.collection.Declared() {
		return schema.SealFailure{}
	}
	if !entry.collection.Available() ||
		(entry.collection.Surface != schema.SurfaceKindQuery && entry.collection.Surface != schema.SurfaceKindObservation) {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawCollectionDeclared, schema.DispositionMalformed)
	}
	return schema.SealFailure{}
}

// sealFact resolves one row's fact reference against the table it is sealed
// into. A reference to a surface below this one must name an entry that is
// actually there; a reference upward names a table that is not sealed yet, and
// the catalog order says that is malformed rather than merely unresolved.
func sealFact(entry *Entry, sealed schema.Sealed) schema.SealFailure {
	if !entry.fact.Declared() {
		return schema.SealFailure{}
	}
	if _, disposition := sealed.Resolve(entry.fact.Surface, entry.fact.Key); disposition != schema.DispositionAccepted {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFactResolves, disposition)
	}
	return schema.SealFailure{}
}

// Table is the derived read model of the sealed diagnostic surface. Every
// lookup a consumer performs is a projection of the same sealed rows, so a row
// added to the surface appears in all of them at once.
type branchObservationKey struct {
	population schema.Key
	site       Site
}

type Table struct {
	entries       []*Entry
	byCode        map[Code]*Entry
	byObservation map[schema.Key]*Entry
	byBranch      map[branchObservationKey]*Entry
}

// NewTable projects one sealed diagnostic view. It is the only construction of
// a diagnostic lookup in the analyzer.
func NewTable(view schema.View) (Table, bool) {
	if view.Kind() != schema.SurfaceKindDiagnostic || !view.Available() {
		return Table{}, false
	}
	table := Table{
		entries:       make([]*Entry, 0, view.Count()),
		byCode:        make(map[Code]*Entry, view.Count()),
		byObservation: make(map[schema.Key]*Entry, view.Count()),
		byBranch:      make(map[branchObservationKey]*Entry, view.Count()),
	}
	for position := 0; position < view.Count(); position++ {
		row, rowOK := view.At(position)
		entry, entryOK := row.(*Entry)
		if !rowOK || !entryOK || !entry.EntryAvailable() {
			return Table{}, false
		}
		table.entries = append(table.entries, entry)
		table.byCode[entry.code] = entry
		if entry.lane == LaneStatic {
			table.byObservation[entry.observation.Key] = entry
		}
		if entry.lane == LaneBranch {
			for _, site := range entry.sites {
				table.byBranch[branchObservationKey{population: entry.observation.Key, site: site}] = entry
			}
		}
	}
	return table, table.Available()
}

func (table Table) Available() bool { return len(table.entries) > 0 }

func (table Table) Count() int { return len(table.entries) }

// At returns one row in declaration order.
func (table Table) At(position int) (*Entry, bool) {
	if position < 0 || position >= len(table.entries) {
		return nil, false
	}
	return table.entries[position], true
}

// ForCode resolves one row by its published identity.
func (table Table) ForCode(code Code) (*Entry, bool) {
	entry, known := table.byCode[code]
	return entry, known
}

// ForStaticObservation resolves the row one artifact-issued observation
// population feeds, by the declared identity of that population. The seal law
// makes that row unique. A consumer holding an artifact-compiled observation
// ordinal resolves the declared member at that ordinal in the sealed structural
// vocabulary and names it here.
func (table Table) ForStaticObservation(population schema.Key) (*Entry, bool) {
	entry, known := table.byObservation[population]
	return entry, known
}

// ForBranchObservation resolves the LaneBranch row one sited observation
// instance feeds. Polarity pairs omit Site and are not in this index.
func (table Table) ForBranchObservation(population schema.Key, site Site) (*Entry, bool) {
	if !population.Available() || !site.Available() {
		return nil, false
	}
	entry, known := table.byBranch[branchObservationKey{population: population, site: site}]
	return entry, known
}
