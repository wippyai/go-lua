package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// PortMode is the static direction of one fragment/base-point connection.
// It is structural metadata, not an activation predicate or runtime capability.
type PortMode uint8

const (
	PortInvalid PortMode = iota
	PortImport
	PortExport
	PortImportExport
)

func (mode PortMode) imports() bool { return mode == PortImport || mode == PortImportExport }
func (mode PortMode) exports() bool { return mode == PortExport || mode == PortImportExport }

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

// Port maps one canonical fragment role to one presealed base Point. Axes
// membership belongs exclusively to ActivationBinding; a binding with a
// different base mapping is a different fixed-shape binding, never a branch
// inside this port table.
type Port struct {
	Role composition.Key
	Mode PortMode
	// Reads are the finite named prototype exact-read slots supplied by this
	// import role.  They are canonicalized by PortRead.Role, never declaration
	// position, and remain optional for control-only ports.
	Reads []PortRead
}

// PortRead is one named import-surface slot. Role is an ABI semantic key, not
// a Factor coordinate or an ordinal. Surface is prototype-side in Port and
// caller-side in PortBinding.
type PortRead struct {
	Role    composition.Key
	Surface Surface
}

// PortBinding attaches one shared-plan role to a trigger-owned base Point.
// Mode belongs to the immutable Template role, never to the binding.
type PortBinding struct {
	Role composition.Key
	Base PointRef
	// Reads binds the exact caller Factor surfaces for the matching Port slots.
	// It cannot introduce a new Factor, form, or slot.
	Reads []PortRead
}

// FragmentPoint is one symbolic point reference in a Template. Exactly one
// coordinate is present: Local names a pair-owned PointSpec, while Port names
// a static Port role. The raw PointRef/Role are builder input only; seal
// resolves them to issued point identities before any graph is compiled.
type FragmentPoint struct {
	Local PointRef
	Port  composition.Key
}

// FragmentInput is an ordinary complete boundary input before its symbolic
// endpoints are resolved.  Pre/post conditions and provenance remain part of
// the template so member expansion cannot silently drop them.
type FragmentInput struct {
	Point      FragmentPoint
	Provenance composition.Key
	Pre        Expr
	Reindex    Reindex
	Post       Expr
}

// FragmentGroup is a finite ordinary Group shape. Expansion resolves its
// Local/Port point references and emits the normal compiled input form used
// by the sole equation compiler.
type FragmentGroup struct {
	Members []RuleRef
	Output  FragmentPoint
	Inputs  []FragmentInput
}

// FragmentFactorEdge is one structural Factor-local transport in an
// activation fragment.  It is deliberately the same boundary shape as an
// ordinary Input, with symbolic endpoints resolved only while an accepted
// Member is expanded.  The edge has no Rule/Group/Member authority of its
// own; after expansion it is appended to TopologySpec.FactorEdges and follows
// the ordinary graph, demand, WTO, and runtime paths.
type FragmentFactorEdge struct {
	// Source is the member-local or named-port endpoint. ExternalSource is
	// an already-issued source Point supplied by the owning SourceAssembly;
	// exactly one of them is present. ExternalSource is not alpha-renamed or
	// copied during Member expansion.
	Source         FragmentPoint
	ExternalSource Site
	Target         FragmentPoint
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
	Ports       []Port
	Groups      []FragmentGroup
	FactorEdges []FragmentFactorEdge
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
func (row PrototypeRow) MatchesPortReads(expected []Port) bool {
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
	if !row.Schema.Available() || !row.OperandFamily.Available() || !row.Occurrence.Available() || !row.Operand.Available() || len(row.Supports) != 0 || len(row.Prunes) != 0 {
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
		if write.Index != uint64(index) || write.Surface.Form != SurfaceWriteExact || write.Surface.Mode != TargetModeStrong || write.Surface.Semantic.Available() || write.Surface.Normalizer.Available() ||
			write.Route != 0 || len(write.Candidates) != 0 || len(write.TargetCandidates) != 0 || len(write.Relations) != 0 {
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
	rows := make([]PrototypeRow, len(variant.template.instances))
	for index, instance := range variant.template.instances {
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
		if len(variant.template.instances) != 0 || len(variant.template.value.FactorEdges) == 0 {
			return false
		}
	}
	return true
}

type sealedVariant struct {
	target   composition.Key
	endpoint composition.Key
	template templatePrototype
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
		template, ok := sealTemplatePrototype(source, value.Template)
		if !ok {
			return VariantPlan{}, false
		}
		if index == 0 {
			roles = clonePortModes(template.ports)
			portReads = clonePortReads(template.portReads)
			selectors = append([]prototypeSelectorSurface(nil), template.selectors...)
			exportRoles = append([]composition.Key(nil), template.exports...)
		} else if !samePortModes(roles, template.ports) || !samePortReads(portReads, template.portReads) || !samePrototypeSelectorSurfaces(selectors, template.selectors) || !sameKeySlice(exportRoles, template.exports) {
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

func canonicalPortReads(values []PortRead) ([]PortRead, bool) {
	if len(values) == 0 {
		return nil, true
	}
	result := append([]PortRead(nil), values...)
	sort.Slice(result, func(left, right int) bool { return lessKey(result[left].Role, result[right].Role) })
	for index, read := range result {
		if !read.Role.Available() || !read.Surface.Available() || index > 0 && result[index-1].Role == read.Role {
			return nil, false
		}
	}
	return result, true
}

func samePortReadSlots(left, right []PortRead) bool {
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

func writePortReads(writer *canonical.DigestWriter, reads []PortRead) bool {
	if writer.Count(uint64(len(reads))) != nil {
		return false
	}
	for _, read := range reads {
		if !writeKey(writer, read.Role) || !writeSurface(writer, read.Surface) {
			return false
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

// templatePrototype is a fully authenticated finite fragment except for
// trigger-owned port bases. Its immutable syntax, source Batch, Rule rows,
// local points, group scopes, and port roles are sealed once. Binding checks
// only the concrete port bases against that sealed ABI.
type templatePrototype struct {
	source    *composition.Composition
	key       composition.Key
	value     Template
	batch     *Batch
	instances []canonicalInstance
	points    map[PointRef]Point
	ports     map[composition.Key]PortMode
	portReads map[composition.Key][]PortRead
	selectors []prototypeSelectorSurface
	exports   []composition.Key
}

func sealTemplatePrototype(source *composition.Composition, value Template) (templatePrototype, bool) {
	if source == nil || len(value.Rules) != len(value.Groups) || len(value.Rules) == 0 && len(value.FactorEdges) == 0 {
		return templatePrototype{}, false
	}
	result := templatePrototype{source: source, value: copyTemplate(value)}
	batch := templateBatch(result.value)
	if batch == nil {
		return templatePrototype{}, false
	}
	for _, row := range result.value.Rules {
		schema, schemaOK := ruleSchema(source, row.Schema)
		if !schemaOK || len(schema.Activations) != 0 {
			return templatePrototype{}, false
		}
	}
	catalog, ok := buildTopologyCatalog(TopologySpec{Rules: result.value.Rules, Summaries: result.value.Summaries, WeakTargets: result.value.WeakTargets})
	if !ok || !validateTopologyCatalogUsage(TopologySpec{Rules: result.value.Rules, Summaries: result.value.Summaries, WeakTargets: result.value.WeakTargets}, catalog) {
		return templatePrototype{}, false
	}
	var instances []canonicalInstance
	if len(result.value.Rules) != 0 {
		instances, ok = buildInstances(source, batch, result.value.Rules, catalog)
		if !ok {
			return templatePrototype{}, false
		}
	}
	selectors, ok := prototypeSelectorSurfaces(instances)
	if !ok {
		return templatePrototype{}, false
	}
	points, _, _, ok := buildPoints(result.value.Points)
	if !ok {
		return templatePrototype{}, false
	}
	ports, portReads, exports, ok := prototypePortRoles(result.value.Ports, result.value.Groups, result.value.FactorEdges, points, instances, source, batch)
	if !ok {
		return templatePrototype{}, false
	}
	key, ok := prototypeTemplateKey(result.value, instances, points, ports, portReads)
	if !ok {
		return templatePrototype{}, false
	}
	result.key, result.batch, result.instances, result.points, result.ports, result.portReads, result.selectors, result.exports = key, batch, instances, points, ports, portReads, selectors, exports
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

// ActivationBinding is the sole declarative attachment of one cold activation
// family to one exact trigger occurrence, closed activation Axes, and finite
// Template. Its canonical identity is derived at seal; callers never author
// a binding key or a tuple list.
type ActivationBinding struct {
	Family  composition.Key
	Trigger RuleRef
	// Application and Plan are the shared family-plan attachment. They replace
	// the old per-trigger axes/template payload. PortBindings maps the Plan's
	// named roles to this trigger's already-issued base points.
	Application  composition.Key
	Plan         VariantPlan
	PortBindings []PortBinding
}

type sealedPort struct {
	role           composition.Key
	base           PointRef
	point          Point
	mode           PortMode
	prototypeReads []PortRead
	reads          []PortRead
}

type sealedTemplate struct {
	source    *composition.Composition
	key       composition.Key
	value     Template
	batch     *Batch
	instances []canonicalInstance // declaration-order row keys
	points    map[PointRef]Point  // declaration-order local point keys
	ports     map[composition.Key]sealedPort
	// substitutions is immutable attachment-local data.  It maps only the
	// prototype exact-read surfaces explicitly declared by import slots.
	substitutions map[Surface]Surface
	// ambient is the one exact caller decision universe for this attachment.
	// Every local template scope is instantiated beneath it; every bound Port
	// already owns this exact scope.  It is deliberately private: Port remains
	// a role-only ABI and no per-port context grammar exists.
	ambient Scope
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

func prototypePortRoles(values []Port, groups []FragmentGroup, edges []FragmentFactorEdge, points map[PointRef]Point, instances []canonicalInstance, source *composition.Composition, batch *Batch) (map[composition.Key]PortMode, map[composition.Key][]PortRead, []composition.Key, bool) {
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
		outputScope, outputScopeOK := prototypeFragmentScope(group.Output, points)
		if !outputScopeOK {
			return nil, nil, nil, false
		}
		if group.Output.Local != 0 {
			if group.Output.Port.Available() {
				return nil, nil, nil, false
			}
			if _, found := points[group.Output.Local]; !found {
				return nil, nil, nil, false
			}
			localOutputs[group.Output.Local] = append(localOutputs[group.Output.Local], groupIndex)
		} else {
			mode, found := ports[group.Output.Port]
			if !found || !mode.exports() {
				return nil, nil, nil, false
			}
			uses[group.Output.Port] |= portUseExport
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
			inputScope, inputScopeOK := prototypeFragmentScope(input.Point, points)
			if !inputScopeOK || !validPrototypeFragmentInput(input, inputScope, outputScope) {
				return nil, nil, nil, false
			}
			if input.Point.Local != 0 {
				if input.Point.Port.Available() {
					return nil, nil, nil, false
				}
				if _, found := points[input.Point.Local]; !found {
					return nil, nil, nil, false
				}
				localInputs[groupIndex] = append(localInputs[groupIndex], input.Point.Local)
			} else {
				mode, found := ports[input.Point.Port]
				if !found || !mode.imports() {
					return nil, nil, nil, false
				}
				uses[input.Point.Port] |= portUseImport
			}
		}
	}
	// Factor-local structural edges use the same named port ABI as ordinary
	// fragment inputs.  A source role is an import and a target role is an
	// export; the endpoint may also be a local point.  Recording these uses in
	// the one port liveness table prevents an edge from smuggling an otherwise
	// unbound port into the sealed variant.
	for _, edge := range edges {
		if !validFragmentFactorEdge(edge, points, source, batch) {
			return nil, nil, nil, false
		}
		if edge.ExternalSource.Available() {
			// The external source is already bound to the owning Assembly;
			// it does not consume a shared-plan port role.
		} else if edge.Source.Local == 0 {
			if _, found := ports[edge.Source.Port]; !found {
				return nil, nil, nil, false
			}
			uses[edge.Source.Port] |= portUseImport
		}
		if edge.ExternalTarget.Available() {
			// The external target is already bound to the owning Assembly;
			// it does not consume a shared-plan port role.
		} else if edge.Target.Local == 0 {
			if _, found := ports[edge.Target.Port]; !found {
				return nil, nil, nil, false
			}
			uses[edge.Target.Port] |= portUseExport
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
	if !structuralOnly && !validateFragmentLiveness(groups, points, localOutputs, localInputs) {
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
// FragmentInput connected to that import role; every declared slot must be
// used at least once.  This closes the otherwise subtle input-crossing leak.
func validatePortReadRoutes(portReads map[composition.Key][]PortRead, groups []FragmentGroup, instances []canonicalInstance, source *composition.Composition) bool {
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
				if point.Local != 0 || point.Port != role {
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

// prototypeFragmentScope is the one formal endpoint typing rule. A Local
// point exposes its declared template scope; a Port is polymorphic over the
// eventual attachment scope and is therefore typed at EmptyScope here.  The
// attachment later lifts this empty formal boundary by ambient identity.
func prototypeFragmentScope(value FragmentPoint, points map[PointRef]Point) (Scope, bool) {
	if (value.Local != 0 && value.Port.Available()) || (value.Local == 0 && !value.Port.Available()) {
		return Scope{}, false
	}
	if value.Local != 0 {
		point, found := points[value.Local]
		return point.Scope(), found && point.Available()
	}
	return EmptyScope(), value.Port.Available()
}

func validPrototypeFragmentInput(input FragmentInput, source, target Scope) bool {
	return input.Provenance.Available() && input.Pre.Available() && input.Reindex.Available() && input.Post.Available() &&
		validScopedExpr(input.Pre, source) && sameScope(input.Reindex.source, source) && sameScope(input.Reindex.target, target) && validScopedExpr(input.Post, target)
}

func validFragmentFactorEdge(edge FragmentFactorEdge, points map[PointRef]Point, source *composition.Composition, batch *Batch) bool {
	if source == nil || !edge.Factor.Available() || !edge.Provenance.Available() || !edge.Pre.Available() || !edge.Reindex.Available() || !edge.Post.Available() {
		return false
	}
	if _, known := source.FactorIndex(edge.Factor); !known {
		return false
	}
	if batch == nil || !batch.Sealed() || edge.ExternalSource.Available() && edge.ExternalSource.batch != batch || edge.ExternalTarget.Available() && edge.ExternalTarget.batch != batch {
		return false
	}
	if edge.ExternalSource.Available() && (edge.Source.Local != 0 || edge.Source.Port.Available()) {
		return false
	}
	if !edge.ExternalSource.Available() && edge.Source.Local == 0 && !edge.Source.Port.Available() {
		return false
	}
	if edge.ExternalTarget.Available() && (edge.Target.Local != 0 || edge.Target.Port.Available()) {
		return false
	}
	if !edge.ExternalTarget.Available() && edge.Target.Local == 0 && !edge.Target.Port.Available() {
		return false
	}
	sourceScope, sourceOK := EmptyScope(), true
	if edge.ExternalSource.Available() {
		sourceScope = edge.ExternalSource.Scope()
	} else {
		sourceScope, sourceOK = prototypeFragmentScope(edge.Source, points)
	}
	targetScope, targetOK := EmptyScope(), true
	if edge.ExternalTarget.Available() {
		targetScope = edge.ExternalTarget.Scope()
	} else {
		targetScope, targetOK = prototypeFragmentScope(edge.Target, points)
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

func prototypeFactorEdgeKeys(values []FragmentFactorEdge, points map[PointRef]Point) ([]composition.Key, bool) {
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

func prototypeGroupKeys(groups []FragmentGroup, instances []canonicalInstance, points map[PointRef]Point) ([]composition.Key, bool) {
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

func prototypePointKey(point FragmentPoint, points map[PointRef]Point) (composition.Key, bool) {
	if point.Local != 0 {
		local, found := points[point.Local]
		return local.key, found && !point.Port.Available() && local.Available()
	}
	return point.Port, point.Port.Available()
}

func sealPlanPortBindings(plan VariantPlan, values []PortBinding, base map[PointRef]Point) (map[composition.Key]sealedPort, Scope, bool) {
	if plan.data == nil || len(values) != len(plan.data.ports) {
		return nil, Scope{}, false
	}
	if len(plan.data.ports) == 0 {
		if len(values) != 0 {
			return nil, Scope{}, false
		}
		return map[composition.Key]sealedPort{}, EmptyScope(), true
	}
	ports := make(map[composition.Key]sealedPort, len(values))
	var ambient Scope
	for _, value := range values {
		mode, declared := plan.data.ports[value.Role]
		point, present := base[value.Base]
		if !declared || !value.Role.Available() || !present || !point.Available() {
			return nil, Scope{}, false
		}
		if _, duplicate := ports[value.Role]; duplicate {
			return nil, Scope{}, false
		}
		if !point.Scope().Available() {
			return nil, Scope{}, false
		}
		if !ambient.Available() {
			ambient = point.Scope()
		} else if !sameScope(ambient, point.Scope()) {
			// A shared plan has one caller world. Silently unioning port
			// scopes would create an unowned cross-world transport.
			return nil, Scope{}, false
		}
		prototypeReads := plan.data.portReads[value.Role]
		reads, readsOK := canonicalPortReads(value.Reads)
		if len(prototypeReads) != 0 {
			if !readsOK || !compatiblePortReads(plan.data.source, prototypeReads, reads) {
				return nil, Scope{}, false
			}
		} else if !readsOK || len(reads) != 0 {
			return nil, Scope{}, false
		}
		ports[value.Role] = sealedPort{role: value.Role, base: value.Base, point: point, mode: mode, prototypeReads: append([]PortRead(nil), prototypeReads...), reads: reads}
	}
	return ports, ambient, len(ports) == len(plan.data.ports) && ambient.Available()
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

// bindPrototype resolves only the selected endpoint's named roles against one
// trigger attachment. Prototype rows are not copied: the returned view carries
// immutable plan-owned rows plus small trigger-local port resolution.
func (value templatePrototype) bindPrototype(ports map[composition.Key]sealedPort, ambient Scope) (sealedTemplate, bool) {
	if value.source == nil || !value.key.Available() || value.batch == nil || !value.batch.Sealed() || !ambient.Available() || len(value.instances) == 0 && len(value.value.FactorEdges) == 0 {
		return sealedTemplate{}, false
	}
	selected := make(map[composition.Key]sealedPort, len(value.ports))
	for role, mode := range value.ports {
		port, found := ports[role]
		if !found || port.mode != mode || !port.point.Available() || !sameScope(port.point.Scope(), ambient) ||
			!samePortReadSlots(port.prototypeReads, value.portReads[role]) {
			return sealedTemplate{}, false
		}
		selected[role] = port
	}
	if len(selected) != len(ports) {
		return sealedTemplate{}, false
	}
	substitutions, substitutionsOK := portReadSubstitutions(value.source, selected)
	if !substitutionsOK {
		return sealedTemplate{}, false
	}
	key, ok := identityKey("analysis/engine/equation/activation-template-binding", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, value.key) || !writeScope(writer, ambient) || writer.Count(uint64(len(selected))) != nil {
			return false
		}
		roles := make([]composition.Key, 0, len(selected))
		for role := range selected {
			roles = append(roles, role)
		}
		sort.Slice(roles, func(left, right int) bool { return lessKey(roles[left], roles[right]) })
		for _, role := range roles {
			port := selected[role]
			if !writeKey(writer, role) || !writePoint(writer, port.point) || writer.Uint(uint64(port.mode)) != nil {
				return false
			}
			if !writePortReads(writer, port.prototypeReads) || !writePortReads(writer, port.reads) {
				return false
			}
		}
		return true
	})
	if !ok {
		return sealedTemplate{}, false
	}
	return sealedTemplate{source: value.source, key: key, value: value.value, batch: value.batch, instances: value.instances, points: value.points, ports: selected, substitutions: substitutions, ambient: ambient}, true
}

func portReadSubstitutions(source *composition.Composition, ports map[composition.Key]sealedPort) (map[Surface]Surface, bool) {
	result := make(map[Surface]Surface)
	for _, port := range ports {
		if !compatiblePortReads(source, port.prototypeReads, port.reads) {
			return nil, false
		}
		for index, prototype := range port.prototypeReads {
			if _, duplicate := result[prototype.Surface]; duplicate {
				return nil, false
			}
			result[prototype.Surface] = port.reads[index].Surface
		}
	}
	return result, true
}

// validateFragmentLiveness reverse-walks the template's ordinary local
// Point/Group graph from export-port outputs. A local Point is live only when
// it lies on a path to an exported base point; a local Init input is a valid
// leaf of that walk and does not need a synthetic producer. Every template
// Rule is already owned by exactly one Group, so live Groups imply live Rules.
func validateFragmentLiveness(groups []FragmentGroup, points map[PointRef]Point, producers map[PointRef][]int, inputs [][]PointRef) bool {
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
		Ports:       make([]Port, len(value.Ports)),
		Groups:      make([]FragmentGroup, len(value.Groups)),
		FactorEdges: make([]FragmentFactorEdge, len(value.FactorEdges)),
		Summaries:   make([]SummaryMapping, len(value.Summaries)),
		WeakTargets: make([]WeakTargetMapping, len(value.WeakTargets)),
	}
	for index, row := range value.Rules {
		result.Rules[index] = copyInstance(row)
	}
	for index, port := range value.Ports {
		result.Ports[index] = Port{Role: port.Role, Mode: port.Mode, Reads: append([]PortRead(nil), port.Reads...)}
	}
	for index, group := range value.Groups {
		copied := FragmentGroup{Members: append([]RuleRef(nil), group.Members...), Output: group.Output, Inputs: append([]FragmentInput(nil), group.Inputs...)}
		result.Groups[index] = copied
	}
	for index, edge := range value.FactorEdges {
		result.FactorEdges[index] = FragmentFactorEdge{Source: edge.Source, ExternalSource: edge.ExternalSource, Target: edge.Target, ExternalTarget: edge.ExternalTarget, Factor: edge.Factor, Provenance: edge.Provenance, Pre: edge.Pre, Reindex: edge.Reindex, Post: edge.Post}
	}
	for index, summary := range value.Summaries {
		result.Summaries[index] = SummaryMapping{Surface: summary.Surface, Keys: append([]uint64(nil), summary.Keys...)}
	}
	for index, weak := range value.WeakTargets {
		result.WeakTargets[index] = WeakTargetMapping{Surface: weak.Surface, Candidates: append([]Surface(nil), weak.Candidates...)}
	}
	return result
}

func copyPortBindings(values []PortBinding) []PortBinding {
	result := make([]PortBinding, len(values))
	for index, value := range values {
		result[index] = PortBinding{Role: value.Role, Base: value.Base, Reads: append([]PortRead(nil), value.Reads...)}
	}
	return result
}

func boundProvenance(base, binding, member, row composition.Key) (composition.Key, bool) {
	if !base.Available() || !binding.Available() || !member.Available() || !row.Available() {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/dynamic-boundary-provenance", func(writer *canonical.DigestWriter) bool {
		return writeKey(writer, base) && writeKey(writer, binding) && writeKey(writer, member) && writeKey(writer, row)
	})
}

// memberNamespace is an ephemeral graph-materialization namespace. It is
// derived only while expanding an already accepted solver tuple; it is not a
// Member identity, candidate handle, or topology/manifest row.
func memberNamespace(member Member) (composition.Key, bool) {
	if !member.Available() {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/accepted-activation-namespace", func(writer *canonical.DigestWriter) bool {
		return writeMemberTuple(writer, member)
	})
}

// decisionAlpha is the invocation-local renaming of raw template decisions.
// Its one key per (raw decision, binding, Member) is deliberately independent
// of a local Point row: branches in different local points of one invocation
// remain correlated, while branches from distinct Members cannot alias.
type decisionAlpha map[composition.Key]Decision

func (template sealedTemplate) decisionAlpha(binding, member composition.Key) (decisionAlpha, bool) {
	if !template.key.Available() || !binding.Available() || !member.Available() {
		return nil, false
	}
	result := make(decisionAlpha)
	addSite := func(site Site) bool {
		scope := site.Scope()
		if !site.Available() || !scope.Available() {
			return false
		}
		for _, decision := range scope.row.decisions {
			if _, found := result[decision.key]; found {
				continue
			}
			key, ok := identityKey("analysis/engine/equation/template-decision", func(writer *canonical.DigestWriter) bool {
				return writeKey(writer, decision.key) && writeKey(writer, binding) && writeKey(writer, member)
			})
			if !ok {
				return false
			}
			bound, ok := NewDecision(key)
			if !ok {
				return false
			}
			result[decision.key] = bound
		}
		return true
	}
	for _, point := range template.value.Points {
		if !addSite(point.Site) {
			return nil, false
		}
	}
	for _, rule := range template.value.Rules {
		if !addSite(rule.Occurrence.Site()) {
			return nil, false
		}
	}
	return result, true
}

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
// local endpoint receives the Member alpha; a static Port endpoint retains
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
		// contain the decision. A source-only decision introduced by a Port's
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

// memberSiteTable is the one alpha-binding authority for every local Site in
// one selected Member.  A base Site can occur as a Point, a Rule occurrence,
// or both; those projections must share one dynamically bound Site and one
// dynamically bound Scope.  The table is intentionally Member-local and
// rejects a Site from another sealed Batch before it can acquire an overlay.
type memberSiteTable struct {
	batch   *Batch
	ambient Scope
	alpha   decisionAlpha
	binding composition.Key
	member  composition.Key
	rows    map[composition.Key]memberBoundSite
}

type memberBoundSite struct {
	base  Site
	site  Site
	scope Scope
}

// memberBoundOccurrence is a table-issued lineage token.  It is deliberately
// not a caller-supplied Site/Occurrence pair: bindOperand accepts only this
// value, which proves the Operand stayed on its admitted base occurrence and
// that occurrence stayed on the canonical Site selected for this Member.
type memberBoundOccurrence struct {
	base       Occurrence
	occurrence Occurrence
	site       memberBoundSite
	row        composition.Key
}

func newMemberSiteTable(batch *Batch, ambient Scope, alpha decisionAlpha, binding, member composition.Key, capacity int) (*memberSiteTable, bool) {
	if batch == nil || !batch.Sealed() || !ambient.Available() || alpha == nil || !binding.Available() || !member.Available() || capacity < 0 {
		return nil, false
	}
	return &memberSiteTable{batch: batch, ambient: ambient, alpha: alpha, binding: binding, member: member, rows: make(map[composition.Key]memberBoundSite, capacity)}, true
}

func (table *memberSiteTable) bind(base Site) (memberBoundSite, bool) {
	if table == nil || table.batch == nil || !table.batch.Sealed() || !base.Available() || base.batch != table.batch {
		return memberBoundSite{}, false
	}
	if bound, found := table.rows[base.Key()]; found {
		return bound, bound.base.Same(base) && bound.site.Available() && sameScope(bound.scope, bound.site.Scope())
	}
	init, disposition, initialized := base.Init()
	if !initialized {
		return memberBoundSite{}, false
	}
	scope, ok := boundScope(base.Scope(), table.ambient, table.alpha)
	if !ok {
		return memberBoundSite{}, false
	}
	boundInit, ok := boundExpr(init, table.alpha)
	if !ok {
		return memberBoundSite{}, false
	}
	return table.admit(base, scope, boundInit, disposition)
}

// admit is deliberately strict even though bind is its only production
// caller.  It makes a duplicate projection prove the same alpha scope, init
// formula, and disposition, so a foreign or divergent rebind has no route to
// a second capability for the same source row.
func (table *memberSiteTable) admit(base Site, scope Scope, init Expr, disposition InitDisposition) (memberBoundSite, bool) {
	if table == nil || table.batch == nil || !table.batch.Sealed() || !base.Available() || base.batch != table.batch || !scope.Available() || !init.Available() {
		return memberBoundSite{}, false
	}
	baseInit, baseDisposition, initialized := base.Init()
	expectedScope, scopeOK := boundScope(base.Scope(), table.ambient, table.alpha)
	expectedInit, initOK := boundExpr(baseInit, table.alpha)
	if !initialized || !scopeOK || !initOK || disposition != baseDisposition || !sameScope(scope, expectedScope) || !sameExpr(init, expectedInit) {
		return memberBoundSite{}, false
	}
	if bound, found := table.rows[base.Key()]; found {
		return bound, bound.base.Same(base) && sameScope(bound.scope, scope) && bound.site.Available() && sameExpr(boundInit(bound.site), init)
	}
	site, ok := boundSite(base, scope, init, disposition, table.binding, table.member)
	if !ok {
		return memberBoundSite{}, false
	}
	bound := memberBoundSite{base: base, site: site, scope: scope}
	table.rows[base.Key()] = bound
	return bound, true
}

func boundInit(site Site) Expr {
	value, _, ok := site.Init()
	if !ok {
		return Expr{}
	}
	return value
}

func (table *memberSiteTable) bindOccurrence(base Occurrence, row composition.Key) (memberBoundOccurrence, bool) {
	if table == nil || table.batch == nil || !table.batch.Sealed() || !base.Available() || base.batch != table.batch || base.dynamic != nil || !row.Available() {
		return memberBoundOccurrence{}, false
	}
	site, ok := table.bind(base.Site())
	if !ok || !site.base.Same(base.Site()) || !site.site.Available() {
		return memberBoundOccurrence{}, false
	}
	key, ok := identityKey("analysis/engine/equation/dynamic-occurrence", func(writer *canonical.DigestWriter) bool {
		return writeOccurrence(writer, base) && writeSite(writer, site.site) && writeKey(writer, table.binding) && writeKey(writer, table.member) && writeKey(writer, row)
	})
	if !ok {
		return memberBoundOccurrence{}, false
	}
	occurrence := Occurrence{batch: base.batch, row: base.row, dynamic: &dynamicOccurrence{site: site.site, key: key, binding: table.binding, member: table.member, row: row}}
	if !occurrence.Available() || !occurrence.Site().Same(site.site) {
		return memberBoundOccurrence{}, false
	}
	return memberBoundOccurrence{base: base, occurrence: occurrence, site: site, row: row}, true
}

func (table *memberSiteTable) bindOperand(base Operand, occurrence memberBoundOccurrence) (Operand, bool) {
	if table == nil || table.batch == nil || !table.batch.Sealed() || !base.Available() || base.batch != table.batch || base.dynamic != nil || !occurrence.base.Available() || !occurrence.occurrence.Available() || occurrence.base.batch != table.batch || occurrence.occurrence.batch != table.batch || !occurrence.row.Available() {
		return Operand{}, false
	}
	baseOccurrence := base.Occurrence()
	if !baseOccurrence.Same(occurrence.base) || !occurrence.site.base.Same(occurrence.base.Site()) || !occurrence.site.site.Available() || !occurrence.occurrence.Site().Same(occurrence.site.site) || occurrence.occurrence.dynamic == nil {
		return Operand{}, false
	}
	dynamic := occurrence.occurrence.dynamic
	if dynamic.binding != table.binding || dynamic.member != table.member || dynamic.row != occurrence.row || !dynamic.site.Same(occurrence.site.site) {
		return Operand{}, false
	}
	key, ok := identityKey("analysis/engine/equation/dynamic-operand", func(writer *canonical.DigestWriter) bool {
		return writeOperand(writer, base) && writeOccurrence(writer, occurrence.occurrence) && writeKey(writer, table.binding) && writeKey(writer, table.member) && writeKey(writer, occurrence.row)
	})
	if !ok {
		return Operand{}, false
	}
	operand := Operand{batch: base.batch, row: base.row, dynamic: &dynamicOperand{occurrence: occurrence.occurrence, key: key, binding: table.binding, member: table.member, row: occurrence.row}}
	return operand, operand.Available() && operand.Occurrence().Same(occurrence.occurrence)
}

func (template sealedTemplate) appendMember(spec *TopologySpec, binding composition.Key, member Member, premise Expr) bool {
	if spec == nil || spec.Batch != template.batch || !template.key.Available() || template.batch == nil || !template.batch.Sealed() || !template.ambient.Available() || !member.Available() || member.Binding() != binding || !premise.Available() || !validScopedExpr(premise, template.ambient) || len(template.instances) != len(template.value.Rules) || len(template.points) != len(template.value.Points) {
		return false
	}
	namespace, namespaceOK := memberNamespace(member)
	if !namespaceOK {
		return false
	}
	alpha, ok := template.decisionAlpha(binding, namespace)
	if !ok {
		return false
	}
	provenanceRow := template.key
	if len(template.instances) != 0 {
		provenanceRow = template.instances[0].key
	}
	sites, ok := newMemberSiteTable(template.batch, template.ambient, alpha, binding, namespace, len(template.value.Points)+len(template.value.Rules))
	if !ok {
		return false
	}
	ruleOffset := len(spec.Rules)
	locals := make(map[PointRef]templateResolvedPoint, len(template.value.Points))
	for pointIndex, row := range template.value.Points {
		local := PointAt(pointIndex)
		original, found := template.points[local]
		if !found || !original.Available() {
			return false
		}
		if !row.Site.Same(original.Site()) {
			return false
		}
		boundSite, ok := sites.bind(row.Site)
		if !ok {
			return false
		}
		bound := PointAt(len(spec.Points))
		locals[local] = templateResolvedPoint{ref: bound, site: boundSite.site, scope: boundSite.scope, rawScope: row.Site.Scope(), local: true}
		spec.Points = append(spec.Points, PointSpec{Site: boundSite.site})
	}
	for ruleIndex, row := range template.value.Rules {
		instance := template.instances[ruleIndex]
		occurrence, ok := sites.bindOccurrence(row.Occurrence, instance.key)
		if !ok {
			return false
		}
		operand, ok := sites.bindOperand(row.Operand, occurrence)
		if !ok {
			return false
		}
		bound := copyInstance(row)
		bound.Occurrence, bound.Operand = occurrence.occurrence, operand
		if !template.substitutePortReads(&bound) {
			return false
		}
		bound.activation = member
		spec.Rules = append(spec.Rules, bound)
	}
	premiseDecisions := premise.Decisions()
	for _, group := range template.value.Groups {
		output, outputOK := template.resolvePoint(spec, locals, group.Output, PortExport)
		if !outputOK || len(group.Members) == 0 {
			return false
		}
		for _, decision := range premiseDecisions {
			if !output.scope.contains(decision) {
				return false
			}
		}
		bound := Group{Members: make([]RuleRef, len(group.Members)), Output: output.ref, Inputs: make([]Input, len(group.Inputs)), premise: premise}
		for memberIndex, ref := range group.Members {
			index, ok := ruleRefIndex(ref, len(template.value.Rules))
			if !ok {
				return false
			}
			bound.Members[memberIndex] = RuleAt(ruleOffset + index)
		}
		for inputIndex, input := range group.Inputs {
			source, sourceOK := template.resolvePoint(spec, locals, input.Point, PortImport)
			if !sourceOK {
				return false
			}
			reindex, ok := boundReindex(input.Reindex, source, output, template.ambient, alpha)
			if !ok {
				return false
			}
			pre, post := input.Pre, input.Post
			if source.local {
				pre, ok = boundExpr(pre, alpha)
				if !ok {
					return false
				}
			}
			if output.local {
				post, ok = boundExpr(post, alpha)
				if !ok {
					return false
				}
			}
			provenance, ok := boundProvenance(input.Provenance, binding, namespace, provenanceRow)
			if !ok {
				return false
			}
			bound.Inputs[inputIndex] = BoundaryInput(source.site, output.site, provenance, pre, reindex, post)
			if !bound.Inputs[inputIndex].Available() {
				return false
			}
		}
		spec.Groups = append(spec.Groups, bound)
	}
	for _, edge := range template.value.FactorEdges {
		var source templateResolvedPoint
		var sourceOK bool
		if edge.ExternalSource.Available() {
			source, sourceOK = resolveExternalPoint(spec, edge.ExternalSource)
		} else {
			source, sourceOK = template.resolvePoint(spec, locals, edge.Source, PortImport)
		}
		var target templateResolvedPoint
		var targetOK bool
		if edge.ExternalTarget.Available() {
			target, targetOK = resolveExternalPoint(spec, edge.ExternalTarget)
		} else {
			target, targetOK = template.resolvePoint(spec, locals, edge.Target, PortExport)
		}
		if !sourceOK || !targetOK {
			return false
		}
		reindex, ok := boundReindex(edge.Reindex, source, target, template.ambient, alpha)
		if !ok {
			return false
		}
		pre, post := edge.Pre, edge.Post
		if source.local {
			pre, ok = boundExpr(pre, alpha)
			if !ok {
				return false
			}
		}
		if target.local {
			post, ok = boundExpr(post, alpha)
			if !ok {
				return false
			}
		}
		// Accepted-member evidence is part of the selected route. Attach it to
		// whichever endpoint owns the attachment scope; this is what permits
		// both external->port and port->external structural rows without a
		// compensating identity producer or an escaped formula.
		if validScopedExpr(premise, source.scope) {
			pre, ok = AndExpr(premise, pre)
		} else if validScopedExpr(premise, target.scope) {
			post, ok = AndExpr(premise, post)
		} else {
			return false
		}
		if !ok {
			return false
		}
		provenance, ok := boundProvenance(edge.Provenance, binding, namespace, provenanceRow)
		if !ok {
			return false
		}
		input := BoundaryInput(source.site, target.site, provenance, pre, reindex, post)
		if !input.Available() {
			return false
		}
		spec.FactorEdges = append(spec.FactorEdges, FactorEdge{Target: target.ref, Input: input, Factor: edge.Factor})
	}
	for _, summary := range template.value.Summaries {
		if !appendSummaryMapping(&spec.Summaries, summary) {
			return false
		}
	}
	for _, weak := range template.value.WeakTargets {
		boundWeak, ok := template.substituteWeakTargetCandidates(weak)
		if !ok || !appendWeakTargetMapping(&spec.WeakTargets, boundWeak) {
			return false
		}
	}
	return true
}

// substitutePortReads replaces only prototype surfaces explicitly named by a
// read-bearing import port.  It runs after member-local occurrence binding
// and before the ordinary topology validator, so the normal Rule schema
// remains the final authority for Factor/form compatibility.
func (template sealedTemplate) substitutePortReads(row *RuleInstance) bool {
	if row == nil {
		return false
	}
	for index := range row.Reads {
		if caller, mapped := template.substitutions[row.Reads[index].Surface]; mapped {
			row.Reads[index].Surface = caller
		}
	}
	return true
}

func (template sealedTemplate) substituteWeakTargetCandidates(value WeakTargetMapping) (WeakTargetMapping, bool) {
	if !value.Surface.Available() {
		return WeakTargetMapping{}, false
	}
	result := WeakTargetMapping{Surface: value.Surface, Candidates: make([]Surface, len(value.Candidates))}
	for index, candidate := range value.Candidates {
		if caller, mapped := template.substitutions[candidate]; mapped {
			candidate = caller
		}
		result.Candidates[index] = candidate
	}
	sort.Slice(result.Candidates, func(left, right int) bool { return lessSurface(result.Candidates[left], result.Candidates[right]) })
	for index := 1; index < len(result.Candidates); index++ {
		// Coverage is a set, but distinct prototype candidates collapsing to
		// one caller Ref would hide an invalid attachment.  Reject rather than
		// silently changing the declared weak relation.
		if result.Candidates[index-1] == result.Candidates[index] {
			return WeakTargetMapping{}, false
		}
	}
	return result, validWeakTargetMapping(result)
}

type templateResolvedPoint struct {
	ref      PointRef
	site     Site
	scope    Scope
	rawScope Scope
	local    bool
}

func (point templateResolvedPoint) available() bool {
	return point.ref != 0 && point.site.Available() && point.scope.Available() && point.rawScope.Available()
}

func (template sealedTemplate) resolvePoint(spec *TopologySpec, locals map[PointRef]templateResolvedPoint, value FragmentPoint, required PortMode) (templateResolvedPoint, bool) {
	if spec == nil || value.Local != 0 && value.Port.Available() || value.Local == 0 && !value.Port.Available() {
		return templateResolvedPoint{}, false
	}
	if value.Local != 0 {
		point, found := locals[value.Local]
		return point, found && point.available() && point.local
	}
	port, found := template.ports[value.Port]
	if !found || required == PortImport && !port.mode.imports() || required == PortExport && !port.mode.exports() {
		return templateResolvedPoint{}, false
	}
	index, valid := pointRefIndex(port.base, len(spec.Points))
	if !valid {
		return templateResolvedPoint{}, false
	}
	row := spec.Points[index]
	point, issued := derivePoint(row.Site)
	if !issued || point.key != port.point.key || !sameScope(point.Scope(), port.point.Scope()) {
		return templateResolvedPoint{}, false
	}
	if !template.ambient.Available() || !sameScope(port.point.Scope(), template.ambient) {
		return templateResolvedPoint{}, false
	}
	// Prototype ports are typed at EmptyScope. The attachment's actual
	// ambient decisions are supplied only by boundReindex's identity lift.
	return templateResolvedPoint{ref: port.base, site: port.point.Site(), scope: template.ambient, rawScope: EmptyScope()}, true
}

func resolveExternalPoint(spec *TopologySpec, site Site) (templateResolvedPoint, bool) {
	if spec == nil || !site.Available() || spec.Batch == nil || !spec.Batch.Sealed() {
		return templateResolvedPoint{}, false
	}
	for index, row := range spec.Points {
		if !row.Site.Available() || !row.Site.Same(site) {
			continue
		}
		point := PointAt(index)
		return templateResolvedPoint{ref: point, site: site, scope: site.Scope(), rawScope: site.Scope()}, true
	}
	return templateResolvedPoint{}, false
}

func ruleRefIndex(ref RuleRef, count int) (int, bool) {
	value := uint64(ref)
	if value == 0 || value > uint64(count) {
		return 0, false
	}
	return int(value - 1), true
}

func pointRefIndex(ref PointRef, count int) (int, bool) {
	value := uint64(ref)
	if value == 0 || value > uint64(count) {
		return 0, false
	}
	return int(value - 1), true
}

func appendSummaryMapping(rows *[]SummaryMapping, value SummaryMapping) bool {
	if rows == nil || !validSummaryMapping(value) {
		return false
	}
	for _, current := range *rows {
		if current.Surface == value.Surface {
			return compareRawKeySets(current.Keys, value.Keys) == 0
		}
	}
	*rows = append(*rows, SummaryMapping{Surface: value.Surface, Keys: append([]uint64(nil), value.Keys...)})
	return true
}

func appendWeakTargetMapping(rows *[]WeakTargetMapping, value WeakTargetMapping) bool {
	if rows == nil || !validWeakTargetMapping(value) {
		return false
	}
	for _, current := range *rows {
		if current.Surface != value.Surface {
			continue
		}
		if len(current.Candidates) != len(value.Candidates) {
			return false
		}
		for index := range current.Candidates {
			if current.Candidates[index] != value.Candidates[index] {
				return false
			}
		}
		return true
	}
	*rows = append(*rows, WeakTargetMapping{Surface: value.Surface, Candidates: append([]Surface(nil), value.Candidates...)})
	return true
}
