package composite

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/composite"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/library"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// ruleRoleLimit bounds the artifact's own role catalog. The declaration table
// numbers its rules by declaration position; this is the size of the ordinal
// space an artifact-addressed row arrives under, and the two agree by law.
const ruleRoleLimit = int(programartifact.RuleRoleValuePresenceRefinement) + 1

// ruleCells is one pass's per-rule payload, indexed by role slot: the rule's
// dense declaration position, numbered from one. Slot zero is the absent rule.
type ruleCells []rule.Cell

func newRuleCells(entries []*template) ruleCells { return make(ruleCells, len(entries)+1) }

// registry is the sealed analyzer declaration table. It is built once and is
// immutable afterwards; a rejected law leaves the table unavailable rather
// than half usable.
var registry struct {
	once      sync.Once
	sealed    *schema.Schema
	failure   schema.SealFailure
	templates []*template
	axes      []*axisTemplate
	// queries is the admitted query inventory, in catalog order. The sealed
	// surface states what each family declares; these rows additionally carry
	// the contributor that answers it, which the declaration and binding passes
	// drive.
	queries []*query.Registration
	// queryPositions is the admitted inventory indexed by authored family key,
	// so a consumer addresses a family's cell by the identity its owning domain
	// declared it under rather than by scanning the inventory.
	queryPositions map[schema.Key]int
	// axisAdopters is the authored adopter table projected onto the sealed axis
	// slots, so the mount path installs an authority by array index.
	axisAdopters []axisAdopter
	diagnostics  diagnostic.Table
	// roles is the resolved semantic role vocabulary the table was composed
	// against. Every identity a surface carries is resolved through it, so the
	// identity a row is sealed under and the identity a consumer reads are one
	// derivation.
	roles vocabulary.Roles
}

func sealRegistry() {
	registry.once.Do(func() {
		// The structural vocabulary is the first surface of the catalog and the
		// one every identity is declared in, so it is admitted first and its
		// semantic roles are resolved once for every inventory below.
		structures, structuresOK := structureEntries()
		if !structuresOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		roles, rolesOK := vocabulary.NewRoles(structures)
		if !rolesOK {
			// A declared role whose spelling derives no identity leaves every
			// surface that names it unresolvable, so the catalog cannot be composed
			// at all rather than composed short one identity.
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
			return
		}
		axes, axesOK := axisTemplates()
		if !axesOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindAxis, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		templates, ok := ruleTemplates()
		if !ok {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		diagnostics, diagnosticsOK := diagnosticEntries()
		if !diagnosticsOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindDiagnostic, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		composites, compositesOK := compositeEntries()
		if !compositesOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindComposite, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		denominators, denominatorsOK := denominatorEntries(axes, roles)
		if !denominatorsOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindDenominator, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		queries, queriesOK := queryRegistrations(roles)
		if !queriesOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		kinds, kindsOK := libraryKinds(roles)
		if !kindsOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindLibrary, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		// The registration order is the catalog order, which is the bind phase
		// order: the structural vocabulary first, because it names no other
		// surface and every surface above it may name a member of it, then axes
		// before the rules that write them and before every surface that names a
		// coordinate space, diagnostics after the rules and axes they reference,
		// denominators after the surfaces they may be owned by, and the contract
		// kinds last, after every surface a validation reference may resolve
		// against.
		builder := schema.NewBuilder()
		builder.Register(structure.NewSurface(structures))
		builder.Register(axis.NewSurface(axes))
		builder.Register(rule.NewSurface(templates))
		builder.Register(diagnostic.NewSurface(diagnostics))
		builder.Register(composite.NewSurface(composites))
		// The denominator catalog is the sole authored relation declaration
		// source. Generated relation entries are admitted directly beside the
		// coordinate worlds; no Program-relations projection or copied owner/form
		// vocabulary is allowed in this composition root.
		builder.Register(denominator.NewSurface(denominators, denominator.GeneratedRelationEntries()))
		builder.Register(query.NewSurface(queries))
		builder.Register(library.NewSurface(kinds))
		sealed, failure := builder.Seal()
		if failure.Available() {
			registry.failure = failure
			return
		}
		// The diagnostic surface states its own population law, so a sealed table
		// always carries rows to project here. A projection that still does not
		// form means the table this catalog composed is not the table it declared.
		diagnosticView, diagnosticViewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
		diagnosticTable, diagnosticTableOK := diagnostic.NewTable(diagnosticView)
		if !diagnosticViewOK || !diagnosticTableOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindDiagnostic, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
			return
		}
		registry.diagnostics = diagnosticTable
		registry.axisAdopters = axisAdopterTable(axes)
		positions := make(map[schema.Key]int, len(queries))
		for position, registration := range queries {
			positions[registration.Key()] = position
		}
		registry.templates, registry.axes = templates, axes
		registry.queries, registry.queryPositions = queries, positions
		registry.roles, registry.sealed = roles, sealed
	})
}

// Table returns the sealed declaration root and the law that rejected it. The
// structure, axis, rule, diagnostic, composite, denominator, query, and
// library surfaces are its members, sealed in catalog order; a later surface
// registers alongside them without touching any.
func Table() (*schema.Schema, schema.SealFailure) {
	sealRegistry()
	return registry.sealed, registry.failure
}

// RuleCount is the size of the sealed rule inventory.
func RuleCount() int {
	sealRegistry()
	return len(registry.templates)
}

// RuleKeyAt returns the declared key of one rule at its table position. The
// position is the rule's role slot less one; the key is the identity.
func RuleKeyAt(position int) (schema.Key, bool) {
	sealRegistry()
	if position < 0 || position >= len(registry.templates) {
		return "", false
	}
	return registry.templates[position].Key(), true
}

// RuleRoleAt returns the artifact role one rule at its table position is
// addressed by. The position is the rule's role slot less one, and the artifact
// ordinal at that slot is what the adoption agreement pins.
func RuleRoleAt(position int) (programartifact.RuleRole, bool) {
	sealRegistry()
	if position < 0 || position >= len(registry.templates) {
		return programartifact.RuleRoleInvalid, false
	}
	return artifactRoleForSlot(position + 1), true
}

// artifactRoleForSlot and templateForRole are the one interim adoption
// boundary: a compiled artifact still numbers its rule roles itself, and a row
// it addresses that way is resolved here to the declaration at the same
// position. The agreement between the two numberings is stated as a law, member
// by member, so the artifact's own catalog can be deleted against a proven map.
func artifactRoleForSlot(slot int) programartifact.RuleRole {
	if slot <= 0 || slot >= ruleRoleLimit {
		return programartifact.RuleRoleInvalid
	}
	return programartifact.RuleRole(slot)
}

// RuleEntryID returns one rule's stable table identity.
func RuleEntryID(role programartifact.RuleRole) (schema.EntryID, bool) {
	entry, ok := templateForRole(role)
	if !ok {
		return schema.EntryID{}, false
	}
	return entry.ID(), true
}

// RuleSemantic returns one rule's canonical Engine identity.
func RuleSemantic(role programartifact.RuleRole) (identity.SemanticKey, bool) {
	entry, ok := templateForRole(role)
	if !ok {
		return identity.SemanticKey{}, false
	}
	return registry.roles.Key(entry.Semantic())
}

// LinkRoles returns the Link-lane roles in table order. A plan admits exactly
// these outside the mounted artifact lane.
func LinkRoles() []programartifact.RuleRole {
	sealRegistry()
	var roles []programartifact.RuleRole
	for position, entry := range registry.templates {
		if entry.Lane() == rule.LaneLink {
			roles = append(roles, artifactRoleForSlot(position+1))
		}
	}
	return roles
}

func templateForRole(role programartifact.RuleRole) (*template, bool) {
	return templateAtSlot(int(role))
}

// templateAtSlot resolves one rule by its slot. The slot is the declaration
// position numbered from one, so slot zero names no rule.
func templateAtSlot(slot int) (*template, bool) {
	sealRegistry()
	if registry.sealed == nil || slot <= 0 || slot > len(registry.templates) {
		return nil, false
	}
	entry := registry.templates[slot-1]
	return entry, entry != nil
}

// ruleSlotForKey resolves one rule's slot from its declared key. It is how a
// rule reaches a peer by identity rather than by position.
func ruleSlotForKey(key schema.Key) (int, bool) {
	sealRegistry()
	if registry.sealed == nil {
		return 0, false
	}
	for position, entry := range registry.templates {
		if entry.Key() == key {
			return position + 1, true
		}
	}
	return 0, false
}

// DiagnosticRule is the closed analyzer-owned classification of one rule. It
// is the rule's role slot: its dense declaration position, numbered from one.
// Unknown covers empty, foreign, and generic engine lifecycle failures without
// a bound analyzer rule.
type DiagnosticRule uint8

const DiagnosticRuleUnknown DiagnosticRule = 0

// DiagnosticRuleForRole projects one artifact role into the closed analyzer
// rule classification.
func DiagnosticRuleForRole(role programartifact.RuleRole) DiagnosticRule {
	if _, ok := templateForRole(role); ok {
		return DiagnosticRule(role)
	}
	return DiagnosticRuleUnknown
}

// DiagnosticRuleForKey classifies one rule by the key it is declared under.
func DiagnosticRuleForKey(key schema.Key) DiagnosticRule {
	slot, ok := ruleSlotForKey(key)
	if !ok {
		return DiagnosticRuleUnknown
	}
	return DiagnosticRule(slot)
}

// DiagnosticRuleForSemantic classifies one Engine rule key. The key is neither
// retained nor decoded; only the table is consulted.
func DiagnosticRuleForSemantic(key identity.SemanticKey) DiagnosticRule {
	sealRegistry()
	if !key.Available() {
		return DiagnosticRuleUnknown
	}
	for position, entry := range registry.templates {
		if semantic, resolved := registry.roles.Key(entry.Semantic()); resolved && semantic == key {
			return DiagnosticRule(position + 1)
		}
	}
	return DiagnosticRuleUnknown
}

func (classification DiagnosticRule) String() string {
	if entry, ok := templateAtSlot(int(classification)); ok {
		return string(entry.Key())
	}
	return "unknown"
}

// declareRules runs the table's cold declaration pass and returns each rule's
// fragment cell at its slot. It is the only place a rule's Schema shape is
// recorded.
func declareRules(builder *engine.SchemaBuilder, roles vocabulary.Roles, owners principals) (ruleCells, DiagnosticRule, bool) {
	sealRegistry()
	fragments := newRuleCells(registry.templates)
	if registry.sealed == nil || builder == nil || !owners.available() {
		return fragments, DiagnosticRuleUnknown, false
	}
	context := declaration{Builder: builder, Roles: roles, Principals: owners}
	for position, entry := range registry.templates {
		slot := position + 1
		if !owners.writes(entry.Writes()) {
			return fragments, DiagnosticRule(slot), false
		}
		fragment, ok := entry.Declare(context)
		if !ok {
			return fragments, DiagnosticRule(slot), false
		}
		fragments[slot] = fragment
	}
	return fragments, DiagnosticRuleUnknown, true
}

// RuleBindStage names the exact pass that rejected one rule.
type RuleBindStage uint8

const (
	RuleBindStageNone RuleBindStage = iota
	RuleBindStagePrincipal
	RuleBindStageFragment
	RuleBindStageBind
	RuleBindStageRegister
	RuleBindStagePair
	RuleBindStageSeal
	RuleBindStageCapability
	RuleBindStageFinalize
)

func (stage RuleBindStage) String() string {
	switch stage {
	case RuleBindStagePrincipal:
		return "principal"
	case RuleBindStageFragment:
		return "fragment"
	case RuleBindStageBind:
		return "bind"
	case RuleBindStageRegister:
		return "register"
	case RuleBindStagePair:
		return "pair"
	case RuleBindStageSeal:
		return "seal"
	case RuleBindStageCapability:
		return "capability"
	case RuleBindStageFinalize:
		return "finalize"
	default:
		return "none"
	}
}

// RuleBinding is the Link-local hot projection of the rule table. Every hot
// rule is reachable by its role and stays inside its cell: consumers drive
// attachment and classification through the table, never through a domain
// type.
type RuleBinding struct {
	binding *engine.SchemaBinding
	hot     ruleCells
}

func (rules *RuleBinding) cell(role programartifact.RuleRole) (rule.Cell, bool) {
	return rules.cellAtSlot(int(role))
}

func (rules *RuleBinding) cellAtSlot(slot int) (rule.Cell, bool) {
	if rules == nil || slot <= 0 || slot >= len(rules.hot) {
		return rule.Cell{}, false
	}
	hot := rules.hot[slot]
	return hot, hot.Available()
}

// Capability resolves one rule's exact sealed slot capability by role. The
// sealed SchemaBinding directory is the sole authority: the rule's canonical
// semantic identity is looked up there on demand, never cached into a second
// per-role registry.
func (rules *RuleBinding) Capability(role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
	return rules.capabilityAtSlot(int(role))
}

// capabilityAtSlot resolves one rule's sealed slot capability by its slot.
func (rules *RuleBinding) capabilityAtSlot(slot int) (engine.RuleSlotCapability, bool) {
	entry, entryOK := templateAtSlot(slot)
	if !entryOK || rules == nil || rules.binding == nil {
		return engine.RuleSlotCapability{}, false
	}
	semantic, semanticOK := registry.roles.Key(entry.Semantic())
	if !semanticOK {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := engine.BindingRuleSlot(rules.binding, semantic)
	return capability, ok && (capability.Mounted() || capability.Link())
}

// DiagnosticForCapability classifies one Engine rule capability against the
// table. It replaces the per-lane capability scans that each rebuilt their own
// role inventory.
func (rules *RuleBinding) DiagnosticForCapability(capability engine.RuleSlotCapability) DiagnosticRule {
	sealRegistry()
	if rules == nil || !(capability.Mounted() || capability.Link()) {
		return DiagnosticRuleUnknown
	}
	for position := range registry.templates {
		candidate, ok := rules.capabilityAtSlot(position + 1)
		if ok && candidate == capability {
			return DiagnosticRule(position + 1)
		}
	}
	return DiagnosticRuleUnknown
}

// Attach admits one artifact-authored occurrence for its role while the
// assembly's sources remain open.
func (rules *RuleBinding) Attach(role programartifact.RuleRole, assembly *engine.ReceiptAssembly, mount, point, occurrence identity.ContentID) bool {
	entry, entryOK := templateForRole(role)
	hot, hotOK := rules.cell(role)
	return entryOK && hotOK && entry.Attach(hot, assembly, mount, point, occurrence)
}

// AttachMember binds one already-admitted occurrence to a committed topology.
// The generic mounted graph row is claimed here for the lane that declares
// one; the activation lane owns its own member bridge instead.
func (rules *RuleBinding) AttachMember(role programartifact.RuleRole, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, mount, point, occurrence identity.ContentID) bool {
	entry, entryOK := templateForRole(role)
	hot, hotOK := rules.cell(role)
	if !entryOK || !hotOK || graph == nil {
		return false
	}
	capability, capabilityOK := rules.Capability(role)
	if !capabilityOK || !capability.Mounted() {
		return false
	}
	if entry.Lane() == rule.LaneMounted {
		if _, rowOK := graph.MountedRuleMember(capability, mount, point, occurrence); !rowOK {
			return false
		}
	}
	return entry.Member(hot, compilation, graph, mount, point, occurrence)
}

// AttachLink admits one Link-owned occurrence for its role.
func (rules *RuleBinding) AttachLink(role programartifact.RuleRole, assembly *engine.ReceiptAssembly, occurrence identity.ContentID) bool {
	entry, entryOK := templateForRole(role)
	hot, hotOK := rules.cell(role)
	return entryOK && hotOK && entry.LinkAttach(hot, assembly, occurrence)
}

// AttachLinkMember binds one admitted Link occurrence to a committed topology.
func (rules *RuleBinding) AttachLinkMember(role programartifact.RuleRole, compilation *engine.ReceiptCompilation, graph *engine.ReceiptGraph, occurrence identity.ContentID) bool {
	entry, entryOK := templateForRole(role)
	hot, hotOK := rules.cell(role)
	return entryOK && hotOK && entry.LinkMember(hot, compilation, graph, occurrence)
}

// LinkCatalog returns one Link rule's occurrence inventory.
func (rules *RuleBinding) LinkCatalog(role programartifact.RuleRole) (rule.LinkCatalog, bool) {
	entry, entryOK := templateForRole(role)
	hot, hotOK := rules.cell(role)
	if !entryOK || !hotOK {
		return nil, false
	}
	return entry.LinkCatalog(hot)
}

// bindRules drives the whole rule table in one transaction: bind every hot
// rule, hand every declared slot to its owner, admit every cross-rule pairing,
// and, once the shared binding is terminal, resolve each rule's sealed
// capability and run its finalizer. Each pass runs over the whole table before
// the next begins, so no rule's admission depends on another's position.
//
// seal is the caller-owned terminal step for the shared SchemaBinding. It runs
// between the pairing and capability passes because an occurrence issuer may
// only be sealed after the binding is terminal.
func bindRules(binding *engine.SchemaBinding, fragments ruleCells, set authorities, seal func() bool) (*RuleBinding, DiagnosticRule, RuleBindStage) {
	sealRegistry()
	if registry.sealed == nil || binding == nil || !set.available() || seal == nil || len(fragments) != len(registry.templates)+1 {
		return nil, DiagnosticRuleUnknown, RuleBindStagePrincipal
	}
	rules := &RuleBinding{binding: binding, hot: newRuleCells(registry.templates)}
	for position, entry := range registry.templates {
		slot := position + 1
		if !set.writes(entry.Writes()) {
			return nil, DiagnosticRule(slot), RuleBindStagePrincipal
		}
		if !fragments[slot].Available() {
			return nil, DiagnosticRule(slot), RuleBindStageFragment
		}
		hot, ok := entry.Bind(binding, set, fragments[slot])
		if !ok {
			return nil, DiagnosticRule(slot), RuleBindStageBind
		}
		rules.hot[slot] = hot
	}
	// Slots are handed to their owners one lane at a time: every artifact
	// mounted plane before the Link-owned plane. The grouping is the entry's
	// declared lane, so the sequence is a property of the table rather than a
	// hand-kept order. Within a lane the table's own order holds, and one rule
	// at a time keeps the first rejected slot's exact owner classification.
	// The issued capability lives only until the pairing pass ends; the sealed
	// binding is the sole later lookup authority.
	issued := make([]engine.RuleSlotCapability, len(registry.templates)+1)
	for _, mountedLane := range [...]bool{true, false} {
		for position, entry := range registry.templates {
			if entry.Lane().Mounted() != mountedLane {
				continue
			}
			slot := position + 1
			capability, ok := entry.Register(binding, fragments[slot])
			if !ok {
				return nil, DiagnosticRule(slot), RuleBindStageRegister
			}
			issued[slot] = capability
		}
	}
	// A rule that joins another rule's plane names its partner by the key that
	// rule is declared under, so the pairing pass resolves by identity and no
	// declaration depends on another's position.
	resolve := func(key schema.Key) (engine.RuleSlotCapability, bool) {
		slot, slotOK := ruleSlotForKey(key)
		if !slotOK || slot >= len(issued) {
			return engine.RuleSlotCapability{}, false
		}
		capability := issued[slot]
		return capability, capability.Mounted() || capability.Link()
	}
	for position, entry := range registry.templates {
		slot := position + 1
		if entry.HasPair() && !entry.Pair(binding, fragments[slot], resolve) {
			return nil, DiagnosticRule(slot), RuleBindStagePair
		}
	}
	if !seal() {
		return nil, DiagnosticRuleUnknown, RuleBindStageSeal
	}
	// The sealed directory must publish every rule on the lane it declared.
	// Nothing is retained: this pass states the law, and later lookups reach
	// the same directory.
	for position, entry := range registry.templates {
		slot := position + 1
		capability, ok := rules.capabilityAtSlot(slot)
		if !ok || entry.Lane().Mounted() != capability.Mounted() || (entry.Lane() == rule.LaneLink) != capability.Link() {
			return nil, DiagnosticRule(slot), RuleBindStageCapability
		}
	}
	for position, entry := range registry.templates {
		slot := position + 1
		if entry.HasFinalize() && !entry.Finalize(set, rules.hot[slot]) {
			return nil, DiagnosticRule(slot), RuleBindStageFinalize
		}
	}
	return rules, DiagnosticRuleUnknown, RuleBindStageNone
}

// RuleHandle recovers one bound hot rule at its declared type. Production
// wiring drives the table instead; this exists for the owning domain's own
// laws, which must reach the implementation they are stating a law about.
func RuleHandle[H any](rules *RuleBinding, role programartifact.RuleRole) (H, bool) {
	var absent H
	hot, ok := rules.cell(role)
	if !ok {
		return absent, false
	}
	return rule.Payload[H](hot)
}

// SemanticRoles is the resolved semantic role vocabulary the sealed table was
// composed against. A consumer that holds a declared role key reaches its
// global identity here rather than deriving one of its own.
func SemanticRoles() (vocabulary.Roles, bool) {
	sealRegistry()
	if registry.sealed == nil {
		return vocabulary.Roles{}, false
	}
	return registry.roles, registry.roles.Available()
}
