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
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// Surface law ordinals. They are numeric identities; rendering a verdict is
// the caller's job, from the identity.
const (
	LawRoleOrdinal schema.LawID = schema.SurfaceLawFloor + iota
	LawRoleUnique
	LawSemanticIdentity
	LawSemanticUnique
	LawEntryShape
	LawMountedRoleCovered
	LawMountedRoleLane
	LawVocabulary
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
type Declaration[P any] struct {
	Builder    *engine.SchemaBuilder
	Bundle     vocabulary.Bundle
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
	Capability func(programartifact.RuleRole) (engine.RuleSlotCapability, bool)
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

// Spec is the authored declaration of one rule. P and A are the composition's
// principal and authority records; F and H are this rule's own cold fragment
// and hot rule. The owning domain keeps its transfer algebra and hot rule;
// what it hands over here is the wiring.
type Spec[P, A, F, H any] struct {
	// Key is the rule's authored identity and its diagnostic name, so a rule
	// has exactly one spelling in the analyzer. It derives the entry identity
	// a verdict carries.
	Key schema.Key
	// Role is the sealed ProgramArtifact row role. The artifact format owns the
	// ordinal; this record maps to it rather than restating the catalog.
	Role programartifact.RuleRole
	Lane Lane
	// Semantic selects the rule identity from the canonical vocabulary.
	Semantic func(vocabulary.Bundle) engine.SemanticKey
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
	role     programartifact.RuleRole
	lane     Lane
	semantic func(vocabulary.Bundle) engine.SemanticKey

	declare     func(Declaration[P]) (Cell, bool)
	register    func(*engine.SchemaBinding, Cell) (engine.RuleSlotCapability, bool)
	pair        func(*engine.SchemaBinding, Cell, func(programartifact.RuleRole) (engine.RuleSlotCapability, bool)) bool
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
		role:     spec.Role,
		lane:     spec.Lane,
		semantic: spec.Semantic,
	}
	template.declare = func(context Declaration[P]) (Cell, bool) {
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
		template.pair = func(binding *engine.SchemaBinding, holder Cell, resolve func(programartifact.RuleRole) (engine.RuleSlotCapability, bool)) bool {
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
	if spec.Semantic == nil || spec.Declare == nil || spec.Register == nil || spec.Bind == nil {
		return false
	}
	mounted, link := spec.Lane.Mounted(), spec.Lane == LaneLink
	if mounted != (spec.Attach != nil) || mounted != (spec.Member != nil) {
		return false
	}
	if link != (spec.LinkAttach != nil) || link != (spec.LinkMember != nil) || link != (spec.LinkCatalog != nil) {
		return false
	}
	return programartifact.RuleOutputKindFor(spec.Role) != programartifact.RuleOutputInvalid
}

func (template *Template[P, A]) Key() schema.Key { return template.key }

func (template *Template[P, A]) ID() schema.EntryID { return template.id }

func (template *Template[P, A]) Role() programartifact.RuleRole { return template.role }

func (template *Template[P, A]) Lane() Lane { return template.lane }

// Principal is the factor lane this rule writes. It is read from the artifact
// format rather than restated, so the two cannot drift.
func (template *Template[P, A]) Principal() programartifact.RuleOutputKind {
	return programartifact.RuleOutputKindFor(template.role)
}

// Semantic resolves this rule's canonical identity in one vocabulary bundle.
func (template *Template[P, A]) Semantic(bundle vocabulary.Bundle) engine.SemanticKey {
	if template == nil || template.semantic == nil {
		return engine.SemanticKey{}
	}
	return template.semantic(bundle)
}

func (template *Template[P, A]) EntryAvailable() bool {
	if template == nil || !template.key.Available() || !template.id.Available() || !template.lane.Available() {
		return false
	}
	if template.semantic == nil || template.declare == nil || template.register == nil || template.bind == nil {
		return false
	}
	mounted, link := template.lane.Mounted(), template.lane == LaneLink
	if mounted != (template.attach != nil) || mounted != (template.member != nil) {
		return false
	}
	if link != (template.linkAttach != nil) || link != (template.linkMember != nil) || link != (template.linkCatalog != nil) {
		return false
	}
	return template.Principal() != programartifact.RuleOutputInvalid
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

func (template *Template[P, A]) Pair(binding *engine.SchemaBinding, fragment Cell, resolve func(programartifact.RuleRole) (engine.RuleSlotCapability, bool)) bool {
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

// Seal states the rule surface's own totality laws. The eighteen-arm switches
// that previously assumed exhaustiveness are replaced by these checks, so an
// unmapped, misordered, or duplicated role is a loud construction error rather
// than a silent default arm at solve time.
func (contribution surface[P, A]) Seal(view schema.View, _ schema.Sealed) schema.SealFailure {
	bundle, bundleOK := vocabulary.New()
	if !bundleOK {
		return schema.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, LawVocabulary, schema.DispositionMalformed)
	}
	roles := make(map[programartifact.RuleRole]*Template[P, A], view.Count())
	semantics := make(map[engine.SemanticKey]schema.EntryID, view.Count())
	for position := 0; position < view.Count(); position++ {
		entry, entryOK := view.At(position)
		template, templateOK := entry.(*Template[P, A])
		if !entryOK || !templateOK {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, LawEntryShape, schema.DispositionMalformed)
		}
		// The diagnostic ordinal is the artifact role ordinal. Pinning the
		// declaration order to that ordinal is what lets every derived view
		// project a rule without a second hand-maintained ordering.
		if int(template.role) != position+1 {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawRoleOrdinal, schema.DispositionMalformed)
		}
		if prior, duplicate := roles[template.role]; duplicate {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, prior.id, LawRoleUnique, schema.DispositionDuplicate)
		}
		roles[template.role] = template
		semantic := template.Semantic(bundle)
		if !semantic.Available() {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawSemanticIdentity, schema.DispositionMalformed)
		}
		if prior, duplicate := semantics[semantic]; duplicate {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, prior, LawSemanticUnique, schema.DispositionDuplicate)
		}
		semantics[semantic] = template.id
	}
	// Every mounted artifact role must be declared exactly once. An artifact
	// row whose role has no rule would otherwise be dropped without notice.
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		role, roleOK := programartifact.MountedRuleRoleAt(index)
		if !roleOK {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, LawMountedRoleCovered, schema.DispositionMalformed)
		}
		template, declared := roles[role]
		if !declared {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, LawMountedRoleCovered, schema.DispositionIncomplete)
		}
		if !template.lane.Mounted() {
			return schema.SurfaceLawFailure(schema.SurfaceKindRule, template.id, LawMountedRoleLane, schema.DispositionMalformed)
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
