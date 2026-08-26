package composite

import (
	"fmt"
	"strconv"

	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	"github.com/wippyai/go-lua/analysis/schema/composite"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	"github.com/wippyai/go-lua/analysis/schema/diagnostic"
	issuanceschema "github.com/wippyai/go-lua/analysis/schema/issuance"
	"github.com/wippyai/go-lua/analysis/schema/observation"
	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/seal"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// ruleTableLimit is one past the last declaration slot (slots are 1-based).

// ruleCells is one pass's per-rule payload, indexed by role slot: the rule's
// dense declaration position, numbered from one. Slot zero is the absent rule.
type ruleCells []rule.Cell

func newRuleCells(entries []*rule.Template) ruleCells { return make(ruleCells, len(entries)+1) }

func newCatalog() (*catalog, schema.SealFailure) {
	state := &catalog{}
	// The structural vocabulary is the first surface of the catalog and the
	// one every identity is declared in, so it is admitted first and its
	// semantic roles are resolved once for every inventory below.
	structures, structuresOK := structureEntries()
	if !structuresOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	roles, rolesOK := vocabulary.NewRoles(structures)
	if !rolesOK {
		// A declared role whose spelling derives no identity leaves every
		// surface that names it unresolvable, so the catalog cannot be composed
		// at all rather than composed short one identity.
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	axes, axisContributors, axesOK := axisTemplates()
	if !axesOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindAxis, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	issuances, issuanceOK := issuanceEntries()
	if !issuanceOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindIssuance, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	templates, ruleContributors, ok := RuleTemplates[principals, authorities]()
	if !ok {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	diagnostics, diagnosticsOK := diagnosticEntries()
	if !diagnosticsOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindDiagnostic, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	composites, compositesOK := compositeEntries()
	if !compositesOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindComposite, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	denominators, denominatorsOK := denominatorEntries(axes, roles)
	if !denominatorsOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindDenominator, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	queries, contributors, queriesOK := queryRegistrations(roles)
	if !queriesOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	observations, observationsOK := observationEntries(queries)
	if !observationsOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindObservation, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	// The registration order is the catalog order, which is the bind phase
	// order: the structural vocabulary first, because it names no other
	// surface and every surface above it may name a member of it, then axes
	// before the rules that write them and before every surface that names a
	// coordinate space, diagnostics after the rules and axes they reference,
	// denominators after the surfaces they may be owned by.
	declarations := analysiscatalog.NewDeclarations()
	declarations.Register(structure.NewSurface(structures))
	declarations.Register(axis.NewSurface(axes))
	declarations.Register(issuanceschema.NewSurface(issuances))
	declarations.Register(rule.NewSurface(templates))
	declarations.Register(diagnostic.NewSurface(diagnostics))
	declarations.Register(composite.NewSurface(composites))
	// The denominator catalog is the sole authored relation declaration
	// source. Generated relation entries are admitted directly beside the
	// coordinate worlds; no Program-relations projection or copied owner/form
	// vocabulary is allowed in this composition root.
	declarations.Register(denominator.NewSurface(denominators, denominator.GeneratedRelationEntries()))
	declarations.Register(query.NewSurface(queries))
	declarations.Register(observation.NewSurface(observations))
	compiledDeclarations, failure := declarations.Seal()
	if failure.Available() || !compiledDeclarations.Available() {
		state.failure = failure
		if !state.failure.Available() {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		}
		return state, state.failure
	}
	sealed := compiledDeclarations.Schema()
	structureView, structureViewOK := sealed.Surface(schema.SurfaceKindStructure)
	structureTable, structureTableOK := structure.NewTable(structureView)
	if !structureViewOK || !structureTableOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	// The diagnostic surface states its own population law, so a sealed table
	// always carries rows to project here. A projection that still does not
	// form means the table this catalog composed is not the table it declared.
	diagnosticView, diagnosticViewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
	diagnosticTable, diagnosticTableOK := diagnostic.NewTable(diagnosticView)
	if !diagnosticViewOK || !diagnosticTableOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindDiagnostic, Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	state.diagnostics = diagnosticTable
	state.structure = structureTable
	state.structureOK = true
	// The layout a family's answers are detached under is held by this seal.
	// It is sealed here, against the vocabulary the declaration table just
	// sealed and the shape the family's own registration derives, and never
	// beside the domain codec that writes it.
	if !sealPublicationLayout(queries, contributors, structureTable) {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: query.LawCodecDeclared, Disposition: schema.DispositionIncomplete}
		return state, state.failure
	}
	observationView, observationViewOK := sealed.Surface(schema.SurfaceKindObservation)
	observationTable, observationTableOK := observation.NewTable(observationView)
	if !observationViewOK || !observationTableOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindObservation, Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	state.observations = observationTable
	state.axisAdopters = axisAdopterTable(axes)
	// Build the one query lookup witness from the exact sealed registration
	// slice. Family and owner-issued EntryID are both identities here: neither
	// may be silently overwritten by a later row, and the map must cover every
	// registration before the catalog can become usable.
	positions := make(map[schema.Key]queryPosition, len(queries))
	entryIDs := make(map[schema.EntryID]schema.EntryID, len(queries))
	selectedOrdinal := uint32(0)
	for position, registration := range queries {
		if registration == nil || !registration.Key().Available() {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: query.LawEntryShape, Disposition: schema.DispositionMalformed}
			return state, state.failure
		}
		family := registration.Key()
		if _, duplicate := positions[family]; duplicate {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Entry: registration.ID(), Law: seal.LawEntryUnique, Disposition: schema.DispositionDuplicate}
			return state, state.failure
		}
		entryID := registration.EntryID()
		if !entryID.Available() {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Entry: registration.ID(), Law: query.LawRegistrationIdentity, Disposition: schema.DispositionIncomplete}
			return state, state.failure
		}
		if prior, duplicate := entryIDs[entryID]; duplicate {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Entry: prior, Law: query.LawRegistrationIdentity, Disposition: schema.DispositionDuplicate}
			return state, state.failure
		}
		if registration.Population() == query.PopulationSelectedPoint {
			selectedOrdinal++
		}
		rowSelectedOrdinal := uint32(0)
		if registration.Population() == query.PopulationSelectedPoint {
			rowSelectedOrdinal = selectedOrdinal
		}
		positions[family] = queryPosition{
			Position: position, EntryID: entryID, Ordinal: uint32(position + 1),
			SelectedOrdinal: rowSelectedOrdinal,
		}
		entryIDs[entryID] = registration.ID()
	}
	if len(positions) != len(queries) || len(entryIDs) != len(queries) {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: query.LawRegistrationIdentity, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	slots := make(map[schema.Key]int, len(templates))
	for position, entry := range templates {
		if entry == nil || !entry.Key().Available() {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return state, state.failure
		}
		if _, duplicate := slots[entry.Key()]; duplicate {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: seal.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return state, state.failure
		}
		slots[entry.Key()] = position + 1
	}
	state.templates, state.ruleContributors, state.axes, state.axisContributors = templates, ruleContributors, axes, axisContributors
	state.queries, state.queryContributors, state.queryPositions = queries, contributors, positions
	state.slotsByKey = slots
	state.roles, state.sealed = roles, sealed
	state.declarations = compiledDeclarations
	return state, state.failure
}

// Table returns the sealed declaration root and the law that rejected it for
// this exact compilation. The structure, axis, rule, diagnostic, composite,
// denominator, query, and observation surfaces are its members, sealed in
// catalog order.
func Table(compilation Compilation) (*seal.Schema, schema.SealFailure) {
	state := compilation.catalog
	if state == nil {
		return nil, schema.SealFailure{Law: seal.LawSurfaceCatalog, Disposition: schema.DispositionIncomplete}
	}
	return state.sealed, state.failure
}

// Observations is the derived read model of this compilation's sealed
// observation surface. Runtime producers use declaration identities; they do
// not register or retain rows here.
func Observations(compilation Compilation) (observation.Table, bool) {
	state := compilation.catalog
	if state == nil {
		return observation.Table{}, false
	}
	return state.observations, state.observations.Available()
}

// RuleCount is the size of this compilation's sealed rule inventory.
func RuleCount(compilation Compilation) int {
	state := compilation.catalog
	if state == nil {
		return 0
	}
	return len(state.templates)
}

// RuleKeyAt returns the declared key of one rule at its table position. The
// position is the rule's role slot less one; the key is the identity.
func RuleKeyAt(compilation Compilation, position int) (schema.Key, bool) {
	state := compilation.catalog
	if state == nil || position < 0 || position >= len(state.templates) {
		return "", false
	}
	return state.templates[position].Key(), true
}

// RuleEntryID returns one rule's stable table identity.
func RuleEntryID(compilation Compilation, key schema.Key) (schema.EntryID, bool) {
	entry, ok := templateForKey(compilation.catalog, key)
	if !ok {
		return schema.EntryID{}, false
	}
	return entry.ID(), true
}

// RuleSemantic returns one rule's canonical Engine identity.
func RuleSemantic(compilation Compilation, key schema.Key) (identity.SemanticKey, bool) {
	state := compilation.catalog
	entry, ok := templateForKey(state, key)
	if !ok {
		return identity.SemanticKey{}, false
	}
	if state == nil {
		return identity.SemanticKey{}, false
	}
	return state.roles.Key(entry.Semantic())
}

// RuleOwner returns the axis that must supply this rule's operand resolver.
// Construction later selects that resolver by the rule's declaration key; the
// owner is the join from the sealed declaration to the principal that installs
// it.
func RuleOwner(compilation Compilation, key schema.Key) (schema.Key, bool) {
	entry, ok := templateForKey(compilation.catalog, key)
	if !ok {
		return "", false
	}
	owner := entry.Owner()
	return owner, owner.Available()
}

// LinkKeys returns the Link-lane declaration keys in table order.
func LinkKeys(compilation Compilation) []schema.Key {
	state := compilation.catalog
	return linkKeys(state)
}

func linkKeys(state *catalog) []schema.Key {
	var keys []schema.Key
	if state == nil {
		return nil
	}
	for _, entry := range state.templates {
		if entry != nil && entry.Lane() == rule.LaneLink {
			keys = append(keys, entry.Key())
		}
	}
	return keys
}

func mountedPointKeys(state *catalog) []schema.Key {
	var keys []schema.Key
	if state == nil {
		return nil
	}
	for _, entry := range state.templates {
		if entry != nil && entry.Lane() == rule.LaneMountedPoint {
			keys = append(keys, entry.Key())
		}
	}
	return keys
}

func templateForKey(state *catalog, key schema.Key) (*rule.Template, bool) {
	slot, ok := ruleSlotForKey(state, key)
	if !ok {
		return nil, false
	}
	return templateAtSlot(state, slot)
}

// MountedRuleKey reports whether key names a mounted or activation rule.
func MountedRuleKey(compilation Compilation, key schema.Key) bool {
	entry, ok := templateForKey(compilation.catalog, key)
	return ok && entry.Lane().Mounted()
}

// templateAtSlot resolves one rule by its slot. The slot is the declaration
// position numbered from one, so slot zero names no rule.
func templateAtSlot(state *catalog, slot int) (*rule.Template, bool) {
	if state == nil || state.sealed == nil || slot <= 0 || slot > len(state.templates) {
		return nil, false
	}
	entry := state.templates[slot-1]
	return entry, entry != nil
}

// ruleSlotForKey resolves one rule's slot from its declared key. It is how a
// rule reaches a peer by identity rather than by position.
func ruleSlotForKey(state *catalog, key schema.Key) (int, bool) {
	if state == nil || state.sealed == nil || !key.Available() || state.slotsByKey == nil {
		return 0, false
	}
	slot, ok := state.slotsByKey[key]
	return slot, ok
}

// DiagnosticRule is the closed analyzer-owned classification of one rule. It
// is the rule's role slot: its dense declaration position, numbered from one.
// Unknown covers empty, foreign, and generic engine lifecycle failures without
// a bound analyzer rule.
type DiagnosticRule uint8

const DiagnosticRuleUnknown DiagnosticRule = 0

// DiagnosticRuleForKey classifies one rule by the key it is declared under.
func DiagnosticRuleForKey(compilation Compilation, key schema.Key) DiagnosticRule {
	return diagnosticRuleForKey(compilation.catalog, key)
}

func diagnosticRuleForKey(state *catalog, key schema.Key) DiagnosticRule {
	slot, ok := ruleSlotForKey(state, key)
	if !ok {
		return DiagnosticRuleUnknown
	}
	return DiagnosticRule(slot)
}

// DiagnosticRuleForSemantic classifies one Engine rule key. The key is neither
// retained nor decoded; only the table is consulted.
func DiagnosticRuleForSemantic(compilation Compilation, key identity.SemanticKey) DiagnosticRule {
	state := compilation.catalog
	if state == nil || !key.Available() {
		return DiagnosticRuleUnknown
	}
	for position, entry := range state.templates {
		if semantic, resolved := state.roles.Key(entry.Semantic()); resolved && semantic == key {
			return DiagnosticRule(position + 1)
		}
	}
	return DiagnosticRuleUnknown
}

// diagnosticRuleNames is the slot-to-key table of the authored rule inventory,
// minted once. The slot is that inventory's own declaration position, so the
// name a slot carries is fixed by the table every compilation seals rather than
// by any one compilation instance.
var diagnosticRuleNames = func() []schema.Key {
	entries, _, ok := RuleTemplates[principals, authorities]()
	if !ok {
		return nil
	}
	names := make([]schema.Key, len(entries)+1)
	for position, entry := range entries {
		if entry == nil {
			continue
		}
		names[position+1] = entry.Key()
	}
	return names
}()

// String spells the rule as its owner declared it. A slot the sealed inventory
// has no row for still names its ordinal: a refusal that reached a rule carries
// that rule's position, and only the absent rule is unknown.
func (classification DiagnosticRule) String() string {
	slot := int(classification)
	if slot <= 0 {
		return "unknown"
	}
	if slot >= len(diagnosticRuleNames) || diagnosticRuleNames[slot] == "" {
		return "slot#" + strconv.Itoa(slot)
	}
	return string(diagnosticRuleNames[slot])
}

// declareRules runs the table's cold declaration pass and returns each rule's
// fragment cell at its slot. It is the only place a rule's Schema shape is
// recorded.
func declareRules(state *catalog, builder *engine.SchemaBuilder, roles vocabulary.Roles, owners principals) (ruleCells, DiagnosticRule, bool) {
	if state == nil {
		return nil, DiagnosticRuleUnknown, false
	}
	fragments := newRuleCells(state.templates)
	if state.sealed == nil || builder == nil || !owners.available() {
		return fragments, DiagnosticRuleUnknown, false
	}
	if len(state.ruleContributors) != len(state.templates) {
		return fragments, DiagnosticRuleUnknown, false
	}
	plans, plansOK := state.declarations.RulePlans()
	if !plansOK {
		return fragments, DiagnosticRuleUnknown, false
	}
	ruleView, ruleViewOK := state.sealed.Surface(schema.SurfaceKindRule)
	if !ruleViewOK {
		return fragments, DiagnosticRuleUnknown, false
	}
	for position, entry := range state.templates {
		slot := position + 1
		if !owners.writes(entry.Writes()) || !owners.writes(entry.Owner()) {
			return fragments, DiagnosticRule(slot), false
		}
		contributor := state.ruleContributors[position]
		canonicalOrdinal, ordinalOK := ruleView.Ordinal(entry.ID())
		if !ordinalOK {
			return fragments, DiagnosticRule(slot), false
		}
		var fragment rule.Cell
		var ok bool
		if entry.Program().Available() {
			fragment, ok = contributor.DeclareGenerated(builder, plans, canonicalOrdinal)
		} else {
			fragment, ok = contributor.Declare(builder, roles, owners)
		}
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
	// RuleBindStageTable is the pass's own precondition: the table, binding, or
	// per-slot cell vector it was handed is not the shape the sealed table
	// declares. No rule has been reached, so the verdict is the table's rather
	// than a rule's.
	RuleBindStageTable
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
	case RuleBindStageTable:
		return "table"
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
// sealed row admission and classification through the table, never through a
// domain type.
type RuleBinding struct {
	catalog    *catalog
	binding    *engine.SchemaBinding
	hot        ruleCells
	activation ActivationRule
}

func (rules *RuleBinding) cellByKey(key schema.Key) (rule.Cell, bool) {
	if rules == nil {
		return rule.Cell{}, false
	}
	slot, ok := ruleSlotForKey(rules.catalog, key)
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
	if rules == nil {
		return engine.RuleSlotCapability{}, false
	}
	slot, ok := ruleSlotForKey(rules.catalog, key)
	if !ok {
		return engine.RuleSlotCapability{}, false
	}
	return rules.capabilityAtSlot(slot)
}

// MountedCapabilityForArtifactRole resolves the capability for one canonical
// scalar artifact role. Lowering has already replaced every transfer axis with
// its declaration-owned transport rule key, so this boundary performs one
// authenticated rule-key lookup and never guesses whether the spelling is an
// axis or a rule.
func (rules *RuleBinding) MountedCapabilityForArtifactRole(key schema.Key) (engine.RuleSlotCapability, bool) {
	if rules == nil || !key.Available() {
		return engine.RuleSlotCapability{}, false
	}
	capability, capabilityOK := rules.CapabilityByKey(key)
	return capability, capabilityOK && capability.Mounted()
}

// capabilityAtSlot resolves one rule's sealed slot capability by its slot.
func (rules *RuleBinding) capabilityAtSlot(slot int) (engine.RuleSlotCapability, bool) {
	entry, entryOK := templateAtSlot(rules.catalog, slot)
	if !entryOK || rules == nil || rules.binding == nil {
		return engine.RuleSlotCapability{}, false
	}
	semantic, semanticOK := rules.catalog.roles.Key(entry.Semantic())
	if !semanticOK {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := engine.BindingRuleSlot(rules.binding, semantic)
	return capability, ok && (capability.Mounted() || capability.Link() || capability.MountedPoint())
}

// DiagnosticForCapability classifies one Engine rule capability against the
// table. It replaces the per-lane capability scans that each rebuilt their own
// role inventory.
func (rules *RuleBinding) DiagnosticForCapability(capability engine.RuleSlotCapability) DiagnosticRule {
	if rules == nil || !(capability.Mounted() || capability.Link() || capability.MountedPoint()) {
		return DiagnosticRuleUnknown
	}
	for position := range rules.catalog.templates {
		candidate, ok := rules.capabilityAtSlot(position + 1)
		if ok && candidate == capability {
			return DiagnosticRule(position + 1)
		}
	}
	return DiagnosticRuleUnknown
}

func (rules *RuleBinding) OccurrenceCatalogByKey(key schema.Key) (rule.OccurrenceCatalog, bool) {
	hot, hotOK := rules.cellByKey(key)
	contributor, contributorOK := ruleContributorFor(rules.catalog, key)
	if !hotOK || !contributorOK {
		return nil, false
	}
	return contributor.OccurrenceCatalog(rules.binding, hot)
}

func ruleContributorFor(state *catalog, key schema.Key) (RuleContributor[principals, authorities], bool) {
	if state == nil || len(state.ruleContributors) != len(state.templates) {
		return RuleContributor[principals, authorities]{}, false
	}
	for index, entry := range state.templates {
		if entry != nil && entry.Key() == key {
			return state.ruleContributors[index], true
		}
	}
	return RuleContributor[principals, authorities]{}, false
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
func bindRules(state *catalog, binding *engine.SchemaBinding, fragments ruleCells, set authorities, seal func() bool) (*RuleBinding, DiagnosticRule, RuleBindStage) {
	if state == nil || state.sealed == nil || binding == nil || !set.available() || seal == nil || len(fragments) != len(state.templates)+1 || len(state.ruleContributors) != len(state.templates) {
		return nil, DiagnosticRuleUnknown, RuleBindStageTable
	}
	rules := &RuleBinding{catalog: state, binding: binding, hot: newRuleCells(state.templates)}
	for position, entry := range state.templates {
		slot := position + 1
		if !set.writes(entry.Writes()) || !set.writes(entry.Owner()) {
			return nil, DiagnosticRule(slot), RuleBindStagePrincipal
		}
		if !fragments[slot].Available() {
			return nil, DiagnosticRule(slot), RuleBindStageFragment
		}
		hot, activation, ok := state.ruleContributors[position].Bind(binding, set, fragments[slot])
		if !ok {
			return nil, DiagnosticRule(slot), RuleBindStageBind
		}
		rules.hot[slot] = hot
		if activation != nil {
			if rules.activation != nil {
				return nil, DiagnosticRule(slot), RuleBindStageBind
			}
			rules.activation = activation
		}
	}
	// Slots are handed to their owners one lane at a time: every artifact
	// mounted plane before the Link-owned plane. The grouping is the entry's
	// declared lane, so the sequence is a property of the table rather than a
	// hand-kept order. Within a lane the table's own order holds, and one rule
	// at a time keeps the first rejected slot's exact owner classification.
	// The issued capability lives only until the pairing pass ends; the sealed
	// binding is the sole later lookup authority.
	issued := make([]engine.RuleSlotCapability, len(state.templates)+1)
	for _, mountedLane := range [...]bool{true, false} {
		for position, entry := range state.templates {
			if entry.Lane().Mounted() != mountedLane {
				continue
			}
			slot := position + 1
			capability, ok := state.ruleContributors[position].Register(binding, fragments[slot])
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
		slot, slotOK := ruleSlotForKey(state, key)
		if !slotOK || slot >= len(issued) {
			return engine.RuleSlotCapability{}, false
		}
		capability := issued[slot]
		return capability, capability.Mounted() || capability.Link() || capability.MountedPoint()
	}
	for position := range state.templates {
		slot := position + 1
		if contributor := state.ruleContributors[position]; !contributor.Pair(binding, fragments[slot], resolve) {
			return nil, DiagnosticRule(slot), RuleBindStagePair
		}
	}
	if !registerLinkBootstrapTransports(state, issued, binding) {
		return nil, DiagnosticRuleUnknown, RuleBindStagePair
	}
	if !seal() {
		return nil, DiagnosticRuleUnknown, RuleBindStageSeal
	}
	// The sealed directory must publish every rule on the lane it declared.
	// Nothing is retained: this pass states the law, and later lookups reach
	// the same directory.
	for position, entry := range state.templates {
		slot := position + 1
		capability, ok := rules.capabilityAtSlot(slot)
		if !ok || entry.Lane().Mounted() != capability.Mounted() ||
			(entry.Lane() == rule.LaneLink) != capability.Link() ||
			(entry.Lane() == rule.LaneMountedPoint) != capability.MountedPoint() {
			return nil, DiagnosticRule(slot), RuleBindStageCapability
		}
	}
	for position := range state.templates {
		slot := position + 1
		if contributor := state.ruleContributors[position]; !contributor.Finalize(set, rules.hot[slot]) {
			return nil, DiagnosticRule(slot), RuleBindStageFinalize
		}
	}
	return rules, DiagnosticRuleUnknown, RuleBindStageNone
}

// registerLinkBootstrapTransports authorizes the factor set allowed to leave
// the Link-global bootstrap point. A factor computed at that point must be able
// to leave it, and it leaves once, so the authorization is keyed by the axis a
// Link rule writes rather than by the rule: two Link rules writing one axis
// name one transport, and the catalog's own order fixes which capability
// stands for it. Nothing here is authored per rule - a hand-kept list would be
// a second statement of the Link lane that could disagree with it.
func registerLinkBootstrapTransports(state *catalog, issued []engine.RuleSlotCapability, binding *engine.SchemaBinding) bool {
	transports := make([]engine.RuleSlotCapability, 0, len(state.templates))
	written := make(map[schema.Key]struct{}, len(state.templates))
	for position, entry := range state.templates {
		if entry == nil || entry.Lane() != rule.LaneLink {
			continue
		}
		if _, duplicate := written[entry.Writes()]; duplicate {
			continue
		}
		written[entry.Writes()] = struct{}{}
		transports = append(transports, issued[position+1])
	}
	if len(transports) == 0 {
		return true
	}
	return engine.RegisterLinkBootstrapTransports(binding, transports...)
}

// SemanticRoles is the resolved semantic role vocabulary the sealed table was
// composed against. A consumer that holds a declared role key reaches this
// compilation's identity rather than deriving one of its own.
func SemanticRoles(compilation Compilation) (vocabulary.Roles, bool) {
	state := compilation.catalog
	if state == nil || state.sealed == nil {
		return vocabulary.Roles{}, false
	}
	return state.roles, state.roles.Available()
}

// CatalogRefusal names the first rule whose program clauses refuse the
// catalog, and which clause refused it.
//
// A SealFailure carries a law and an entry identity but no authored key, which
// is right for a value that crosses surfaces and useless for locating the rule
// that raised it. This reads the same templates the catalog is built from and
// says which one, so a refused composition names itself instead of costing a
// bisect. It admits nothing: newCatalog remains the authority.
func CatalogRefusal() string {
	return catalogRefusal(schema.SealFailure{})
}

// catalogRefusal names the rule a failure points at, and the program clause
// that refuses it when one does. A Rule-surface failure carries an entry
// identity, which is the only handle on WHICH rule refused; resolving it back
// to the authored key is what makes the refusal readable.
func catalogRefusal(failure schema.SealFailure) string {
	templates, _, ok := RuleTemplates[principals, authorities]()
	if !ok {
		return "the rule templates do not wire"
	}
	named := ""
	if failure.Contributor == schema.SurfaceKindRule && failure.Entry != (schema.EntryID{}) {
		for _, template := range templates {
			if template.ID() == failure.Entry {
				named = fmt.Sprintf("rule %q", string(template.Key()))
				if refusal := template.ProgramRefusal(); refusal != "" {
					return refusal
				}
				break
			}
		}
		if named == "" {
			named = "an entry no wired rule declares"
		}
	}
	for _, template := range templates {
		if refusal := template.ProgramRefusal(); refusal != "" {
			return refusal
		}
	}
	return named
}

// BuildRefusal is the failure the catalog composition refuses with, beside the
// named cause when one of the rule program clauses is what refused it.
//
// Build answers a bool because a caller either has a compilation or does not.
// This answers the question that bool leaves open, and it is the surface a law
// or a tool reports a refused composition through rather than re-deriving the
// composition to guess at it.
func BuildRefusal() (schema.SealFailure, string) {
	state, failure := newCatalog()
	if state != nil && state.failure.Available() {
		failure = state.failure
	}
	return failure, catalogRefusal(failure)
}
