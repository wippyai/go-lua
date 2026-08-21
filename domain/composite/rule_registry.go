package composite

import (
	analysiscatalog "github.com/wippyai/go-lua/analysis/catalog"
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

func newRuleCells(entries []*rule.Template) ruleCells { return make(ruleCells, len(entries)+1) }

func newCatalog() (*catalog, schema.SealFailure) {
	state := &catalog{}
	// The structural vocabulary is the first surface of the catalog and the
	// one every identity is declared in, so it is admitted first and its
	// semantic roles are resolved once for every inventory below.
	structures, structuresOK := structureEntries()
	if !structuresOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	roles, rolesOK := vocabulary.NewRoles(structures)
	if !rolesOK {
		// A declared role whose spelling derives no identity leaves every
		// surface that names it unresolvable, so the catalog cannot be composed
		// at all rather than composed short one identity.
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	axes, axisContributors, axesOK := axisTemplates()
	if !axesOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindAxis, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	templates, ruleContributors, ok := RuleTemplates[principals, authorities]()
	if !ok {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	diagnostics, diagnosticsOK := diagnosticEntries()
	if !diagnosticsOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindDiagnostic, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	composites, compositesOK := compositeEntries()
	if !compositesOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindComposite, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	denominators, denominatorsOK := denominatorEntries(axes, roles)
	if !denominatorsOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindDenominator, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	queries, contributors, queriesOK := queryRegistrations(roles)
	if !queriesOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	observations, observationsOK := observationEntries(queries)
	if !observationsOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindObservation, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
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
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		}
		return state, state.failure
	}
	sealed := compiledDeclarations.Schema()
	structureView, structureViewOK := sealed.Surface(schema.SurfaceKindStructure)
	structureTable, structureTableOK := structure.NewTable(structureView)
	if !structureViewOK || !structureTableOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	// The diagnostic surface states its own population law, so a sealed table
	// always carries rows to project here. A projection that still does not
	// form means the table this catalog composed is not the table it declared.
	diagnosticView, diagnosticViewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
	diagnosticTable, diagnosticTableOK := diagnostic.NewTable(diagnosticView)
	if !diagnosticViewOK || !diagnosticTableOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindDiagnostic, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	state.diagnostics = diagnosticTable
	state.structure = structureTable
	state.structureOK = true
	observationView, observationViewOK := sealed.Surface(schema.SurfaceKindObservation)
	observationTable, observationTableOK := observation.NewTable(observationView)
	if !observationViewOK || !observationTableOK {
		state.failure = schema.SealFailure{Contributor: schema.SurfaceKindObservation, Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionMalformed}
		return state, state.failure
	}
	state.observations = observationTable
	state.axisAdopters = axisAdopterTable(axes)
	positions := make(map[schema.Key]int, len(queries))
	for position, registration := range queries {
		positions[registration.Key()] = position
	}
	slots := make(map[schema.Key]int, len(templates))
	for position, entry := range templates {
		if entry == nil || !entry.Key().Available() {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return state, state.failure
		}
		if _, duplicate := slots[entry.Key()]; duplicate {
			state.failure = schema.SealFailure{Contributor: schema.SurfaceKindRule, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
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
func Table(compilation Compilation) (*schema.Schema, schema.SealFailure) {
	state := compilation.catalog
	if state == nil {
		return nil, schema.SealFailure{Law: schema.LawSurfaceCatalog, Disposition: schema.DispositionIncomplete}
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

func (classification DiagnosticRule) String() string {
	return "unknown"
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
	for position, entry := range state.templates {
		slot := position + 1
		if !owners.writes(entry.Writes()) || !owners.writes(entry.Owner()) {
			return fragments, DiagnosticRule(slot), false
		}
		fragment, ok := state.ruleContributors[position].Declare(builder, roles, owners)
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
	return capability, ok && (capability.Mounted() || capability.Link())
}

// DiagnosticForCapability classifies one Engine rule capability against the
// table. It replaces the per-lane capability scans that each rebuilt their own
// role inventory.
func (rules *RuleBinding) DiagnosticForCapability(capability engine.RuleSlotCapability) DiagnosticRule {
	if rules == nil || !(capability.Mounted() || capability.Link()) {
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

func (rules *RuleBinding) LinkCatalogByKey(key schema.Key) (rule.LinkCatalog, bool) {
	hot, hotOK := rules.cellByKey(key)
	contributor, contributorOK := ruleContributorFor(rules.catalog, key)
	if !hotOK || !contributorOK {
		return nil, false
	}
	return contributor.LinkCatalog(hot)
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
		return nil, DiagnosticRuleUnknown, RuleBindStagePrincipal
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
		return capability, capability.Mounted() || capability.Link()
	}
	for position := range state.templates {
		slot := position + 1
		if contributor := state.ruleContributors[position]; !contributor.Pair(binding, fragments[slot], resolve) {
			return nil, DiagnosticRule(slot), RuleBindStagePair
		}
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
		if !ok || entry.Lane().Mounted() != capability.Mounted() || (entry.Lane() == rule.LaneLink) != capability.Link() {
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
