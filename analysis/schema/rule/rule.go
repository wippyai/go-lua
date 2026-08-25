// Package rule owns the rule surface of the analyzer declaration table: the
// record one rule is declared as, the typed contexts its hooks receive, and
// the surface laws the declaration root seals it under.
//
// The surface is blind to every domain. The principal set a rule declares
// against and the authority set it binds against are type parameters supplied
// by the composition, and a rule's own cold fragment and hot rule are type
// parameters of its declaration, so an authored hook is fully typed and never
// asserts. Erasure exists only in the private cell this package uses to hold
// declarations of different rules in one table.
//
// Every field is exported and every hook is a plain function value, so a
// declaration may be hand-written at the composition today and emitted by a
// generator into the owning package later without changing this interface.
// Nothing registers itself: templates are values, handed to the table at
// composition.
package rule

import (
	"crypto/sha256"
	"fmt"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	issuanceschema "github.com/wippyai/go-lua/analysis/schema/issuance"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	seal "github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
	"github.com/wippyai/go-lua/internal/framing"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	// The ordinal here is retired. A rule's role is its declaration position,
	// so the position is a construction rather than a property a row could
	// state differently from where it sits.
	_ schema.LawID = seal.SurfaceLawFloor + iota
	// The ordinal here is retired. Role uniqueness is the root's own law: one
	// role is one row, and two rows carrying one identity is stated over the
	// entry identity this surface derives.
	_
	LawSemanticIdentity
	LawSemanticUnique
	LawEntryShape
	// LawIssuanceDeclared states that a rule admitted from a compiled artifact
	// subscribes to at least one occurrence family. A mounted rule that
	// subscribes to nothing would be admitted onto a lane no artifact row could
	// ever reach it on.
	LawIssuanceDeclared
	// LawIssuanceLane states the converse: a rule that subscribes to an
	// occurrence family enters on the lane those rows are materialized from.
	LawIssuanceLane
	// The ordinal here is retired. Whether the semantic role vocabulary is
	// itself complete is the structural surface's own law, stated over the
	// category the roles are declared in. A role this surface names and that
	// vocabulary does not declare is an unresolved reference, which is stated
	// under LawSemanticIdentity.
	_
	// LawWritesResolves states that the axis a rule writes is a declared axis.
	// An axis is a writer principal, so a rule that names an undeclared
	// coordinate space would be admitted to write facts no table holds.
	LawWritesResolves
	// LawIssuanceResolves states that every term of a declared subscription is a
	// declared member of the vocabulary it names: the occurrence family, the
	// placement form, the operand polarity, and the execution cut.
	LawIssuanceResolves
	// LawOwnerResolves states that the axis that must supply this rule's
	// operand resolver is a declared axis. The resolver itself stays on the
	// bound cell; the table only names who must supply it and refuses a rule
	// whose owner is not a writer principal.
	LawOwnerResolves
	// LawIssuanceRequirementDeclared states that a subscription names the
	// operand shape it consumes. A subscription that names none would be placed
	// on every row of its occurrence family while its owner seals an operand for
	// a subset, so the two denominators would part company at construction. The
	// unrestricted shape is a declared member like any other, which is why
	// silence is a verdict rather than a default.
	LawIssuanceRequirementDeclared
	// LawIssuanceRequirementResolves states that the named operand shape is a
	// declared member of the requirement vocabulary.
	LawIssuanceRequirementResolves
	// LawProgramShape states that a Rule-owned execution program is a valid
	// callback-free ordered typed-join/fold declaration.
	LawProgramShape
	// LawProgramOutput states that writes map only to declared output columns.
	LawProgramOutput
)

// Lane is the closed admission lane of one rule. Mounted rules enter through a
// reusable Program artifact row. Activation is the mounted structural lane
// whose members are attached by their own owner rather than a generic graph
// row. Link rules are Link-owned and execute at the bootstrap point.
// Mounted-point rules are artifact-independent closures instantiated once at
// every mounted Point; they declare no artifact issuance row of their own.
type Lane uint8

const (
	LaneInvalid Lane = iota
	LaneMounted
	LaneActivation
	LaneLink
	LaneMountedPoint
)

func (lane Lane) Available() bool { return lane >= LaneMounted && lane <= LaneMountedPoint }

// Mounted reports whether the lane is materialized from an artifact row.
func (lane Lane) Mounted() bool { return lane == LaneMounted || lane == LaneActivation }

// Declaration is the cold context a rule's Declare hook receives. Principals
// is the composition's own principal record.
//
// Roles resolves exactly the roles this rule declared: its own identity and
// the further roles named on its Spec. A hook that reaches for a role the rule
// never declared resolves nothing, so an identity a rule consumes is an
// identity it is on record as consuming.
type Declaration[P any] struct {
	Roles      vocabulary.Roles
	Principals P
}

// Registration is the pre-seal slot handoff context. The schema binding is
// composition wiring.
type Registration[F any] struct {
	Fragment F
}

// Pairing is the cross-rule admission context. The schema binding and
// capability resolver are composition wiring.
type Pairing[F any] struct {
	Fragment F
}

// Binding is the hot binding context. Fragment is the cold fragment this
// rule's Declare hook produced; Authorities is the composition's authority
// record. The schema binding is composition wiring.
type Binding[A, F any] struct {
	Fragment    F
	Authorities A
}

// Finalization is the post-seal context. It runs only once the shared binding
// is terminal, which is the earliest point an occurrence issuer may be sealed.
type Finalization[A, H any] struct {
	Rule        H
	Authorities A
}

// OccurrenceCatalog is the owner-issued inventory for a rule lane whose
// occurrences are not issued by authored program rows. It is the neutral shape
// construction uses to enumerate the occurrences it must admit.
type OccurrenceCatalog interface {
	Count() int
	IDAt(index int) (identity.ContentID, bool)
}

// Issuance is one program-occurrence subscription. Occurrence names a sealed
// family predicate, Requirement names its admitted selection ABI, and Form
// names one closed geometry/stage/input program. Inputs and stages are not
// independently combinable here: impossible tuples are unrepresentable.
type Issuance struct {
	Occurrence schema.Key
	Form       schema.Key
	// Requirement is the operand shape this rule consumes at this issuance: the
	// structural admissibility a compiled row must carry for the placement to be
	// one the owner can seal an operand for. It is the shared denominator of
	// cold placement and owner issuance, so a rule that consumes every row of
	// its occurrence family names the unrestricted member rather than leaving
	// the term absent.
	Requirement schema.Key
}

func (issuance Issuance) Available() bool {
	return issuance.Occurrence.Available() && issuance.Form.Available() &&
		issuance.Requirement.Available()
}

// Spec is the authored declaration of one rule. The owning domain keeps its
// transfer algebra, hot rule, and contributor; what it hands over here is the
// sealed declaration.
type Spec struct {
	// Key is the rule's authored identity and its diagnostic name, so a rule
	// has exactly one spelling in the analyzer. It derives the entry identity
	// a verdict carries.
	Key schema.Key
	// The role is not a field. A rule's role is its declaration position in the
	// catalog, so the row's identity and its slot are the declaration itself
	// rather than an ordinal restated beside it.
	Lane Lane
	// Writes is the axis this rule's occurrences write. It resolves against the
	// sealed axis surface, so a rule cannot write a coordinate space that is not
	// declared, and the lane it writes is named by the axis that owns it rather
	// than by a projection of the role.
	Writes schema.Key
	// Owner is the axis that must supply this rule's operand resolver. It is
	// the join key from the declaration to the bound cell: construction later
	// selects the resolver by this rule's identity, and the owner names who
	// must have installed it. Owner and Writes may differ: a rule can write
	// one coordinate space while its operand is resolved by another principal.
	Owner schema.Key
	// Issues are this rule's program-occurrence subscriptions, in the order the
	// compiler places them. A rule materialized from a compiled artifact
	// declares which rows issue it; a Link-owned rule declares none, because its
	// occurrences are admitted through the Link table instead.
	Issues []Issuance
	// Semantic is this rule's canonical identity: the semantic role row it is
	// declared under. The row's declared spelling derives the identity the
	// engine registers this rule's slot with, so the declaration and the
	// binding name one role.
	Semantic schema.Key
	// Roles are the further semantic roles this rule's Declare hook resolves:
	// the operand form its occurrences read and the transform form a normalized
	// output is admitted under. Rule-admission evidence is not a semantic role.
	// They are content,
	// so the identity set a rule is declared against is part of the table
	// digest, and the hook reaches no identity this list omits.
	Roles []schema.Key
	// Program is the immutable, domain-neutral execution declaration. It
	// contains no callback, domain value, or runtime handle.
	Program ruleprogram.Program
}

// Cell is the opaque per-rule payload the table carries between passes. It is
// produced and consumed only by the typed thunks one Spec instantiated, so an
// authored hook never sees it and never asserts.
type Cell struct{ payload any }

func (holder Cell) Available() bool { return holder.payload != nil }

// NewCell holds one typed contributor payload in an opaque cell.
func NewCell(value any) Cell {
	if value == nil {
		return Cell{}
	}
	return Cell{payload: value}
}

// Template is one admitted rule declaration. It is immutable once built.
// Contributor wiring is composition-owned.
type Template struct {
	key      schema.Key
	id       schema.EntryID
	digest   identity.ContentID
	lane     Lane
	writes   schema.Key
	owner    schema.Key
	issues   []Issuance
	semantic schema.Key
	roles    []schema.Key
	program  ruleprogram.Program
}

// New admits one authored declaration. A rejected spec returns false rather
// than a partially usable template.
func New(spec Spec) (*Template, bool) {
	program := spec.Program.Clone()
	spec.Program = program
	if !specAdmissible(spec) {
		return nil, false
	}
	template := &Template{
		key:      spec.Key,
		id:       schema.NewEntryID(schema.SurfaceKindRule, spec.Key),
		lane:     spec.Lane,
		writes:   spec.Writes,
		owner:    spec.Owner,
		issues:   append([]Issuance(nil), spec.Issues...),
		semantic: spec.Semantic,
		roles:    append([]schema.Key(nil), spec.Roles...),
		program:  program.Clone(),
	}
	template.digest = template.contentDigest()
	return template, template.EntryAvailable() && template.digest.Available()
}

func specAdmissible(spec Spec) bool {
	if !spec.Key.Available() || !spec.Lane.Available() {
		return false
	}
	for _, role := range spec.Roles {
		if !role.Available() || role == spec.Semantic {
			return false
		}
	}
	if !spec.Semantic.Available() {
		return false
	}
	if !spec.Writes.Available() || !spec.Owner.Available() {
		return false
	}
	for _, issuance := range spec.Issues {
		if !issuance.Available() {
			return false
		}
	}
	if !spec.Program.Valid() {
		return false
	}
	if spec.Program.Available() && !containsRole(spec.Roles, spec.Program.OperandRole) {
		return false
	}
	return true
}

func containsRole(roles []schema.Key, key schema.Key) bool {
	if !key.Available() {
		return false
	}
	for _, role := range roles {
		if role == key {
			return true
		}
	}
	return false
}

func (template *Template) Key() schema.Key { return template.key }

func (template *Template) ID() schema.EntryID { return template.id }

func (template *Template) Lane() Lane { return template.lane }

// Writes is the axis this rule's occurrences write, by the key that axis is
// declared under.
func (template *Template) Writes() schema.Key { return template.writes }

// Owner is the axis that must supply this rule's operand resolver.
func (template *Template) Owner() schema.Key { return template.owner }

// Digest is the Rule declaration digest, including its canonical cold
// Program/issuance bytes. It is distinct from the root table digest but
// is derived from the same EntryContent stream.
func (template *Template) Digest() identity.ContentID {
	if template == nil {
		return identity.ContentID{}
	}
	return template.digest
}

func (template *Template) Program() ruleprogram.Program {
	if template == nil {
		return ruleprogram.Program{}
	}
	return template.program.Clone()
}

// IssuanceCount is the number of occurrence subscriptions this rule declares.
func (template *Template) IssuanceCount() int {
	if template == nil {
		return 0
	}
	return len(template.issues)
}

// IssuanceAt returns one declared subscription at its declaration position. The
// position is the order the compiler places the issuances in.
func (template *Template) IssuanceAt(index int) (Issuance, bool) {
	if template == nil || index < 0 || index >= len(template.issues) {
		return Issuance{}, false
	}
	return template.issues[index], true
}

// Semantic is the semantic role row this rule is declared under. A consumer
// resolves the identity through the sealed vocabulary rather than deriving it
// from this key, so the declaration and the derivation stay one step apart.
func (template *Template) Semantic() schema.Key {
	if template == nil {
		return ""
	}
	return template.semantic
}

// RoleCount is the number of further semantic roles this rule declared.
func (template *Template) RoleCount() int {
	if template == nil {
		return 0
	}
	return len(template.roles)
}

// RoleAt returns one further declared semantic role at its declaration
// position.
func (template *Template) RoleAt(index int) (schema.Key, bool) {
	if template == nil || index < 0 || index >= len(template.roles) {
		return "", false
	}
	return template.roles[index], true
}

// declaredRoles is the whole role set this rule is declared against: its own
// identity first, then the roles its hook resolves.
func (template *Template) declaredRoles() []schema.Key {
	if template == nil {
		return nil
	}
	return append([]schema.Key{template.semantic}, template.roles...)
}

func (template *Template) EntryAvailable() bool {
	if template == nil || !template.key.Available() || !template.id.Available() || !template.lane.Available() {
		return false
	}
	if !template.semantic.Available() {
		return false
	}
	if !template.writes.Available() || !template.owner.Available() {
		return false
	}
	return template.digest.Available()
}

// DeclaredRoles is the whole role set this rule is declared against.
func (template *Template) DeclaredRoles() []schema.Key {
	return template.declaredRoles()
}

// EntryContent writes this rule's declarative half: the semantic roles it is
// declared against, the admission lane it enters on, the axis it writes, the
// owner that must supply its operand resolver, and the occurrence
// subscriptions it declares - operand shape included - each in declaration
// order. The lane decides
// which admission path an occurrence takes, the write axis names the
// coordinate space this rule's facts land in, the owner names the principal
// that must install the resolver construction later selects by this rule's
// key, and the subscriptions are the mapping from compiled rows to issued
// occurrences, so all of them are content.
//
// The role is not written. A rule's role is its position in this very order, so
// writing it would write one value twice.
//
// The declared roles are content: the role this rule is identified by, and the
// further roles its hook resolves. The engine slot this rule binds is resolved
// under the first and the hook consumes the rest, so two catalogs whose rules
// name different roles declare different rules and are declared against
// different identity sets. The rows are written by the key they are declared
// under rather than by the identity that key resolves to, because the
// resolution is the vocabulary surface's derivation from its own declared
// spelling and is already folded there.
//
// The typed hooks are not content. A hook is a function value: it has no
// canonical bytes, and the shape of the hook set a rule declares is a property
// of those values rather than declared data, so neither is written. The
// What the hooks are declared against is covered: the axis names the coordinate
// space they write, and the surface's own admission laws bind the hook set to
// the lane.
func (template *Template) EntryContent(content *framing.Writer) error {
	return template.writeContent(content)
}

const (
	ruleContentDomain  = "wippy.analysis/schema/rule/template"
	ruleContentVersion = 1
)

func (template *Template) writeContent(content *framing.Writer) error {
	if err := content.String(string(template.semantic)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(template.roles))); err != nil {
		return err
	}
	for _, role := range template.roles {
		if err := content.String(string(role)); err != nil {
			return err
		}
	}
	if err := content.Uint(uint64(template.lane)); err != nil {
		return err
	}
	if err := content.String(string(template.writes)); err != nil {
		return err
	}
	if err := content.String(string(template.owner)); err != nil {
		return err
	}
	if err := content.Count(uint64(len(template.issues))); err != nil {
		return err
	}
	for _, issuance := range template.issues {
		if err := content.String(string(issuance.Occurrence)); err != nil {
			return err
		}
		if err := content.String(string(issuance.Form)); err != nil {
			return err
		}
		if err := content.String(string(issuance.Requirement)); err != nil {
			return err
		}
	}
	return template.program.WriteContent(content)
}

// References exposes the complete common reference stream to schema/seal.
// The seal snapshots it before running this surface and validates it against
// the completed catalog, so upward Program references are checked at the root
// rather than by a local table or callback.
func (template *Template) References() schema.EntryReferences {
	if template == nil {
		return nil
	}
	refs := schema.EntryReferences{
		{Surface: schema.SurfaceKindAxis, Key: template.writes},
		{Surface: schema.SurfaceKindAxis, Key: template.owner},
	}
	for _, issuance := range template.issues {
		refs = append(refs,
			schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: issuance.Occurrence},
			schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: issuance.Form},
			schema.EntryReference{Surface: schema.SurfaceKindIssuance, Key: issuance.Requirement})
	}
	return append(refs, template.program.References()...)
}

func (template *Template) contentDigest() identity.ContentID {
	if template == nil {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var content framing.Writer
	if content.Reset(hash, ruleContentDomain, ruleContentVersion) != nil || template.writeContent(&content) != nil || content.Finish() != nil {
		return identity.ContentID{}
	}
	var digest identity.ContentID
	copy(digest[:], hash.Sum(nil))
	return digest
}

// surface is the rule contribution to the analyzer declaration root.
type surface struct{ templates []*Template }

// NewSurface hands one ordered set of rule declarations to the table.
func NewSurface(templates []*Template) seal.Surface {
	return surface{templates: templates}
}

func (contribution surface) Kind() schema.SurfaceKind { return schema.SurfaceKindRule }

func (contribution surface) Entries() []schema.Entry {
	entries := make([]schema.Entry, len(contribution.templates))
	for index, template := range contribution.templates {
		entries[index] = template
	}
	return entries
}

// Seal states the rule surface's own totality laws. A rule's role is its
// position here, so what is left to state is that the row identifies itself,
// that the axis it writes and the vocabulary members its subscriptions name are
// declared, and that its subscriptions and its admission lane agree.
//
// The axis surface and the structural vocabulary seal below this one, so both
// references resolve downward against the table this surface is being sealed
// into rather than against a catalog restated here.
func (contribution surface) Seal(view seal.View, sealed seal.Sealed) schema.SealFailure {
	semantics := make(map[schema.Key]schema.EntryID, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		template, templateOK := entry.(*Template)
		if !entryOK || !templateOK {
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// Every role a rule names is a declared member of the semantic role
		// vocabulary. The vocabulary raises the two ways the name fails - one it
		// does not declare, and one it declares in another category - and this
		// surface raises them as its own verdict, because what the role means
		// here is this declaration.
		for _, role := range template.declaredRoles() {
			if _, disposition := structure.Resolve(sealed, role, structure.CategorySemanticRole); disposition != schema.DispositionAccepted {
				return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawSemanticIdentity, disposition)
			}
		}
		// One role is one rule. Two rules declared under one role would be one
		// engine slot two declarations bind, so the repeat is a verdict here
		// rather than a slot whichever rule reaches it first wins.
		if prior, duplicate := semantics[template.semantic]; duplicate {
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, prior, LawSemanticUnique, schema.DispositionDuplicate)
		}
		semantics[template.semantic] = template.id
		// An axis is a writer principal, so the lane a rule writes is a declared
		// axis and nothing else names it.
		if _, disposition := sealed.Resolve(schema.SurfaceKindAxis, template.writes); disposition != schema.DispositionAccepted {
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawWritesResolves, disposition)
		}
		// The owner is the writer principal that must install this rule's
		// operand resolver. It is resolved against the same axis surface as
		// Writes, so a rule cannot name an owner that is not a declared
		// principal.
		if _, disposition := sealed.Resolve(schema.SurfaceKindAxis, template.owner); disposition != schema.DispositionAccepted {
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawOwnerResolves, disposition)
		}
		// A rule admitted from a compiled artifact is reached by the rows it
		// subscribes to. One that subscribes to nothing would sit on that lane
		// unreachable, and one that subscribes from the Link lane would declare a
		// path its own occurrences never take.
		if template.lane.Mounted() != (len(template.issues) > 0) {
			law := LawIssuanceDeclared
			if !template.lane.Mounted() {
				law = LawIssuanceLane
			}
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, law, schema.DispositionIncomplete)
		}
		for _, issuance := range template.issues {
			if failure := sealIssuance(template.id, issuance, sealed); failure.Available() {
				return failure
			}
		}
		if template.contentDigest() != template.digest {
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawProgramShape, schema.DispositionMalformed)
		}
		if problem, valid := template.program.Check(); !valid {
			law := LawProgramShape
			if problem.Kind == ruleprogram.ProblemOutput {
				law = LawProgramOutput
			}
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, law, schema.DispositionMalformed)
		}
		if template.program.Available() && !template.declaresRole(template.program.OperandRole) {
			// The execution program consumes this semantic identity. It must be
			// one of the rule's authored roles, never an unrecorded name threaded
			// into runtime construction.
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawProgramShape, schema.DispositionIncomplete)
		}
		if template.program.ActivationRole.Available() && !template.declaresRole(template.program.ActivationRole) {
			// The activation family the program's candidate branches are grouped
			// under is the same kind of consumed identity as the operand family,
			// and it is held to the same rule: a family named nowhere in the
			// rule's own roles is a name threaded into cold construction.
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawProgramShape, schema.DispositionIncomplete)
		}
	}
	return schema.SealFailure{}
}

func (template *Template) declaresRole(key schema.Key) bool {
	return template != nil && containsRole(template.roles, key)
}

// sealIssuance resolves the complete subscription against the issuance
// machine. It also proves that the chosen requirement publishes every output
// the chosen form reads, preventing a downstream form from reconstructing a
// selection the admission program did not issue.
func sealIssuance(entry schema.EntryID, issuance Issuance, sealed seal.Sealed) schema.SealFailure {
	view, viewOK := sealed.Surface(schema.SurfaceKindIssuance)
	table, tableOK := issuanceschema.NewTable(view)
	if !viewOK || !tableOK {
		return seal.SurfaceLawFailure(schema.SurfaceKindRule, entry, LawIssuanceResolves, schema.DispositionIncomplete)
	}
	if !issuance.Available() {
		return seal.SurfaceLawFailure(schema.SurfaceKindRule, entry, LawIssuanceRequirementDeclared, schema.DispositionIncomplete)
	}
	if _, ok := table.Entry(issuance.Occurrence, issuanceschema.KindFamily); !ok {
		return seal.SurfaceLawFailure(schema.SurfaceKindRule, entry, LawIssuanceResolves, schema.DispositionMalformed)
	}
	form, formOK := table.Entry(issuance.Form, issuanceschema.KindForm)
	requirement, requirementOK := table.Entry(issuance.Requirement, issuanceschema.KindRequirement)
	if !formOK {
		return seal.SurfaceLawFailure(schema.SurfaceKindRule, entry, LawIssuanceResolves, schema.DispositionMalformed)
	}
	if !requirementOK {
		return seal.SurfaceLawFailure(schema.SurfaceKindRule, entry, LawIssuanceRequirementResolves, schema.DispositionMalformed)
	}
	published := make(map[schema.Key]struct{}, len(requirement.Outputs()))
	for _, output := range requirement.Outputs() {
		published[output.Output] = struct{}{}
	}
	for _, required := range form.Requires() {
		if _, ok := published[required]; !ok {
			return seal.SurfaceLawFailure(schema.SurfaceKindRule, entry, LawIssuanceRequirementResolves, schema.DispositionIncomplete)
		}
	}
	return schema.SealFailure{}
}

// Payload recovers one cell's value at its declared type. It is how a rule's
// own owner reaches its bound implementation; the table itself never needs it.
func Payload[T any](holder Cell) (T, bool) {
	value, ok := holder.payload.(T)
	return value, ok
}

// ProgramRefusal names why this template's program clauses would refuse it.
//
// It admits nothing and gates nothing: Seal stays the authority, and this
// reads the same rows to say which clause it tripped over and for which rule.
// A SealFailure deliberately renders no authored key, which is right for a
// value that crosses surfaces - but it leaves a refused program identifiable
// only by an entry identity, and the four clauses below are otherwise
// indistinguishable from one another. An empty string means these clauses
// refuse nothing.
func (template *Template) ProgramRefusal() string {
	if template == nil {
		return "a rule template is absent"
	}
	if digest := template.contentDigest(); digest != template.digest {
		if digest == (identity.ContentID{}) {
			return fmt.Sprintf("rule %q: its declaration does not encode, so it has no content identity to be sealed under", string(template.key))
		}
		return fmt.Sprintf("rule %q: its content identity disagrees with the declaration it was admitted with", string(template.key))
	}
	if problem, valid := template.program.Check(); !valid {
		return fmt.Sprintf("rule %q: its execution program is not well formed (problem kind %d, join %d, input %d, output %d)",
			string(template.key), int(problem.Kind), int(problem.Join), int(problem.Input), int(problem.Output))
	}
	if template.program.Available() && !template.declaresRole(template.program.OperandRole) {
		return fmt.Sprintf("rule %q: its program consumes operand role %q, which the rule does not declare",
			string(template.key), string(template.program.OperandRole))
	}
	if template.program.ActivationRole.Available() && !template.declaresRole(template.program.ActivationRole) {
		return fmt.Sprintf("rule %q: its program consumes activation role %q, which the rule does not declare",
			string(template.key), string(template.program.ActivationRole))
	}
	return ""
}
