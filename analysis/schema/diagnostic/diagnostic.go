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
// Deferred resolutions. Three identities a row will eventually name are not
// yet spellable, because the surfaces that own them are not in the declaration
// catalog:
//
//   - the collection plan, which awaits the query surface. The branch lane's
//     subjects arrive through the value-summary query today, unnamed here.
//   - the denominator, which awaits the observation surface. It is declared
//     today as the observation population the row is measured over, in the
//     artifact's own closed observation catalog.
//   - the evidence projection, which awaits the same observation surface. It is
//     declared today as the row's own evidence lines.
//
// Nothing here fakes those resolutions. The machinery that will perform them
// exists and is exercised: Reference names an entry on another surface, and
// Seal resolves it against the same table it is sealed into, so a reference to
// a surface added later resolves the moment that surface is registered.
package diagnostic

import (
	"strings"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
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

// Family is the closed publication family catalog. Publication is gated by
// family at the query boundary, so every row names exactly one.
type Family uint8

const (
	FamilyInvalid Family = iota
	FamilyAdvice
	FamilyType
	FamilyValue
	FamilyLint
	familyLimit
)

func (family Family) Available() bool { return family > FamilyInvalid && family < familyLimit }

func (family Family) String() string {
	switch family {
	case FamilyAdvice:
		return "advice"
	case FamilyType:
		return "type"
	case FamilyValue:
		return "value"
	case FamilyLint:
		return "lint"
	default:
		return ""
	}
}

// Severity is the closed severity a row defaults to. A policy may refine a
// row's severity within this vocabulary; it can never invent one.
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

func (severity Severity) String() string {
	switch severity {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityHint:
		return "hint"
	default:
		return ""
	}
}

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
	RequiresProofLocation
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
	// AnchorProof is the already-authenticated proof location the payload
	// carries.
	AnchorProof
	anchorLimit
)

func (anchor Anchor) Available() bool { return anchor > AnchorInvalid && anchor < anchorLimit }

// Requires is the payload field one anchor reads.
func (anchor Anchor) Requires() Requirement {
	if anchor == AnchorProof {
		return RequiresProofLocation
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
	SectionHelp
	sectionLimit
)

func (section Section) Available() bool { return section > SectionInvalid && section < sectionLimit }

// Evidence is one authored proof line of a row.
type Evidence struct {
	Anchor              Anchor
	Kind, Trust, Reason string
	Detail              Text
}

// Label is one authored source label of a row.
type Label struct {
	Anchor Anchor
	Text   Text
}

// EvidenceRow is one admitted proof line: the authored classification and the
// parsed detail line.
type EvidenceRow struct {
	Anchor              Anchor
	Kind, Trust, Reason string
	Detail              Line
}

// LabelRow is one admitted source label.
type LabelRow struct {
	Anchor Anchor
	Text   Line
}

// Spec is the authored declaration of one diagnostic. Every field is data.
type Spec struct {
	// Code is the row's published identity and its authored key.
	Code   Code
	Family Family
	// DefaultSeverity is the severity a policy that enables this row without an
	// override receives. The publication tier is derived from it.
	DefaultSeverity Severity
	Lane            Lane
	// Observation is the population this row is measured over. A producing lane
	// declares exactly one; a declared lane declares none.
	Observation programartifact.DiagnosticObservationKind
	// Fact names the declaration whose facts decide this row. A solver-observed
	// row names one; a static row reads no fact and names none.
	Fact Reference
	// Requirements is the typed payload a producer must supply. It is exactly
	// the set the row's own presentation reads.
	Requirements  Requirement
	Message, Help Text
	Evidence      []Evidence
	Labels        []Label
	Render        []Section
}

// Entry is one admitted diagnostic declaration. It is immutable once built.
type Entry struct {
	code            Code
	id              schema.EntryID
	family          Family
	defaultSeverity Severity
	lane            Lane
	observation     programartifact.DiagnosticObservationKind
	fact            Reference
	requirements    Requirement
	message, help   Line
	evidence        []EvidenceRow
	labels          []LabelRow
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
	// lane with no producer is measured over none.
	if spec.Lane.Produces() != observationAvailable(spec.Observation) {
		return nil, false
	}
	// A solver-observed row is decided by facts, so it names the declaration
	// that produces them. A static row reads no fact and names none.
	if spec.Fact.Declared() != spec.Fact.Available() || (spec.Lane == LaneBranch) != spec.Fact.Declared() {
		return nil, false
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
		fact:            spec.Fact,
		requirements:    spec.Requirements,
		message:         message,
		help:            help,
	}
	for _, evidence := range spec.Evidence {
		detail, detailOK := newLine(evidence.Detail)
		if !detailOK || !evidence.Anchor.Available() || evidence.Kind == "" || evidence.Trust == "" || evidence.Reason == "" {
			return nil, false
		}
		entry.evidence = append(entry.evidence, EvidenceRow{Anchor: evidence.Anchor, Kind: evidence.Kind, Trust: evidence.Trust, Reason: evidence.Reason, Detail: detail})
	}
	for _, label := range spec.Labels {
		text, textOK := newLine(label.Text)
		if !textOK || !label.Anchor.Available() {
			return nil, false
		}
		entry.labels = append(entry.labels, LabelRow{Anchor: label.Anchor, Text: text})
	}
	if !renderPlanAdmissible(spec.Render) {
		return nil, false
	}
	entry.render = append([]Section(nil), spec.Render...)
	return entry, entry.EntryAvailable()
}

// observationAvailable admits one artifact observation kind. The artifact
// format owns the catalog; this bound follows its declaration order, the same
// way the role-indexed projections of the rule table do.
func observationAvailable(kind programartifact.DiagnosticObservationKind) bool {
	return kind > programartifact.DiagnosticObservationInvalid && kind <= programartifact.DiagnosticObservationValueReferenceUnresolved
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

func (entry *Entry) Family() Family { return entry.family }

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

// Observation is the population this row is measured over.
func (entry *Entry) Observation() programartifact.DiagnosticObservationKind {
	return entry.observation
}

// Fact is the declaration whose facts decide this row.
func (entry *Entry) Fact() Reference { return entry.fact }

func (entry *Entry) Requirements() Requirement { return entry.requirements }

func (entry *Entry) Message() Line { return entry.message }

func (entry *Entry) Help() Line { return entry.help }

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

// reads is the payload every part of this row's presentation consumes.
func (entry *Entry) reads() Requirement {
	reads := entry.message.Requires() | entry.help.Requires()
	for _, evidence := range entry.evidence {
		reads |= evidence.Detail.Requires() | evidence.Anchor.Requires()
	}
	for _, label := range entry.labels {
		reads |= label.Text.Requires() | label.Anchor.Requires()
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
	observations := make(map[programartifact.DiagnosticObservationKind]schema.EntryID, view.Count())
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
		// The published code carries its own family, so a consumer gating on
		// the family it reads and the boundary gating on the declared family
		// reach the same decision.
		family, familyOK := entry.code.Family()
		if !entry.family.Available() || !familyOK || family != entry.family.String() {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFamilyDeclared, schema.DispositionMalformed)
		}
		if !entry.defaultSeverity.Available() || entry.Tier() == TierInvalid {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawTierValid, schema.DispositionIncomplete)
		}
		// The payload a producer must supply is exactly the payload the row's
		// own presentation reads. A field nothing reads is dead weight on every
		// producer; a read nothing requires is a hole a renderer would find at
		// render time.
		reads := entry.reads()
		if !entry.requirements.has(reads) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRequirementCovered, schema.DispositionIncomplete)
		}
		if !reads.has(entry.requirements) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRequirementCovered, schema.DispositionMalformed)
		}
		if entry.lane == LaneStatic {
			if prior, duplicate := observations[entry.observation]; duplicate {
				return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, prior, LawObservationUnique, schema.DispositionDuplicate)
			}
			observations[entry.observation] = entry.id
		}
		if failure := sealFact(entry, sealed); failure.Available() {
			return failure
		}
		// A section a row never renders publishes nothing, so declaring the
		// content without the section is an incomplete row.
		if len(entry.evidence) > 0 && !entry.Renders(SectionEvidence) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRenderComplete, schema.DispositionIncomplete)
		}
		if entry.help.Available() && !entry.Renders(SectionHelp) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRenderComplete, schema.DispositionIncomplete)
		}
		if !entry.Renders(SectionSummary) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRenderComplete, schema.DispositionIncomplete)
		}
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
	if !entry.fact.Available() {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFactResolves, schema.DispositionMalformed)
	}
	producer, registered := sealed.Surface(entry.fact.Surface)
	if !registered {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFactResolves, schema.DispositionMalformed)
	}
	if _, resolved := producer.ByID(schema.NewEntryID(entry.fact.Surface, entry.fact.Key)); !resolved {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawFactResolves, schema.DispositionIncomplete)
	}
	return schema.SealFailure{}
}

// Table is the derived read model of the sealed diagnostic surface. Every
// lookup a consumer performs is a projection of the same sealed rows, so a row
// added to the surface appears in all of them at once.
type Table struct {
	entries       []*Entry
	byCode        map[Code]*Entry
	byObservation map[programartifact.DiagnosticObservationKind]*Entry
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
		byObservation: make(map[programartifact.DiagnosticObservationKind]*Entry, view.Count()),
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
			table.byObservation[entry.observation] = entry
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
// population feeds. The seal law makes that row unique.
func (table Table) ForStaticObservation(kind programartifact.DiagnosticObservationKind) (*Entry, bool) {
	entry, known := table.byObservation[kind]
	return entry, known
}
