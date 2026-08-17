package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

func validScopedExpr(expr Expr, scope Scope) bool {
	if !expr.Available() || !scope.Available() {
		return false
	}
	for _, decision := range expr.Decisions() {
		if !scope.contains(decision) {
			return false
		}
	}
	return true
}

// TemplateRole maps one canonical fragment role to one presealed base Point. Axes
// membership belongs exclusively to the sealed receipt; a binding with a
// different base mapping is a different fixed-shape binding, never a branch
// inside this port table.
type TemplateRole struct {
	Role composition.Key
	Mode PortMode
	// Reads are the finite named prototype exact-read slots supplied by this
	// import role.  They are canonicalized by PortRead.Role, never declaration
	// position, and remain optional for control-only ports.
	Reads []PortRead
}

// TargetPoint is one symbolic point reference in a Template. Exactly one
// coordinate is present: Local names a pair-owned PointSpec, while TemplateRole names
// a static TemplateRole role. The raw PointRef/Role are builder input only; seal
// resolves them to issued point identities before any graph is compiled.
type TargetPoint struct {
	Local PointRef
	Role  composition.Key
}

// TemplateInput is an ordinary complete boundary input before its symbolic
// endpoints are resolved.  Pre/post conditions and provenance remain part of
// the template so member expansion cannot silently drop them.
type TemplateInput struct {
	Point      TargetPoint
	Provenance composition.Key
	Pre        Expr
	Reindex    Reindex
	Post       Expr
}

// TargetGroup is a finite ordinary Group shape. Expansion resolves its
// Local/TemplateRole point references and emits the normal compiled input form used
// by the sole equation compiler.
type TargetGroup struct {
	Members []RuleRef
	Output  TargetPoint
	Inputs  []TemplateInput
}

// TargetFactorEdge is one structural Factor-local transport in an
// activation fragment.  It is deliberately the same boundary shape as an
// ordinary Input, with symbolic endpoints resolved only while an accepted
// Member is expanded.  The edge has no Rule/Group/Member authority of its
// own; after expansion it is appended to TopologySpec.FactorEdges and follows
// the ordinary graph, demand, WTO, and runtime paths.
type TargetFactorEdge struct {
	// Source is the member-local or named-port endpoint. ExternalSource is
	// an already-issued source Point supplied by the owning target Batch;
	// exactly one of them is present. ExternalSource is not alpha-renamed or
	// copied during Member expansion.
	Source         TargetPoint
	ExternalSource Site
	Target         TargetPoint
	// ExternalTarget is the symmetric already-issued target Point form. The
	// ordinary FactorEdge topology remains the only runtime edge plane; this
	// field merely lets a structural fragment terminate at an existing base
	// point rather than forcing a synthetic export port.
	ExternalTarget Site
	Factor         composition.Key
	Provenance     composition.Key
	Pre            Expr
	Reindex        Reindex
	Post           Expr
}

// Template is a finite, data-only ordinary topology fragment. Its Rules use
// fixed identity placeholders; Local points and rule anchors become pair-owned
// at expansion, while static Ports bind only to already sealed base Points.
// It has no callback, conditional, iterator, or executable authority.
type Template struct {
	Rules       []RuleInstance
	Points      []PointSpec
	Roles       []TemplateRole
	Groups      []TargetGroup
	FactorEdges []TargetFactorEdge
	Summaries   []SummaryMapping
	WeakTargets []WeakTargetMapping
}

// Variant is one target-owned endpoint branch of a shared activation family
// plan. It deliberately has no Application coordinate: one immutable plan is
// reused by every application trigger, while an accepted Member contributes
// the exact application only at graph expansion.
type Variant struct {
	Target   composition.Key
	Endpoint composition.Key
	Template Template
}

// VariantPlan is the immutable target-indexed family vocabulary. Its private
// data holds each data-only Template exactly once. Bindings may reference it,
// but cannot enumerate or alter its variants.
type VariantPlan struct{ data *variantPlanData }

// PrototypeRow is one immutable, authenticated factor or structural row in a
// VariantPlan.  It is deliberately not a builder reference: callers may use
// its identity to attach typed payloads, but cannot manufacture a row or
// select a different variant by declaration position.
type PrototypeRow struct {
	plan       *variantPlanData
	key        composition.Key
	schema     composition.Key
	family     composition.Key
	occurrence Occurrence
	operand    Operand
	row        RuleInstance
}

func (row PrototypeRow) Available() bool {
	return row.plan != nil && row.key.Available() && row.schema.Available() && row.family.Available() && row.occurrence.Available() && row.operand.Available()
}

func (row PrototypeRow) Key() composition.Key           { return row.key }
func (row PrototypeRow) Schema() composition.Key        { return row.schema }
func (row PrototypeRow) OperandFamily() composition.Key { return row.family }
func (row PrototypeRow) Occurrence() Occurrence         { return row.occurrence }
func (row PrototypeRow) Operand() Operand               { return row.operand }

// MatchesExact proves that a typed engine declaration lowered to the exact
// immutable row sealed in this plan. Exact and staged-selector reads plus
// exact writes are admitted here. A staged selector is Rule-schema-owned and
// retains only a local static surface in the template; no callback or route
// catalog crosses this boundary. Summary/selector-write/weak rows still
// require a whole-template authentication cut.
func (row PrototypeRow) MatchesExact(value RuleInstance) bool {
	if !row.Available() || !activationPrototypeInstance(row.row) || !activationPrototypeInstance(value) ||
		row.row.Schema != value.Schema || row.row.OperandFamily != value.OperandFamily || !row.row.Occurrence.Same(value.Occurrence) || !row.row.Operand.Same(value.Operand) ||
		len(row.row.Reads) != len(value.Reads) || len(row.row.Carries) != len(value.Carries) || len(row.row.Writes) != len(value.Writes) ||
		len(row.row.Supports) != len(value.Supports) || len(row.row.Prunes) != len(value.Prunes) {
		return false
	}
	for index := range value.Reads {
		if row.row.Reads[index] != value.Reads[index] {
			return false
		}
	}
	for index := range value.Carries {
		if row.row.Carries[index] != value.Carries[index] {
			return false
		}
	}
	for index := range value.Writes {
		if row.row.Writes[index].Index != value.Writes[index].Index || row.row.Writes[index].Surface != value.Writes[index].Surface {
			return false
		}
	}
	for index := range value.Supports {
		if row.row.Supports[index] != value.Supports[index] {
			return false
		}
	}
	for index := range value.Prunes {
		if row.row.Prunes[index] != value.Prunes[index] {
			return false
		}
	}
	return true
}

// MatchesPortReads authenticates every typed import placeholder used to lower
// this prototype. A source compiler may choose whether the same role also
// exports control, but it cannot rename a role/slot, replace its Factor
// surface, or bind one placeholder through a second slot.
func (row PrototypeRow) MatchesPortReads(expected []TemplateRole) bool {
	if !row.Available() {
		return false
	}
	type route struct {
		role composition.Key
		slot composition.Key
	}
	wanted := make(map[Surface]route)
	seenRoles := make(map[composition.Key]struct{}, len(expected))
	rowReads := make(map[Surface]struct{}, len(row.row.Reads))
	for _, read := range row.row.Reads {
		rowReads[read.Surface] = struct{}{}
	}
	for _, port := range expected {
		if !port.Role.Available() || !row.plan.ports[port.Role].imports() || len(port.Reads) == 0 {
			return false
		}
		if _, duplicate := seenRoles[port.Role]; duplicate {
			return false
		}
		seenRoles[port.Role] = struct{}{}
		for _, read := range port.Reads {
			if !read.Role.Available() || !read.Surface.Available() {
				return false
			}
			if _, present := rowReads[read.Surface]; !present {
				return false
			}
			if _, duplicate := wanted[read.Surface]; duplicate {
				return false
			}
			wanted[read.Surface] = route{role: port.Role, slot: read.Role}
		}
	}
	matched := make(map[Surface]struct{}, len(wanted))
	for role, reads := range row.plan.portReads {
		for _, read := range reads {
			if _, belongs := rowReads[read.Surface]; !belongs {
				continue
			}
			want, present := wanted[read.Surface]
			if !present || want.role != role || want.slot != read.Role {
				return false
			}
			if _, duplicate := matched[read.Surface]; duplicate {
				return false
			}
			matched[read.Surface] = struct{}{}
		}
	}
	return len(matched) == len(wanted)
}

func activationPrototypeInstance(row RuleInstance) bool {
	if !row.Schema.Available() || !row.OperandFamily.Available() || !row.Occurrence.Available() || !row.Operand.Available() {
		return false
	}
	selectorLocal := uint64(0)
	for index, read := range row.Reads {
		if read.Index != uint64(index) || read.Surface.Mode != TargetModeNone {
			return false
		}
		switch read.Surface.Form {
		case SurfaceReadExact:
			if read.Surface.Semantic.Available() || read.Surface.Normalizer.Available() {
				return false
			}
		case SurfaceReadSelect:
			selectorLocal++
			if read.Surface.Local != selectorLocal || !read.Surface.Semantic.Available() || read.Surface.Semantic != read.Surface.Factor || read.Surface.Normalizer.Available() {
				return false
			}
		default:
			return false
		}
	}
	for index, write := range row.Writes {
		if write.Index != uint64(index) || write.Surface.Form != SurfaceWriteExact || (write.Surface.Mode != TargetModeStrong && write.Surface.Mode != TargetModeWeak) || write.Surface.Semantic.Available() || write.Surface.Normalizer.Available() ||
			write.Route != 0 {
			return false
		}
	}
	for _, support := range row.Supports {
		if !support.Surface.Available() {
			return false
		}
	}
	for _, prune := range row.Prunes {
		if !prune.Surface.Available() {
			return false
		}
	}
	return true
}

// PrototypeRows exposes the authenticated rows for one exact target/endpoint
// variant.  The returned order is canonical row identity order, never source
// declaration order.
func (plan VariantPlan) PrototypeRows(target, endpoint composition.Key) []PrototypeRow {
	variant, found := plan.variant(target, endpoint)
	if !found {
		return nil
	}
	rows := make([]PrototypeRow, len(variant.template.formal.prototypes))
	for index, instance := range variant.template.formal.prototypes {
		key, ok := identityKey("analysis/engine/equation/activation-prototype-row", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, plan.data.key) && writeKey(writer, variant.target) && writeKey(writer, variant.endpoint) && writeKey(writer, instance.key)
		})
		if !ok {
			return nil
		}
		rows[index] = PrototypeRow{plan: plan.data, key: key, schema: instance.row.Schema, family: instance.row.OperandFamily, occurrence: instance.row.Occurrence, operand: instance.row.Operand, row: copyInstance(instance.row)}
	}
	sort.Slice(rows, func(left, right int) bool { return lessKey(rows[left].key, rows[right].key) })
	return rows
}

// OwnsPrototypeRow proves that row was issued by this exact shared plan. It
// closes the replay boundary: a row from an equal-shaped but distinct plan is
// not a capability to attach a payload here.
func (plan VariantPlan) OwnsPrototypeRow(row PrototypeRow) bool {
	return plan.data != nil && row.Available() && row.plan == plan.data
}

type variantPlanData struct {
	source    *composition.Composition
	family    composition.Key
	key       composition.Key
	variants  []sealedVariant
	ports     map[composition.Key]PortMode
	portReads map[composition.Key][]PortRead
	selectors []prototypeSelectorSurface
	exports   []composition.Key
}

// structuralOnly reports the zero-payload activation shape. Every variant
// must retain at least one structural FactorEdge and no Rule instance; mixed
// typed/structural plans would otherwise make the shared ABI ambiguous.
func (data *variantPlanData) structuralOnly() bool {
	if data == nil || len(data.variants) == 0 {
		return false
	}
	for _, variant := range data.variants {
		if len(variant.template.formal.prototypes) != 0 || variant.template.formal.factorEdges == 0 {
			return false
		}
	}
	return true
}

type sealedVariant struct {
	target   composition.Key
	endpoint composition.Key
	template formalVariantDescriptor
}

// NewVariantPlan seals target/endpoint membership and immutable prototype
// rows once. Templates carry only named port roles here: concrete base Points
// are supplied by an exact trigger binding later, so plan storage is O(E),
// never application×endpoint.
func NewVariantPlan(source *composition.Composition, family composition.Key, values []Variant) (VariantPlan, bool) {
	if source == nil || !family.Available() || len(values) == 0 {
		return VariantPlan{}, false
	}
	if _, known := source.ActivationFamily(family); !known {
		return VariantPlan{}, false
	}
	variants := make([]sealedVariant, len(values))
	var roles map[composition.Key]PortMode
	var portReads map[composition.Key][]PortRead
	var selectors []prototypeSelectorSurface
	var exportRoles []composition.Key
	for index, value := range values {
		if !value.Target.Available() || !value.Endpoint.Available() {
			return VariantPlan{}, false
		}
		template, ok := sealFormalVariantDescriptor(source, value.Template)
		if !ok {
			return VariantPlan{}, false
		}
		if index == 0 {
			roles = clonePortModes(template.formal.portModes)
			portReads = clonePortReads(template.formal.portReads)
			selectors = append([]prototypeSelectorSurface(nil), template.formal.selectors...)
			exportRoles = append([]composition.Key(nil), template.formal.exports...)
		} else if !samePortModes(roles, template.formal.portModes) || !samePortReads(portReads, template.formal.portReads) || !samePrototypeSelectorSurfaces(selectors, template.formal.selectors) || !sameKeySlice(exportRoles, template.formal.exports) {
			// One plan is one fixed ABI shape. Accepting endpoint-specific
			// port vocabularies would force every Application attachment to
			// retain the union, recreating the forbidden A×E storage plane.
			return VariantPlan{}, false
		}
		variants[index] = sealedVariant{target: value.Target, endpoint: value.Endpoint, template: template}
	}
	sort.Slice(variants, func(left, right int) bool {
		if variants[left].target != variants[right].target {
			return lessKey(variants[left].target, variants[right].target)
		}
		return lessKey(variants[left].endpoint, variants[right].endpoint)
	})
	for index := range variants {
		if index > 0 && variants[index-1].target == variants[index].target && variants[index-1].endpoint == variants[index].endpoint {
			return VariantPlan{}, false
		}
	}
	key, ok := identityKey("analysis/engine/equation/activation-variant-plan", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, family) || writer.Count(uint64(len(variants))) != nil {
			return false
		}
		for _, variant := range variants {
			if !writeKey(writer, variant.target) || !writeKey(writer, variant.endpoint) || !writeKey(writer, variant.template.key) {
				return false
			}
		}
		return true
	})
	if !ok {
		return VariantPlan{}, false
	}
	return VariantPlan{data: &variantPlanData{source: source, family: family, key: key, variants: variants, ports: roles, portReads: portReads, selectors: selectors, exports: exportRoles}}, true
}

// prototypeSelectorSurface is the fixed local staged-read ABI of one
// Template. Schema and read ordinal authenticate the sole Rule-owned selector
// implementation; Surface authenticates its target Factor/form/local. Operand
// and occurrence are deliberately absent because those vary by endpoint.
type prototypeSelectorSurface struct {
	schema  composition.Key
	read    uint64
	surface Surface
}

func samePrototypeSelectorSurfaces(left, right []prototypeSelectorSurface) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func clonePortModes(input map[composition.Key]PortMode) map[composition.Key]PortMode {
	result := make(map[composition.Key]PortMode, len(input))
	for role, mode := range input {
		result[role] = mode
	}
	return result
}

func clonePortReads(input map[composition.Key][]PortRead) map[composition.Key][]PortRead {
	result := make(map[composition.Key][]PortRead, len(input))
	for role, reads := range input {
		result[role] = append([]PortRead(nil), reads...)
	}
	return result
}

func samePortReads(left, right map[composition.Key][]PortRead) bool {
	if len(left) != len(right) {
		return false
	}
	for role, reads := range left {
		other, found := right[role]
		if !found || len(reads) != len(other) {
			return false
		}
		for index := range reads {
			if reads[index] != other[index] {
				return false
			}
		}
	}
	return true
}

func samePortModes(left, right map[composition.Key]PortMode) bool {
	if len(left) != len(right) {
		return false
	}
	for role, mode := range left {
		if right[role] != mode {
			return false
		}
	}
	return true
}

func sameKeySlice(left, right []composition.Key) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (plan VariantPlan) available(source *composition.Composition, family composition.Key) bool {
	return plan.data != nil && source != nil && plan.data.source == source && plan.data.family == family && plan.data.key.Available() && len(plan.data.variants) != 0
}

func (plan VariantPlan) variant(target, endpoint composition.Key) (sealedVariant, bool) {
	if plan.data == nil || !target.Available() || !endpoint.Available() {
		return sealedVariant{}, false
	}
	index := sort.Search(len(plan.data.variants), func(index int) bool {
		candidate := plan.data.variants[index]
		if candidate.target != target {
			return !lessKey(candidate.target, target)
		}
		return !lessKey(candidate.endpoint, endpoint)
	})
	if index >= len(plan.data.variants) {
		return sealedVariant{}, false
	}
	variant := plan.data.variants[index]
	return variant, variant.target == target && variant.endpoint == endpoint
}

// formalVariantDescriptor is the equation-issued endpoint receipt. Its
// FormalTemplate owns the one sealed formal Batch (including target rows and
// formal ports); this descriptor adds only canonical prototype projections and
// the fixed selector/port ABI used by typed payload attachment.
type formalVariantDescriptor struct {
	source *composition.Composition
	key    composition.Key
	formal FormalTemplate
}

// normalizeTemplateFormal reissues one authored template into a dedicated
// formal Batch. Roles become FormalPorts; ordinary local sites, occurrences,
// and operands are reissued in the same transaction. This is the cold
// boundary used later by TemplateBinding; no caller Batch capability is
// retained in the shared plan.
func normalizeTemplateFormal(value Template) (result Template, resultBatch *Batch, resultPorts map[composition.Key]FormalPort, ok bool) {
	stage := "start"
	_ = stage
	base := templateBatch(value)
	if base != nil && !base.Sealed() {
		return Template{}, nil, nil, false
	}
	if base == nil {
		if len(value.Points) != 0 || len(value.Rules) != 0 {
			return Template{}, nil, nil, false
		}
		for _, edge := range value.FactorEdges {
			if edge.ExternalSource.Available() || edge.ExternalTarget.Available() {
				return Template{}, nil, nil, false
			}
		}
	}
	formal := NewBatch()
	stage = "roles"
	ports := make(map[composition.Key]FormalPort, len(value.Roles))
	for _, role := range value.Roles {
		port, ok := formal.AdmitFormalPort(role.Role, role.Mode, role.Reads)
		if !ok {
			return Template{}, nil, nil, false
		}
		ports[role.Role] = port
	}
	baseSiteCount := 0
	if base != nil {
		baseSiteCount = len(base.sites)
	}
	sites := make([]Site, baseSiteCount)
	stage = "sites"
	if base != nil {
		for index, row := range base.sites {
			if row.formal {
				port, ok := ports[row.formalRole]
				if !ok {
					return Template{}, nil, nil, false
				}
				sites[index] = port.Site()
				continue
			}
			site, ok := formal.AdmitSite(row.source, row.scope, row.init, row.disposition)
			if !ok {
				return Template{}, nil, nil, false
			}
			sites[index] = site
		}
	}
	occurrenceCount := 0
	if base != nil {
		occurrenceCount = len(base.occurrences)
	}
	occurrences := make([]Occurrence, occurrenceCount)
	stage = "occurrences"
	if base != nil {
		for index, row := range base.occurrences {
			if row.site == 0 || uint64(row.site) > uint64(len(sites)) {
				return Template{}, nil, nil, false
			}
			var ok bool
			switch row.kind {
			case OccurrenceAt:
				occurrences[index], ok = formal.At(sites[row.site-1])
			case OccurrenceFrom:
				occurrences[index], ok = formal.From(sites[row.site-1], row.entity)
			case OccurrenceRelation:
				occurrences[index], ok = formal.Relation(sites[row.site-1], row.entity)
			}
			if !ok {
				return Template{}, nil, nil, false
			}
		}
	}
	operandCount := 0
	if base != nil {
		operandCount = len(base.operands)
	}
	operands := make([]Operand, operandCount)
	stage = "operands"
	if base != nil {
		for index, row := range base.operands {
			if row.occurrence == 0 || uint64(row.occurrence) > uint64(len(occurrences)) {
				return Template{}, nil, nil, false
			}
			operand, ok := formal.AdmitOperand(occurrences[row.occurrence-1], row.entity)
			if !ok {
				return Template{}, nil, nil, false
			}
			operands[index] = operand
		}
	}
	result = copyTemplate(value)
	for index, point := range result.Points {
		if point.Site.row == 0 || uint64(point.Site.row) > uint64(len(sites)) {
			return Template{}, nil, nil, false
		}
		result.Points[index].Site = sites[point.Site.row-1]
	}
	for index, rule := range result.Rules {
		if rule.Occurrence.row == 0 || uint64(rule.Occurrence.row) > uint64(len(occurrences)) || rule.Operand.row == 0 || uint64(rule.Operand.row) > uint64(len(operands)) {
			return Template{}, nil, nil, false
		}
		result.Rules[index].Occurrence = occurrences[rule.Occurrence.row-1]
		result.Rules[index].Operand = operands[rule.Operand.row-1]
	}
	for index, edge := range result.FactorEdges {
		if edge.ExternalSource.Available() {
			if edge.ExternalSource.row == 0 || uint64(edge.ExternalSource.row) > uint64(len(sites)) {
				return Template{}, nil, nil, false
			}
			result.FactorEdges[index].ExternalSource = sites[edge.ExternalSource.row-1]
		}
		if edge.ExternalTarget.Available() {
			if edge.ExternalTarget.row == 0 || uint64(edge.ExternalTarget.row) > uint64(len(sites)) {
				return Template{}, nil, nil, false
			}
			result.FactorEdges[index].ExternalTarget = sites[edge.ExternalTarget.row-1]
		}
	}
	// The formal owner admits the complete ordinary target grammar into this
	// same open Batch. No engine-side copy may construct a second row graph.
	pointRefs := make(map[Site]PointRef, len(result.Points)+len(ports))
	pointSites := make(map[PointRef]Site, len(result.Points)+len(ports))
	for _, point := range result.Points {
		ref, admitted := formal.AdmitPoint(point.Site)
		if !admitted {
			return Template{}, nil, nil, false
		}
		pointRefs[point.Site] = ref
		pointSites[ref] = point.Site
	}
	// Boundary point ordinals follow the declared role vector. The ports map is
	// a lookup index into that vector and carries no order authority.
	for _, role := range result.Roles {
		port, present := ports[role.Role]
		if !present {
			return Template{}, nil, nil, false
		}
		if _, issued := pointRefs[port.Site()]; issued {
			continue
		}
		ref, admitted := formal.AdmitPoint(port.Site())
		if !admitted {
			return Template{}, nil, nil, false
		}
		pointRefs[port.Site()] = ref
		pointSites[ref] = port.Site()
	}
	for _, row := range result.Rules {
		if !formal.AdmitRule(row) {
			return Template{}, nil, nil, false
		}
	}
	resolvePoint := func(point TargetPoint) (PointRef, bool) {
		if point.Role.Available() {
			port, present := ports[point.Role]
			if !present {
				return 0, false
			}
			ref, present := pointRefs[port.Site()]
			return ref, present && ref != 0
		}
		if point.Local == 0 || uint64(point.Local) > uint64(len(result.Points)) {
			return 0, false
		}
		ref, present := pointRefs[result.Points[point.Local-1].Site]
		return ref, present && ref != 0
	}
	for _, group := range result.Groups {
		output, outputOK := resolvePoint(group.Output)
		if !outputOK {
			return Template{}, nil, nil, false
		}
		bound := BatchGroup{Output: output, Members: append([]RuleRef(nil), group.Members...)}
		for _, input := range group.Inputs {
			source, sourceOK := resolvePoint(input.Point)
			if !sourceOK {
				return Template{}, nil, nil, false
			}
			sourceSite, sourcePresent := pointSites[source]
			targetSite, targetPresent := pointSites[output]
			if !sourcePresent || !targetPresent {
				return Template{}, nil, nil, false
			}
			bound.Inputs = append(bound.Inputs, TargetBoundaryInput(sourceSite, targetSite, input.Provenance, input.Pre, input.Reindex, input.Post))
		}
		if !formal.AdmitGroup(bound) {
			return Template{}, nil, nil, false
		}
	}
	for _, edge := range result.FactorEdges {
		target, targetOK := resolvePoint(edge.Target)
		var source Site
		var sourceOK bool
		if edge.ExternalSource.Available() {
			source, sourceOK = edge.ExternalSource, true
		} else {
			ref, ok := resolvePoint(edge.Source)
			if ok {
				source, sourceOK = pointSites[ref]
				if !sourceOK {
					return Template{}, nil, nil, false
				}
			}
		}
		if !targetOK || !sourceOK {
			return Template{}, nil, nil, false
		}
		targetSite, targetPresent := pointSites[target]
		if !targetPresent {
			return Template{}, nil, nil, false
		}
		input := TargetBoundaryInput(source, targetSite, edge.Provenance, edge.Pre, edge.Reindex, edge.Post)
		if !formal.AdmitFactorEdge(BatchFactorEdge{Target: target, Input: input, Factor: edge.Factor}) {
			return Template{}, nil, nil, false
		}
	}
	for _, summary := range result.Summaries {
		if !formal.AdmitSummary(summary) {
			return Template{}, nil, nil, false
		}
	}
	for _, weak := range result.WeakTargets {
		if !formal.AdmitWeakTarget(weak) {
			return Template{}, nil, nil, false
		}
	}
	if !formal.Seal() {
		return Template{}, nil, nil, false
	}
	return result, formal, ports, true
}

func sealFormalVariantDescriptor(source *composition.Composition, value Template) (formalVariantDescriptor, bool) {
	if source == nil || len(value.Rules) != len(value.Groups) || len(value.Rules) == 0 && len(value.FactorEdges) == 0 {
		return formalVariantDescriptor{}, false
	}
	normalized, batch, formals, formalOK := normalizeTemplateFormal(value)
	if !formalOK || batch == nil || !batch.Sealed() {
		return formalVariantDescriptor{}, false
	}
	formal := formalTemplateFromParts(batch, formals, len(normalized.Roles))
	if !formal.Available() {
		return formalVariantDescriptor{}, false
	}
	result := formalVariantDescriptor{source: source, formal: formal}
	for _, row := range normalized.Rules {
		schema, schemaOK := ruleSchema(source, row.Schema)
		if !schemaOK || len(schema.Activations) != 0 {
			return formalVariantDescriptor{}, false
		}
	}
	catalog, ok := buildTopologyCatalog(TopologySpec{Rules: normalized.Rules, Summaries: normalized.Summaries, WeakTargets: normalized.WeakTargets})
	if !ok || !validateTopologyCatalogUsage(TopologySpec{Rules: normalized.Rules, Summaries: normalized.Summaries, WeakTargets: normalized.WeakTargets}, catalog) {
		return formalVariantDescriptor{}, false
	}
	var instances []canonicalInstance
	if len(normalized.Rules) != 0 {
		instances, ok = buildInstances(source, batch, normalized.Rules, catalog)
		if !ok {
			return formalVariantDescriptor{}, false
		}
	}
	selectors, ok := prototypeSelectorSurfaces(instances)
	if !ok {
		return formalVariantDescriptor{}, false
	}
	points, _, _, ok := buildPoints(normalized.Points)
	if !ok {
		return formalVariantDescriptor{}, false
	}
	ports, portReads, exports, ok := prototypePortRoles(normalized.Roles, normalized.Groups, normalized.FactorEdges, points, instances, source, batch)
	if !ok {
		return formalVariantDescriptor{}, false
	}
	key, ok := prototypeTemplateKey(normalized, instances, points, ports, portReads)
	if !ok {
		return formalVariantDescriptor{}, false
	}
	result.key = key
	result.formal = result.formal.withProjections(instances, points, ports, portReads, selectors, exports, len(normalized.FactorEdges))
	return result, true
}

func prototypeSelectorSurfaces(instances []canonicalInstance) ([]prototypeSelectorSurface, bool) {
	result := make([]prototypeSelectorSurface, 0)
	for _, instance := range instances {
		selectorLocal := uint64(0)
		for index, read := range instance.row.Reads {
			if read.Surface.Form != SurfaceReadSelect {
				continue
			}
			selectorLocal++
			if read.Index != uint64(index) || read.Surface.Local != selectorLocal || read.Surface.Mode != TargetModeNone ||
				!read.Surface.Semantic.Available() || read.Surface.Semantic != read.Surface.Factor || read.Surface.Normalizer.Available() {
				return nil, false
			}
			result = append(result, prototypeSelectorSurface{schema: instance.row.Schema, read: uint64(index), surface: read.Surface})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].schema != result[right].schema {
			return lessKey(result[left].schema, result[right].schema)
		}
		if result[left].read != result[right].read {
			return result[left].read < result[right].read
		}
		return lessSurface(result[left].surface, result[right].surface)
	})
	return result, true
}

func templateBatch(value Template) *Batch {
	var batch *Batch
	accept := func(candidate *Batch) bool {
		if candidate == nil || !candidate.Sealed() {
			return false
		}
		if batch == nil {
			batch = candidate
		}
		return batch == candidate
	}
	for _, point := range value.Points {
		if !point.Site.Available() || !accept(point.Site.batch) {
			return nil
		}
	}
	for _, rule := range value.Rules {
		if !rule.Occurrence.Available() || !rule.Operand.Available() || !rule.Operand.Occurrence().Same(rule.Occurrence) || !accept(rule.Occurrence.batch) || !accept(rule.Operand.batch) {
			return nil
		}
	}
	for _, edge := range value.FactorEdges {
		if edge.ExternalSource.Available() && !accept(edge.ExternalSource.batch) {
			return nil
		}
		if edge.ExternalTarget.Available() && !accept(edge.ExternalTarget.batch) {
			return nil
		}
	}
	return batch
}

func prototypePortRoles(values []TemplateRole, groups []TargetGroup, edges []TargetFactorEdge, points map[PointRef]Point, instances []canonicalInstance, source *composition.Composition, batch *Batch) (map[composition.Key]PortMode, map[composition.Key][]PortRead, []composition.Key, bool) {
	structuralOnly := len(groups) == 0 && len(instances) == 0
	if source == nil || len(groups) != len(instances) || structuralOnly && len(points) != 0 {
		return nil, nil, nil, false
	}
	ports := make(map[composition.Key]PortMode, len(values))
	portReads := make(map[composition.Key][]PortRead)
	declaredReads := make(map[Surface]struct{})
	boundReadSurfaces := make(map[Surface]struct{})
	for _, instance := range instances {
		schema, found := ruleSchema(source, instance.row.Schema)
		if !found || len(schema.Reads) != len(instance.row.Reads) {
			return nil, nil, nil, false
		}
		for index, read := range instance.row.Reads {
			if !matchesReadSurface(read.Surface, schema.Reads[index]) {
				return nil, nil, nil, false
			}
			declaredReads[read.Surface] = struct{}{}
		}
	}
	for _, value := range values {
		// Variant templates carry a named role only. A concrete Base here
		// would make the allegedly shared plan application-specific.
		if !value.Role.Available() || !value.Mode.imports() && !value.Mode.exports() {
			return nil, nil, nil, false
		}
		if _, duplicate := ports[value.Role]; duplicate {
			return nil, nil, nil, false
		}
		reads, validReads := canonicalPortReads(value.Reads)
		if !validReads || len(reads) != 0 && !value.Mode.imports() {
			return nil, nil, nil, false
		}
		for _, read := range reads {
			if !validPortRead(source, read, declaredReads) {
				return nil, nil, nil, false
			}
			if _, duplicate := boundReadSurfaces[read.Surface]; duplicate {
				return nil, nil, nil, false
			}
			boundReadSurfaces[read.Surface] = struct{}{}
		}
		if len(reads) != 0 {
			portReads[value.Role] = reads
		}
		ports[value.Role] = value.Mode
	}
	usedRules := make([]bool, len(instances))
	localOutputs := make(map[PointRef][]int, len(points))
	localInputs := make([][]PointRef, len(groups))
	uses := make(map[composition.Key]uint8, len(ports))
	const (
		portUseImport = 1 << iota
		portUseExport
	)
	for groupIndex, group := range groups {
		if len(group.Members) == 0 {
			return nil, nil, nil, false
		}
		outputScope, outputScopeOK := templatePointScope(group.Output, points)
		if !outputScopeOK {
			return nil, nil, nil, false
		}
		if group.Output.Local != 0 {
			if group.Output.Role.Available() {
				return nil, nil, nil, false
			}
			if _, found := points[group.Output.Local]; !found {
				return nil, nil, nil, false
			}
			localOutputs[group.Output.Local] = append(localOutputs[group.Output.Local], groupIndex)
		} else {
			mode, found := ports[group.Output.Role]
			if !found || !mode.exports() {
				return nil, nil, nil, false
			}
			uses[group.Output.Role] |= portUseExport
		}
		for _, ref := range group.Members {
			index, ok := ruleRefIndex(ref, len(instances))
			if !ok || usedRules[index] {
				return nil, nil, nil, false
			}
			schema, found := ruleSchema(source, instances[index].row.Schema)
			if !found || schema.Inputs != uint64(len(group.Inputs)) {
				return nil, nil, nil, false
			}
			usedRules[index] = true
		}
		for _, input := range group.Inputs {
			if !input.Provenance.Available() || !input.Pre.Available() || !input.Reindex.Available() || !input.Post.Available() {
				return nil, nil, nil, false
			}
			inputScope, inputScopeOK := templatePointScope(input.Point, points)
			if !inputScopeOK || !validPrototypeTemplateInput(input, inputScope, outputScope) {
				return nil, nil, nil, false
			}
			if input.Point.Local != 0 {
				if input.Point.Role.Available() {
					return nil, nil, nil, false
				}
				if _, found := points[input.Point.Local]; !found {
					return nil, nil, nil, false
				}
				localInputs[groupIndex] = append(localInputs[groupIndex], input.Point.Local)
			} else {
				mode, found := ports[input.Point.Role]
				if !found || !mode.imports() {
					return nil, nil, nil, false
				}
				uses[input.Point.Role] |= portUseImport
			}
		}
	}
	// Factor-local structural edges use the same named port ABI as ordinary
	// fragment inputs.  A source role is an import and a target role is an
	// export; the endpoint may also be a local point.  Recording these uses in
	// the one port liveness table prevents an edge from smuggling an otherwise
	// unbound port into the sealed variant.
	for _, edge := range edges {
		if !validTargetFactorEdge(edge, points, source, batch) {
			return nil, nil, nil, false
		}
		if edge.ExternalSource.Available() {
			// The external source is already bound to the owning Assembly;
			// it does not consume a shared-plan port role.
		} else if edge.Source.Local == 0 {
			if _, found := ports[edge.Source.Role]; !found {
				return nil, nil, nil, false
			}
			uses[edge.Source.Role] |= portUseImport
		}
		if edge.ExternalTarget.Available() {
			// The external target is already bound to the owning Assembly;
			// it does not consume a shared-plan port role.
		} else if edge.Target.Local == 0 {
			if _, found := ports[edge.Target.Role]; !found {
				return nil, nil, nil, false
			}
			uses[edge.Target.Role] |= portUseExport
		}
	}
	if !structuralOnly {
		for _, used := range usedRules {
			if !used {
				return nil, nil, nil, false
			}
		}
	}
	if !validatePortReadRoutes(portReads, groups, instances, source) {
		return nil, nil, nil, false
	}
	if !structuralOnly && !validateTemplateLiveness(groups, points, localOutputs, localInputs) {
		return nil, nil, nil, false
	}
	exports := make([]composition.Key, 0, len(ports))
	for role, mode := range ports {
		use := uses[role]
		switch mode {
		case PortImport:
			if use != portUseImport {
				return nil, nil, nil, false
			}
		case PortExport:
			if use != portUseExport {
				return nil, nil, nil, false
			}
		case PortImportExport:
			if use != portUseImport|portUseExport {
				return nil, nil, nil, false
			}
		default:
			return nil, nil, nil, false
		}
		if use&portUseExport != 0 {
			exports = append(exports, role)
		}
	}
	sort.Slice(exports, func(left, right int) bool { return lessKey(exports[left], exports[right]) })
	return ports, portReads, exports, true
}

// validatePortReadRoutes proves a named prototype read is not merely present
// somewhere in a template.  Its Rule schema input must be the exact
// TemplateInput connected to that import role; every declared slot must be
// used at least once.  This closes the otherwise subtle input-crossing leak.
func validatePortReadRoutes(portReads map[composition.Key][]PortRead, groups []TargetGroup, instances []canonicalInstance, source *composition.Composition) bool {
	if len(portReads) == 0 {
		return true
	}
	roleForSurface := make(map[Surface]composition.Key)
	seen := make(map[Surface]bool)
	for role, reads := range portReads {
		for _, read := range reads {
			if _, duplicate := roleForSurface[read.Surface]; duplicate || !role.Available() {
				return false
			}
			roleForSurface[read.Surface] = role
		}
	}
	for _, group := range groups {
		for _, member := range group.Members {
			index, ok := ruleRefIndex(member, len(instances))
			if !ok {
				return false
			}
			instance := instances[index].row
			schema, schemaOK := ruleSchema(source, instance.Schema)
			if !schemaOK || len(schema.Reads) != len(instance.Reads) {
				return false
			}
			for readIndex, read := range instance.Reads {
				role, slotted := roleForSurface[read.Surface]
				if !slotted {
					continue
				}
				input := schema.Reads[readIndex].Input
				if input >= uint64(len(group.Inputs)) {
					return false
				}
				point := group.Inputs[input].Point
				if point.Local != 0 || point.Role != role {
					return false
				}
				seen[read.Surface] = true
			}
		}
	}
	for surface := range roleForSurface {
		if !seen[surface] {
			return false
		}
	}
	return true
}

// templatePointScope is the one formal endpoint typing rule. A Local
// point exposes its declared template scope; a TemplateRole is polymorphic over the
// eventual attachment scope and is therefore typed at EmptyScope here.  The
// attachment later lifts this empty formal boundary by ambient identity.
func templatePointScope(value TargetPoint, points map[PointRef]Point) (Scope, bool) {
	if (value.Local != 0 && value.Role.Available()) || (value.Local == 0 && !value.Role.Available()) {
		return Scope{}, false
	}
	if value.Local != 0 {
		point, found := points[value.Local]
		return point.Scope(), found && point.Available()
	}
	return EmptyScope(), value.Role.Available()
}

func validPrototypeTemplateInput(input TemplateInput, source, target Scope) bool {
	return input.Provenance.Available() && input.Pre.Available() && input.Reindex.Available() && input.Post.Available() &&
		validScopedExpr(input.Pre, source) && sameScope(input.Reindex.source, source) && sameScope(input.Reindex.target, target) && validScopedExpr(input.Post, target)
}

func validTargetFactorEdge(edge TargetFactorEdge, points map[PointRef]Point, source *composition.Composition, batch *Batch) bool {
	if source == nil || !edge.Factor.Available() || !edge.Provenance.Available() || !edge.Pre.Available() || !edge.Reindex.Available() || !edge.Post.Available() {
		return false
	}
	if _, known := source.FactorIndex(edge.Factor); !known {
		return false
	}
	if batch == nil || !batch.Sealed() || edge.ExternalSource.Available() && edge.ExternalSource.batch != batch || edge.ExternalTarget.Available() && edge.ExternalTarget.batch != batch {
		return false
	}
	if edge.ExternalSource.Available() && (edge.Source.Local != 0 || edge.Source.Role.Available()) {
		return false
	}
	if !edge.ExternalSource.Available() && edge.Source.Local == 0 && !edge.Source.Role.Available() {
		return false
	}
	if edge.ExternalTarget.Available() && (edge.Target.Local != 0 || edge.Target.Role.Available()) {
		return false
	}
	if !edge.ExternalTarget.Available() && edge.Target.Local == 0 && !edge.Target.Role.Available() {
		return false
	}
	sourceScope, sourceOK := EmptyScope(), true
	if edge.ExternalSource.Available() {
		sourceScope = edge.ExternalSource.Scope()
	} else {
		sourceScope, sourceOK = templatePointScope(edge.Source, points)
	}
	targetScope, targetOK := EmptyScope(), true
	if edge.ExternalTarget.Available() {
		targetScope = edge.ExternalTarget.Scope()
	} else {
		targetScope, targetOK = templatePointScope(edge.Target, points)
	}
	return sourceOK && targetOK && validScopedExpr(edge.Pre, sourceScope) &&
		sameScope(edge.Reindex.source, sourceScope) && sameScope(edge.Reindex.target, targetScope) && validScopedExpr(edge.Post, targetScope)
}

func prototypeTemplateKey(value Template, instances []canonicalInstance, points map[PointRef]Point, ports map[composition.Key]PortMode, portReads map[composition.Key][]PortRead) (composition.Key, bool) {
	groups, groupsOK := prototypeGroupKeys(value.Groups, instances, points)
	if !groupsOK {
		return composition.Key{}, false
	}
	edges, edgesOK := prototypeFactorEdgeKeys(value.FactorEdges, points)
	if !edgesOK {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/activation-template-prototype", func(writer *canonical.DigestWriter) bool {
		if writer.Count(uint64(len(instances))) != nil {
			return false
		}
		instanceKeys := make([]composition.Key, len(instances))
		for index, instance := range instances {
			instanceKeys[index] = instance.key
		}
		sort.Slice(instanceKeys, func(left, right int) bool { return lessKey(instanceKeys[left], instanceKeys[right]) })
		for _, key := range instanceKeys {
			if !writeKey(writer, key) {
				return false
			}
		}
		pointKeys := make([]composition.Key, 0, len(points))
		for _, point := range points {
			pointKeys = append(pointKeys, point.key)
		}
		sort.Slice(pointKeys, func(left, right int) bool { return lessKey(pointKeys[left], pointKeys[right]) })
		if writer.Count(uint64(len(pointKeys))) != nil {
			return false
		}
		for _, key := range pointKeys {
			if !writeKey(writer, key) {
				return false
			}
		}
		roles := make([]composition.Key, 0, len(ports))
		for role := range ports {
			roles = append(roles, role)
		}
		sort.Slice(roles, func(left, right int) bool { return lessKey(roles[left], roles[right]) })
		if writer.Count(uint64(len(roles))) != nil {
			return false
		}
		for _, role := range roles {
			if !writeKey(writer, role) || writer.Uint(uint64(ports[role])) != nil {
				return false
			}
			if !writePortReads(writer, portReads[role]) {
				return false
			}
		}
		if writer.Count(uint64(len(groups))) != nil {
			return false
		}
		for _, group := range groups {
			if !writeKey(writer, group) {
				return false
			}
		}
		if writer.Count(uint64(len(edges))) != nil {
			return false
		}
		for _, edge := range edges {
			if !writeKey(writer, edge) {
				return false
			}
		}
		return true
	})
}

func prototypeFactorEdgeKeys(values []TargetFactorEdge, points map[PointRef]Point) ([]composition.Key, bool) {
	result := make([]composition.Key, len(values))
	for index, edge := range values {
		source, sourceOK := composition.Key{}, false
		if edge.ExternalSource.Available() {
			source = edge.ExternalSource.Key()
			sourceOK = source.Available()
		} else {
			source, sourceOK = prototypePointKey(edge.Source, points)
		}
		target, targetOK := composition.Key{}, false
		if edge.ExternalTarget.Available() {
			target = edge.ExternalTarget.Key()
			targetOK = target.Available()
		} else {
			target, targetOK = prototypePointKey(edge.Target, points)
		}
		if !sourceOK || !targetOK || !edge.Factor.Available() || !edge.Provenance.Available() || !edge.Pre.Available() || !edge.Reindex.Available() || !edge.Post.Available() {
			return nil, false
		}
		key, ok := identityKey("analysis/engine/equation/activation-template-factor-edge", func(writer *canonical.DigestWriter) bool {
			return writeKey(writer, source) && writeKey(writer, target) && writeKey(writer, edge.Factor) && writeKey(writer, edge.Provenance) &&
				writeExpr(writer, edge.Pre) && writeReindex(writer, edge.Reindex) && writeExpr(writer, edge.Post)
		})
		if !ok {
			return nil, false
		}
		result[index] = key
	}
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left], result[right]) })
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, false
		}
	}
	return result, true
}

func prototypeGroupKeys(groups []TargetGroup, instances []canonicalInstance, points map[PointRef]Point) ([]composition.Key, bool) {
	result := make([]composition.Key, len(groups))
	for groupIndex, group := range groups {
		output, outputOK := prototypePointKey(group.Output, points)
		if !outputOK || len(group.Members) == 0 {
			return nil, false
		}
		members := make([]composition.Key, len(group.Members))
		for index, ref := range group.Members {
			memberIndex, memberOK := ruleRefIndex(ref, len(instances))
			if !memberOK {
				return nil, false
			}
			members[index] = instances[memberIndex].key
		}
		sort.Slice(members, func(left, right int) bool { return lessKey(members[left], members[right]) })
		key, ok := identityKey("analysis/engine/equation/activation-template-prototype-group", func(writer *canonical.DigestWriter) bool {
			if !writeKey(writer, output) || writer.Count(uint64(len(members))) != nil {
				return false
			}
			for _, member := range members {
				if !writeKey(writer, member) {
					return false
				}
			}
			if writer.Count(uint64(len(group.Inputs))) != nil {
				return false
			}
			for _, input := range group.Inputs {
				point, pointOK := prototypePointKey(input.Point, points)
				if !pointOK || !writeKey(writer, point) || !writeKey(writer, input.Provenance) || !writeExpr(writer, input.Pre) || !writeReindex(writer, input.Reindex) || !writeExpr(writer, input.Post) {
					return false
				}
			}
			return true
		})
		if !ok {
			return nil, false
		}
		result[groupIndex] = key
	}
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left], result[right]) })
	for index := 1; index < len(result); index++ {
		if result[index-1] == result[index] {
			return nil, false
		}
	}
	return result, true
}

func prototypePointKey(point TargetPoint, points map[PointRef]Point) (composition.Key, bool) {
	if point.Local != 0 {
		local, found := points[point.Local]
		return local.key, found && !point.Role.Available() && local.Available()
	}
	return point.Role, point.Role.Available()
}

// compatiblePortRead validates the only dynamic data capability crossing an
// activation attachment.  The prototype fixes Factor and exact-read form;
// the caller supplies only a different issued dense Ref (the Local field).
func validPortRead(source *composition.Composition, read PortRead, declared map[Surface]struct{}) bool {
	if source == nil || !read.Role.Available() || !read.Surface.Available() || read.Surface.Form != SurfaceReadExact || read.Surface.Mode != TargetModeNone ||
		read.Surface.Semantic.Available() || read.Surface.Normalizer.Available() {
		return false
	}
	if _, found := source.FactorIndex(read.Surface.Factor); !found {
		return false
	}
	_, found := declared[read.Surface]
	return found
}

func compatiblePortReads(source *composition.Composition, prototype, caller []PortRead) bool {
	if source == nil || len(prototype) != len(caller) {
		return false
	}
	for index := range prototype {
		if prototype[index].Role != caller[index].Role || !compatiblePortRead(source, prototype[index].Surface, caller[index].Surface) {
			return false
		}
	}
	return true
}

func compatiblePortRead(source *composition.Composition, prototype, caller Surface) bool {
	if source == nil || !prototype.Available() || !caller.Available() || prototype.Form != SurfaceReadExact || caller.Form != SurfaceReadExact ||
		prototype.Mode != TargetModeNone || caller.Mode != TargetModeNone || prototype.Semantic.Available() || caller.Semantic.Available() ||
		prototype.Normalizer.Available() || caller.Normalizer.Available() || prototype.Factor != caller.Factor {
		return false
	}
	_, found := source.FactorIndex(prototype.Factor)
	return found
}

// validateTemplateLiveness reverse-walks the template's ordinary local
// Point/Group graph from export-port outputs. A local Point is live only when
// it lies on a path to an exported base point; a local Init input is a valid
// leaf of that walk and does not need a synthetic producer. Every template
// Rule is already owned by exactly one Group, so live Groups imply live Rules.
func validateTemplateLiveness(groups []TargetGroup, points map[PointRef]Point, producers map[PointRef][]int, inputs [][]PointRef) bool {
	if len(groups) == 0 || len(inputs) != len(groups) {
		return false
	}
	reachableGroups := make([]bool, len(groups))
	reachablePoints := make(map[PointRef]struct{}, len(points))
	work := make([]int, 0, len(groups))
	for groupIndex, group := range groups {
		if group.Output.Local == 0 {
			reachableGroups[groupIndex] = true
			work = append(work, groupIndex)
		}
	}
	for len(work) != 0 {
		groupIndex := work[len(work)-1]
		work = work[:len(work)-1]
		group := groups[groupIndex]
		if group.Output.Local != 0 {
			reachablePoints[group.Output.Local] = struct{}{}
		}
		for _, point := range inputs[groupIndex] {
			reachablePoints[point] = struct{}{}
			for _, producer := range producers[point] {
				if producer < 0 || producer >= len(groups) {
					return false
				}
				if !reachableGroups[producer] {
					reachableGroups[producer] = true
					work = append(work, producer)
				}
			}
		}
	}
	for _, reachable := range reachableGroups {
		if !reachable {
			return false
		}
	}
	for point := range points {
		if _, reachable := reachablePoints[point]; !reachable {
			return false
		}
	}
	return true
}

func copyTemplate(value Template) Template {
	result := Template{
		Rules:       make([]RuleInstance, len(value.Rules)),
		Points:      append([]PointSpec(nil), value.Points...),
		Roles:       make([]TemplateRole, len(value.Roles)),
		Groups:      make([]TargetGroup, len(value.Groups)),
		FactorEdges: make([]TargetFactorEdge, len(value.FactorEdges)),
		Summaries:   make([]SummaryMapping, len(value.Summaries)),
		WeakTargets: make([]WeakTargetMapping, len(value.WeakTargets)),
	}
	for index, row := range value.Rules {
		result.Rules[index] = copyInstance(row)
	}
	for index, port := range value.Roles {
		result.Roles[index] = TemplateRole{Role: port.Role, Mode: port.Mode, Reads: append([]PortRead(nil), port.Reads...)}
	}
	for index, group := range value.Groups {
		copied := TargetGroup{Members: append([]RuleRef(nil), group.Members...), Output: group.Output, Inputs: append([]TemplateInput(nil), group.Inputs...)}
		result.Groups[index] = copied
	}
	for index, edge := range value.FactorEdges {
		result.FactorEdges[index] = TargetFactorEdge{Source: edge.Source, ExternalSource: edge.ExternalSource, Target: edge.Target, ExternalTarget: edge.ExternalTarget, Factor: edge.Factor, Provenance: edge.Provenance, Pre: edge.Pre, Reindex: edge.Reindex, Post: edge.Post}
	}
	for index, summary := range value.Summaries {
		result.Summaries[index] = SummaryMapping{Surface: summary.Surface, Keys: append([]uint64(nil), summary.Keys...)}
	}
	for index, weak := range value.WeakTargets {
		result.WeakTargets[index] = WeakTargetMapping{Surface: weak.Surface, Candidates: append([]Surface(nil), weak.Candidates...)}
	}
	return result
}

// decisionAlpha is the invocation-local renaming of raw template decisions.
// Its one key per (raw decision, binding, Member) is deliberately independent
// of a local Point row: branches in different local points of one invocation
// remain correlated, while branches from distinct Members cannot alias.
type decisionAlpha map[composition.Key]Decision

func (alpha decisionAlpha) bind(decision Decision, local bool) (Decision, bool) {
	if !decision.Available() {
		return Decision{}, false
	}
	if !local {
		return decision, true
	}
	bound, found := alpha[decision.key]
	return bound, found && bound.Available()
}

// boundScope is exactly the canonical decision universe S union alpha(L).
// Site and Point retain source-row/member identity; Scope must not duplicate
// that identity or equal formal local universes cease to compose exactly.
func boundScope(scope, ambient Scope, alpha decisionAlpha) (Scope, bool) {
	if !scope.Available() || !ambient.Available() || alpha == nil {
		return Scope{}, false
	}
	decisions := make([]Decision, 0, len(ambient.row.decisions)+len(scope.row.decisions))
	decisions = append(decisions, ambient.row.decisions...)
	for _, decision := range scope.row.decisions {
		bound, ok := alpha.bind(decision, true)
		if !ok {
			return Scope{}, false
		}
		decisions = append(decisions, bound)
	}
	// NewScope sorts canonically and rejects alpha/ambient collisions.
	return NewScope(decisions...)
}

// boundExpr rebuilds a target-side substitution expression through alpha.
// A decision hash can change its global ROBDD order, so copying nodes would
// retain an invalid ordered DAG; rebuilding ITEs is the canonical operation.
func boundExpr(value Expr, alpha decisionAlpha) (Expr, bool) {
	if !value.Available() {
		return Expr{}, false
	}
	builder := newExprBuilder()
	seen := map[uint32]uint32{0: 0, 1: 1}
	var rewrite func(uint32) (uint32, bool)
	rewrite = func(index uint32) (uint32, bool) {
		if result, found := seen[index]; found {
			return result, true
		}
		if index < 2 || index > uint32(len(value.nodes)+1) {
			return 0, false
		}
		node := value.nodes[index-2]
		decision, ok := alpha.bind(node.decision, true)
		if !ok {
			return 0, false
		}
		low, ok := rewrite(node.low)
		if !ok {
			return 0, false
		}
		high, ok := rewrite(node.high)
		if !ok {
			return 0, false
		}
		test, ok := builder.node(decision, 0, 1)
		if !ok {
			return 0, false
		}
		result, ok := builder.ite(test, high, low)
		if !ok {
			return 0, false
		}
		seen[index] = result
		return result, true
	}
	root, ok := rewrite(value.root)
	if !ok {
		return Expr{}, false
	}
	return builder.freeze(root)
}

// boundReindex rewrites one simultaneous relation at its two endpoints. A
// local endpoint receives the Member alpha; a static TemplateRole endpoint retains
// its base decisions. Substitution expressions belong to the target scope.
func boundReindex(value Reindex, source, target templateResolvedPoint, ambient Scope, alpha decisionAlpha) (Reindex, bool) {
	if !value.Available() || !source.available() || !target.available() || !ambient.Available() || !sameScope(value.source, source.rawScope) || !sameScope(value.target, target.rawScope) {
		return Reindex{}, false
	}
	bySource := make(map[composition.Key]DecisionMap, len(ambient.row.decisions)+len(value.maps))
	ambientSources := make(map[composition.Key]struct{}, len(ambient.row.decisions))
	for _, decision := range ambient.row.decisions {
		// A static external source may be context-free while the selected
		// caller target carries the attachment ambient scope. Such ambient
		// decisions are newly introduced at the target and therefore require
		// no source map. Retain the identity fast path when both endpoints
		// contain the decision. A source-only decision introduced by a TemplateRole's
		// ambient lift is forgotten exactly once; a source-only decision already
		// present in the raw source retains its authored formal mapping below.
		sourceHas, targetHas := source.scope.contains(decision), target.scope.contains(decision)
		if sourceHas && !targetHas {
			if !source.rawScope.contains(decision) {
				bySource[decision.Key()] = Forget(decision)
				ambientSources[decision.Key()] = struct{}{}
			}
			continue
		}
		if sourceHas && targetHas {
			bySource[decision.Key()] = Identity(decision)
			ambientSources[decision.Key()] = struct{}{}
		}
	}
	for _, mapping := range value.maps {
		boundSource, ok := alpha.bind(mapping.Source, source.local)
		if !ok {
			return Reindex{}, false
		}
		if _, ambientSource := ambientSources[boundSource.Key()]; ambientSource {
			continue
		}
		var bound DecisionMap
		switch mapping.Disposition {
		case DecisionIdentity, DecisionRename:
			boundTarget, ok := alpha.bind(mapping.Target, target.local)
			if !ok {
				return Reindex{}, false
			}
			if boundSource == boundTarget {
				bound = Identity(boundSource)
			} else {
				bound = Rename(boundSource, boundTarget)
			}
		case DecisionForget:
			bound = Forget(boundSource)
		case DecisionSubstitute:
			expr := mapping.Expr
			if target.local {
				expr, ok = boundExpr(expr, alpha)
				if !ok {
					return Reindex{}, false
				}
			}
			bound = Substitute(boundSource, expr)
		default:
			return Reindex{}, false
		}
		bySource[boundSource.Key()] = bound
	}
	maps := make([]DecisionMap, 0, source.scope.Count())
	for index := 0; index < source.scope.Count(); index++ {
		decision, ok := source.scope.At(index)
		if !ok {
			return Reindex{}, false
		}
		mapping, found := bySource[decision.Key()]
		if !found {
			return Reindex{}, false
		}
		maps = append(maps, mapping)
	}
	return NewReindex(source.scope, target.scope, maps)
}

type templateResolvedPoint struct {
	ref      PointRef
	site     Site
	scope    Scope
	rawScope Scope
	local    bool
	open     bool
}

func (point templateResolvedPoint) available() bool {
	return point.ref != 0 && (point.site.Available() || point.open && point.site.batch != nil && point.site.dynamic == nil) && point.scope.Available() && point.rawScope.Available()
}

func ruleRefIndex(ref RuleRef, count int) (int, bool) {
	value := uint64(ref)
	if value == 0 || value > uint64(count) {
		return 0, false
	}
	return int(value - 1), true
}
