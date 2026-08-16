package grammar

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
	"github.com/wippyai/go-lua/analysis/schema/queryreg"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

// ruleRoleLimit bounds the role-indexed projections below. The surface seal
// law pins the table to the artifact role ordinals, so this is the exact size
// of the catalog.
const ruleRoleLimit = int(programartifact.RuleRoleValuePresenceRefinement) + 1

// registry is the sealed analyzer declaration table. It is built once and is
// immutable afterwards; a rejected law leaves the table unavailable rather
// than half usable.
var registry struct {
	once            sync.Once
	sealed          *schema.Schema
	failure         schema.SealFailure
	templates       []*template
	byRole          [ruleRoleLimit]*template
	axes            []*axisTemplate
	axisByPrincipal [axisPrincipalLimit]*axisTemplate
	diagnostics     diagnostic.Table
	bundle          vocabulary.Bundle
}

func sealRegistry() {
	registry.once.Do(func() {
		// The closed semantic vocabulary is the identity source every inventory
		// below draws its declared identities from, so it is built first and
		// handed to each of them rather than rebuilt per surface.
		bundle, bundleOK := vocabulary.New()
		if !bundleOK {
			registry.failure = schema.SurfaceLawFailure(schema.SurfaceKindRule, schema.EntryID{}, rule.LawVocabulary, schema.DispositionMalformed)
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
		denominators, denominatorsOK := denominatorEntries(axes, bundle)
		if !denominatorsOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindDenominator, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		queries, queriesOK := queryRegistrations(bundle)
		if !queriesOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindQuery, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		structures, structuresOK := structureEntries()
		if !structuresOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindStructure, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		kinds, kindsOK := libraryKinds()
		if !kindsOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindLibrary, Law: schema.LawEntryAdmissible, Disposition: schema.DispositionMalformed}
			return
		}
		// The registration order is the catalog order, which is the bind phase
		// order: axes are sealed before the rules that write them and before
		// every surface that names a coordinate space, diagnostics after the
		// rules and axes they reference, denominators after the surfaces they
		// may be owned by, the structural vocabulary naming none of them, and
		// the contract kinds last, after every surface a validation reference
		// may resolve against.
		builder := schema.NewBuilder()
		builder.Register(axis.NewSurface(axes))
		builder.Register(rule.NewSurface(templates))
		builder.Register(diagnostic.NewSurface(diagnostics))
		builder.Register(composite.NewSurface(composites))
		builder.Register(denominator.NewSurface(denominators))
		builder.Register(queryreg.NewSurface(queries))
		builder.Register(structure.NewSurface(structures))
		builder.Register(library.NewSurface(kinds))
		sealed, failure := builder.Seal()
		if failure.Available() {
			registry.failure = failure
			return
		}
		diagnosticView, diagnosticViewOK := sealed.Surface(schema.SurfaceKindDiagnostic)
		diagnosticTable, diagnosticTableOK := diagnostic.NewTable(diagnosticView)
		if !diagnosticViewOK || !diagnosticTableOK {
			registry.failure = schema.SealFailure{Contributor: schema.SurfaceKindDiagnostic, Law: schema.LawSurfacePopulated, Disposition: schema.DispositionIncomplete}
			return
		}
		registry.diagnostics = diagnosticTable
		for _, entry := range templates {
			registry.byRole[entry.Role()] = entry
		}
		for _, entry := range axes {
			registry.axisByPrincipal[entry.Principal()] = entry
		}
		registry.templates, registry.axes, registry.bundle, registry.sealed = templates, axes, bundle, sealed
	})
}

// Table returns the sealed declaration root and the law that rejected it. The
// axis, rule, diagnostic, composite, denominator, query, structure, and
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

// RuleRoleAt returns the artifact role of one rule at its table position. The
// position is a traversal convenience; the role is the identity.
func RuleRoleAt(position int) (programartifact.RuleRole, bool) {
	sealRegistry()
	if position < 0 || position >= len(registry.templates) {
		return programartifact.RuleRoleInvalid, false
	}
	return registry.templates[position].Role(), true
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
func RuleSemantic(role programartifact.RuleRole) (engine.SemanticKey, bool) {
	entry, ok := templateForRole(role)
	if !ok {
		return engine.SemanticKey{}, false
	}
	semantic := entry.Semantic(registry.bundle)
	return semantic, semantic.Available()
}

// LinkRoles returns the Link-lane roles in table order. A plan admits exactly
// these outside the mounted artifact lane.
func LinkRoles() []programartifact.RuleRole {
	sealRegistry()
	var roles []programartifact.RuleRole
	for _, entry := range registry.templates {
		if entry.Lane() == rule.LaneLink {
			roles = append(roles, entry.Role())
		}
	}
	return roles
}

func templateForRole(role programartifact.RuleRole) (*template, bool) {
	sealRegistry()
	if registry.sealed == nil || int(role) <= 0 || int(role) >= ruleRoleLimit {
		return nil, false
	}
	entry := registry.byRole[role]
	return entry, entry != nil
}

// DiagnosticRule is the closed analyzer-owned classification of one rule. It
// is the rule's artifact role ordinal. Unknown covers empty, foreign, and
// generic engine lifecycle failures without a bound analyzer rule.
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

// DiagnosticRuleForSemantic classifies one Engine rule key. The key is neither
// retained nor decoded; only the table is consulted.
func DiagnosticRuleForSemantic(key engine.SemanticKey) DiagnosticRule {
	sealRegistry()
	if !key.Available() {
		return DiagnosticRuleUnknown
	}
	for _, entry := range registry.templates {
		if entry.Semantic(registry.bundle) == key {
			return DiagnosticRule(entry.Role())
		}
	}
	return DiagnosticRuleUnknown
}

func (classification DiagnosticRule) String() string {
	if entry, ok := templateForRole(programartifact.RuleRole(classification)); ok {
		return string(entry.Key())
	}
	return "unknown"
}

// declareRules runs the table's cold declaration pass and returns each rule's
// fragment cell at its role. It is the only place a rule's Schema shape is
// recorded.
func declareRules(builder *engine.SchemaBuilder, bundle vocabulary.Bundle, owners principals) ([ruleRoleLimit]rule.Cell, DiagnosticRule, bool) {
	var fragments [ruleRoleLimit]rule.Cell
	sealRegistry()
	if registry.sealed == nil || builder == nil || !owners.available() {
		return fragments, DiagnosticRuleUnknown, false
	}
	context := declaration{Builder: builder, Bundle: bundle, Principals: owners}
	for _, entry := range registry.templates {
		if !owners.has(entry.Principal()) {
			return fragments, DiagnosticRule(entry.Role()), false
		}
		fragment, ok := entry.Declare(context)
		if !ok {
			return fragments, DiagnosticRule(entry.Role()), false
		}
		fragments[entry.Role()] = fragment
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
	hot     [ruleRoleLimit]rule.Cell
}

func (rules *RuleBinding) cell(role programartifact.RuleRole) (rule.Cell, bool) {
	if rules == nil || int(role) <= 0 || int(role) >= ruleRoleLimit {
		return rule.Cell{}, false
	}
	hot := rules.hot[role]
	return hot, hot.Available()
}

// Capability resolves one rule's exact sealed slot capability by role. The
// sealed SchemaBinding directory is the sole authority: the rule's canonical
// semantic identity is looked up there on demand, never cached into a second
// per-role registry.
func (rules *RuleBinding) Capability(role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
	entry, entryOK := templateForRole(role)
	if !entryOK || rules == nil || rules.binding == nil {
		return engine.RuleSlotCapability{}, false
	}
	capability, ok := engine.BindingRuleSlot(rules.binding, entry.Semantic(registry.bundle))
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
	for _, entry := range registry.templates {
		candidate, ok := rules.Capability(entry.Role())
		if ok && candidate == capability {
			return DiagnosticRule(entry.Role())
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
func bindRules(binding *engine.SchemaBinding, fragments [ruleRoleLimit]rule.Cell, set authorities, seal func() bool) (*RuleBinding, DiagnosticRule, RuleBindStage) {
	sealRegistry()
	if registry.sealed == nil || binding == nil || !set.available() || seal == nil {
		return nil, DiagnosticRuleUnknown, RuleBindStagePrincipal
	}
	rules := &RuleBinding{binding: binding}
	for _, entry := range registry.templates {
		role := entry.Role()
		if !set.has(entry.Principal()) {
			return nil, DiagnosticRule(role), RuleBindStagePrincipal
		}
		if !fragments[role].Available() {
			return nil, DiagnosticRule(role), RuleBindStageFragment
		}
		hot, ok := entry.Bind(binding, set, fragments[role])
		if !ok {
			return nil, DiagnosticRule(role), RuleBindStageBind
		}
		rules.hot[role] = hot
	}
	// Slots are handed to their owners one lane at a time: every artifact
	// mounted plane before the Link-owned plane. The grouping is the entry's
	// declared lane, so the sequence is a property of the table rather than a
	// hand-kept order. Within a lane the table's own order holds, and one rule
	// at a time keeps the first rejected slot's exact owner classification.
	// The issued capability lives only until the pairing pass ends; the sealed
	// binding is the sole later lookup authority.
	var issued [ruleRoleLimit]engine.RuleSlotCapability
	for _, mountedLane := range [...]bool{true, false} {
		for _, entry := range registry.templates {
			if entry.Lane().Mounted() != mountedLane {
				continue
			}
			role := entry.Role()
			capability, ok := entry.Register(binding, fragments[role])
			if !ok {
				return nil, DiagnosticRule(role), RuleBindStageRegister
			}
			issued[role] = capability
		}
	}
	resolve := func(role programartifact.RuleRole) (engine.RuleSlotCapability, bool) {
		if int(role) <= 0 || int(role) >= ruleRoleLimit {
			return engine.RuleSlotCapability{}, false
		}
		capability := issued[role]
		return capability, capability.Mounted() || capability.Link()
	}
	for _, entry := range registry.templates {
		role := entry.Role()
		if entry.HasPair() && !entry.Pair(binding, fragments[role], resolve) {
			return nil, DiagnosticRule(role), RuleBindStagePair
		}
	}
	if !seal() {
		return nil, DiagnosticRuleUnknown, RuleBindStageSeal
	}
	// The sealed directory must publish every rule on the lane it declared.
	// Nothing is retained: this pass states the law, and later lookups reach
	// the same directory.
	for _, entry := range registry.templates {
		role := entry.Role()
		capability, ok := rules.Capability(role)
		if !ok || entry.Lane().Mounted() != capability.Mounted() || (entry.Lane() == rule.LaneLink) != capability.Link() {
			return nil, DiagnosticRule(role), RuleBindStageCapability
		}
	}
	for _, entry := range registry.templates {
		role := entry.Role()
		if entry.HasFinalize() && !entry.Finalize(set, rules.hot[role]) {
			return nil, DiagnosticRule(role), RuleBindStageFinalize
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
