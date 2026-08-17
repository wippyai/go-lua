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
// Deferred resolutions. Three identities a row is eventually read under are
// not yet spellable:
//
//   - the collection plan, which awaits the query surface. The branch lane's
//     subjects arrive through the value-summary query today, unnamed here.
//   - the denominator, which awaits an observation surface of its own. The
//     denominator surface is in the catalog and declares closed worlds, but it
//     sits above this one and a denominator names the entry that owns it, so the
//     resolution lands there rather than as a reference out of a row. The worlds
//     declared today are the axes' coordinate populations; none quantifies over
//     a diagnostic row's population.
//   - the evidence projection, which awaits the same surface. It is declared
//     today as the row's own evidence lines.
package diagnostic

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// Content record markers. They separate the presentation collections one row
// writes, so an evidence line can never be read as a label.
const (
	contentRecordEvidence uint64 = iota + 1
	contentRecordLabel
	contentRecordSection
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
	family          Reference
	defaultSeverity Severity
	lane            Lane
	observation     Reference
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
	if err := referenceContent(content, entry.fact); err != nil {
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

// presentationContent writes the row's evidence lines, source labels, and
// render plan, each in declaration order behind its own arity: the order a row
// declares them in is the order they are published in.
func (entry *Entry) presentationContent(content *framing.Writer) error {
	if err := content.Count(uint64(len(entry.evidence))); err != nil {
		return err
	}
	for _, evidence := range entry.evidence {
		if err := content.Record(contentRecordEvidence); err != nil {
			return err
		}
		if err := content.Uint(uint64(evidence.Anchor)); err != nil {
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
	if err := content.Count(uint64(len(entry.labels))); err != nil {
		return err
	}
	for _, label := range entry.labels {
		if err := content.Record(contentRecordLabel); err != nil {
			return err
		}
		if err := content.Uint(uint64(label.Anchor)); err != nil {
			return err
		}
		if err := content.String(string(label.Text.text)); err != nil {
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
	// How many rows a surface holds is that surface's own law. This one is the
	// analyzer's whole published vocabulary and every reader of a verdict
	// resolves its row here, so an inventory of none is an unusable table rather
	// than a surface with nothing in it yet.
	if view.Count() == 0 {
		return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, schema.EntryID{}, LawSurfacePopulated, schema.DispositionIncomplete)
	}
	observations := make(map[schema.Key]schema.EntryID, view.Count())
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
		reads := entry.reads()
		if !entry.requirements.has(reads) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRequirementCovered, schema.DispositionIncomplete)
		}
		if !reads.has(entry.requirements) {
			return schema.SurfaceLawFailure(schema.SurfaceKindDiagnostic, entry.id, LawRequirementCovered, schema.DispositionMalformed)
		}
		if failure := sealObservation(entry, sealed); failure.Available() {
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
type Table struct {
	entries       []*Entry
	byCode        map[Code]*Entry
	byObservation map[schema.Key]*Entry
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
