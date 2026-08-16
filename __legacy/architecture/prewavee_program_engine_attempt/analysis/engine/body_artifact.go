package engine

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/link"
)

// EquationCache emits one canonical, module-local derived section after the
// ordinary Solver seal.  It is intentionally unavailable unless every Factor
// and Rule that contributes to the selected shard supplied an exact semantic
// key.  Missing identity is a clean cache miss, never a guessed declaration
// order or closure-derived key.
func (solver *Solver) EquationCache(shard link.Shard) (artifact.EquationCache, bool) {
	if solver == nil || !solver.sealed || solver.link == nil || shard == 0 {
		return artifact.EquationCache{}, false
	}
	p, ok := solver.link.Program(shard)
	if !ok || p == nil {
		return artifact.EquationCache{}, false
	}
	admission, ok := solver.cacheAdmission()
	if !ok || !admission.factorsValid {
		return artifact.EquationCache{}, false
	}
	module := solver.link.ModuleKey(shard)
	rows, ok := admission.rules[module]
	if !ok || !rows.valid {
		return artifact.EquationCache{}, false
	}
	bodies, ok := artifact.CanonicalEquationBodies(p)
	if !ok {
		return artifact.EquationCache{}, false
	}
	if !module.Available() {
		return artifact.EquationCache{}, false
	}
	cache := artifact.EquationCache{
		Program:  p.ContentID(),
		Module:   module,
		Engine:   artifact.SemanticKey(equationCacheEngineKey),
		Factors:  admission.factors,
		Rules:    rows.rules,
		Bodies:   bodies,
		Boundary: rows.boundaries,
	}
	return cache, cache.Program.Available()
}

func (solver *Solver) cacheFactorKeys() ([]artifact.SemanticKey, bool) {
	if solver == nil || len(solver.factors) == 0 {
		return nil, false
	}
	keys := make([]artifact.SemanticKey, 0, len(solver.factors))
	for _, declaration := range solver.factors {
		if declaration.semantic == nil {
			return nil, false
		}
		key := declaration.semantic()
		if !availableSemanticKey(key) {
			return nil, false
		}
		keys = append(keys, artifact.SemanticKey(key))
	}
	return schemaSemanticKeys(keys)
}

func schemaSemanticKeys(keys []artifact.SemanticKey) ([]artifact.SemanticKey, bool) {
	seen := make(map[program.ContentID]struct{}, len(keys))
	for _, key := range keys {
		if !key.ID.Available() || key.Version == 0 {
			return nil, false
		}
		if _, duplicate := seen[key.ID]; duplicate {
			return nil, false
		}
		seen[key.ID] = struct{}{}
	}
	return keys, true
}

func canonicalSemanticKeys(keys []artifact.SemanticKey) ([]artifact.SemanticKey, bool) {
	seen := make(map[program.ContentID]struct{}, len(keys))
	for _, key := range keys {
		if !key.ID.Available() || key.Version == 0 {
			return nil, false
		}
		if _, duplicate := seen[key.ID]; duplicate {
			return nil, false
		}
		seen[key.ID] = struct{}{}
	}
	radixSemanticKeys(keys)
	return keys, true
}

// radixSemanticKeys establishes the fixed canonical key order in 40 stable
// byte passes: numeric Version first (least-significant byte first), then
// ContentID from its least-significant byte to its most-significant byte.
// Factor schemas never call this; their declaration order is semantic.
func radixSemanticKeys(keys []artifact.SemanticKey) {
	if len(keys) < 2 {
		return
	}
	work := make([]artifact.SemanticKey, len(keys))
	source, destination := keys, work
	inWork := false
	for shift := uint(0); shift < 64; shift += 8 {
		radixSemanticPass(source, destination, int(shift), -1)
		source, destination = destination, source
		inWork = !inWork
	}
	for index := len(program.ContentID{}) - 1; index >= 0; index-- {
		radixSemanticPass(source, destination, 0, index)
		source, destination = destination, source
		inWork = !inWork
	}
	if inWork {
		copy(keys, source)
	}
}

func radixSemanticPass(source, destination []artifact.SemanticKey, shift, contentIndex int) {
	var counts [256]int
	for _, key := range source {
		value := byte(key.Version >> uint(shift))
		if contentIndex >= 0 {
			value = key.ID[contentIndex]
		}
		counts[value]++
	}
	total := 0
	for index, count := range counts {
		counts[index], total = total, total+count
	}
	for _, key := range source {
		value := byte(key.Version >> uint(shift))
		if contentIndex >= 0 {
			value = key.ID[contentIndex]
		}
		destination[counts[value]] = key
		counts[value]++
	}
}

func radixEquationBoundaries(boundaries []artifact.EquationBoundary) {
	if len(boundaries) < 2 {
		return
	}
	work := make([]artifact.EquationBoundary, len(boundaries))
	source, destination := boundaries, work
	inWork := false
	for shift := uint(0); shift < 64; shift += 8 {
		radixBoundaryPass(source, destination, int(shift), -1)
		source, destination = destination, source
		inWork = !inWork
	}
	for index := len(program.ContentID{}) - 1; index >= 0; index-- {
		radixBoundaryPass(source, destination, 0, index)
		source, destination = destination, source
		inWork = !inWork
	}
	if inWork {
		copy(boundaries, source)
	}
}

func radixBoundaryPass(source, destination []artifact.EquationBoundary, shift, contentIndex int) {
	var counts [256]int
	for _, boundary := range source {
		value := byte(boundary.Rule.Version >> uint(shift))
		if contentIndex >= 0 {
			value = boundary.Rule.ID[contentIndex]
		}
		counts[value]++
	}
	total := 0
	for index, count := range counts {
		counts[index], total = total, total+count
	}
	for _, boundary := range source {
		value := byte(boundary.Rule.Version >> uint(shift))
		if contentIndex >= 0 {
			value = boundary.Rule.ID[contentIndex]
		}
		destination[counts[value]] = boundary
		counts[value]++
	}
}

func cloneEquationCache(source artifact.EquationCache) artifact.EquationCache {
	result := source
	result.Factors = append([]artifact.SemanticKey(nil), source.Factors...)
	result.Rules = append([]artifact.SemanticKey(nil), source.Rules...)
	result.Bodies = make([]artifact.EquationBody, len(source.Bodies))
	for index, body := range source.Bodies {
		result.Bodies[index] = body
		result.Bodies[index].Terms = append([]program.Term(nil), body.Terms...)
		result.Bodies[index].Edges = make([]artifact.EquationEdge, len(body.Edges))
		for edgeIndex, edge := range body.Edges {
			result.Bodies[index].Edges[edgeIndex] = edge
			result.Bodies[index].Edges[edgeIndex].MuDecisions = append([]program.Term(nil), edge.MuDecisions...)
		}
	}
	result.Boundary = make([]artifact.EquationBoundary, len(source.Boundary))
	for index, boundary := range source.Boundary {
		result.Boundary[index] = boundary
		result.Boundary[index].Reads = append([]artifact.EquationRead(nil), boundary.Reads...)
		result.Boundary[index].Writes = append([]uint64(nil), boundary.Writes...)
	}
	return result
}

// cachedModule is a private cold-admission index.  It names no semantic
// entity: it merely prevents cache count from multiplying Link shard scans.
type cachedModule struct {
	shard   link.Shard
	program *program.Program
}

// cacheRuleExpectation is one module's complete static declaration witness.
// It is built once during cold admission and contains no Rule closure, state,
// or execution topology.
type cacheRuleExpectation struct {
	rules      []artifact.SemanticKey
	boundaries []artifact.EquationBoundary
	valid      bool
}

// cacheAdmission is one cold snapshot of the live composition used to admit
// all supplied cache sections. It prevents cache count from multiplying scans
// of Factors, Rules, or Link shards.
type cacheAdmission struct {
	factors      []artifact.SemanticKey
	factorsValid bool
	modules      map[program.ContentID]cachedModule
	rules        map[program.ContentID]cacheRuleExpectation
}

func (solver *Solver) cacheAdmission() (cacheAdmission, bool) {
	modules, ok := solver.cacheModuleIndex()
	if !ok {
		return cacheAdmission{}, false
	}
	factors, factorsValid := solver.cacheFactorKeys()
	admission := cacheAdmission{factors: factors, factorsValid: factorsValid, modules: modules}
	if !factorsValid {
		return admission, true
	}
	admission.rules = solver.cacheRuleExpectations(modules)
	return admission, true
}

// cacheRuleExpectations scans declarations once, groups them by the already
// indexed module, and canonicalizes each group's immutable identifiers once.
// A malformed group is cache-ineligible while other modules remain eligible.
func (solver *Solver) cacheRuleExpectations(modules map[program.ContentID]cachedModule) map[program.ContentID]cacheRuleExpectation {
	result := make(map[program.ContentID]cacheRuleExpectation, len(modules))
	byShard := make(map[link.Shard]program.ContentID, len(modules))
	for key, module := range modules {
		result[key] = cacheRuleExpectation{valid: true}
		byShard[module.shard] = key
	}
	if solver == nil {
		return result
	}
	for _, declaration := range solver.rules {
		if declaration.base == nil || declaration.identity == nil || declaration.semantic == nil || declaration.outputSemantic == nil {
			return invalidateCacheGroups(result)
		}
		identity := declaration.identity()
		if identity == nil || !identity.sealed {
			return invalidateCacheGroups(result)
		}
		shard, at, ok := solver.equationBoundaryOccurrence(declaration.base.anchor)
		if !ok || shard == 0 || at == 0 {
			return invalidateCacheGroups(result)
		}
		key, known := byShard[shard]
		if !known {
			return invalidateCacheGroups(result)
		}
		group := result[key]
		if !group.valid {
			continue
		}
		module := modules[key]
		rule, output := declaration.semantic(), declaration.outputSemantic()
		reads, readsOK := equationBoundaryReads(identity)
		writes, writesOK := equationBoundaryWrites(identity)
		activation, activationOK := module.program.Activation(at)
		if shard != module.shard || !availableSemanticKey(rule) || !availableSemanticKey(output) || !readsOK || !writesOK || !activationOK || activation == 0 {
			group.valid = false
			result[key] = group
			continue
		}
		group.rules = append(group.rules, artifact.SemanticKey(rule))
		group.boundaries = append(group.boundaries, artifact.EquationBoundary{
			Rule:       artifact.SemanticKey(rule),
			Output:     artifact.SemanticKey(output),
			InputArity: identity.anchor.inputArity,
			Activation: activation,
			At:         at,
			Reads:      reads,
			Writes:     writes,
		})
		result[key] = group
	}
	for key, group := range result {
		if !group.valid {
			continue
		}
		var ok bool
		group.rules, ok = canonicalSemanticKeys(group.rules)
		if !ok {
			group.valid = false
			result[key] = group
			continue
		}
		radixEquationBoundaries(group.boundaries)
		for index := 1; index < len(group.boundaries); index++ {
			if group.boundaries[index-1].Rule.ID == group.boundaries[index].Rule.ID {
				group.valid = false
				break
			}
		}
		result[key] = group
	}
	return result
}

func invalidateCacheGroups(groups map[program.ContentID]cacheRuleExpectation) map[program.ContentID]cacheRuleExpectation {
	for key, group := range groups {
		group.valid = false
		groups[key] = group
	}
	return groups
}

// equationBoundaryOccurrence resolves the one current Program occurrence
// represented by a sealed Rule form. A Relation deliberately delegates its
// occurrence to Link; At and From carry the exact Program occurrence selected
// during declaration. From persists its edge destination rather than an
// equivalent cached anchor field.
func (solver *Solver) equationBoundaryOccurrence(anchor ruleAnchor) (link.Shard, program.Term, bool) {
	if solver == nil || solver.link == nil || !solver.validRuleAnchor(anchor) {
		return 0, 0, false
	}
	switch anchor.form {
	case ruleAt:
		return anchor.shard, anchor.term, true
	case ruleFrom:
		return anchor.shard, anchor.edge.To(), true
	case ruleRelation:
		return solver.link.ApplicationOccurrence(anchor.application)
	default:
		return 0, 0, false
	}
}

// equationBoundaryReads preserves the complete ordered input schema. The
// same Factor may occur at two different tuple positions, so this must not
// collapse to a Factor set while preparing the portable cache row.
func equationBoundaryReads(identity *ruleIdentity) ([]artifact.EquationRead, bool) {
	if identity == nil || !identity.sealed || identity.anchor.inputArity <= 0 {
		return nil, false
	}
	reads := make([]artifact.EquationRead, len(identity.reads))
	for index, read := range identity.reads {
		if read.position < 0 || read.position >= identity.anchor.inputArity || !availableSemanticKey(read.factor) {
			return nil, false
		}
		reads[index] = artifact.EquationRead{Position: read.position, Factor: artifact.SemanticKey(read.factor), Exact: read.exact, Key: read.key}
	}
	sort.Slice(reads, func(left, right int) bool {
		if reads[left].Position != reads[right].Position {
			return reads[left].Position < reads[right].Position
		}
		if order := compareSemanticKey(SemanticKey(reads[left].Factor), SemanticKey(reads[right].Factor)); order != 0 {
			return order < 0
		}
		if reads[left].Exact != reads[right].Exact {
			return !reads[left].Exact
		}
		return reads[left].Key < reads[right].Key
	})
	for index := 1; index < len(reads); index++ {
		if reads[index-1].Position == reads[index].Position &&
			compareSemanticKey(SemanticKey(reads[index-1].Factor), SemanticKey(reads[index].Factor)) == 0 &&
			(!reads[index-1].Exact || !reads[index].Exact || reads[index-1].Key == reads[index].Key) {
			return nil, false
		}
	}
	return reads, true
}

func equationBoundaryWrites(identity *ruleIdentity) ([]uint64, bool) {
	if identity == nil || !identity.sealed {
		return nil, false
	}
	writes := append([]uint64(nil), identity.writes...)
	for index := 1; index < len(writes); index++ {
		if writes[index-1] >= writes[index] {
			return nil, false
		}
	}
	return writes, true
}

// prepareEquationCaches admits each section atomically.  A section is useful
// only when all of its Program, engine, Factor, Rule, boundary, and body rows
// match the live sealed composition.  A failed section is discarded before
// body compilation starts, so no caller can observe a partly cached equation
// inventory.
func (solver *Solver) prepareEquationCaches() bool {
	if solver == nil || solver.link == nil || solver.cacheBodies != nil {
		return false
	}
	solver.cacheBodies = make(map[bodyOrigin]compiledBody)
	if len(solver.equationCaches) == 0 {
		return true
	}
	admission, ok := solver.cacheAdmission()
	if !ok {
		return false
	}
	if !admission.factorsValid {
		return true
	}
	// A cache is module-scoped: identical Program content may occur in several
	// named shards with different link environments.  Only duplicate claims to
	// that one exact module are ambiguous.
	duplicates := make(map[program.ContentID]uint32, len(solver.equationCaches))
	for _, cache := range solver.equationCaches {
		if cache.Module.Available() {
			duplicates[cache.Module]++
		}
	}
	for _, cache := range solver.equationCaches {
		if duplicates[cache.Module] != 1 {
			continue
		}
		shard, p, ok := solver.cacheShard(cache, admission)
		if !ok {
			continue
		}
		rows, ok := solver.decodeCachedBodies(shard, p, cache)
		if !ok {
			continue
		}
		for origin, row := range rows {
			solver.cacheBodies[origin] = row
		}
	}
	return true
}

// cacheModuleIndex computes every Link-owned module identity exactly once at
// cache admission.  A duplicate is an internal identity failure, never a
// reason to choose an arbitrary first shard.
func (solver *Solver) cacheModuleIndex() (map[program.ContentID]cachedModule, bool) {
	if solver == nil || solver.link == nil {
		return nil, false
	}
	modules := make(map[program.ContentID]cachedModule, solver.link.ShardCount())
	for index := 0; index < solver.link.ShardCount(); index++ {
		shard, ok := solver.link.ShardAt(index)
		if !ok {
			return nil, false
		}
		p, ok := solver.link.Program(shard)
		key := solver.link.ModuleKey(shard)
		if !ok || p == nil || !key.Available() {
			return nil, false
		}
		if _, duplicate := modules[key]; duplicate {
			return nil, false
		}
		modules[key] = cachedModule{shard: shard, program: p}
	}
	return modules, true
}

func (solver *Solver) cacheShard(cache artifact.EquationCache, admission cacheAdmission) (link.Shard, *program.Program, bool) {
	if solver == nil || solver.link == nil || cache.Program == (program.ContentID{}) ||
		!cache.Module.Available() || cache.Engine != artifact.SemanticKey(equationCacheEngineKey) || !admission.factorsValid {
		return 0, nil, false
	}
	if !sameSemanticKeys(cache.Factors, admission.factors) {
		return 0, nil, false
	}
	module, ok := admission.modules[cache.Module]
	if !ok || module.program == nil || module.program.ContentID() != cache.Program {
		return 0, nil, false
	}
	want, ok := admission.rules[cache.Module]
	if !ok || !want.valid || !sameSemanticKeys(cache.Rules, want.rules) || !sameEquationBoundaries(cache.Boundary, want.boundaries) {
		return 0, nil, false
	}
	return module.shard, module.program, true
}

func sameSemanticKeys(left, right []artifact.SemanticKey) bool {
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

func sameEquationBoundaries(left, right []artifact.EquationBoundary) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Rule != right[index].Rule || left[index].Output != right[index].Output ||
			left[index].InputArity != right[index].InputArity || left[index].Activation != right[index].Activation || left[index].At != right[index].At ||
			!sameEquationReads(left[index].Reads, right[index].Reads) || !sameEquationWrites(left[index].Writes, right[index].Writes) {
			return false
		}
	}
	return true
}

func sameEquationWrites(left, right []uint64) bool {
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

func sameEquationReads(left, right []artifact.EquationRead) bool {
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

func (solver *Solver) decodeCachedBodies(shard link.Shard, p *program.Program, cache artifact.EquationCache) (map[bodyOrigin]compiledBody, bool) {
	if solver == nil || p == nil {
		return nil, false
	}
	if !artifact.MatchesCanonicalEquationBodies(p, cache.Bodies) {
		return nil, false
	}
	result := make(map[bodyOrigin]compiledBody, len(cache.Bodies))
	for index := 0; index < p.BodyCount(); index++ {
		body, ok := p.BodyAt(index)
		if !ok || cache.Bodies[index].Body != body {
			return nil, false
		}
		activation, ok := p.Activation(body)
		if !ok || activation == 0 {
			return nil, false
		}
		row, ok := decodeCompiledBody(shard, p, body, activation, cache.Bodies[index])
		if !ok {
			return nil, false
		}
		result[bodyOrigin{shard: shard, body: row.body}] = row
	}
	return result, true
}

func decodeCompiledBody(shard link.Shard, p *program.Program, body, activation program.Term, cached artifact.EquationBody) (compiledBody, bool) {
	if shard == 0 || p == nil || body == 0 || activation == 0 || cached.Body != body || len(cached.Terms) == 0 {
		return compiledBody{}, false
	}
	terms := append([]program.Term(nil), cached.Terms...)
	for index, term := range terms {
		if term == 0 || !p.Valid(term) || (index != 0 && terms[index-1] >= term) {
			return compiledBody{}, false
		}
		owner, ok := p.Activation(term)
		if !ok || owner != activation {
			return compiledBody{}, false
		}
	}
	count, ok := p.BodyEdgeCount(body)
	if !ok || count != len(cached.Edges) {
		return compiledBody{}, false
	}
	edges := make([]program.Edge, 0, count)
	for index := 0; index < count; index++ {
		edge, ok := p.BodyEdgeAt(body, index)
		if !ok || !containsBodyTerm(terms, edge.From()) || !containsBodyTerm(terms, edge.To()) {
			return compiledBody{}, false
		}
		edges = append(edges, edge)
	}
	entry, entryOK := p.BodyEntry(body)
	normal, normalOK := p.BodyNormalExit(body)
	thrown, throwOK := p.BodyThrowExit(body)
	yielded, yieldOK := p.BodyYieldExit(body)
	canceled, cancelOK := p.BodyCancelExit(body)
	if !entryOK || !normalOK || !throwOK || !yieldOK || !cancelOK {
		return compiledBody{}, false
	}
	row := compiledBody{
		body:       body,
		activation: activation,
		ingress:    bodyIngress{entry: entry},
		outcomes:   bodyOutcomes{normal: normal, thrown: thrown, yielded: yielded, canceled: canceled},
		terms:      terms,
		edges:      edges,
	}
	if first, ok := p.BodyFirst(body); ok {
		row.ingress.first = first
	}
	if returned, ok := p.BodyReturnExit(body); ok {
		row.outcomes.returned = returned
	}
	return row.withIndexes(p)
}
