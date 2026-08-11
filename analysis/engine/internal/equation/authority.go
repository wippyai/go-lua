package equation

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// Topology is the one sealed structural authority for a Solver. It owns the
// base ordinary topology plus finite activation bindings. A binding stores
// one closed constituent Axes declaration and one finite Template once; Graph materializes only the
// fragments selected by accepted Members. No candidate universe, bitmap,
// alternate graph, or callback survives sealing.
type Topology struct {
	source *composition.Composition
	base   TopologySpec
	key    composition.Key

	bindings         []activationBinding
	bindingAt        map[composition.Key]int
	bindingByTrigger map[composition.Key]int
	reverses         []derivedActivationReverse
}

// activationBinding is sealed, topology-owned metadata. Its key is derived
// from the cold Schema, exact trigger occurrence, Axes, Template/ports, and
// its cold family, exact trigger occurrence, Axes, and Template/ports; it is
// never supplied by a builder or selected by a Rule.
type activationBinding struct {
	key              composition.Key
	family           composition.Key
	trigger          composition.Key
	triggerRule      composition.Key
	triggerAdmission composition.Key
	application      composition.Key
	plan             VariantPlan
	ports            map[composition.Key]sealedPort
	ambient          Scope
}

// derivedActivationReverse is produced exclusively by a sealed binding's
// actual export ports. It is not a second user-authored declaration.
type derivedActivationReverse struct {
	binding composition.Key
	target  composition.Key
	trigger composition.Key
}

func (binding *activationBinding) issue(owner *Topology, locator PairLocator) (Member, bool) {
	if binding == nil || owner == nil || !binding.key.Available() || !binding.family.Available() || !binding.trigger.Available() {
		return Member{}, false
	}
	if !binding.plan.available(owner.source, binding.family) || binding.application != locator.Application {
		return Member{}, false
	}
	if _, found := binding.plan.variant(locator.Target, locator.Endpoint); !found {
		return Member{}, false
	}
	return Member{owner: owner, binding: binding.key, locator: locator}, true
}

func (binding *activationBinding) appendMember(spec *TopologySpec, member Member, premise Expr) bool {
	if binding == nil || spec == nil || !member.Available() || member.Binding() != binding.key || !premise.Available() || !validScopedExpr(premise, binding.ambient) {
		return false
	}
	locator, located := member.Locator()
	if !located || locator.Application != binding.application {
		return false
	}
	variant, found := binding.plan.variant(locator.Target, locator.Endpoint)
	if !found {
		return false
	}
	template, bound := variant.template.bindPrototype(binding.ports, binding.ambient)
	if !bound {
		return false
	}
	if !template.appendMember(spec, binding.key, member, premise) {
		return false
	}
	return true
}

// SealTopology freezes a single base graph and its binding descriptors. It
// derives every binding identity from its exact trigger occurrence, rather
// than trusting a caller-authored family key. A later semantic Run can submit
// a raw locator only through Topology.SelectMember, which validates that
// trigger's sole binding and mints one Member. It deliberately does not
// compile one graph per inactive relation.
func SealTopology(source *composition.Composition, spec TopologySpec) (*Topology, bool) {
	if source == nil || !validTopologyBatch(spec.Batch, spec) {
		return nil, false
	}
	sealed := copyTopologySpec(spec)
	basePoints, _, _, baseOK := buildPoints(sealed.Points)
	if !baseOK {
		return nil, false
	}
	catalog, catalogOK := buildTopologyCatalog(TopologySpec{Rules: sealed.Rules, Summaries: sealed.Summaries, WeakTargets: sealed.WeakTargets})
	if !catalogOK {
		return nil, false
	}
	instances, instancesOK := buildInstances(source, sealed.Batch, sealed.Rules, catalog)
	if !instancesOK {
		return nil, false
	}
	topology := &Topology{
		source:           source,
		bindingAt:        make(map[composition.Key]int, len(sealed.ActivationBindings)),
		bindingByTrigger: make(map[composition.Key]int, len(sealed.ActivationBindings)),
	}
	if !topology.sealBindings(sealed.Batch, sealed.ActivationBindings, basePoints, instances) {
		return nil, false
	}
	if !sealed.Batch.closesOperandRealms(sealed.Rules, sealed.ActivationBindings) {
		return nil, false
	}
	sealed.ActivationBindings = nil
	graph, compiled := compileTopology(source, sealed, topology.reverses)
	if !compiled || graph == nil {
		return nil, false
	}
	baseKey, keyed := graphSemanticKey(graph)
	if !keyed {
		return nil, false
	}
	key, keyed := topology.deriveKey(baseKey)
	if !keyed {
		return nil, false
	}
	topology.base, topology.key = sealed, key
	return topology, true
}

func (topology *Topology) sealBindings(batch *Batch, values []ActivationBinding, base map[PointRef]Point, instances []canonicalInstance) bool {
	if topology == nil || topology.source == nil || batch == nil || !batch.Sealed() {
		return false
	}
	if !distinctActivationBindingTriggers(values, instances) {
		return false
	}
	bindings := make([]activationBinding, len(values))
	boundTriggers := make(map[composition.Key]struct{}, len(values))
	checkedPlans := make(map[*variantPlanData]struct{}, len(values))
	for index, value := range values {
		if !value.Family.Available() {
			return false
		}
		_, known := topology.source.ActivationFamily(value.Family)
		if !known {
			return false
		}
		triggerIndex, triggerOK := ruleRefIndex(value.Trigger, len(instances))
		if !triggerOK {
			return false
		}
		trigger := instances[triggerIndex]
		triggerSchema, schemaOK := ruleSchema(topology.source, trigger.row.Schema)
		if !schemaOK || len(triggerSchema.Activations) != 1 || triggerSchema.Activations[0].Family != value.Family {
			return false
		}
		if _, alreadyBound := boundTriggers[trigger.key]; alreadyBound {
			return false
		}
		if !trigger.row.Schema.Available() || !triggerSchema.Admission.Identity.Available() {
			return false
		}
		if !value.Plan.available(topology.source, value.Family) || !value.Application.Available() {
			return false
		}
		if _, checked := checkedPlans[value.Plan.data]; !checked {
			for _, variant := range value.Plan.data.variants {
				if variant.template.batch != batch {
					return false
				}
			}
			checkedPlans[value.Plan.data] = struct{}{}
		}
		ports, ambient, portsOK := sealPlanPortBindings(value.Plan, value.PortBindings, base)
		if !portsOK || len(value.Plan.data.exports) == 0 && !value.Plan.data.structuralOnly() {
			return false
		}
		key, keyOK := deriveVariantBindingKey(value.Family, trigger.key, value.Application, value.Plan.data.key, ports, ambient)
		if !keyOK {
			return false
		}
		bindings[index] = activationBinding{key: key, family: value.Family, trigger: trigger.key, triggerRule: trigger.row.Schema, triggerAdmission: triggerSchema.Admission.Identity, application: value.Application, plan: value.Plan, ports: ports, ambient: ambient}
		boundTriggers[trigger.key] = struct{}{}
	}
	for _, instance := range instances {
		schema, schemaOK := ruleSchema(topology.source, instance.row.Schema)
		if !schemaOK {
			return false
		}
		if len(schema.Activations) != 0 {
			if len(schema.Activations) != 1 {
				return false
			}
			if _, bound := boundTriggers[instance.key]; !bound {
				return false
			}
		}
	}
	sort.Slice(bindings, func(left, right int) bool { return lessKey(bindings[left].key, bindings[right].key) })
	topology.bindingAt = make(map[composition.Key]int, len(bindings))
	reverseSeen := make(map[[2]composition.Key]struct{}, len(bindings))
	for index := range bindings {
		if index > 0 && bindings[index].key == bindings[index-1].key {
			return false
		}
		topology.bindingAt[bindings[index].key] = index
		if _, duplicate := topology.bindingByTrigger[bindings[index].trigger]; duplicate {
			return false
		}
		topology.bindingByTrigger[bindings[index].trigger] = index
		exports := make([]sealedPort, 0, len(bindings[index].plan.data.exports))
		for _, role := range bindings[index].plan.data.exports {
			port, found := bindings[index].ports[role]
			if !found {
				return false
			}
			exports = append(exports, port)
		}
		for _, port := range exports {
			if !port.point.Available() {
				return false
			}
			incidence := [2]composition.Key{port.point.key, bindings[index].trigger}
			if _, duplicate := reverseSeen[incidence]; duplicate {
				continue
			}
			reverseSeen[incidence] = struct{}{}
			topology.reverses = append(topology.reverses, derivedActivationReverse{binding: bindings[index].key, target: port.point.key, trigger: bindings[index].trigger})
		}
	}
	topology.bindings = bindings
	return true
}

// distinctActivationBindingTriggers closes the raw TopologySpec boundary
// before template sealing. One structural trigger owns one family binding;
// accepting a second binding would reintroduce a tuple compatibility table.
func distinctActivationBindingTriggers(values []ActivationBinding, instances []canonicalInstance) bool {
	seen := make(map[composition.Key]struct{}, len(values))
	for _, value := range values {
		index, ok := ruleRefIndex(value.Trigger, len(instances))
		if !ok || !instances[index].key.Available() {
			return false
		}
		if _, duplicate := seen[instances[index].key]; duplicate {
			return false
		}
		seen[instances[index].key] = struct{}{}
	}
	return true
}

func deriveVariantBindingKey(schema, trigger, application, plan composition.Key, ports map[composition.Key]sealedPort, ambient Scope) (composition.Key, bool) {
	if !schema.Available() || !trigger.Available() || !application.Available() || !plan.Available() || !ambient.Available() {
		return composition.Key{}, false
	}
	roles := make([]composition.Key, 0, len(ports))
	for role := range ports {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(left, right int) bool { return lessKey(roles[left], roles[right]) })
	return identityKey("analysis/engine/equation/activation-variant-binding", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, schema) || !writeKey(writer, trigger) || !writeKey(writer, application) || !writeKey(writer, plan) || !writeScope(writer, ambient) || writer.Count(uint64(len(roles))) != nil {
			return false
		}
		for _, role := range roles {
			port := ports[role]
			if !writeKey(writer, role) || !writePoint(writer, port.point) || writer.Uint(uint64(port.mode)) != nil {
				return false
			}
			if !writePortReads(writer, port.prototypeReads) || !writePortReads(writer, port.reads) {
				return false
			}
		}
		return true
	})
}

func (topology *Topology) binding(key composition.Key) (*activationBinding, bool) {
	if topology == nil || !key.Available() {
		return nil, false
	}
	index, found := topology.bindingAt[key]
	if !found || index < 0 || index >= len(topology.bindings) || topology.bindings[index].key != key {
		return nil, false
	}
	return &topology.bindings[index], true
}

// ownsMember rechecks the opaque Member against this topology's sealed Axes.
// Members are deliberately issued on demand, not retained in a candidate
// catalog, so pointer ownership alone is not an authority check.
func (topology *Topology) ownsMember(member Member) bool {
	if topology == nil || member.owner != topology || !member.binding.Available() || !member.locator.Available() {
		return false
	}
	binding, found := topology.binding(member.binding)
	if !found || !binding.plan.available(topology.source, binding.family) || binding.application != member.locator.Application {
		return false
	}
	if _, variant := binding.plan.variant(member.locator.Target, member.locator.Endpoint); !variant {
		return false
	}
	return true
}

// ActivationBinding returns the one sealed binding owned by one exact trigger
// occurrence and cold family. One trigger has exactly one binding: allowing a
// slice here would let singleton axes reconstruct a compatibility table.
func (topology *Topology) ActivationBinding(trigger, family composition.Key) (composition.Key, bool) {
	if topology == nil || !trigger.Available() || !family.Available() {
		return composition.Key{}, false
	}
	index, found := topology.bindingByTrigger[trigger]
	if !found || index < 0 || index >= len(topology.bindings) {
		return composition.Key{}, false
	}
	binding := topology.bindings[index]
	if binding.trigger != trigger || binding.family != family {
		return composition.Key{}, false
	}
	return binding.key, true
}

// ActivationApplication returns the exact application constituent sealed on
// one trigger binding. It exposes no plan, target, endpoint, or candidate
// enumeration and therefore cannot reconstruct the forbidden product plane.
func (topology *Topology) ActivationApplication(trigger, family composition.Key) (composition.Key, bool) {
	if topology == nil || !trigger.Available() || !family.Available() {
		return composition.Key{}, false
	}
	index, found := topology.bindingByTrigger[trigger]
	if !found || index < 0 || index >= len(topology.bindings) {
		return composition.Key{}, false
	}
	binding := topology.bindings[index]
	if binding.trigger != trigger || binding.family != family || !binding.application.Available() {
		return composition.Key{}, false
	}
	return binding.application, true
}

// SelectMember is the only structural conversion from one trigger's semantic
// locator to its one Member. Each trigger owns one binding, so it cannot union
// singleton bindings into a compatibility table and does not allocate.
func (topology *Topology) SelectMember(trigger composition.Key, locator PairLocator) (Member, bool) {
	if topology == nil || !trigger.Available() || !locator.Available() {
		return Member{}, false
	}
	index, found := topology.bindingByTrigger[trigger]
	if !found || index < 0 || index >= len(topology.bindings) {
		return Member{}, false
	}
	return topology.bindings[index].issue(topology, locator)
}

func (topology *Topology) deriveKey(base composition.Key) (composition.Key, bool) {
	if topology == nil || !base.Available() {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/topology", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, base) || writer.Count(uint64(len(topology.bindings))) != nil {
			return false
		}
		for _, binding := range topology.bindings {
			if !writeKey(writer, binding.key) || !writeKey(writer, binding.family) || !writeKey(writer, binding.trigger) || !writeKey(writer, binding.triggerRule) || !writeKey(writer, binding.triggerAdmission) || !writeKey(writer, binding.application) || !writeKey(writer, binding.plan.data.key) {
				return false
			}
		}
		return true
	})
}

// Key is the opaque canonical identity of the complete sealed topology.  It
// commits to the base ordinary graph and the symbolic family descriptors, not
// to an inactive candidate enumeration or a physical layout.
func (topology *Topology) Key() composition.Key {
	if topology == nil || !topology.key.Available() {
		return composition.Key{}
	}
	return topology.key
}

// Revision derives the exact structural identity for an accepted set.  The
// evidence digest is part of the token: cancellation and a fresh epoch keep
// the same accepted fact instead of remembering only the Member coordinate.
func (topology *Topology) Revision(accepted []AcceptedMember) (composition.Key, bool) {
	if topology == nil || !topology.key.Available() || !topology.validAccepted(accepted) {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/topology-revision", func(writer *canonical.DigestWriter) bool {
		if !writeKey(writer, topology.key) || writer.Count(uint64(len(accepted))) != nil {
			return false
		}
		for _, row := range accepted {
			if !writeMemberTuple(writer, row.Member()) || !writeExpr(writer, row.Premise()) {
				return false
			}
		}
		return true
	})
}

// OwnsComposition proves that this sealed topology was bound to the exact
// cold Composition later used by the runtime binder.
func (topology *Topology) OwnsComposition(source *composition.Composition) bool {
	return topology != nil && topology.source != nil && topology.source == source
}

// Accept validates one exact admitted activation selection and retains its
// exact equation premise. The binding itself owns the exact
// trigger Rule, registered admission identity, and anchor, so callers cannot
// supply parallel/forgeable echoes of those coordinates. The guard FormulaID
// Physical product-row/read indexes never enter identity: they are scheduler
// artifacts, not serializable evidence.
func (topology *Topology) Accept(member Member, premise Expr) (AcceptedMember, bool) {
	if topology == nil || !member.ownedBy(topology) || !premise.Available() {
		return AcceptedMember{}, false
	}
	binding, bound := topology.binding(member.Binding())
	if !bound || !validScopedExpr(premise, binding.ambient) {
		return AcceptedMember{}, false
	}
	evidence, ok := topology.acceptedEvidenceKey(member, premise)
	if !ok {
		return AcceptedMember{}, false
	}
	return AcceptedMember{member: member, premise: premise, evidence: evidence}, true
}

// acceptedEvidenceKey is the one evidence constructor for admission and
// monotone premise union. Binding provenance is semantic: a Member alone is
// not an admission proof, so the exact trigger Rule/admission/occurrence are
// retained in both construction paths.
func (topology *Topology) acceptedEvidenceKey(member Member, premise Expr) (composition.Key, bool) {
	if topology == nil || !member.ownedBy(topology) || !premise.Available() {
		return composition.Key{}, false
	}
	binding, bound := topology.binding(member.Binding())
	if !bound || !binding.triggerRule.Available() || !binding.triggerAdmission.Available() || !binding.trigger.Available() {
		return composition.Key{}, false
	}
	return identityKey("analysis/engine/equation/activation-evidence", func(writer *canonical.DigestWriter) bool {
		return writeMemberTuple(writer, member) && writeKey(writer, binding.triggerRule) && writeKey(writer, binding.triggerAdmission) && writeKey(writer, binding.trigger) && writeExpr(writer, premise)
	})
}

// Graph is the sole derived graph view T(A) = base U selected Template
// fragments. It visits only accepted Members, never an eager candidate set.
func (topology *Topology) Graph(accepted []AcceptedMember) (*Graph, bool) {
	if topology == nil || topology.source == nil || !topology.validAccepted(accepted) {
		return nil, false
	}
	spec := copyTopologySpec(topology.base)
	for _, row := range accepted {
		member := row.Member()
		binding, found := topology.binding(member.Binding())
		if !found || !binding.appendMember(&spec, member, row.Premise()) {
			return nil, false
		}
	}
	if _, ok := topology.Revision(accepted); !ok {
		return nil, false
	}
	graph, ok := compileTopology(topology.source, spec, topology.reverses)
	return graph, ok && graph != nil
}

func (topology *Topology) validAccepted(values []AcceptedMember) bool {
	if topology == nil {
		return false
	}
	for index, value := range values {
		if !value.Available() || !value.member.ownedBy(topology) || index > 0 && !lessAcceptedMember(values[index-1], value) {
			return false
		}
		binding, bound := topology.binding(value.member.Binding())
		if !bound || !validScopedExpr(value.premise, binding.ambient) {
			return false
		}
		if index > 0 && sameMember(values[index-1].Member(), value.Member()) {
			return false
		}
	}
	return true
}

// MergeAccepted unions immutable evidence facts for one Member.  The caller
// uses it only at the Solver monotone-set boundary; no epoch or Rule callback
// can mutate a previously accepted record.
func (topology *Topology) MergeAccepted(left, right AcceptedMember) (AcceptedMember, bool) {
	if topology == nil || !left.Available() || !right.Available() || !left.member.ownedBy(topology) || !right.member.ownedBy(topology) || !sameMember(left.member, right.member) {
		return AcceptedMember{}, false
	}
	binding, bound := topology.binding(left.member.Binding())
	if !bound || !validScopedExpr(left.premise, binding.ambient) || !validScopedExpr(right.premise, binding.ambient) {
		return AcceptedMember{}, false
	}
	premise, ok := OrExpr(left.premise, right.premise)
	if !ok {
		return AcceptedMember{}, false
	}
	evidence, ok := topology.acceptedEvidenceKey(left.member, premise)
	if !ok {
		return AcceptedMember{}, false
	}
	return AcceptedMember{member: left.member, premise: premise, evidence: evidence}, true
}

func graphSemanticKey(graph *Graph) (composition.Key, bool) {
	if graph == nil || graph.self != graph || graph.composition == nil {
		return composition.Key{}, false
	}
	type reverse struct{ target, trigger composition.Key }
	reverses := make([]reverse, 0)
	for target, triggers := range graph.activationReverses {
		if target < 0 || target >= len(graph.points) {
			return composition.Key{}, false
		}
		for _, trigger := range triggers {
			if trigger < 0 || int(trigger) >= len(graph.points) {
				return composition.Key{}, false
			}
			reverses = append(reverses, reverse{target: graph.points[target].key, trigger: graph.points[trigger].key})
		}
	}
	sort.Slice(reverses, func(left, right int) bool {
		if reverses[left].target != reverses[right].target {
			return lessKey(reverses[left].target, reverses[right].target)
		}
		return lessKey(reverses[left].trigger, reverses[right].trigger)
	})
	return identityKey("analysis/engine/equation/graph", func(writer *canonical.DigestWriter) bool {
		if writer.Count(uint64(len(graph.points))) != nil {
			return false
		}
		for _, point := range graph.points {
			if !writeKey(writer, point.key) {
				return false
			}
		}
		if writer.Count(uint64(len(graph.groups))) != nil {
			return false
		}
		for _, group := range graph.groups {
			if !writeKey(writer, group.key) {
				return false
			}
		}
		if len(graph.environments) != 0 {
			if writer.Uint(1) != nil || writer.Count(uint64(len(graph.environments))) != nil {
				return false
			}
			for _, edge := range graph.environments {
				if !writeKey(writer, edge.key) || !writeKey(writer, edge.target.key) || !writeKey(writer, edge.input.key) {
					return false
				}
			}
		}
		if len(graph.factorEdges) != 0 {
			if writer.Uint(2) != nil || writer.Count(uint64(len(graph.factorEdges))) != nil {
				return false
			}
			for _, edge := range graph.factorEdges {
				if !writeKey(writer, edge.key) || !writeKey(writer, edge.target.key) || !writeKey(writer, edge.input.key) || !writeKey(writer, edge.factor) {
					return false
				}
			}
		}
		if writer.Count(uint64(len(graph.queries))) != nil {
			return false
		}
		for _, query := range graph.queries {
			if !writeKey(writer, query.key) {
				return false
			}
		}
		if writer.Count(uint64(len(reverses))) != nil {
			return false
		}
		for _, row := range reverses {
			if !writeKey(writer, row.target) || !writeKey(writer, row.trigger) {
				return false
			}
		}
		return true
	})
}

func copyTopologySpec(spec TopologySpec) TopologySpec {
	result := TopologySpec{
		Batch:              spec.Batch,
		Rules:              make([]RuleInstance, len(spec.Rules)),
		Points:             append([]PointSpec(nil), spec.Points...),
		Groups:             make([]Group, len(spec.Groups)),
		Queries:            cloneQueryInstances(spec.Queries),
		EnvironmentEdges:   make([]EnvironmentEdge, len(spec.EnvironmentEdges)),
		FactorEdges:        make([]FactorEdge, len(spec.FactorEdges)),
		ActivationBindings: make([]ActivationBinding, len(spec.ActivationBindings)),
		Summaries:          make([]SummaryMapping, len(spec.Summaries)),
		WeakTargets:        make([]WeakTargetMapping, len(spec.WeakTargets)),
	}
	for index, rule := range spec.Rules {
		result.Rules[index] = copyInstance(rule)
	}
	for index, group := range spec.Groups {
		result.Groups[index] = Group{Members: append([]RuleRef(nil), group.Members...), Output: group.Output, Inputs: append([]Input(nil), group.Inputs...), EnvironmentInput: group.EnvironmentInput, premise: group.premise}
	}
	for index, edge := range spec.EnvironmentEdges {
		result.EnvironmentEdges[index] = EnvironmentEdge{Target: edge.Target, Input: edge.Input}
	}
	for index, edge := range spec.FactorEdges {
		result.FactorEdges[index] = FactorEdge{Target: edge.Target, Input: edge.Input, Factor: edge.Factor}
	}
	for index, binding := range spec.ActivationBindings {
		result.ActivationBindings[index] = ActivationBinding{
			Family:       binding.Family,
			Trigger:      binding.Trigger,
			Application:  binding.Application,
			Plan:         binding.Plan,
			PortBindings: copyPortBindings(binding.PortBindings),
		}
	}
	for index, summary := range spec.Summaries {
		result.Summaries[index] = SummaryMapping{Surface: summary.Surface, Keys: append([]uint64(nil), summary.Keys...)}
	}
	for index, mapping := range spec.WeakTargets {
		result.WeakTargets[index] = WeakTargetMapping{Surface: mapping.Surface, Candidates: append([]Surface(nil), mapping.Candidates...)}
	}
	return result
}

func cloneQueryInstances(rows []QueryInstance) []QueryInstance {
	result := make([]QueryInstance, len(rows))
	for index, row := range rows {
		result[index] = QueryInstance{Family: row.Family, Point: row.Point, Surfaces: append([]Surface(nil), row.Surfaces...)}
	}
	return result
}
