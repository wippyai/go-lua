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
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/internal/framing"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	// The ordinal here is retired. A rule's role is its declaration position,
	// so the position is a construction rather than a property a row could
	// state differently from where it sits.
	_ schema.LawID = schema.SurfaceLawFloor + iota
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
)

// Lane is the closed admission lane of one rule. Mounted rules enter through a
// reusable Program artifact row. Activation is the mounted structural lane
// whose members are attached by their own owner rather than a generic graph
// row. Link rules are Link-owned and never appear in an artifact.
type Lane uint8

const (
	LaneInvalid Lane = iota
	LaneMounted
	LaneActivation
	LaneLink
)

func (lane Lane) Available() bool { return lane >= LaneMounted && lane <= LaneLink }

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
	Builder    *engine.SchemaBuilder
	Roles      vocabulary.Roles
	Principals P
}

// Registration is the pre-seal slot handoff context.
type Registration[F any] struct {
	Binding  *engine.SchemaBinding
	Fragment F
}

// Pairing is the cross-rule admission context. It runs in its own pass after
// every rule has registered, so a rule that joins another rule's plane resolves
// it by stable role identity and never by declaration order.
type Pairing[F any] struct {
	Binding    *engine.SchemaBinding
	Fragment   F
	Capability func(schema.Key) (engine.RuleSlotCapability, bool)
}

// Binding is the hot binding context. Fragment is the cold fragment this
// rule's Declare hook produced; Authorities is the composition's own Link
// authority record.
type Binding[A, F any] struct {
	Binding     *engine.SchemaBinding
	Fragment    F
	Authorities A
}

// Finalization is the post-seal context. It runs only once the shared binding
// is terminal, which is the earliest point an occurrence issuer may be sealed.
type Finalization[A, H any] struct {
	Rule        H
	Authorities A
}

// Attach admits one artifact-authored occurrence while the assembly's sources
// remain open.
type Attach[H any] struct {
	Rule       H
	Assembly   *engine.ReceiptAssembly
	Mount      identity.ContentID
	Point      identity.ContentID
	Occurrence identity.ContentID
}

// Member binds one already-admitted occurrence to a committed topology.
type Member[H any] struct {
	Rule        H
	Compilation *engine.ReceiptCompilation
	Graph       *engine.ReceiptGraph
	Mount       identity.ContentID
	Point       identity.ContentID
	Occurrence  identity.ContentID
}

// LinkAttach admits one Link-owned occurrence. A Link rule is never
// materialized from an artifact row, so it carries no mount or point.
type LinkAttach[H any] struct {
	Rule       H
	Assembly   *engine.ReceiptAssembly
	Occurrence identity.ContentID
}

// LinkMember binds one admitted Link occurrence to a committed topology.
type LinkMember[H any] struct {
	Rule        H
	Compilation *engine.ReceiptCompilation
	Graph       *engine.ReceiptGraph
	Occurrence  identity.ContentID
}

// LinkCatalog is the Link-owned occurrence inventory a Link-lane rule
// publishes. It is the neutral shape a plan needs to enumerate the occurrences
// it must admit.
type LinkCatalog interface {
	Count() int
	IDAt(index int) (identity.ContentID, bool)
}

// Issuance is one program-occurrence subscription: the occurrence family whose
// compiled rows issue an occurrence of this rule, the placement form that
// issuance takes, the operand polarity it reads, and the execution cut it is
// placed at. Every term names a member of the structural vocabulary, so a
// subscription is declared data and resolves against the sealed table.
//
// What a form does with the program's geometry stays with the compiler that
// places it; what this record states is which rows issue this rule and in what
// shape, which is the mapping that used to be a switch over a foreign role
// catalog.
type Issuance struct {
	Occurrence schema.Key
	Form       schema.Key
	Input      schema.Key
	Stage      schema.Key
	// Code, when declared, narrows the subscription to occurrence rows carrying
	// this payload code. It is the one payload predicate the placement needs and
	// it is exact: a row whose code differs issues nothing.
	Code    uint64
	HasCode bool
}

func (issuance Issuance) Available() bool {
	return issuance.Occurrence.Available() && issuance.Form.Available() &&
		issuance.Input.Available() && issuance.Stage.Available()
}

// Spec is the authored declaration of one rule. P and A are the composition's
// principal and authority records; F and H are this rule's own cold fragment
// and hot rule. The owning domain keeps its transfer algebra and hot rule;
// what it hands over here is the wiring.
type Spec[P, A, F, H any] struct {
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
	// the operand and evidence forms its occurrences read and produce, and the
	// transform form a normalized output is admitted under. They are content,
	// so the identity set a rule is declared against is part of the table
	// digest, and the hook reaches no identity this list omits.
	Roles []schema.Key
	// Declare records the rule's cold Schema shape and returns its fragment.
	Declare func(Declaration[P]) (F, bool)
	// Register performs the pre-seal owner handoff for the declared slot.
	Register func(Registration[F]) (engine.RuleSlotCapability, bool)
	// Pair is the optional cross-rule admission a transported plane needs.
	Pair func(Pairing[F]) bool
	// Bind produces the Link-local hot rule from the cold fragment.
	Bind func(Binding[A, F]) (H, bool)
	// Finalize is the optional post-seal owner step.
	Finalize func(Finalization[A, H]) bool
	// Attach and Member are the mounted artifact lanes. Link rules declare
	// neither; they declare the Link trio below instead.
	Attach func(Attach[H]) bool
	Member func(Member[H]) bool
	// LinkAttach, LinkMember, and LinkCatalog are the Link lane. Mounted rules
	// declare none of them.
	LinkAttach  func(LinkAttach[H]) bool
	LinkMember  func(LinkMember[H]) bool
	LinkCatalog func(H) (LinkCatalog, bool)
}

// Cell is the opaque per-rule payload the table carries between passes. It is
// produced and consumed only by the typed thunks one Spec instantiated, so an
// authored hook never sees it and never asserts.
type Cell struct{ payload any }

func (holder Cell) Available() bool { return holder.payload != nil }

// Template is one admitted rule declaration, erased in its own fragment and
// hot rule but still typed in the composition's principal and authority
// records. It is immutable once built.
type Template[P, A any] struct {
	key      schema.Key
	id       schema.EntryID
	lane     Lane
	writes   schema.Key
	issues   []Issuance
	semantic schema.Key
	roles    []schema.Key

	declare     func(Declaration[P]) (Cell, bool)
	register    func(*engine.SchemaBinding, Cell) (engine.RuleSlotCapability, bool)
	pair        func(*engine.SchemaBinding, Cell, func(schema.Key) (engine.RuleSlotCapability, bool)) bool
	bind        func(*engine.SchemaBinding, A, Cell) (Cell, bool)
	finalize    func(A, Cell) bool
	attach      func(Cell, *engine.ReceiptAssembly, identity.ContentID, identity.ContentID, identity.ContentID) bool
	member      func(Cell, *engine.ReceiptCompilation, *engine.ReceiptGraph, identity.ContentID, identity.ContentID, identity.ContentID) bool
	linkAttach  func(Cell, *engine.ReceiptAssembly, identity.ContentID) bool
	linkMember  func(Cell, *engine.ReceiptCompilation, *engine.ReceiptGraph, identity.ContentID) bool
	linkCatalog func(Cell) (LinkCatalog, bool)
}

// New admits one authored declaration and instantiates its typed hooks. A
// rejected spec returns false rather than a partially usable template.
func New[P, A, F, H any](spec Spec[P, A, F, H]) (*Template[P, A], bool) {
	if !specAdmissible(spec) {
		return nil, false
	}
	template := &Template[P, A]{
		key:      spec.Key,
		id:       schema.NewEntryID(schema.SurfaceKindRule, spec.Key),
		lane:     spec.Lane,
		writes:   spec.Writes,
		issues:   append([]Issuance(nil), spec.Issues...),
		semantic: spec.Semantic,
		roles:    append([]schema.Key(nil), spec.Roles...),
	}
	// The hook receives exactly the roles this rule declared. Narrowing here is
	// what makes the declared role list the whole of what a hook can consume,
	// so an identity reaching a Declare body is one the table has on record.
	declared := template.declaredRoles()
	template.declare = func(context Declaration[P]) (Cell, bool) {
		roles, rolesOK := context.Roles.Restrict(declared...)
		if !rolesOK {
			return Cell{}, false
		}
		context.Roles = roles
		fragment, ok := spec.Declare(context)
		if !ok {
			return Cell{}, false
		}
		return Cell{payload: fragment}, true
	}
	template.register = func(binding *engine.SchemaBinding, holder Cell) (engine.RuleSlotCapability, bool) {
		fragment, ok := holder.payload.(F)
		if !ok {
			return engine.RuleSlotCapability{}, false
		}
		return spec.Register(Registration[F]{Binding: binding, Fragment: fragment})
	}
	if spec.Pair != nil {
		template.pair = func(binding *engine.SchemaBinding, holder Cell, resolve func(schema.Key) (engine.RuleSlotCapability, bool)) bool {
			fragment, ok := holder.payload.(F)
			return ok && spec.Pair(Pairing[F]{Binding: binding, Fragment: fragment, Capability: resolve})
		}
	}
	template.bind = func(binding *engine.SchemaBinding, authorities A, holder Cell) (Cell, bool) {
		fragment, fragmentOK := holder.payload.(F)
		if !fragmentOK {
			return Cell{}, false
		}
		hot, ok := spec.Bind(Binding[A, F]{Binding: binding, Fragment: fragment, Authorities: authorities})
		if !ok {
			return Cell{}, false
		}
		return Cell{payload: hot}, true
	}
	if spec.Finalize != nil {
		template.finalize = func(authorities A, holder Cell) bool {
			hot, ok := holder.payload.(H)
			return ok && spec.Finalize(Finalization[A, H]{Rule: hot, Authorities: authorities})
		}
	}
	if spec.Attach != nil {
		template.attach = func(holder Cell, assembly *engine.ReceiptAssembly, mount, point, occurrence identity.ContentID) bool {
			hot, ok := holder.payload.(H)
			return ok && assembly != nil && spec.Attach(Attach[H]{Rule: hot, Assembly: assembly, Mount: mount, Point: point, Occurrence: occurrence})
		}
	}
	if spec.Member != nil {
		template.member = func(holder Cell, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mount, point, occurrence identity.ContentID) bool {
			hot, ok := holder.payload.(H)
			return ok && compilation != nil && graph != nil && spec.Member(Member[H]{Rule: hot, Compilation: compilation, Graph: graph, Mount: mount, Point: point, Occurrence: occurrence})
		}
	}
	if spec.LinkAttach != nil {
		template.linkAttach = func(holder Cell, assembly *engine.ReceiptAssembly, occurrence identity.ContentID) bool {
			hot, ok := holder.payload.(H)
			return ok && assembly != nil && spec.LinkAttach(LinkAttach[H]{Rule: hot, Assembly: assembly, Occurrence: occurrence})
		}
	}
	if spec.LinkMember != nil {
		template.linkMember = func(holder Cell, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, occurrence identity.ContentID) bool {
			hot, ok := holder.payload.(H)
			return ok && compilation != nil && graph != nil && spec.LinkMember(LinkMember[H]{Rule: hot, Compilation: compilation, Graph: graph, Occurrence: occurrence})
		}
	}
	if spec.LinkCatalog != nil {
		template.linkCatalog = func(holder Cell) (LinkCatalog, bool) {
			hot, hotOK := holder.payload.(H)
			if !hotOK {
				return nil, false
			}
			catalog, ok := spec.LinkCatalog(hot)
			return catalog, ok && catalog != nil
		}
	}
	return template, template.EntryAvailable()
}

func specAdmissible[P, A, F, H any](spec Spec[P, A, F, H]) bool {
	if !spec.Key.Available() || !spec.Lane.Available() {
		return false
	}
	for _, role := range spec.Roles {
		if !role.Available() || role == spec.Semantic {
			return false
		}
	}
	if !spec.Semantic.Available() || spec.Declare == nil || spec.Register == nil || spec.Bind == nil {
		return false
	}
	mounted, link := spec.Lane.Mounted(), spec.Lane == LaneLink
	if mounted != (spec.Attach != nil) || mounted != (spec.Member != nil) {
		return false
	}
	if link != (spec.LinkAttach != nil) || link != (spec.LinkMember != nil) || link != (spec.LinkCatalog != nil) {
		return false
	}
	if !spec.Writes.Available() {
		return false
	}
	for _, issuance := range spec.Issues {
		if !issuance.Available() {
			return false
		}
	}
	return true
}

func (template *Template[P, A]) Key() schema.Key { return template.key }

func (template *Template[P, A]) ID() schema.EntryID { return template.id }

func (template *Template[P, A]) Lane() Lane { return template.lane }

// Writes is the axis this rule's occurrences write, by the key that axis is
// declared under.
func (template *Template[P, A]) Writes() schema.Key { return template.writes }

// IssuanceCount is the number of occurrence subscriptions this rule declares.
func (template *Template[P, A]) IssuanceCount() int {
	if template == nil {
		return 0
	}
	return len(template.issues)
}

// IssuanceAt returns one declared subscription at its declaration position. The
// position is the order the compiler places the issuances in.
func (template *Template[P, A]) IssuanceAt(index int) (Issuance, bool) {
	if template == nil || index < 0 || index >= len(template.issues) {
		return Issuance{}, false
	}
	return template.issues[index], true
}

// Semantic is the semantic role row this rule is declared under. A consumer
// resolves the identity through the sealed vocabulary rather than deriving it
// from this key, so the declaration and the derivation stay one step apart.
func (template *Template[P, A]) Semantic() schema.Key {
	if template == nil {
		return ""
	}
	return template.semantic
}

// RoleCount is the number of further semantic roles this rule declared.
func (template *Template[P, A]) RoleCount() int {
	if template == nil {
		return 0
	}
	return len(template.roles)
}

// RoleAt returns one further declared semantic role at its declaration
// position.
func (template *Template[P, A]) RoleAt(index int) (schema.Key, bool) {
	if template == nil || index < 0 || index >= len(template.roles) {
		return "", false
	}
	return template.roles[index], true
}

// declaredRoles is the whole role set this rule is declared against: its own
// identity first, then the roles its hook resolves.
func (template *Template[P, A]) declaredRoles() []schema.Key {
	if template == nil {
		return nil
	}
	return append([]schema.Key{template.semantic}, template.roles...)
}

func (template *Template[P, A]) EntryAvailable() bool {
	if template == nil || !template.key.Available() || !template.id.Available() || !template.lane.Available() {
		return false
	}
	if !template.semantic.Available() || template.declare == nil || template.register == nil || template.bind == nil {
		return false
	}
	mounted, link := template.lane.Mounted(), template.lane == LaneLink
	if mounted != (template.attach != nil) || mounted != (template.member != nil) {
		return false
	}
	if link != (template.linkAttach != nil) || link != (template.linkMember != nil) || link != (template.linkCatalog != nil) {
		return false
	}
	return template.writes.Available()
}

// EntryContent writes this rule's declarative half: the semantic roles it is
// declared against, the admission lane it enters on, the axis it writes, and the
// occurrence subscriptions it declares, each in declaration order. The lane
// decides which admission path an occurrence takes, the axis names the
// coordinate space this rule's facts land in, and the subscriptions are the
// mapping from compiled rows to issued occurrences, so all of them are content.
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
func (template *Template[P, A]) EntryContent(content *framing.Writer) error {
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
		if err := content.String(string(issuance.Input)); err != nil {
			return err
		}
		if err := content.String(string(issuance.Stage)); err != nil {
			return err
		}
		if err := content.Bool(issuance.HasCode); err != nil {
			return err
		}
		if err := content.Uint(issuance.Code); err != nil {
			return err
		}
	}
	return nil
}

func (template *Template[P, A]) HasPair() bool { return template.pair != nil }

func (template *Template[P, A]) HasFinalize() bool { return template.finalize != nil }

func (template *Template[P, A]) Declare(context Declaration[P]) (Cell, bool) {
	if context.Builder == nil {
		return Cell{}, false
	}
	holder, ok := template.declare(context)
	return holder, ok && holder.Available()
}

func (template *Template[P, A]) Register(binding *engine.SchemaBinding, fragment Cell) (engine.RuleSlotCapability, bool) {
	if binding == nil || !fragment.Available() {
		return engine.RuleSlotCapability{}, false
	}
	return template.register(binding, fragment)
}

func (template *Template[P, A]) Pair(binding *engine.SchemaBinding, fragment Cell, resolve func(schema.Key) (engine.RuleSlotCapability, bool)) bool {
	return template.pair != nil && binding != nil && fragment.Available() && resolve != nil && template.pair(binding, fragment, resolve)
}

func (template *Template[P, A]) Bind(binding *engine.SchemaBinding, authorities A, fragment Cell) (Cell, bool) {
	if binding == nil || !fragment.Available() {
		return Cell{}, false
	}
	holder, ok := template.bind(binding, authorities, fragment)
	return holder, ok && holder.Available()
}

func (template *Template[P, A]) Finalize(authorities A, hot Cell) bool {
	return template.finalize != nil && hot.Available() && template.finalize(authorities, hot)
}

func (template *Template[P, A]) Attach(hot Cell, assembly *engine.ReceiptAssembly, mount, point, occurrence identity.ContentID) bool {
	return template.attach != nil && hot.Available() && template.attach(hot, assembly, mount, point, occurrence)
}

func (template *Template[P, A]) Member(hot Cell, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mount, point, occurrence identity.ContentID) bool {
	return template.member != nil && hot.Available() && template.member(hot, compilation, graph, mount, point, occurrence)
}

func (template *Template[P, A]) LinkAttach(hot Cell, assembly *engine.ReceiptAssembly, occurrence identity.ContentID) bool {
	return template.linkAttach != nil && hot.Available() && template.linkAttach(hot, assembly, occurrence)
}

func (template *Template[P, A]) LinkMember(hot Cell, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, occurrence identity.ContentID) bool {
	return template.linkMember != nil && hot.Available() && template.linkMember(hot, compilation, graph, occurrence)
}

func (template *Template[P, A]) LinkCatalog(hot Cell) (LinkCatalog, bool) {
	if template.linkCatalog == nil || !hot.Available() {
		return nil, false
	}
	return template.linkCatalog(hot)
}

// surface is the rule contribution to the analyzer declaration root.
type surface[P, A any] struct{ templates []*Template[P, A] }

// NewSurface hands one ordered set of rule declarations to the table.
func NewSurface[P, A any](templates []*Template[P, A]) schema.Surface {
	return surface[P, A]{templates: templates}
}

func (contribution surface[P, A]) Kind() schema.SurfaceKind { return schema.SurfaceKindRule }

func (contribution surface[P, A]) Entries() []schema.Entry {
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
func (contribution surface[P, A]) Seal(view schema.View, sealed schema.Sealed) schema.SealFailure {
	semantics := make(map[schema.Key]schema.EntryID, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		template, templateOK := entry.(*Template[P, A])
		if !entryOK || !templateOK {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// Every role a rule names is a declared member of the semantic role
		// vocabulary. The vocabulary raises the two ways the name fails - one it
		// does not declare, and one it declares in another category - and this
		// surface raises them as its own verdict, because what the role means
		// here is this declaration.
		for _, role := range template.declaredRoles() {
			if _, disposition := structure.Resolve(sealed, role, structure.CategorySemanticRole); disposition != schema.DispositionAccepted {
				return schema.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawSemanticIdentity, disposition)
			}
		}
		// One role is one rule. Two rules declared under one role would be one
		// engine slot two declarations bind, so the repeat is a verdict here
		// rather than a slot whichever rule reaches it first wins.
		if prior, duplicate := semantics[template.semantic]; duplicate {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, prior, LawSemanticUnique, schema.DispositionDuplicate)
		}
		semantics[template.semantic] = template.id
		// An axis is a writer principal, so the lane a rule writes is a declared
		// axis and nothing else names it.
		if _, disposition := sealed.Resolve(schema.SurfaceKindAxis, template.writes); disposition != schema.DispositionAccepted {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawWritesResolves, disposition)
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
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, template.id, law, schema.DispositionIncomplete)
		}
		for _, issuance := range template.issues {
			if failure := sealIssuance(template.id, issuance, sealed); failure.Available() {
				return failure
			}
		}
	}
	return schema.SealFailure{}
}

// sealIssuance resolves one subscription's four vocabulary references. Each
// names a member of its own category, so a term that names another category is
// a member of the wrong vocabulary rather than an unresolved one, and the
// structural surface states that difference once for every surface above it.
func sealIssuance(entry schema.EntryID, issuance Issuance, sealed schema.Sealed) schema.SealFailure {
	references := [...]struct {
		key      schema.Key
		category structure.Category
	}{
		{issuance.Occurrence, structure.CategoryOccurrenceKind},
		{issuance.Form, structure.CategoryIssuanceForm},
		{issuance.Input, structure.CategoryIssuanceInput},
		{issuance.Stage, structure.CategoryIssuanceStage},
	}
	for _, reference := range references {
		if _, disposition := structure.Resolve(sealed, reference.key, reference.category); disposition != schema.DispositionAccepted {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, entry, LawIssuanceResolves, disposition)
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
