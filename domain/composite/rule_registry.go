package composite

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/composite"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/observation"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// ruleTableLimit is one past the last declaration slot (slots are 1-based).

// ruleCells is one pass's per-rule payload, indexed by role slot: the rule's
// dense declaration position, numbered from one. Slot zero is the absent rule.
type ruleCells []rule.Cell

func newRuleCells(entries []*template) ruleCells { return make(ruleCells, len(entries)+1) }

// registry is the sealed analyzer declaration table. It is built once and is
// immutable afterwards; a rejected law leaves the table unavailable rather
// than half usable.
var registry struct {
	once             sync.Once
	sealed           *schema.Schema
	failure          schema.SealFailure
	templates        []*template
	ruleContributors []ruleContributor
	axes             []*axisTemplate
	axisContributors []axisContributor
	// queries is the admitted query inventory, in catalog order. The sealed
	// surface states what each family declares; these rows additionally carry
	// the contributor that answers it, which the declaration and binding passes
	// drive.
	queries           []*query.Registration
	queryContributors []queryContributor
	// observations is the pure-data inventory of the query result populations
	// consumed by the live publication producers.
	observations observation.Table
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
	// slotsByKey is the sealed rule inventory indexed by declaration key, so
	// attach and classification address a rule by identity rather than by
	// scanning the table.
	slotsByKey map[schema.Key]int
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
		axes, axisContributors, axesOK := axisTemplates()
		if !axesOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindAxis, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		templates, ruleContributors, ok := ruleTemplates()
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
		queries, contributors, queriesOK := queryRegistrations(roles)
		if !queriesOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		observations, observationsOK := observationEntries(queries)
		if !observationsOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindObservation, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		// The registration order is the catalog order, which is the bind phase
		// order: the structural vocabulary first, because it names no other
		// surface and every surface above it may name a member of it, then axes
		// before the rules that write them and before every surface that names a
		// coordinate space, diagnostics after the rules and axes they reference,
		// denominators after the surfaces they may be owned by.
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
		builder.Register(observation.NewSurface(observations))
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
		observationView, observationViewOK := sealed.Surface(schema.SurfaceKindObservation)
		observationTable, observationTableOK := observation.NewTable(observationView)
		if !observationViewOK || !observationTableOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindObservation, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
			return
		}
		registry.observations = observationTable
		registry.axisAdopters = axisAdopterTable(axes)
		positions := make(map[schema.Key]int, len(queries))
		for position, registration := range queries {
			positions[registration.Key()] = position
		}
		slots := make(map[schema.Key]int, len(templates))
		for position, entry := range templates {
			if entry == nil || !entry.Key().Available() {
				registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
				return
			}
			if _, duplicate := slots[entry.Key()]; duplicate {
				registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
				return
			}
			slots[entry.Key()] = position + 1
		}
		registry.templates, registry.ruleContributors, registry.axes, registry.axisContributors = templates, ruleContributors, axes, axisContributors
		registry.queries, registry.queryContributors, registry.queryPositions = queries, contributors, positions
		registry.slotsByKey = slots
		registry.roles, registry.sealed = roles, sealed
	})
}

// Table returns the sealed declaration root and the law that rejected it. The
// structure, axis, rule, diagnostic, composite, denominator, query, and
// observation surfaces are its members, sealed in catalog order; a later
// surface registers alongside them without touching any.
func Table() (*schema.Schema, schema.SealFailure) {
	sealRegistry()
	return registry.sealed, registry.failure
}

// Observations is the derived read model of the sealed observation surface.
// Runtime producers use the declaration table's identities; they do not
// register or retain rows here.
func Observations() (observation.Table, bool) {
	sealRegistry()
	return registry.observations, registry.observations.Available()
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

// RuleEntryID returns one rule's stable table identity.
func RuleEntryID(key schema.Key) (schema.EntryID, bool) {
	entry, ok := templateForKey(key)
	if !ok {
		return schema.EntryID{}, false
	}
	return entry.ID(), true
}

// RuleSemantic returns one rule's canonical Engine identity.
func RuleSemantic(key schema.Key) (identity.SemanticKey, bool) {
	entry, ok := templateForKey(key)
	if !ok {
		return identity.SemanticKey{}, false
	}
	return registry.roles.Key(entry.Semantic())
}

// RuleOwner returns the axis that must supply this rule's operand resolver.
// Construction later selects that resolver by the rule's declaration key; the
// owner is the join from the sealed declaration to the principal that installs
// it.
func RuleOwner(key schema.Key) (schema.Key, bool) {
	entry, ok := templateForKey(key)
	if !ok {
		return "", false
	}
	owner := entry.Owner()
	return owner, owner.Available()
}

// LinkKeys returns the Link-lane declaration keys in table order.
func LinkKeys() []schema.Key {
	sealRegistry()
	var keys []schema.Key
	for _, entry := range registry.templates {
		if entry != nil && entry.Lane() == rule.LaneLink {
			keys = append(keys, entry.Key())
		}
	}
	return keys
}

func templateForKey(key schema.Key) (*template, bool) {
	slot, ok := ruleSlotForKey(key)
	if !ok {
		return nil, false
	}
	return templateAtSlot(slot)
}

// MountedRuleKey reports whether key names a mounted or activation rule.
func MountedRuleKey(key schema.Key) bool {
	entry, ok := templateForKey(key)
	return ok && entry.Lane().Mounted()
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
	if registry.sealed == nil || !key.Available() || registry.slotsByKey == nil {
		return 0, false
	}
	slot, ok := registry.slotsByKey[key]
	return slot, ok
}

// DiagnosticRule is the closed analyzer-owned classification of one rule. It
// is the rule's role slot: its dense declaration position, numbered from one.
// Unknown covers empty, foreign, and generic engine lifecycle failures without
// a bound analyzer rule.
type DiagnosticRule uint8

const DiagnosticRuleUnknown DiagnosticRule = 0

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
	if len(registry.ruleContributors) != len(registry.templates) {
		return fragments, DiagnosticRuleUnknown, false
	}
	for position, entry := range registry.templates {
		slot := position + 1
		if !owners.writes(entry.Writes()) || !owners.writes(entry.Owner()) {
			return fragments, DiagnosticRule(slot), false
		}
		fragment, ok := registry.ruleContributors[position].declare(builder, roles, owners)
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
	RuleBindStageProgram
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
	case RuleBindStageProgram:
		return "program"
	default:
		return "none"
	}
}

// RuleBinding is the Link-local hot projection of the rule table. Every hot
// rule is reachable by its role and stays inside its cell: consumers drive
// attachment and classification through the table, never through a domain
// type.
type RuleBinding struct {
	binding   *engine.SchemaBinding
	hot       ruleCells
	attachers map[schema.Key]engine.RuleProgramAttach
}

func (rules *RuleBinding) cellByKey(key schema.Key) (rule.Cell, bool) {
	slot, ok := ruleSlotForKey(key)
	if !ok {
		return rule.Cell{}, false
	}
	return rules.cellAtSlot(slot)
}

func (rules *RuleBinding) cellAtSlot(slot int) (rule.Cell, bool) {
	if rules == nil || slot <= 0 || slot >= len(rules.hot) {
		return rule.Cell{}, false
	}
	hot := rules.hot[slot]
	return hot, hot.Available()
}

// CapabilityByKey resolves one rule's sealed slot capability by declaration key.
func (rules *RuleBinding) CapabilityByKey(key schema.Key) (engine.RuleSlotCapability, bool) {
	slot, ok := ruleSlotForKey(key)
	if !ok {
		return engine.RuleSlotCapability{}, false
	}
	return rules.capabilityAtSlot(slot)
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

// mountedOccurrenceAttach is the owner-held post-commit attach for the
// activation lane. Operand and Link rules publish RuleProgramAttach instead.
type mountedOccurrenceAttach interface {
	AttachMountedReceiptMember(*engine.ProgramConstruction, identity.ContentID, identity.ContentID, identity.ContentID) bool
}

// AttachMemberByKey binds one already-admitted occurrence to a committed
// topology. The construction owns the graph row; activation keeps its own
// member bridge.
func (rules *RuleBinding) AttachMemberByKey(key schema.Key, compilation *engine.ProgramConstruction, mount, point, occurrence identity.ContentID) bool {
	entry, entryOK := templateForKey(key)
	if !entryOK || compilation == nil {
		return false
	}
	capability, capabilityOK := rules.CapabilityByKey(key)
	if !capabilityOK || !capability.Mounted() {
		return false
	}
	if entry.Lane() == rule.LaneActivation {
		hot, hotOK := rules.cellByKey(key)
		attach, attachOK := rule.Payload[mountedOccurrenceAttach](hot)
		return hotOK && attachOK && attach.AttachMountedReceiptMember(compilation, mount, point, occurrence)
	}
	attach, attachOK := rules.programAttach(key)
	if !attachOK {
		return false
	}
	if !attach.AdmitsMounted(mount, point, occurrence) {
		return true
	}
	return attach.AttachMountedMember(compilation, capability, mount, point, occurrence)
}

// AttachLinkMemberByKey binds one admitted Link occurrence to a committed
// topology.
func (rules *RuleBinding) AttachLinkMemberByKey(key schema.Key, compilation *engine.ProgramConstruction, occurrence identity.ContentID) bool {
	attach, attachOK := rules.programAttach(key)
	capability, capabilityOK := rules.CapabilityByKey(key)
	return attachOK && capabilityOK && capability.Link() && attach.AttachLinkMember(compilation, capability, occurrence)
}

func (rules *RuleBinding) programAttach(key schema.Key) (engine.RuleProgramAttach, bool) {
	if rules == nil || rules.attachers == nil || !key.Available() {
		return nil, false
	}
	attach, ok := rules.attachers[key]
	return attach, ok && attach != nil
}

// ProgramAttachByKey is the sealed construction join for one declared rule.
// Mounted operand and Link rules publish exactly one attach; activation
// publishes none.
func (rules *RuleBinding) ProgramAttachByKey(key schema.Key) (engine.RuleProgramAttach, bool) {
	return rules.programAttach(key)
}

func (rules *RuleBinding) LinkCatalogByKey(key schema.Key) (rule.LinkCatalog, bool) {
	hot, hotOK := rules.cellByKey(key)
	contributor, contributorOK := ruleContributorFor(key)
	if !hotOK || !contributorOK || contributor.linkCatalog == nil {
		return nil, false
	}
	return contributor.linkCatalog(hot)
}

func ruleContributorFor(key schema.Key) (ruleContributor, bool) {
	sealRegistry()
	if len(registry.ruleContributors) != len(registry.templates) {
		return ruleContributor{}, false
	}
	for index, entry := range registry.templates {
		if entry != nil && entry.Key() == key {
			return registry.ruleContributors[index], true
		}
	}
	return ruleContributor{}, false
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
	if registry.sealed == nil || binding == nil || !set.available() || seal == nil || len(fragments) != len(registry.templates)+1 || len(registry.ruleContributors) != len(registry.templates) {
		return nil, DiagnosticRuleUnknown, RuleBindStagePrincipal
	}
	rules := &RuleBinding{binding: binding, hot: newRuleCells(registry.templates), attachers: make(map[schema.Key]engine.RuleProgramAttach, len(registry.templates))}
	for position, entry := range registry.templates {
		slot := position + 1
		if !set.writes(entry.Writes()) || !set.writes(entry.Owner()) {
			return nil, DiagnosticRule(slot), RuleBindStagePrincipal
		}
		if !fragments[slot].Available() {
			return nil, DiagnosticRule(slot), RuleBindStageFragment
		}
		hot, ok := registry.ruleContributors[position].bind(binding, set, fragments[slot])
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
			capability, ok := registry.ruleContributors[position].register(binding, fragments[slot])
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
	for position := range registry.templates {
		slot := position + 1
		if contributor := registry.ruleContributors[position]; contributor.pair != nil && !contributor.pair(binding, fragments[slot], resolve) {
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
	for position := range registry.templates {
		slot := position + 1
		if contributor := registry.ruleContributors[position]; contributor.finalize != nil && !contributor.finalize(set, rules.hot[slot]) {
			return nil, DiagnosticRule(slot), RuleBindStageFinalize
		}
	}
	// Operand and Link rules publish one cell-owned program attach. Activation
	// keeps its own member bridge. Missing or duplicate attachers leave the
	// binding unusable rather than half constructed.
	for position, entry := range registry.templates {
		slot := position + 1
		if entry.Lane() == rule.LaneActivation {
			continue
		}
		source, sourceOK := rule.Payload[engine.RuleProgramSource](rules.hot[slot])
		attach, attachOK := source.ProgramAttach()
		if !sourceOK || !attachOK || attach == nil {
			return nil, DiagnosticRule(slot), RuleBindStageProgram
		}
		if _, duplicate := rules.attachers[entry.Key()]; duplicate {
			return nil, DiagnosticRule(slot), RuleBindStageProgram
		}
		rules.attachers[entry.Key()] = attach
	}
	return rules, DiagnosticRuleUnknown, RuleBindStageNone
}

// RuleHandle recovers one bound hot rule at its declared type. Production
// wiring drives the table instead; this exists for the owning domain's own
// laws, which must reach the implementation they are stating a law about.
func RuleHandleByKey[H any](rules *RuleBinding, key schema.Key) (H, bool) {
	var absent H
	hot, ok := rules.cellByKey(key)
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
