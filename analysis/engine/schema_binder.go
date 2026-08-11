package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

// Binders retain cold typed ownership on sealed schemas. They are private and
// inert: the later sole Program/Link compiler may use them to construct the
// runtime, but declaration and Seal never invoke transfer/projector behavior.

type factorBinder interface {
	validFactorBind(*factorSchema) bool
	// bindRuntimeFactor is the one cold cut from a sealed Factor schema and
	// the compiled equation catalog to concrete carrier handles.  It is used
	// only by the sole Program/Link compiler; no caller supplies a parallel
	// materialization plan.
	bindRuntimeFactor(*factorSchema, *runtimeBinding) (runtimeFactor, bool)
}

type factorBind[K ~uint32 | ~uint64, V any] struct{ owner *Factor[K, V] }

func (bind factorBind[K, V]) validFactorBind(schema *factorSchema) bool {
	if bind.owner == nil || bind.owner.schema != schema || !bind.owner.valid() || len(bind.owner.formBinds) != len(schema.forms) {
		return false
	}
	for _, form := range schema.forms {
		formBind, present := bind.owner.formBinds[form]
		if !present || formBind == nil || !formBind.validFormBind(form) {
			return false
		}
	}
	return true
}

func (bind factorBind[K, V]) bindRuntimeFactor(schema *factorSchema, runtime *runtimeBinding) (runtimeFactor, bool) {
	if !bind.validFactorBind(schema) || bind.owner == nil || bind.owner.schema != schema {
		return nil, false
	}
	bound, ok := bindFactorFromGraph(bind.owner, runtime)
	if !ok {
		return nil, false
	}
	return bound, true
}

type formBinder interface {
	validFormBind(*formSchema) bool
}

type readFormBind[V, S any] struct{ form ReadForm[V, S] }

func (bind readFormBind[V, S]) validFormBind(schema *formSchema) bool {
	return bind.form.schema == schema && bind.form.valid()
}

type writeFormBind[V any] struct{ form WriteForm[V] }

func (bind writeFormBind[V]) validFormBind(schema *formSchema) bool {
	return bind.form.schema == schema && bind.form.valid()
}

type ruleBinder interface {
	validRuleBind(*ruleSchema) bool
	bindStructuralMember(*ruleSchema, equation.RuleMember, map[composition.Key]runtimeFactor, *equation.Topology, composition.Key, *equation.Graph) (runtimeMember, bool)
}

type factorRuleBind[V, O any] struct {
	owner *Rule[V, O]
}

func (bind *factorRuleBind[V, O]) validRuleBind(schema *ruleSchema) bool {
	if bind.owner == nil || bind.owner.schema != schema || !bind.owner.available() || bind.owner.transfer == nil || !bind.owner.admission.same(schema.admission) || schema.outputKind != ruleFactorOutput {
		return false
	}
	if len(schema.carries) != 1 || !schema.carries[0].transform.Available() {
		return !bind.owner.carryTransform.Available() && bind.owner.carryApply == nil
	}
	return bind.owner.carryTransform == schema.carries[0].transform && bind.owner.carryApply != nil
}

func (bind *factorRuleBind[V, O]) bindStructuralMember(*ruleSchema, equation.RuleMember, map[composition.Key]runtimeFactor, *equation.Topology, composition.Key, *equation.Graph) (runtimeMember, bool) {
	// A Factor Rule has a typed per-topology O.  Only topologyOperands may
	// install it, after Graph(accepted) issued the exact Member.  Keeping this
	// route closed prevents a Rule-global payload registry or a default O.
	return nil, false
}

type supportRuleBind struct{ owner *SupportRule }

func (bind supportRuleBind) validRuleBind(schema *ruleSchema) bool {
	return bind.owner != nil && bind.owner.schema == schema && bind.owner.available() &&
		bind.owner.run != nil && bind.owner.admission.same(schema.admission) && schema.outputKind == ruleStructuralOutput && schema.support != nil
}

func (bind supportRuleBind) bindStructuralMember(schema *ruleSchema, member equation.RuleMember, factors map[composition.Key]runtimeFactor, _ *equation.Topology, _ composition.Key, _ *equation.Graph) (runtimeMember, bool) {
	if !bind.validRuleBind(schema) {
		return nil, false
	}
	row, ok := bindSupportMember(member, bind.owner)
	if !ok || !bindRuntimeRuleReads(schema, row.rule, member, factors) {
		return nil, false
	}
	return row, true
}

type activationRuleBind struct{ owner *ActivationRule }

func (bind activationRuleBind) validRuleBind(schema *ruleSchema) bool {
	return bind.owner != nil && bind.owner.schema == schema && bind.owner.available() &&
		bind.owner.admission.same(schema.admission) && bind.owner.run != nil && schema.outputKind == ruleStructuralOutput && schema.activation != nil
}

func (bind activationRuleBind) bindStructuralMember(schema *ruleSchema, member equation.RuleMember, factors map[composition.Key]runtimeFactor, topology *equation.Topology, trigger composition.Key, graph *equation.Graph) (runtimeMember, bool) {
	if !bind.validRuleBind(schema) || topology == nil || graph == nil || !trigger.Available() || trigger != member.Key() {
		return nil, false
	}
	row, ok := bindActivationMember(member, bind.owner, topology, trigger, graph)
	if !ok || !bindRuntimeRuleReads(schema, row.rule, member, factors) {
		return nil, false
	}
	return row, true
}

type readBinder interface {
	validReadBind(*ruleSchema, int, coldRead) bool
	bindRuntimeRead(*ruleSchema, readBinding, equation.RuleMember, runtimeFactor) bool
}

type ruleReadBind[V, S any] struct {
	read Read[S]
	form ReadForm[V, S]
}

func (bind ruleReadBind[V, S]) validReadBind(schema *ruleSchema, index int, declared coldRead) bool {
	return bind.read.rule == schema && bind.read.index == index && bind.read.input.rule == schema &&
		bind.read.input.index == declared.input && bind.read.resolve != nil && bind.form.schema == declared.form && bind.form.valid()
}

func (bind ruleReadBind[V, S]) bindRuntimeRead(schema *ruleSchema, target readBinding, member equation.RuleMember, factor runtimeFactor) bool {
	if schema == nil || bind.read.index < 0 || bind.read.index >= len(schema.reads) {
		return false
	}
	declared := schema.reads[bind.read.index]
	return bind.validReadBind(schema, bind.read.index, declared) && bind.form.bindRule != nil && bind.form.bindRule(target, member, bind.read, factor)
}

// stagedReadBind retains the typed target observation and tag transport for a
// dynamic exact-read node. The target Factor is recovered through the narrow
// stagedFactor[V] interface at bind time; no raw key or candidate vector
// crosses this boundary.
type stagedReadBind[V, S any, Tag selectionTag] struct {
	read Read[Selection[Tag, S]]
	form ReadForm[V, S]
}

func (bind stagedReadBind[V, S, Tag]) validReadBind(schema *ruleSchema, index int, declared coldRead) bool {
	return bind.read.rule == schema && bind.read.index == index && bind.read.input.rule == schema &&
		bind.read.input.index == declared.input && bind.read.resolve != nil && bind.form.schema == declared.form &&
		bind.form.valid() && declared.form != nil && declared.form.readKind == exactReadForm && len(declared.depends) != 0
}

func (bind stagedReadBind[V, S, Tag]) bindRuntimeRead(schema *ruleSchema, target readBinding, member equation.RuleMember, factor runtimeFactor) bool {
	if schema == nil || bind.read.index < 0 || bind.read.index >= len(schema.reads) {
		return false
	}
	declared := schema.reads[bind.read.index]
	selector := coldReadSelectorForRead(schema, bind.read.index)
	targetFactor, ok := factor.(stagedFactor[V])
	if !bind.validReadBind(schema, bind.read.index, declared) || !ok || selector == nil || selector.bind == nil || !sameDependencies(declared.depends, selector.depends) {
		return false
	}
	locate, bound := selector.bind(target)
	if !bound || locate == nil {
		return false
	}
	return bindMemberStagedRead(target, member, bind.read, targetFactor, bind.form, selector, locate)
}

type queryBinder interface {
	validQueryBind(*querySchema) bool
	bindRuntimeQuery(*querySchema, equation.Query, map[composition.Key]runtimeFactor) (runtimeQuery, bool)
}

type queryBind[R any] struct{ owner *Query[R] }

func (bind queryBind[R]) validQueryBind(schema *querySchema) bool {
	return bind.owner != nil && bind.owner.schema == schema && bind.owner.composition != nil &&
		bind.owner.composition.available() && !schema.support && bind.owner.project != nil && validFrozenResult(bind.owner.result)
}

func (bind queryBind[R]) bindRuntimeQuery(schema *querySchema, identity equation.Query, factors map[composition.Key]runtimeFactor) (runtimeQuery, bool) {
	if !bind.validQueryBind(schema) || len(identity.Surfaces()) != len(schema.reads) || factors == nil {
		return nil, false
	}
	reads := make([]queryReadRuntime, len(schema.reads))
	for index, surface := range identity.Surfaces() {
		if !surface.Available() || schema.reads[index].bind == nil {
			return nil, false
		}
		factor, ok := factors[surface.Factor]
		if !ok || factor == nil {
			return nil, false
		}
		read, ok := schema.reads[index].bind.bindRuntimeRead(schema, identity, factor, surface)
		if !ok || read == nil {
			return nil, false
		}
		reads[index] = read
	}
	return bindQuery(identity, bind.owner, reads)
}

type supportQueryBind[R any] struct{ owner *Query[R] }

func (bind supportQueryBind[R]) validQueryBind(schema *querySchema) bool {
	return bind.owner != nil && bind.owner.schema == schema && bind.owner.composition != nil &&
		bind.owner.composition.available() && schema.support && bind.owner.supportProject != nil && validFrozenResult(bind.owner.result)
}

func (bind supportQueryBind[R]) bindRuntimeQuery(schema *querySchema, identity equation.Query, _ map[composition.Key]runtimeFactor) (runtimeQuery, bool) {
	if !bind.validQueryBind(schema) {
		return nil, false
	}
	return bindSupportQuery(identity, bind.owner)
}

type queryReadBinder interface {
	validQueryReadBind(*querySchema, int, coldQueryRead) bool
	bindRuntimeRead(*querySchema, equation.Query, runtimeFactor, equation.Surface) (queryReadRuntime, bool)
}

type queryReadBind[V, S, R any] struct {
	read QueryRead[S]
	form ReadForm[V, S]
}

func (bind queryReadBind[V, S, R]) validQueryReadBind(schema *querySchema, index int, declared coldQueryRead) bool {
	return bind.read.schema == schema && bind.read.index == index && bind.read.resolve != nil &&
		bind.form.schema == declared.form && bind.form.valid()
}

func (bind queryReadBind[V, S, R]) bindRuntimeRead(schema *querySchema, _ equation.Query, factor runtimeFactor, surface equation.Surface) (queryReadRuntime, bool) {
	if !bind.validQueryReadBind(schema, bind.read.index, coldQueryRead{form: bind.form.schema, bind: bind}) || bind.form.bindQuery == nil {
		return nil, false
	}
	return bind.form.bindQuery(schema, bind.read, factor, surface)
}

func validFactorBind(schema *factorSchema) bool {
	return schema != nil && schema.bind != nil && schema.bind.validFactorBind(schema)
}

// bindRuntimeFactor is deliberately schema-owned: the graph determines every
// resolved surface and target discipline, while the Factor binder retains the
// only typed K/V conversion and carrier declaration authority.
func bindRuntimeFactor(schema *factorSchema, runtime *runtimeBinding) (runtimeFactor, bool) {
	if schema == nil || !schema.bound || schema.bind == nil || !schema.bind.validFactorBind(schema) || runtime == nil || !runtime.valid() {
		return nil, false
	}
	return schema.bind.bindRuntimeFactor(schema, runtime)
}

func validRuleBind(schema *ruleSchema) bool {
	if schema == nil || schema.bind == nil || !schema.bind.validRuleBind(schema) {
		return false
	}
	for index, read := range schema.reads {
		if read.bind == nil || !read.bind.validReadBind(schema, index, read) {
			return false
		}
	}
	return true
}

func validQueryBind(schema *querySchema) bool {
	if schema == nil || schema.bind == nil || !schema.bind.validQueryBind(schema) {
		return false
	}
	for index, read := range schema.reads {
		if read.bind == nil || !read.bind.validQueryReadBind(schema, index, read) {
			return false
		}
	}
	return true
}
