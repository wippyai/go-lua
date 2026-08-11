package engine

// FrozenResult is the typed persistence contract for one Query projection.
// Its callbacks are monomorphic behavior, not cold-schema identity. A Query
// runs only after its State is complete and never feeds this result into Rule
// evaluation.
type FrozenResult[R any] struct {
	Semantic    SemanticKey
	Freeze      func(R) R
	Clone       func(R) R
	Equal       func(R, R) bool
	Fingerprint func(R) uint64
}

// Observation is the synchronous frame passed to a normal Query projector.
// It deliberately exposes neither State nor a Factor/carrier capability.
type Observation struct {
	execution *queryExecution
	epoch     uint64
}

// QueryRow is one correlated product row from an Observation. It expires as
// soon as the projector returns.
type QueryRow struct {
	execution *queryExecution
	epoch     uint64
	index     int
}

// SupportObservation is the completed structural support projection supplied
// to a support Query. It has no Factor read, unit, State lookup, or rule
// capability. Its only observation is SupportReachable below; a copied value
// expires with the synchronous projector frame.
type SupportObservation struct {
	execution *supportQueryExecution
	epoch     uint64
}

// QueryRead is the typed positional capability for one declared Factor read.
// It may be consumed only through QueryValue during the synchronous projector
// invocation that materialized its owning Query.
type QueryRead[S any] struct {
	schema  *querySchema
	index   int
	resolve func(*queryExecution, int, int) (S, bool)
}

// QuerySpec supplies the sole normal projector and persistence law. Its
// declaration callback below is the only route for adding typed read tokens.
type QuerySpec[R any] struct {
	Semantic SemanticKey
	Project  func(Observation) R
	Result   FrozenResult[R]
}

// Query is the typed cold projection capability. A normal Query owns one
// projector over all its ordered reads. A support Query owns only its separate
// structural support projector and has no Factor read vector.
type Query[R any] struct {
	composition    *Composition
	schema         *querySchema
	project        func(Observation) R
	supportProject func(SupportObservation) R
	result         FrozenResult[R]
}

type querySchema struct {
	composition *Composition
	semantic    SemanticKey
	reads       []coldQueryRead
	support     bool
	freezer     SemanticKey
	open        bool
	bind        queryBinder
	bindIndex   uint64
	bound       bool
	authority   *queryAuthority
}

// queryAuthority is the one sealed retrieval capability for a Query family.
// It deliberately retains cold schema identity and its canonical sealed slot,
// never an equation Query.  An equation Query is a per-structural-revision
// observation binding and therefore belongs only to boundQuery.
type queryAuthority struct {
	schema *querySchema
	index  uint64
}

type coldQueryRead struct {
	form *formSchema
	bind queryReadBinder
}

type coldQuery struct{ schema *querySchema }

// DeclareQuery opens exactly one typed declaration callback for an ordered
// vector of exact/summary Factor reads. The callback cannot select a Point,
// inspect State, write a Factor, or install a second projector.
func DeclareQuery[R any](composition *Composition, spec QuerySpec[R], declare func(*Query[R]) bool) (*Query[R], bool) {
	if composition == nil || !validQuerySpec(spec) || declare == nil || !composition.acceptsChild(spec.Semantic) || !composition.claim(spec.Result.Semantic) {
		if composition != nil {
			composition.poison()
		}
		return nil, false
	}
	schema := &querySchema{composition: composition, semantic: spec.Semantic, freezer: spec.Result.Semantic, open: true}
	query := &Query[R]{composition: composition, schema: schema, project: spec.Project, result: spec.Result}
	schema.bind = queryBind[R]{owner: query}
	declared := false
	func() {
		defer func() {
			if recover() != nil {
				composition.poison()
				declared = false
			}
			schema.open = false
		}()
		declared = declare(query)
	}()
	if !declared || len(schema.reads) == 0 || !composition.usable() {
		composition.poison()
		return nil, false
	}
	composition.queries = append(composition.queries, coldQuery{schema: schema})
	return query, true
}

// DeclareSupportQuery records one output-only structural support Query. Its
// retained capability is the same Query[R] retrieval authority as a normal
// Query, while its cold read vector is exactly empty.
func DeclareSupportQuery[R any](composition *Composition, semantic SemanticKey, project func(SupportObservation) R, result FrozenResult[R]) (*Query[R], bool) {
	if composition == nil || !semantic.Available() || project == nil || !validFrozenResult(result) || result.Semantic == semantic || !composition.acceptsChild(semantic) || !composition.claim(result.Semantic) {
		if composition != nil {
			composition.poison()
		}
		return nil, false
	}
	schema := &querySchema{composition: composition, semantic: semantic, support: true, freezer: result.Semantic}
	query := &Query[R]{composition: composition, schema: schema, supportProject: project, result: result}
	schema.bind = supportQueryBind[R]{owner: query}
	composition.queries = append(composition.queries, coldQuery{schema: schema})
	return query, true
}

// QueryReadFrom appends one exact or Factor-owned summary form in declaration
// order. Selector forms are intentionally not Query reads: a Query has only
// one semantic projector and may not acquire a selector decision channel.
func QueryReadFrom[V, S, R any](query *Query[R], form ReadForm[V, S]) (QueryRead[S], bool) {
	if !query.validOpen() || !form.valid() || form.schema.factor.composition != query.composition || (form.schema.readKind != exactReadForm && form.schema.readKind != summaryReadForm) {
		query.poison()
		return QueryRead[S]{}, false
	}
	index := len(query.schema.reads)
	read := QueryRead[S]{schema: query.schema, index: index, resolve: resolveTypedQueryValue[V, S]}
	query.schema.reads = append(query.schema.reads, coldQueryRead{form: form.schema, bind: queryReadBind[V, S, R]{read: read, form: form}})
	return read, true
}

// ProjectRows visits every nonempty correlated product row exactly once. A
// second invocation, an escaped Observation, or a failed visit is rejected.
func ProjectRows(observation Observation, visit func(QueryRow) bool) bool {
	execution := observation.execution
	if execution == nil || visit == nil || observation.epoch == 0 || execution.active.Load() != observation.epoch || !execution.requireCheckpoint() || !execution.ready || !execution.started.CompareAndSwap(false, true) {
		if execution != nil {
			execution.failed.Store(true)
		}
		return false
	}
	for index := range execution.values {
		if !execution.requireCheckpoint() || !visit(QueryRow{execution: execution, epoch: observation.epoch, index: index}) || !execution.requireCheckpoint() {
			execution.failed.Store(true)
			return false
		}
	}
	return true
}

// QueryValue resolves one typed token from one live correlated row. Tokens
// are positional and capability-scoped, so a foreign Query, stale row, or
// token/form mismatch fails closed.
func QueryValue[S any](row QueryRow, read QueryRead[S]) (S, bool) {
	var zero S
	execution := row.execution
	if execution == nil || row.epoch == 0 || execution.active.Load() != row.epoch || !execution.requireCheckpoint() || read.schema == nil || read.schema != execution.schema || read.index < 0 || read.index >= len(execution.reads) || read.resolve == nil || row.index < 0 || row.index >= len(execution.values) {
		if execution != nil {
			execution.failed.Store(true)
		}
		return zero, false
	}
	value, ok := read.resolve(execution, row.index, read.index)
	if !ok || !execution.requireCheckpoint() {
		execution.failed.Store(true)
		return zero, false
	}
	return value, true
}

// SupportReachable reports the sole completed structural projection available
// to a support Query: whether its exact queried Point has nonempty shared
// support. It is not a Guard, a Factor read, or a reachability fact. The
// projection is available only to the live synchronous projector; a foreign
// or expired observation fails closed.
func SupportReachable(observation SupportObservation) (bool, bool) {
	execution := observation.execution
	if execution == nil || observation.epoch == 0 || execution.epoch != observation.epoch || execution.active.Load() != observation.epoch || !execution.checkpoint() {
		return false, false
	}
	return execution.reachable, true
}

func validQuerySpec[R any](spec QuerySpec[R]) bool {
	return spec.Semantic.Available() && spec.Project != nil && validFrozenResult(spec.Result) && spec.Result.Semantic != spec.Semantic
}

func validFrozenResult[R any](result FrozenResult[R]) bool {
	return result.Semantic.Available() && result.Freeze != nil && result.Clone != nil && result.Equal != nil && result.Fingerprint != nil
}

func (query *Query[R]) validOpen() bool {
	return query != nil && query.composition != nil && query.schema != nil && !query.schema.support && query.schema.composition == query.composition && query.schema.open && query.composition.usable()
}

func (query *Query[R]) poison() {
	if query != nil && query.composition != nil {
		query.composition.poison()
	}
}

func validQueryAuthority(authority *queryAuthority) bool {
	return authority != nil && authority.schema != nil && authority.schema.authority == authority &&
		authority.schema.bound && authority.schema.bindIndex == authority.index &&
		authority.schema.composition != nil && authority.schema.composition.Sealed()
}

// sealedQueryAuthority returns only the immutable schema authority created at
// Seal.  A typed Query never learns an equation-instance identity, so the
// same capability can bind a later structural revision without changing its
// result slot or invalidating completed States.
func sealedQueryAuthority[R any](query *Query[R]) *queryAuthority {
	if query == nil || query.composition == nil || query.schema == nil || query.schema.composition != query.composition || !query.composition.Sealed() {
		return nil
	}
	authority := query.schema.authority
	if !validQueryAuthority(authority) || authority.schema != query.schema {
		return nil
	}
	return authority
}

func validColdQuery(composition *Composition, query coldQuery) bool {
	schema := query.schema
	if composition == nil || schema == nil || schema.composition != composition || schema.open || !schema.semantic.Available() || !schema.freezer.Available() || schema.freezer == schema.semantic || !validQueryBind(schema) {
		return false
	}
	if schema.support {
		return len(schema.reads) == 0
	}
	if len(schema.reads) == 0 {
		return false
	}
	for _, read := range schema.reads {
		if read.form == nil || read.form.factor == nil || read.form.factor.composition != composition || read.form.writeKind != 0 || !read.form.semantic.Available() || !read.form.factor.hasForm(read.form) || (read.form.readKind != exactReadForm && read.form.readKind != summaryReadForm) {
			return false
		}
	}
	return true
}
