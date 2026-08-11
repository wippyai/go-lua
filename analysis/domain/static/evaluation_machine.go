package static

import (
	"errors"
	"math"
	"sort"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	linkstatic "github.com/wippyai/go-lua/program/link/static"
	programsource "github.com/wippyai/go-lua/program/source"
	programstatic "github.com/wippyai/go-lua/program/static"
	"github.com/wippyai/go-lua/program/target"
)

// evaluationMachine is a construction-time interpreter for the finite,
// already-sealed Static authority. It owns an explicit work stack; no semantic
// dependency is represented by a Go call stack, fuel counter, or budget.
// Every queued item is keyed by a finite Program/Link coordinate, and every
// active recurrence is answered by Static's existing symbolic boundary.
type evaluationMachine struct {
	authority *Authority
	operation target.Operation
	work      []evaluationWork

	evaluations map[evaluationKey]*evaluationJob
	active      map[evaluationBase]keyspace.ContentID
	contained   map[containedKey]*containedJob
	namespaces  map[namespaceKey]*namespaceJob
	err         error
}

type evaluationState uint8

const (
	evaluationQueued evaluationState = iota + 1
	evaluationActive
	evaluationComplete
)

type evaluationWorkKind uint8

const (
	workEvaluationStart evaluationWorkKind = iota + 1
	workEvaluationResume
	workContainedStart
	workContainedResume
	workNamespaceStart
	workNamespaceResume
)

type evaluationWork struct {
	kind       evaluationWorkKind
	evaluation *evaluationJob
	contained  *containedJob
	namespace  *namespaceJob
}

type evaluationInput struct {
	job       *evaluationJob
	direct    Value
	hasDirect bool
}

type containedInput struct {
	job       *containedJob
	direct    ContainedOperand
	hasDirect bool
}

type namespaceInput struct {
	job       *namespaceJob
	direct    Value
	hasDirect bool
}

type evaluationMode uint8

const (
	evaluationModeNone evaluationMode = iota
	evaluationModeAlias
	evaluationModeTypeof
	evaluationModeKeyOf
	evaluationModeIndex
	evaluationModeConditionalInputs
	evaluationModeConditionalBranch
	evaluationModeQualified
)

type evaluationJob struct {
	key         evaluationKey
	ref         typeauthority.StaticTypeRef
	resolver    linkstatic.Resolver
	environment Environment
	state       evaluationState
	mode        evaluationMode
	first       evaluationInput
	second      evaluationInput
	contained   containedInput
	branch      keyspace.Term
	value       Value
}

type containedKey struct {
	owner          keyspace.ContentID
	term           keyspace.Term
	site           keyspace.Term
	resolver       keyspace.ContentID
	frontierBody   keyspace.Term
	frontierCursor uint32
	env            keyspace.ContentID
	operation      target.Operation
}

type containedMode uint8

const (
	containedModeNone containedMode = iota
	containedModeClaim
	containedModeTypeValue
	containedModeNamespace
)

type containedJob struct {
	key         containedKey
	p           *program.Program
	owner       keyspace.ContentID
	term        keyspace.Term
	resolver    linkstatic.Resolver
	environment Environment
	state       evaluationState
	mode        containedMode
	child       containedInput
	evaluation  evaluationInput
	namespace   namespaceInput
	dependency  keyspace.ContentID
	value       ContainedOperand
}

type namespaceKey struct {
	namespace   linkstatic.Namespace
	environment keyspace.ContentID
	operation   target.Operation
}

type namespaceEntry struct {
	path  []keyspace.LiteralValue
	value evaluationInput
}

type namespaceJob struct {
	key         namespaceKey
	namespace   linkstatic.Namespace
	environment Environment
	state       evaluationState
	entries     []namespaceEntry
	value       Value
}

func newEvaluationMachine(authority *Authority, operation target.Operation) *evaluationMachine {
	return &evaluationMachine{
		authority:   authority,
		operation:   operation,
		evaluations: make(map[evaluationKey]*evaluationJob),
		active:      make(map[evaluationBase]keyspace.ContentID),
		contained:   make(map[containedKey]*containedJob),
		namespaces:  make(map[namespaceKey]*namespaceJob),
	}
}

func (a *Authority) evaluate(ref typeauthority.StaticTypeRef, resolver linkstatic.Resolver, environment Environment, operation target.Operation) (Value, error) {
	machine := newEvaluationMachine(a, operation)
	input, ok := machine.requestEvaluation(ref, resolver, environment)
	if !ok {
		return Value{}, machine.err
	}
	if err := machine.run(); err != nil {
		return Value{}, err
	}
	value, resolved := machine.evaluationValue(input)
	if !resolved {
		return Value{}, errors.New("static: incomplete evaluation")
	}
	return value, nil
}

func (m *evaluationMachine) requestEvaluation(ref typeauthority.StaticTypeRef, resolver linkstatic.Resolver, environment Environment) (evaluationInput, bool) {
	if m == nil {
		return evaluationInput{}, false
	}
	if m.authority == nil || m.authority.source == nil || !ref.Valid() || (environment.owner != nil && environment.owner != m.authority) {
		m.fail(errors.New("static: foreign evaluation coordinate"))
		return evaluationInput{}, false
	}
	shard, shardOK := m.authority.source.Static().Namespaces().ResolverShard(resolver)
	p, programOK := m.authority.source.Project().Mounts().Program(shard)
	if !shardOK || !programOK || p == nil || p.ContentID() != ref.Owner() {
		m.fail(errors.New("static: foreign evaluation owner"))
		return evaluationInput{}, false
	}
	hotReference, hotOK := p.Static().StaticTypes().Ref(ref.Root())
	if !hotOK || hotReference.Term() != ref.Root() {
		m.fail(errors.New("static: invalid evaluation reference"))
		return evaluationInput{}, false
	}
	if _, ok := m.authority.source.Static().Expressions().For(resolver, hotReference); !ok {
		m.fail(errors.New("static: invalid evaluation reference"))
		return evaluationInput{}, false
	}
	namespace, ok := m.authority.source.Static().Namespaces().ResolverContentID(resolver)
	if !ok {
		m.fail(errors.New("static: invalid resolver"))
		return evaluationInput{}, false
	}
	key := evaluationKey{reference: ref, resolver: namespace, env: environment.ContentID(), operation: m.operation}
	if value, found := m.authority.memo[key]; found {
		return evaluationInput{direct: value, hasDirect: true}, true
	}
	if job, found := m.evaluations[key]; found {
		switch job.state {
		case evaluationComplete:
			return evaluationInput{direct: job.value, hasDirect: true}, true
		case evaluationActive:
			return evaluationInput{direct: m.symbolic(Symbolic{reference: ref, sourceOwner: ref.Owner(), source: ref.Root(), namespace: namespace, environment: key.env, operation: key.operation, dependency: ref.Owner(), reason: ReasonUnresolvedProjection}), hasDirect: true}, true
		case evaluationQueued:
			return evaluationInput{job: job}, true
		default:
			m.fail(errors.New("static: invalid evaluation state"))
			return evaluationInput{}, false
		}
	}
	base := evaluationBase{reference: ref, resolver: namespace}
	if activeEnvironment, active := m.active[base]; active && activeEnvironment != key.env {
		return evaluationInput{direct: m.symbolic(Symbolic{reference: ref, sourceOwner: ref.Owner(), source: ref.Root(), namespace: namespace, environment: key.env, operation: key.operation, dependency: ref.Owner(), reason: ReasonGenerativeRecurrence}), hasDirect: true}, true
	}
	job := &evaluationJob{key: key, ref: ref, resolver: resolver, environment: environment, state: evaluationQueued}
	m.evaluations[key] = job
	m.push(evaluationWork{kind: workEvaluationStart, evaluation: job})
	return evaluationInput{job: job}, true
}

func (m *evaluationMachine) requestTerm(p *program.Program, term keyspace.Term, resolver linkstatic.Resolver, environment Environment) (evaluationInput, bool) {
	if p == nil {
		m.fail(errors.New("static: invalid term source"))
		return evaluationInput{}, false
	}
	if _, ok := p.Static().StaticTypes().Ref(term); !ok {
		m.fail(errors.New("static: invalid term source"))
		return evaluationInput{}, false
	}
	selector, ok := m.authority.types.Find(p.ContentID(), term)
	if !ok {
		m.fail(errors.New("static: invalid term source"))
		return evaluationInput{}, false
	}
	ref, ok := m.authority.types.Ref(selector)
	if !ok {
		m.fail(errors.New("static: invalid term source"))
		return evaluationInput{}, false
	}
	return m.requestEvaluation(ref, resolver, environment)
}

func (m *evaluationMachine) requestContained(p *program.Program, owner keyspace.ContentID, term, site, frontierBody keyspace.Term, frontierCursor uint32, resolver linkstatic.Resolver, environment Environment) (containedInput, bool) {
	if m == nil {
		return containedInput{}, false
	}
	if m.authority == nil || p == nil || (environment.owner != nil && environment.owner != m.authority) {
		m.fail(errors.New("static: foreign contained operand"))
		return containedInput{}, false
	}
	shard, shardOK := m.authority.source.Static().Namespaces().ResolverShard(resolver)
	indexed, indexedOK := m.authority.source.Project().Mounts().Program(shard)
	if !shardOK || !indexedOK || indexed != p || p.ContentID() != owner {
		m.fail(errors.New("static: foreign contained operand"))
		return containedInput{}, false
	}
	resolverID, ok := m.authority.source.Static().Namespaces().ResolverContentID(resolver)
	if !ok {
		m.fail(errors.New("static: invalid contained operand resolver"))
		return containedInput{}, false
	}
	if term == 0 || site == 0 || frontierBody == 0 {
		m.fail(errors.New("static: invalid contained input frontier"))
		return containedInput{}, false
	}
	key := containedKey{owner: owner, term: term, site: site, resolver: resolverID, frontierBody: frontierBody, frontierCursor: frontierCursor, env: environment.ContentID(), operation: m.operation}
	if job, found := m.contained[key]; found {
		switch job.state {
		case evaluationComplete:
			return containedInput{direct: job.value, hasDirect: true}, true
		case evaluationActive:
			return containedInput{direct: unknownOperand(m.authority, key, resolverID, ReasonUnresolvedProjection), hasDirect: true}, true
		case evaluationQueued:
			return containedInput{job: job}, true
		default:
			m.fail(errors.New("static: invalid contained operand state"))
			return containedInput{}, false
		}
	}
	job := &containedJob{key: key, p: p, owner: owner, term: term, resolver: resolver, environment: environment, state: evaluationQueued}
	m.contained[key] = job
	m.push(evaluationWork{kind: workContainedStart, contained: job})
	return containedInput{job: job}, true
}

func (m *evaluationMachine) requestNamespace(namespace linkstatic.Namespace, environment Environment) (namespaceInput, bool) {
	if m == nil {
		return namespaceInput{}, false
	}
	var zeroNamespace linkstatic.Namespace
	if m.authority == nil || m.authority.source == nil || namespace == zeroNamespace || (environment.owner != nil && environment.owner != m.authority) {
		m.fail(errors.New("static: invalid static namespace"))
		return namespaceInput{}, false
	}
	key := namespaceKey{namespace: namespace, environment: environment.ContentID(), operation: m.operation}
	if job, found := m.namespaces[key]; found {
		switch job.state {
		case evaluationComplete:
			return namespaceInput{direct: job.value, hasDirect: true}, true
		case evaluationActive:
			namespaceID, _ := m.authority.source.Static().Namespaces().ContentID(namespace)
			site, found := m.namespaceRecurrenceSite(job)
			if !found || !namespaceID.Available() {
				m.fail(errors.New("static: namespace recurrence lacks an exact source"))
				return namespaceInput{}, false
			}
			site.namespace, site.environment, site.operation = namespaceID, key.environment, key.operation
			site.dependency, site.reason = namespaceID, ReasonUnresolvedProjection
			return namespaceInput{direct: m.symbolic(site), hasDirect: true}, true
		case evaluationQueued:
			return namespaceInput{job: job}, true
		default:
			m.fail(errors.New("static: invalid static namespace state"))
			return namespaceInput{}, false
		}
	}
	job := &namespaceJob{key: key, namespace: namespace, environment: environment, state: evaluationQueued}
	m.namespaces[key] = job
	m.push(evaluationWork{kind: workNamespaceStart, namespace: job})
	return namespaceInput{job: job}, true
}

func (m *evaluationMachine) run() error {
	for len(m.work) != 0 && m.err == nil {
		index := len(m.work) - 1
		work := m.work[index]
		m.work = m.work[:index]
		switch work.kind {
		case workEvaluationStart:
			m.startEvaluation(work.evaluation)
		case workEvaluationResume:
			m.resumeEvaluation(work.evaluation)
		case workContainedStart:
			m.startContained(work.contained)
		case workContainedResume:
			m.resumeContained(work.contained)
		case workNamespaceStart:
			m.startNamespace(work.namespace)
		case workNamespaceResume:
			m.resumeNamespace(work.namespace)
		default:
			m.fail(errors.New("static: invalid evaluation work item"))
		}
	}
	if m.err != nil {
		return m.err
	}
	return nil
}

func (m *evaluationMachine) push(work evaluationWork) { m.work = append(m.work, work) }

func (m *evaluationMachine) fail(err error) {
	if m.err == nil {
		m.err = err
	}
}

func (m *evaluationMachine) symbolic(value Symbolic) Value {
	if m == nil || m.authority == nil {
		return Value{}
	}
	if value.operation == 0 {
		value.operation = m.operation
	}
	if !value.law.Available() {
		value.law = m.authority.lawID
	}
	result, err := m.authority.addSymbolic(value)
	if err != nil {
		m.fail(err)
	}
	return result
}

func (m *evaluationMachine) invalid(site Symbolic, fault Fault) Value {
	if m == nil || m.authority == nil {
		return Value{}
	}
	if site.operation == 0 {
		site.operation = m.operation
	}
	if !site.law.Available() {
		site.law = m.authority.lawID
	}
	result, err := m.authority.addInvalid(site, fault)
	if err != nil {
		m.fail(err)
	}
	return result
}

func (m *evaluationMachine) evaluationSite(job *evaluationJob) Symbolic {
	if job == nil {
		return Symbolic{}
	}
	return Symbolic{
		reference: job.ref, sourceOwner: job.ref.Owner(), source: job.ref.Root(),
		namespace: job.key.resolver, environment: job.key.env,
		operation: job.key.operation, law: m.authority.lawID, dependency: job.ref.Owner(),
	}
}

func (m *evaluationMachine) containedSite(value ContainedOperand, reference typeauthority.StaticTypeRef) Symbolic {
	return Symbolic{
		reference: reference, sourceOwner: value.owner, source: value.source,
		namespace: value.namespace, environment: value.environment,
		operation: value.operation, law: value.law, dependency: value.dependency,
	}
}

func (m *evaluationMachine) namespaceRecurrenceSite(job *namespaceJob) (Symbolic, bool) {
	if m == nil || m.authority == nil || job == nil {
		return Symbolic{}, false
	}
	for _, entry := range job.entries {
		if entry.value.job != nil {
			return m.evaluationSite(entry.value.job), true
		}
		if entry.value.hasDirect && m.authority.Owns(entry.value.direct) {
			row := m.authority.results[entry.value.direct.index]
			if row.kind == KindSymbolic || row.kind == KindInvalid {
				return row.symbolic, row.symbolic.exactOperand()
			}
		}
	}
	return Symbolic{}, false
}

func (m *evaluationMachine) startEvaluation(job *evaluationJob) {
	if job == nil || job.state != evaluationQueued {
		m.fail(errors.New("static: evaluation start is not queued"))
		return
	}
	job.state = evaluationActive
	base := evaluationBase{reference: job.ref, resolver: job.key.resolver}
	m.active[base] = job.key.env
	invalid := func(fault Fault) Value { return m.invalid(m.evaluationSite(job), fault) }
	shard, shardOK := m.authority.source.Static().Namespaces().ResolverShard(job.resolver)
	p, ok := m.authority.source.Project().Mounts().Program(shard)
	if !shardOK || !ok || p == nil || p.ContentID() != job.ref.Owner() {
		m.completeEvaluation(job, invalid(FaultReference))
		return
	}
	term := job.ref.Root()
	hotReference, hotReferenceOK := p.Static().StaticTypes().Ref(term)
	if !hotReferenceOK || hotReference.Term() != term {
		m.completeEvaluation(job, invalid(FaultReference))
		return
	}
	symbolic := func(reason Reason) Value {
		site := m.evaluationSite(job)
		site.reason = reason
		return m.symbolic(site)
	}
	if _, _, _, ok := p.Static().Declarations().TypeParams().Get(term); ok {
		m.completeEvaluation(job, symbolic(ReasonOpenFormal))
		return
	}
	if resolution, _, _, ok := p.Static().References().Get(term); ok {
		switch resolution {
		case programstatic.TypeRefUnresolved, programstatic.TypeRefCanonicalPath:
			consumer, consumerOK := m.authority.source.Static().Expressions().For(job.resolver, hotReference)
			expression, found := m.authority.source.Static().Expressions().Qualified(consumer)
			if !consumerOK || !found {
				m.completeEvaluation(job, invalid(FaultReference))
				return
			}
			hotReference, found := m.authority.source.Static().Expressions().Reference(expression)
			resolver, resolverOK := m.authority.source.Static().Expressions().Resolver(expression)
			if !found || !resolverOK {
				m.completeEvaluation(job, invalid(FaultReference))
				return
			}
			providerShard, shardOK := m.authority.source.Static().Namespaces().ResolverShard(resolver)
			provider, providerOK := m.authority.source.Project().Mounts().Program(providerShard)
			if !shardOK || !providerOK || provider == nil {
				m.completeEvaluation(job, invalid(FaultReference))
				return
			}
			selector, referenceOK := m.authority.types.Find(provider.ContentID(), hotReference.Term())
			if !referenceOK {
				m.completeEvaluation(job, invalid(FaultReference))
				return
			}
			reference, referenceOK := m.authority.types.Ref(selector)
			if !referenceOK {
				m.completeEvaluation(job, invalid(FaultReference))
				return
			}
			job.mode = evaluationModeQualified
			m.push(evaluationWork{kind: workEvaluationResume, evaluation: job})
			job.first, _ = m.requestEvaluation(reference, resolver, job.environment)
			return
		default:
			// Declaration references are resolved below by typeauthority.
		}
	}
	if value, ok := m.authority.types.Resolve(job.ref); ok {
		if typ.ContainsTypeParam(value) {
			m.completeEvaluation(job, symbolic(ReasonOpenFormal))
			return
		}
		result, err := m.authority.addClosed(value)
		if err != nil {
			m.fail(err)
			return
		}
		m.completeEvaluation(job, result)
		return
	}
	if _, target, _, _, ok := p.Static().Declarations().Aliases().Get(term); ok {
		count, _ := p.Static().Declarations().Aliases().ParamCount(term)
		if count != 0 {
			m.completeEvaluation(job, symbolic(ReasonOpenFormal))
			return
		}
		job.mode = evaluationModeAlias
		m.push(evaluationWork{kind: workEvaluationResume, evaluation: job})
		job.first, _ = m.requestTerm(p, target, job.resolver, job.environment)
		return
	}
	if _, operand, ok := p.Static().Operators().TypeOfs().Get(term); ok {
		job.mode = evaluationModeTypeof
		m.push(evaluationWork{kind: workEvaluationResume, evaluation: job})
		body, cursor, frontierOK := p.Source().Index().Frontier(term)
		if !frontierOK || body == 0 || cursor < 0 || uint64(cursor) > uint64(^uint32(0)) {
			m.completeEvaluation(job, invalid(FaultReference))
			return
		}
		job.contained, _ = m.requestContained(p, job.ref.Owner(), operand, term, body, uint32(cursor), job.resolver, job.environment)
		return
	}
	if inner, ok := p.Static().Operators().KeyOfs().Get(term); ok {
		job.mode = evaluationModeKeyOf
		m.push(evaluationWork{kind: workEvaluationResume, evaluation: job})
		job.first, _ = m.requestTerm(p, inner, job.resolver, job.environment)
		return
	}
	if object, index, ok := p.Static().Operators().IndexAccesses().Get(term); ok {
		job.mode = evaluationModeIndex
		m.push(evaluationWork{kind: workEvaluationResume, evaluation: job})
		job.first, _ = m.requestTerm(p, object, job.resolver, job.environment)
		job.second, _ = m.requestTerm(p, index, job.resolver, job.environment)
		return
	}
	if check, extends, thenTerm, otherwise, ok := p.Static().Operators().Conditionals().Get(term); ok {
		job.mode = evaluationModeConditionalInputs
		job.branch = otherwise
		m.push(evaluationWork{kind: workEvaluationResume, evaluation: job})
		job.first, _ = m.requestTerm(p, check, job.resolver, job.environment)
		job.second, _ = m.requestTerm(p, extends, job.resolver, job.environment)
		if thenTerm == 0 {
			m.fail(errors.New("static: malformed conditional branch"))
		}
		return
	}
	if _, _, _, _, _, ok := p.Static().Signatures().Assertions().Get(term); ok {
		m.completeEvaluation(job, invalid(FaultContainment))
		return
	}
	m.completeEvaluation(job, symbolic(ReasonStaticUnknown))
}

func (m *evaluationMachine) resumeEvaluation(job *evaluationJob) {
	if job == nil || job.state != evaluationActive {
		m.fail(errors.New("static: evaluation resume is not active"))
		return
	}
	invalid := func(fault Fault) Value { return m.invalid(m.evaluationSite(job), fault) }
	symbolic := func(reason Reason) Value {
		site := m.evaluationSite(job)
		site.reason = reason
		return m.symbolic(site)
	}
	switch job.mode {
	case evaluationModeAlias:
		value, ok := m.evaluationValue(job.first)
		if !ok {
			return
		}
		if residue, symbolicResult := value.Symbolic(); symbolicResult {
			residue.reference = job.ref
			m.completeEvaluation(job, m.symbolic(residue))
			return
		}
		m.completeEvaluation(job, value)
	case evaluationModeTypeof:
		operand, ok := m.containedValue(job.contained)
		if !ok {
			return
		}
		switch operand.kind {
		case OperandKnown:
			m.completeEvaluation(job, operand.known)
		case OperandRuntimeSubject:
			m.completeEvaluation(job, m.symbolic(Symbolic{reference: job.ref, sourceOwner: operand.owner, source: operand.source, namespace: operand.namespace, environment: operand.environment, operation: job.key.operation, law: operand.law, dependency: operand.dependency, reason: ReasonRuntimeSubject, subject: operand.subject}))
		case OperandUnknown:
			reason := operand.reason
			if reason == 0 {
				reason = ReasonStaticUnknown
			}
			m.completeEvaluation(job, m.symbolic(Symbolic{reference: job.ref, sourceOwner: operand.owner, source: operand.source, namespace: operand.namespace, environment: operand.environment, operation: job.key.operation, law: operand.law, dependency: operand.dependency, reason: reason}))
		case OperandInvalid:
			fault := operand.fault
			if fault == 0 {
				fault = FaultContainment
			}
			m.completeEvaluation(job, m.invalid(m.containedSite(operand, job.ref), fault))
		default:
			m.fail(errors.New("static: invalid contained operand disposition"))
		}
	case evaluationModeKeyOf:
		value, ok := m.evaluationValue(job.first)
		if !ok {
			return
		}
		if value.IsInvalid() {
			m.completeEvaluation(job, value)
			return
		}
		if !value.IsClosed() {
			m.completeEvaluation(job, symbolic(ReasonUnresolvedProjection))
			return
		}
		inner, _ := m.authority.ClosedType(value)
		projected, valid := projection.KeyOf(inner)
		if !valid {
			m.completeEvaluation(job, invalid(FaultProjection))
			return
		}
		result, err := m.authority.addClosed(projected)
		if err != nil {
			m.fail(err)
			return
		}
		m.completeEvaluation(job, result)
	case evaluationModeIndex:
		object, objectOK := m.evaluationValue(job.first)
		index, indexOK := m.evaluationValue(job.second)
		if !objectOK || !indexOK {
			return
		}
		if object.IsInvalid() {
			m.completeEvaluation(job, object)
			return
		}
		if index.IsInvalid() {
			m.completeEvaluation(job, index)
			return
		}
		if !object.IsClosed() || !index.IsClosed() {
			m.completeEvaluation(job, symbolic(ReasonUnresolvedProjection))
			return
		}
		objectType, _ := m.authority.ClosedType(object)
		indexType, _ := m.authority.ClosedType(index)
		projected, valid := access.Index(objectType, indexType)
		if !valid {
			m.completeEvaluation(job, invalid(FaultProjection))
			return
		}
		result, err := m.authority.addClosed(projected)
		if err != nil {
			m.fail(err)
			return
		}
		m.completeEvaluation(job, result)
	case evaluationModeConditionalInputs:
		check, checkOK := m.evaluationValue(job.first)
		extends, extendsOK := m.evaluationValue(job.second)
		if !checkOK || !extendsOK {
			return
		}
		if check.IsInvalid() {
			m.completeEvaluation(job, check)
			return
		}
		if extends.IsInvalid() {
			m.completeEvaluation(job, extends)
			return
		}
		if !check.IsClosed() || !extends.IsClosed() {
			m.completeEvaluation(job, symbolic(ReasonUnresolvedProjection))
			return
		}
		checkType, _ := m.authority.ClosedType(check)
		extendsType, _ := m.authority.ClosedType(extends)
		if subtype.IsSubtype(checkType, extendsType) {
			shard, shardOK := m.authority.source.Static().Namespaces().ResolverShard(job.resolver)
			p, exists := m.authority.source.Project().Mounts().Program(shard)
			if !shardOK || !exists || p == nil || p.ContentID() != job.ref.Owner() {
				m.completeEvaluation(job, invalid(FaultReference))
				return
			}
			_, _, thenTerm, _, valid := p.Static().Operators().Conditionals().Get(job.ref.Root())
			if !valid || thenTerm == 0 {
				m.fail(errors.New("static: malformed conditional branch"))
				return
			}
			job.branch = thenTerm
		}
		job.mode = evaluationModeConditionalBranch
		m.push(evaluationWork{kind: workEvaluationResume, evaluation: job})
		shard, shardOK := m.authority.source.Static().Namespaces().ResolverShard(job.resolver)
		p, exists := m.authority.source.Project().Mounts().Program(shard)
		if !shardOK || !exists || p == nil || p.ContentID() != job.ref.Owner() {
			m.completeEvaluation(job, invalid(FaultReference))
			return
		}
		job.first, _ = m.requestTerm(p, job.branch, job.resolver, job.environment)
	case evaluationModeConditionalBranch, evaluationModeQualified:
		value, ok := m.evaluationValue(job.first)
		if ok {
			m.completeEvaluation(job, value)
		}
	default:
		m.fail(errors.New("static: invalid evaluation resume mode"))
	}
}

func (m *evaluationMachine) completeEvaluation(job *evaluationJob, value Value) {
	if job == nil || job.state != evaluationActive {
		m.fail(errors.New("static: invalid evaluation completion"))
		return
	}
	if !m.authority.Owns(value) {
		m.fail(errors.New("static: evaluation completed with unowned result"))
		return
	}
	job.value = value
	job.state = evaluationComplete
	delete(m.active, evaluationBase{reference: job.ref, resolver: job.key.resolver})
	m.authority.memo[job.key] = value
}

func (m *evaluationMachine) evaluationValue(input evaluationInput) (Value, bool) {
	if input.hasDirect {
		return input.direct, true
	}
	if input.job == nil || input.job.state != evaluationComplete {
		m.fail(errors.New("static: unresolved evaluation dependency"))
		return Value{}, false
	}
	return input.job.value, true
}

func (m *evaluationMachine) startContained(job *containedJob) {
	if job == nil || job.state != evaluationQueued {
		m.fail(errors.New("static: contained operand start is not queued"))
		return
	}
	job.state = evaluationActive
	p := job.p
	term := job.term
	sourceView := p.Source()
	ordinal := keyspace.TermOrdinal(term)
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyNil:
		if ordinal != 0 {
			if literalTerm, _, ok := sourceView.Literals().Nils().At(int(ordinal - 1)); ok && literalTerm == term {
				m.completeContainedClosed(job, typ.Nil)
				return
			}
		}
	case keyspace.FamilyBool:
		if ordinal != 0 {
			if literalTerm, _, value, ok := sourceView.Literals().Bools().At(int(ordinal - 1)); ok && literalTerm == term {
				m.completeContainedClosed(job, typ.LiteralBool(value))
				return
			}
		}
	case keyspace.FamilyInteger:
		if ordinal != 0 {
			if literalTerm, _, value, ok := sourceView.Literals().Integers().At(int(ordinal - 1)); ok && literalTerm == term {
				m.completeContainedClosed(job, typ.LiteralInt(value))
				return
			}
		}
	case keyspace.FamilyFloat:
		if ordinal != 0 {
			if literalTerm, _, bits, ok := sourceView.Literals().Floats().At(int(ordinal - 1)); ok && literalTerm == term {
				value := math.Float64frombits(bits)
				if value == 0 {
					value = 0
				}
				if math.IsNaN(value) {
					m.completeContained(job, unknownOperand(m.authority, job.key, m.authority.targetID, ReasonStaticUnknown))
					return
				}
				m.completeContainedClosed(job, typ.LiteralNumber(value))
				return
			}
		}
	case keyspace.FamilyString:
		if ordinal != 0 {
			if literalTerm, _, value, ok := sourceView.Literals().Strings().At(int(ordinal - 1)); ok && literalTerm == term {
				m.completeContainedClosed(job, typ.LiteralString(value))
				return
			}
		}
	}
	flow := p.Flow()
	staticView := p.Static()
	if _, operand, _, ok := flow.Authored().Claims().Get(term); ok {
		job.mode = containedModeClaim
		m.push(evaluationWork{kind: workContainedResume, contained: job})
		job.child, _ = m.requestContained(p, job.owner, operand, job.key.site, job.key.frontierBody, job.key.frontierCursor, job.resolver, job.environment)
		return
	}
	if _, ok := flow.Authored().TypeValues().Get(term); ok {
		target, targetOK := staticView.Operands().TypeValues().Target(term)
		if !targetOK {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		job.mode = containedModeTypeValue
		m.push(evaluationWork{kind: workContainedResume, contained: job})
		job.evaluation, _ = m.requestTerm(p, target, job.resolver, job.environment)
		return
	}
	if _, source, _, ok := flow.Authored().Storage().Reads().Get(term); ok {
		shard, shardOK := m.authority.source.Static().Namespaces().ResolverShard(job.resolver)
		owner, ownerOK := m.authority.source.Project().Mounts().Program(shard)
		if !shardOK || !ownerOK || owner == nil || owner.ContentID() != job.owner {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		if _, _, _, cell := flow.Authored().Storage().Cells().Get(source); !cell {
			// A Lens needs its exact root/key/access premises. Static's frozen
			// dialect does not own such a projection yet, and it must never
			// reinterpret a Lens as a Cell-copy read.
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultContainment))
			return
		}
		boundary := m.authority.source.Boundary()
		if boundary == nil {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		value, found := boundary.Values().Of(shard, source)
		if !found {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		id, found := boundary.Values().ID(value)
		if !found {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		result := containedOperand(m.authority, job.key, id)
		result.kind = OperandRuntimeSubject
		result.subject = RuntimeSubject{linkID: m.authority.source.ContentID(), value: value, id: id, body: job.key.frontierBody, cursor: job.key.frontierCursor}
		m.completeContained(job, result)
		return
	}
	if _, _, _, _, ok := flow.Authored().Calls().Get(term); ok {
		resolution, found := m.authority.source.Static().Resolutions().ForCallInShard(job.resolver, term)
		if !found {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultContainment))
			return
		}
		disposition, found := m.authority.source.Static().Resolutions().Disposition(resolution)
		if !found || disposition == linkstatic.ResolutionUnresolved {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultContainment))
			return
		}
		if disposition != linkstatic.ResolutionResolved {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		namespace, found := m.authority.source.Static().Resolutions().Namespace(resolution)
		if !found {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		job.dependency, found = m.authority.source.Static().Namespaces().ContentID(namespace)
		if !found || !job.dependency.Available() {
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultReference))
			return
		}
		job.mode = containedModeNamespace
		m.push(evaluationWork{kind: workContainedResume, contained: job})
		job.namespace, _ = m.requestNamespace(namespace, job.environment)
		return
	}
	m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, FaultContainment))
}

func (m *evaluationMachine) resumeContained(job *containedJob) {
	if job == nil || job.state != evaluationActive {
		m.fail(errors.New("static: contained operand resume is not active"))
		return
	}
	switch job.mode {
	case containedModeClaim:
		value, ok := m.containedValue(job.child)
		if ok {
			result := containedOperand(m.authority, job.key, value.dependency)
			result.kind, result.known, result.subject = value.kind, value.known, value.subject
			result.reason, result.fault = value.reason, value.fault
			m.completeContained(job, result)
		}
	case containedModeTypeValue:
		value, ok := m.evaluationValue(job.evaluation)
		if !ok {
			return
		}
		if value.IsInvalid() {
			fault, _ := value.Fault()
			m.completeContained(job, invalidOperand(m.authority, job.key, job.owner, fault))
			return
		}
		if !value.IsClosed() {
			dependency := job.key.resolver
			if symbolic, ok := value.Symbolic(); ok && symbolic.Dependency().Available() {
				dependency = symbolic.Dependency()
			}
			m.completeContained(job, unknownOperand(m.authority, job.key, dependency, ReasonUnresolvedProjection))
			return
		}
		projected, _ := m.authority.ClosedType(value)
		m.completeContainedClosed(job, typ.NewMeta(projected))
	case containedModeNamespace:
		value, ok := m.namespaceValue(job.namespace)
		if !ok {
			return
		}
		if value.IsInvalid() {
			fault, _ := value.Fault()
			m.completeContained(job, invalidOperand(m.authority, job.key, job.dependency, fault))
			return
		}
		if !value.IsClosed() {
			m.completeContained(job, unknownOperand(m.authority, job.key, job.dependency, ReasonStaticUnknown))
			return
		}
		result := containedOperand(m.authority, job.key, job.dependency)
		result.kind, result.known = OperandKnown, value
		m.completeContained(job, result)
	default:
		m.fail(errors.New("static: invalid contained operand resume mode"))
	}
}

func (m *evaluationMachine) completeContainedClosed(job *containedJob, value typ.Type) {
	result, err := m.authority.addClosed(value)
	if err != nil {
		m.fail(err)
		return
	}
	valueResult := containedOperand(m.authority, job.key, m.authority.targetID)
	valueResult.kind, valueResult.known = OperandKnown, result
	m.completeContained(job, valueResult)
}

func (m *evaluationMachine) completeContained(job *containedJob, value ContainedOperand) {
	if job == nil || job.state != evaluationActive {
		m.fail(errors.New("static: invalid contained operand completion"))
		return
	}
	if value.kind == OperandInvalid && value.fault == 0 ||
		value.kind == OperandUnknown && value.reason == 0 ||
		value.owner != job.key.owner || value.source != job.key.term || value.site != job.key.site ||
		value.namespace != job.key.resolver || value.environment != job.key.env ||
		value.operation != job.key.operation || value.frontierBody != job.key.frontierBody || value.frontierCursor != job.key.frontierCursor || !value.law.Available() || !value.dependency.Available() {
		m.fail(errors.New("static: incomplete contained operand provenance"))
		return
	}
	job.value = value
	job.state = evaluationComplete
}

func (m *evaluationMachine) containedValue(input containedInput) (ContainedOperand, bool) {
	if input.hasDirect {
		return input.direct, true
	}
	if input.job == nil || input.job.state != evaluationComplete {
		m.fail(errors.New("static: unresolved contained operand dependency"))
		return ContainedOperand{}, false
	}
	return input.job.value, true
}

func (m *evaluationMachine) startNamespace(job *namespaceJob) {
	if job == nil || job.state != evaluationQueued {
		m.fail(errors.New("static: namespace start is not queued"))
		return
	}
	job.state = evaluationActive
	if _, ok := m.authority.source.Static().Namespaces().ContentID(job.namespace); !ok {
		m.fail(errors.New("static: namespace identity unavailable"))
		return
	}
	shard, ok := m.authority.source.Static().Namespaces().Shard(job.namespace)
	if !ok {
		m.fail(errors.New("static: namespace shard unavailable"))
		return
	}
	p, ok := m.authority.source.Project().Mounts().Program(shard)
	if !ok || p == nil {
		m.fail(errors.New("static: namespace Program unavailable"))
		return
	}
	resolver, ok := m.authority.source.Static().Namespaces().Resolver(job.namespace)
	if !ok {
		m.fail(errors.New("static: namespace resolver unavailable"))
		return
	}
	entries := make([]namespaceEntry, m.authority.source.Static().Namespaces().ExportCount(job.namespace))
	m.push(evaluationWork{kind: workNamespaceResume, namespace: job})
	for index := range entries {
		expression, valid := m.authority.source.Static().Namespaces().ExportExpression(job.namespace, index)
		if !valid {
			m.fail(errors.New("static: malformed export"))
			return
		}
		hotReference, valid := m.authority.source.Static().Expressions().Reference(expression)
		if !valid {
			m.fail(errors.New("static: malformed export expression"))
			return
		}
		selector, valid := m.authority.types.Find(p.ContentID(), hotReference.Term())
		if !valid {
			m.fail(errors.New("static: malformed export portable reference"))
			return
		}
		reference, valid := m.authority.types.Ref(selector)
		if !valid {
			m.fail(errors.New("static: malformed export portable reference"))
			return
		}
		keys, valid := m.authority.source.Static().Namespaces().ExportPath(job.namespace, index, nil)
		if !valid || len(keys) == 0 {
			m.fail(errors.New("static: malformed export path"))
			return
		}
		path := make([]keyspace.LiteralValue, len(keys))
		for at, key := range keys {
			path[at], valid = p.Source().Keys().Exact(key)
			if !valid {
				m.fail(errors.New("static: export key unavailable"))
				return
			}
		}
		entries[index].path = path
		entries[index].value, _ = m.requestEvaluation(reference, resolver, job.environment)
	}
	job.entries = entries
}

func (m *evaluationMachine) resumeNamespace(job *namespaceJob) {
	if job == nil || job.state != evaluationActive {
		m.fail(errors.New("static: namespace resume is not active"))
		return
	}
	root := &namespaceNode{children: make(map[keyspace.LiteralValue]*namespaceNode)}
	for _, entry := range job.entries {
		result, ok := m.evaluationValue(entry.value)
		if !ok {
			return
		}
		if result.IsInvalid() {
			m.completeNamespace(job, result)
			return
		}
		if !result.IsClosed() {
			m.completeNamespace(job, result)
			return
		}
		value, _ := m.authority.ClosedType(result)
		node := root
		for _, key := range entry.path {
			next := node.children[key]
			if next == nil {
				next = &namespaceNode{children: make(map[keyspace.LiteralValue]*namespaceNode)}
				node.children[key] = next
			}
			node = next
		}
		if node.value != nil {
			m.fail(errors.New("static: duplicate namespace export path"))
			return
		}
		node.value = value
	}
	materialized, valid := materializeNamespaceIterative(root)
	if !valid {
		m.fail(errors.New("static: malformed namespace export tree"))
		return
	}
	result, err := m.authority.addClosed(materialized)
	if err != nil {
		m.fail(err)
		return
	}
	m.completeNamespace(job, result)
}

func (m *evaluationMachine) completeNamespace(job *namespaceJob, value Value) {
	if job == nil || job.state != evaluationActive {
		m.fail(errors.New("static: invalid namespace completion"))
		return
	}
	if !m.authority.Owns(value) {
		m.fail(errors.New("static: namespace completed with unowned result"))
		return
	}
	job.value = value
	job.state = evaluationComplete
}

func (m *evaluationMachine) namespaceValue(input namespaceInput) (Value, bool) {
	if input.hasDirect {
		return input.direct, true
	}
	if input.job == nil || input.job.state != evaluationComplete {
		m.fail(errors.New("static: unresolved namespace dependency"))
		return Value{}, false
	}
	return input.job.value, true
}

func containedOperand(authority *Authority, key containedKey, dependency keyspace.ContentID) ContainedOperand {
	result := ContainedOperand{
		owner: key.owner, source: key.term, namespace: key.resolver,
		environment: key.env, operation: key.operation, dependency: dependency,
		site: key.site, frontierBody: key.frontierBody, frontierCursor: key.frontierCursor,
	}
	if authority != nil {
		result.law = authority.lawID
		if !result.dependency.Available() {
			result.dependency = authority.targetID
		}
	}
	return result
}

func unknownOperand(authority *Authority, key containedKey, dependency keyspace.ContentID, reason Reason) ContainedOperand {
	if reason == 0 {
		reason = ReasonStaticUnknown
	}
	result := containedOperand(authority, key, dependency)
	result.kind, result.reason = OperandUnknown, reason
	return result
}

func invalidOperand(authority *Authority, key containedKey, dependency keyspace.ContentID, fault Fault) ContainedOperand {
	if fault == 0 {
		fault = FaultContainment
	}
	result := containedOperand(authority, key, dependency)
	result.kind, result.fault = OperandInvalid, fault
	return result
}

type namespaceNode struct {
	value    typ.Type
	children map[keyspace.LiteralValue]*namespaceNode
}

type namespaceBuildFrame struct {
	node     *namespaceNode
	expanded bool
}

func materializeNamespaceIterative(root *namespaceNode) (typ.Type, bool) {
	if root == nil {
		return nil, false
	}
	values := make(map[*namespaceNode]typ.Type)
	stack := []namespaceBuildFrame{{node: root}}
	for len(stack) != 0 {
		index := len(stack) - 1
		frame := stack[index]
		stack = stack[:index]
		if frame.node == nil || (frame.node.value != nil && len(frame.node.children) != 0) {
			return nil, false
		}
		if !frame.expanded {
			if frame.node.value != nil {
				values[frame.node] = frame.node.value
				continue
			}
			stack = append(stack, namespaceBuildFrame{node: frame.node, expanded: true})
			keys, ok := sortedNamespaceKeys(frame.node)
			if !ok {
				return nil, false
			}
			for index := len(keys) - 1; index >= 0; index-- {
				stack = append(stack, namespaceBuildFrame{node: frame.node.children[keys[index]]})
			}
			continue
		}
		keys, ok := sortedNamespaceKeys(frame.node)
		if !ok {
			return nil, false
		}
		fields := make([]typ.Field, 0, len(keys))
		members := make([]typ.StaticMember, 0, len(keys))
		for _, key := range keys {
			child := frame.node.children[key]
			value, exists := values[child]
			if !exists || value == nil {
				return nil, false
			}
			switch key.Kind {
			case keyspace.LiteralString:
				fields = append(fields, typ.Field{Name: key.String, Type: value, Readonly: true})
			case keyspace.LiteralInteger:
				members = append(members, typ.StaticMember{Kind: typ.StaticMemberIntIndex, Index: key.Integer, Type: value, Readonly: true})
			default:
				return nil, false
			}
		}
		values[frame.node] = typ.RebuildRecord(typ.RecordParts{Fields: fields, StaticMembers: members, AssumeSorted: true})
	}
	value, ok := values[root]
	return value, ok && value != nil
}

func sortedNamespaceKeys(node *namespaceNode) ([]keyspace.LiteralValue, bool) {
	if node == nil {
		return nil, false
	}
	keys := make([]keyspace.LiteralValue, 0, len(node.children))
	for key := range node.children {
		if _, valid := programsource.NormalizeExactKey(key); !valid {
			return nil, false
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		order, _ := programsource.CompareExactKey(keys[left], keys[right])
		return order < 0
	})
	return keys, true
}
