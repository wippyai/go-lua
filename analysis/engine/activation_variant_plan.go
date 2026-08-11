package engine

import (
	"bytes"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// activationVariantPlan is the engine half of one shared equation VariantPlan.
// Equation owns target/endpoint membership and immutable fragment shape; this
// wrapper owns the one typed O payload for every Factor-output prototype row.
// It is never a callback or per-Member materializer.
type activationVariantPlan struct {
	source         *Composition
	sourceAssembly *SourceAssembly
	plan           equation.VariantPlan
	family         SemanticKey
	endpoints      []activationPlanEndpoint
	expected       map[composition.Key]struct{}
	payloads       map[composition.Key]activationPrototypePayload
	mu             sync.Mutex
	assembly       *Assembly
}

// activationPlanEndpoint is retained solely so the source owner can build a
// static route table without reopening equation's sealed VariantPlan.  It is
// not an activation tuple and never contains an Application.
type activationPlanEndpoint struct {
	target   SemanticKey
	endpoint SemanticKey
}

type activationPrototypeOrigin struct {
	schema     composition.Key
	occurrence equation.Occurrence
	operand    equation.Operand
}

type activationPrototypePayload interface {
	prototypeComposition() *Composition
	prototypeState() *ruleInstanceState
	canClaimPrototype(*activationVariantPlan) bool
	claimPrototypeLocked(*activationVariantPlan)
	validatePrototype(*activationVariantPlan, *Assembly) bool
	bindPrototype(*activationVariantPlan, *Assembly) bool
	prototypeRow() equation.PrototypeRow
}

type typedActivationPrototypePayload[V, O any] struct {
	instance *RuleInstance[V, O]
	row      equation.PrototypeRow
}

// newActivationVariantPlan is the sole engine constructor for a shared
// target-indexed plan. Payloads are attached afterwards by authenticated
// PrototypeRow identity; declaration position is never a payload authority.
// Structural rows require no payload. Template ports are symbolic and are
// resolved only by the trigger attachment.
func newActivationVariantPlan(source *Composition, family ActivationFamily, variants []equation.Variant) (*activationVariantPlan, bool) {
	if source == nil || !source.Sealed() || family.schema == nil || family.schema.composition != source || !family.available() || len(variants) == 0 {
		return nil, false
	}
	canonicalVariants := make([]equation.Variant, len(variants))
	endpoints := make([]activationPlanEndpoint, len(variants))
	for index, spec := range variants {
		if !spec.Target.Available() || !spec.Endpoint.Available() {
			return nil, false
		}
		template := spec.Template
		template.Rules = append([]equation.RuleInstance(nil), spec.Template.Rules...)
		for rowIndex := range template.Rules {
			row := &template.Rules[rowIndex]
			schema, found := source.ruleAt(row.Schema)
			if !found || row.OperandFamily.Available() && row.OperandFamily != schema.operandFamily.compositionKey() {
				return nil, false
			}
			row.OperandFamily = schema.operandFamily.compositionKey()
			switch schema.outputKind {
			case ruleFactorOutput:
			case ruleStructuralOutput:
			default:
				return nil, false
			}
		}
		canonicalVariants[index] = equation.Variant{Target: spec.Target, Endpoint: spec.Endpoint, Template: template}
		endpoints[index] = activationPlanEndpoint{target: semanticKeyFromComposition(spec.Target), endpoint: semanticKeyFromComposition(spec.Endpoint)}
	}
	plan, planned := equation.NewVariantPlan(source.coldComposition(), family.schema.semantic.compositionKey(), canonicalVariants)
	if !planned {
		return nil, false
	}
	origins := make([]activationPrototypeOrigin, 0)
	for _, spec := range canonicalVariants {
		for _, row := range plan.PrototypeRows(spec.Target, spec.Endpoint) {
			schema, found := source.ruleAt(row.Schema())
			if !found {
				return nil, false
			}
			if schema.outputKind == ruleFactorOutput {
				origins = append(origins, activationPrototypeOrigin{schema: row.Schema(), occurrence: row.Occurrence(), operand: row.Operand()})
			}
		}
	}
	sort.Slice(origins, func(left, right int) bool {
		if origins[left].schema != origins[right].schema {
			return lessCompositionKey(origins[left].schema, origins[right].schema)
		}
		return lessOperandOrigin(origins[left].occurrence, origins[left].operand, origins[right].occurrence, origins[right].operand)
	})
	for index := 1; index < len(origins); index++ {
		if origins[index-1].schema == origins[index].schema &&
			sameOperandOrigin(origins[index-1].occurrence, origins[index-1].operand, origins[index].occurrence, origins[index].operand) {
			return nil, false
		}
	}
	result := &activationVariantPlan{
		source:    source,
		plan:      plan,
		family:    family.schema.semantic,
		endpoints: endpoints,
		expected:  make(map[composition.Key]struct{}),
		payloads:  make(map[composition.Key]activationPrototypePayload),
	}
	for _, spec := range canonicalVariants {
		for _, row := range plan.PrototypeRows(spec.Target, spec.Endpoint) {
			schema, found := source.ruleAt(row.Schema())
			if !found {
				return nil, false
			}
			if schema.outputKind == ruleFactorOutput {
				if _, duplicate := result.expected[row.Key()]; duplicate {
					return nil, false
				}
				result.expected[row.Key()] = struct{}{}
			}
		}
	}
	sort.Slice(result.endpoints, func(left, right int) bool {
		if compareSemanticKey(result.endpoints[left].target, result.endpoints[right].target) != 0 {
			return compareSemanticKey(result.endpoints[left].target, result.endpoints[right].target) < 0
		}
		return compareSemanticKey(result.endpoints[left].endpoint, result.endpoints[right].endpoint) < 0
	})
	return result, true
}

func (payload *typedActivationPrototypePayload[V, O]) prototypeComposition() *Composition {
	if payload == nil || payload.instance == nil || payload.instance.rule == nil {
		return nil
	}
	return payload.instance.rule.composition
}

func (payload *typedActivationPrototypePayload[V, O]) prototypeState() *ruleInstanceState {
	if payload == nil || payload.instance == nil {
		return nil
	}
	return payload.instance.state
}

// canClaimPrototype requires the instance admission lock to be held.
func (payload *typedActivationPrototypePayload[V, O]) canClaimPrototype(plan *activationVariantPlan) bool {
	if payload == nil || payload.instance == nil || payload.instance.state == nil || plan == nil || plan.source == nil || payload.prototypeComposition() != plan.source || !payload.row.Available() ||
		!plan.plan.OwnsPrototypeRow(payload.row) {
		return false
	}
	state := payload.instance.state
	if state.admit == nil || !state.admit.occurrence.Same(payload.row.Occurrence()) || !state.admit.operand.Same(payload.row.Operand()) ||
		state.activationPlan != nil || state.activationRow.Available() {
		return false
	}
	return true
}

// claimPrototypeLocked requires canClaimPrototype to have succeeded while the
// instance admission lock remains held.
func (payload *typedActivationPrototypePayload[V, O]) claimPrototypeLocked(plan *activationVariantPlan) {
	state := payload.instance.state
	state.activationPlan = plan
	state.activationRow = payload.row.Key()
}

func (payload *typedActivationPrototypePayload[V, O]) validatePrototype(plan *activationVariantPlan, assembly *Assembly) bool {
	if payload == nil || payload.instance == nil || payload.instance.state == nil || payload.instance.kind != ruleInstanceActivation || payload.instance.declare == nil ||
		payload.instance.rule == nil || payload.instance.rule.schema == nil || plan == nil || plan.source == nil || assembly == nil || !validAssembly(assembly) || payload.instance.rule.composition != assembly.composition ||
		!payload.instance.rule.available() || !payload.instance.rule.schema.bound || payload.instance.rule.schema.outputKind != ruleFactorOutput || payload.instance.rule.schema.output == nil || payload.instance.rule.schema.support != nil || payload.instance.rule.schema.activation != nil ||
		payload.instance.state.used.Load() {
		return false
	}
	rule := payload.instance.rule
	if !payload.row.Available() || payload.row.Schema() != rule.schema.semantic.compositionKey() || payload.row.OperandFamily().Available() && payload.row.OperandFamily() != rule.schema.operandFamily.compositionKey() ||
		!matchesInstanceAdmission(payload.instance, assembly.batch, payload.row.Occurrence(), payload.row.Operand()) {
		return false
	}
	_, content, contentOK := canonicalInstanceOperand(payload.instance)
	if !contentOK || !(OperandEntity{key: payload.row.Operand().Entity()}).MatchesContentDigest(content) {
		return false
	}
	state := payload.instance.state
	state.admitMu.Lock()
	defer state.admitMu.Unlock()
	return state.activationPlan == plan && state.activationPlan.source == assembly.composition && plan.plan.OwnsPrototypeRow(payload.row) &&
		state.activationRow == payload.row.Key() && state.activationPrototype != nil && payload.row.MatchesExact(*state.activationPrototype) && payload.row.MatchesPortReads(state.activationPrototypePorts) &&
		state.admit != nil && state.admit.occurrence.Same(payload.row.Occurrence()) && state.admit.operand.Same(payload.row.Operand())
}

// activationPrototypePayloadFor binds a pre-issued typed activation instance
// to one authenticated Template row. It is intentionally internal: Program
// lowering obtains row from the sealed VariantPlan, and domains never
// manufacture a generic source capability.
func activationPrototypePayloadFor[V, O any](instance *RuleInstance[V, O], row equation.PrototypeRow) activationPrototypePayload {
	return &typedActivationPrototypePayload[V, O]{instance: instance, row: row}
}

func (payload *typedActivationPrototypePayload[V, O]) bindPrototype(plan *activationVariantPlan, assembly *Assembly) bool {
	if !payload.validatePrototype(plan, assembly) {
		return false
	}
	if !payload.instance.state.used.CompareAndSwap(false, true) {
		return false
	}
	rule := payload.instance.rule
	frozen, content, contentOK := canonicalInstanceOperand(payload.instance)
	if !contentOK || !(OperandEntity{key: payload.row.Operand().Entity()}).MatchesContentDigest(content) {
		return false
	}
	return addTopologyOperand(&assembly.operands, rule, payload.row.Occurrence(), payload.row.Operand(), frozen)
}

func (payload *typedActivationPrototypePayload[V, O]) prototypeRow() equation.PrototypeRow {
	if payload == nil {
		return equation.PrototypeRow{}
	}
	return payload.row
}

// attachPrototypePayloads authenticates and claims the complete payload set
// while the plan and every instance admission remain locked.  All claims are
// checked before the first plan or instance field is published.
func (plan *activationVariantPlan) attachPrototypePayloads(payloads []activationPrototypePayload) bool {
	if plan == nil || plan.source == nil {
		return false
	}
	plan.mu.Lock()
	defer plan.mu.Unlock()
	if plan.assembly != nil || len(plan.payloads) != 0 || len(payloads) != len(plan.expected) {
		return false
	}
	ordered := append([]activationPrototypePayload(nil), payloads...)
	sort.Slice(ordered, func(left, right int) bool {
		return lessActivationPayloadAdmission(ordered[left], ordered[right])
	})
	seenRows := make(map[composition.Key]struct{}, len(ordered))
	seenStates := make(map[*ruleInstanceState]struct{}, len(ordered))
	for _, payload := range ordered {
		if payload == nil || payload.prototypeComposition() != plan.source {
			return false
		}
		row := payload.prototypeRow()
		state := payload.prototypeState()
		if !row.Available() || !plan.plan.OwnsPrototypeRow(row) || state == nil {
			return false
		}
		if _, expected := plan.expected[row.Key()]; !expected {
			return false
		}
		if _, duplicate := seenRows[row.Key()]; duplicate {
			return false
		}
		if _, duplicate := seenStates[state]; duplicate {
			return false
		}
		seenRows[row.Key()] = struct{}{}
		seenStates[state] = struct{}{}
	}
	for _, payload := range ordered {
		payload.prototypeState().admitMu.Lock()
	}
	defer func() {
		for index := len(ordered) - 1; index >= 0; index-- {
			ordered[index].prototypeState().admitMu.Unlock()
		}
	}()
	for _, payload := range ordered {
		if !payload.canClaimPrototype(plan) {
			return false
		}
	}
	for _, payload := range ordered {
		payload.claimPrototypeLocked(plan)
		plan.payloads[payload.prototypeRow().Key()] = payload
	}
	return true
}

// lessActivationPayloadAdmission uses the same canonical source/operand/rule
// order as complete Batch admission. Holding instance admission locks in that
// common order keeps a concurrent stale admission attempt from inverting the
// plan attachment lock order.
func lessActivationPayloadAdmission(left, right activationPrototypePayload) bool {
	leftRow, rightRow := left.prototypeRow(), right.prototypeRow()
	leftSource, rightSource := leftRow.Occurrence().Site().Source(), rightRow.Occurrence().Site().Source()
	if leftSource != rightSource {
		return lessCompositionKey(leftSource, rightSource)
	}
	leftOperand, rightOperand := leftRow.Operand().Entity(), rightRow.Operand().Entity()
	if leftOperand != rightOperand {
		return lessCompositionKey(leftOperand, rightOperand)
	}
	if leftRow.Schema() != rightRow.Schema() {
		return lessCompositionKey(leftRow.Schema(), rightRow.Schema())
	}
	return lessCompositionKey(leftRow.Key(), rightRow.Key())
}

func (plan *activationVariantPlan) bind(assembly *Assembly) bool {
	if plan == nil || plan.source == nil || plan.plan == (equation.VariantPlan{}) || assembly == nil || !validAssembly(assembly) || assembly.composition != plan.source ||
		plan.sourceAssembly != nil && assembly.sourceAssembly != plan.sourceAssembly {
		return false
	}
	plan.mu.Lock()
	defer plan.mu.Unlock()
	if plan.assembly != nil {
		return plan.assembly == assembly
	}
	if len(plan.payloads) != len(plan.expected) {
		return false
	}
	rows := make([]composition.Key, 0, len(plan.expected))
	for row := range plan.expected {
		rows = append(rows, row)
	}
	sort.Slice(rows, func(left, right int) bool { return lessCompositionKey(rows[left], rows[right]) })
	for _, row := range rows {
		payload := plan.payloads[row]
		if payload == nil || payload.prototypeComposition() != plan.source || !payload.validatePrototype(plan, assembly) {
			return false
		}
	}
	for _, row := range rows {
		payload := plan.payloads[row]
		if !payload.bindPrototype(plan, assembly) {
			return false
		}
	}
	plan.assembly = assembly
	return true
}

func lessCompositionKey(left, right composition.Key) bool {
	if left.ID != right.ID {
		return bytes.Compare(left.ID[:], right.ID[:]) < 0
	}
	return left.Version < right.Version
}
