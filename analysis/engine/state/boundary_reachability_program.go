package state

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/symbol"
)

type boundaryReachabilityTriggerKind uint8

const (
	boundaryReachabilityTriggerInvalid boundaryReachabilityTriggerKind = iota
	boundaryReachabilityTriggerUnconditional
	boundaryReachabilityTriggerPathCone
	boundaryReachabilityTriggerIdentity
	boundaryReachabilityTriggerAllIdentities
)

// boundaryReachabilityClause is one finite monotone implication. Product
// values never survive sealing: their only closure-relevant projection is an
// exact identity term or the exact AllIdentities lattice element.
type boundaryReachabilityClause struct {
	trigger            boundaryReachabilityTriggerKind
	paths              []keyspace.Key
	versionInsensitive bool
	identity           identity.Term
	addPaths           []keyspace.Key
	addIdentities      []identity.Term
	addAllIdentities   bool
	addHeapSuffixes    []boundaryHeapSuffix
}

// BoundaryReachabilityProgram is a sealed, indexed least-fixed-point program.
// It contains no State, factor payload, callback, inventory cursor, or solve
// budget. Programs are immutable and may be retained beside formal terminals.
type BoundaryReachabilityProgram struct {
	seal    *boundaryReachabilityProgramSeal
	reg     *axis.Registry
	keys    *keyspace.KeySpace
	data    *boundaryReachabilityProgramSetData
	clauses []boundaryReachabilityClause
}

// BoundaryReachabilityProgramSet is a sealed execution plan over immutable
// program references. Sealing indexes clause references once; Close runs one
// worklist and one fired-bit vector across the whole set without copying or
// re-sealing clauses and without inspecting a lane factor.
type BoundaryReachabilityProgramSet struct {
	seal *boundaryReachabilityProgramSetSeal
	reg  *axis.Registry
	keys *keyspace.KeySpace
	data *boundaryReachabilityProgramSetData
}

type boundaryReachabilityProgramRef struct {
	clauses []boundaryReachabilityClause
}

type boundaryReachabilityClauseRef struct {
	program int
	clause  int
}

type boundaryReachabilityProgramSetData struct {
	programs    []boundaryReachabilityProgramRef
	clauseRefs  []boundaryReachabilityClauseRef
	path        map[keyspace.Key][]int
	version     map[symbol.ID][]int
	identities  map[identity.Term][]int
	identityAll []int
	allOnly     []int
	always      []int
}

type boundaryReachabilityProgramSeal struct{ owned byte }
type boundaryReachabilityProgramSetSeal struct{ owned byte }

func (p BoundaryReachabilityProgram) Valid() bool {
	return p.seal != nil && p.reg != nil && p.keys != nil && p.keys.Valid()
}

func (p BoundaryReachabilityProgramSet) Valid() bool {
	return p.seal != nil && p.reg != nil && p.keys != nil && p.keys.Valid() && p.data != nil
}

// BoundaryReachabilityProgramBuilder is the registration-only clause writer.
// The type is intentionally opaque outside state; lanes receive it through
// their typed registration and can express only the closed trigger algebra.
type boundaryReachabilityProgramBuilder struct {
	reg     *axis.Registry
	keys    *keyspace.KeySpace
	clauses []boundaryReachabilityClause
	current int
	err     error
}

func newBoundaryReachabilityProgramBuilder(reg *axis.Registry, keys *keyspace.KeySpace) *boundaryReachabilityProgramBuilder {
	return &boundaryReachabilityProgramBuilder{reg: reg, keys: keys, current: -1}
}

func (b *boundaryReachabilityProgramBuilder) begin(trigger boundaryReachabilityTriggerKind) *boundaryReachabilityClause {
	if b == nil || b.err != nil {
		return nil
	}
	b.clauses = append(b.clauses, boundaryReachabilityClause{trigger: trigger})
	b.current = len(b.clauses) - 1
	return &b.clauses[b.current]
}

func (b *boundaryReachabilityProgramBuilder) currentClause() *boundaryReachabilityClause {
	if b == nil || b.err != nil || b.current < 0 || b.current >= len(b.clauses) {
		return nil
	}
	return &b.clauses[b.current]
}

func (b *boundaryReachabilityProgramBuilder) resetClause() { b.current = -1 }

// pathCone emits one hyperedge: touching any endpoint retains every endpoint.
func (b *boundaryReachabilityProgramBuilder) pathCone(versionInsensitive bool, paths ...keyspace.Key) bool {
	clause := b.begin(boundaryReachabilityTriggerPathCone)
	if clause == nil {
		return false
	}
	clause.versionInsensitive = versionInsensitive
	for _, path := range paths {
		if path.Kind == keyspace.KindInvalid {
			continue
		}
		if b.keys == nil || b.keys.FormatReadOnly(path) == "" {
			b.err = fmt.Errorf("state: reachability PathCone contains a foreign path")
			return false
		}
		clause.paths = append(clause.paths, path)
		clause.addPaths = append(clause.addPaths, path)
	}
	if len(clause.paths) == 0 {
		b.err = fmt.Errorf("state: reachability PathCone is empty")
		return false
	}
	return true
}

func (b *boundaryReachabilityProgramBuilder) identity(term identity.Term) bool {
	if !term.Valid() {
		b.err = fmt.Errorf("state: reachability Identity trigger is empty")
		return false
	}
	clause := b.begin(boundaryReachabilityTriggerIdentity)
	clause.identity = term
	return true
}

func (b *boundaryReachabilityProgramBuilder) allIdentities() bool {
	return b.begin(boundaryReachabilityTriggerAllIdentities) != nil
}

func (b *boundaryReachabilityProgramBuilder) addPath(path keyspace.Key) {
	clause := b.currentClause()
	if clause == nil || path.Kind == keyspace.KindInvalid || b.keys.FormatReadOnly(path) == "" {
		if path.Kind != keyspace.KindInvalid {
			b.err = fmt.Errorf("state: reachability conclusion contains a foreign path")
		}
		return
	}
	clause.addPaths = append(clause.addPaths, path)
}

func (b *boundaryReachabilityProgramBuilder) addValue(value product.Value) {
	clause := b.currentClause()
	if clause == nil || !product.BelongsToRegistry(b.reg, value) {
		b.err = fmt.Errorf("state: reachability value conclusion is unowned")
		return
	}
	if term, ok := identityvalue.ExactTerm(b.reg, value); ok {
		clause.addIdentities = append(clause.addIdentities, term)
		return
	}
	if identity.Equal(product.Get(b.reg, value, identity.Key), identity.Top()) {
		clause.addAllIdentities = true
	}
}

func (b *boundaryReachabilityProgramBuilder) addIdentityTerm(term identity.Term) {
	clause := b.currentClause()
	if clause == nil || !term.Valid() {
		b.err = fmt.Errorf("state: reachability identity conclusion is empty")
		return
	}
	clause.addIdentities = append(clause.addIdentities, term)
}

func (b *boundaryReachabilityProgramBuilder) addHeapSuffix(owner identity.Term, suffix keyspace.Key) {
	clause := b.currentClause()
	if clause == nil || !owner.Valid() || suffix.Kind == keyspace.KindInvalid || b.keys.FormatReadOnly(suffix) == "" {
		b.err = fmt.Errorf("state: reachability heap suffix is invalid")
		return
	}
	clause.addHeapSuffixes = append(clause.addHeapSuffixes, boundaryHeapSuffix{owner: owner, suffix: suffix})
}

func (b *boundaryReachabilityProgramBuilder) addStateKey(raw interface{ String() string }) keyspace.Key {
	if b == nil || b.keys == nil || raw == nil {
		return keyspace.Key{}
	}
	path, ok := b.keys.FromStateKey(pathdom.PathKey(raw.String()))
	if !ok {
		return keyspace.Key{}
	}
	return path
}

func (b *boundaryReachabilityProgramBuilder) seal() (BoundaryReachabilityProgram, error) {
	if b == nil || b.reg == nil || b.keys == nil || !b.keys.Valid() || b.err != nil {
		if b != nil && b.err != nil {
			return BoundaryReachabilityProgram{}, b.err
		}
		return BoundaryReachabilityProgram{}, fmt.Errorf("state: reachability program is unowned")
	}
	p := BoundaryReachabilityProgram{
		seal: &boundaryReachabilityProgramSeal{}, reg: b.reg, keys: b.keys,
		clauses: append([]boundaryReachabilityClause(nil), b.clauses...),
	}
	for index := range p.clauses {
		clause := &p.clauses[index]
		clause.paths = append([]keyspace.Key(nil), clause.paths...)
		clause.addPaths = append([]keyspace.Key(nil), clause.addPaths...)
		clause.addIdentities = append([]identity.Term(nil), clause.addIdentities...)
		clause.addHeapSuffixes = append([]boundaryHeapSuffix(nil), clause.addHeapSuffixes...)
	}
	singleton, err := SealBoundaryReachabilityProgramSet(p)
	if err != nil {
		return BoundaryReachabilityProgram{}, err
	}
	p.data = singleton.data
	return p, nil
}

// Close executes the exact least fixed point over an already-sealed boundary
// selection. Every clause fires at most once; queues grow only by finite
// program conclusions. There is no depth, work, or time cap.
func (p BoundaryReachabilityProgram) Close(selection BoundaryFactorSelection, seedValues []product.Value) (BoundaryFactorSelection, error) {
	if !p.Valid() || p.data == nil || !selection.valid() || selection.keys != p.keys {
		return BoundaryFactorSelection{}, fmt.Errorf("state: reachability program and selection have different ownership")
	}
	return (BoundaryReachabilityProgramSet{seal: &boundaryReachabilityProgramSetSeal{}, reg: p.reg, keys: p.keys, data: p.data}).close(selection, seedValues, false)
}

// closeSelected computes the same uncapped closure as Close, but treats every
// clause in this program as an explicitly selected coordinate. This is the
// inverse-image operation used by formal point publication: its input program
// was already built from the exact slots whose keys rekey into the point
// environment, so their registered structural support is seed authority, not
// a reachability condition. Call-boundary projection continues to use Close.
func (p BoundaryReachabilityProgram) closeSelected(selection BoundaryFactorSelection) (BoundaryFactorSelection, error) {
	if !p.Valid() || p.data == nil || !selection.valid() || selection.keys != p.keys {
		return BoundaryFactorSelection{}, fmt.Errorf("state: reachability program and selection have different ownership")
	}
	return (BoundaryReachabilityProgramSet{seal: &boundaryReachabilityProgramSetSeal{}, reg: p.reg, keys: p.keys, data: p.data}).close(selection, nil, true)
}

// SealBoundaryReachabilityProgramSet builds one global clause index over
// immutable program references. It neither copies clauses nor inspects factor
// payloads, so it can be frozen beside an artifact and reused by every Apply.
func SealBoundaryReachabilityProgramSet(programs ...BoundaryReachabilityProgram) (BoundaryReachabilityProgramSet, error) {
	if len(programs) == 0 || !programs[0].Valid() {
		return BoundaryReachabilityProgramSet{}, fmt.Errorf("state: reachability program set is empty")
	}
	data := &boundaryReachabilityProgramSetData{
		programs: make([]boundaryReachabilityProgramRef, 0, len(programs)),
		path:     make(map[keyspace.Key][]int), version: make(map[symbol.ID][]int), identities: make(map[identity.Term][]int),
	}
	for programIndex, program := range programs {
		if !program.Valid() || program.reg != programs[0].reg || program.keys != programs[0].keys {
			return BoundaryReachabilityProgramSet{}, fmt.Errorf("state: reachability program set has foreign ownership")
		}
		data.programs = append(data.programs, boundaryReachabilityProgramRef{clauses: program.clauses})
		for clauseIndex, clause := range program.clauses {
			index := len(data.clauseRefs)
			data.clauseRefs = append(data.clauseRefs, boundaryReachabilityClauseRef{program: programIndex, clause: clauseIndex})
			switch clause.trigger {
			case boundaryReachabilityTriggerUnconditional:
				data.always = append(data.always, index)
			case boundaryReachabilityTriggerPathCone:
				roots := make(map[keyspace.Key]struct{})
				for _, path := range clause.paths {
					if root, ok := program.keys.StructuralRoot(path); ok {
						roots[root] = struct{}{}
					}
					if clause.versionInsensitive && path.Kind == keyspace.KindUnversionedSym && path.Segs == 0 {
						data.version[path.Sym] = append(data.version[path.Sym], index)
					}
				}
				for root := range roots {
					data.path[root] = append(data.path[root], index)
				}
			case boundaryReachabilityTriggerIdentity:
				data.identities[clause.identity] = append(data.identities[clause.identity], index)
				data.identityAll = append(data.identityAll, index)
			case boundaryReachabilityTriggerAllIdentities:
				data.allOnly = append(data.allOnly, index)
			default:
				return BoundaryReachabilityProgramSet{}, fmt.Errorf("state: reachability program set contains an invalid trigger")
			}
		}
	}
	return BoundaryReachabilityProgramSet{
		seal: &boundaryReachabilityProgramSetSeal{}, reg: programs[0].reg, keys: programs[0].keys, data: data,
	}, nil
}

// Close executes one uncapped least-fixed-point worklist across every program
// in the sealed set. Cross-program conclusions wake clauses through the shared
// indexes, so ordering and partitioning do not change the result.
func (p BoundaryReachabilityProgramSet) Close(selection BoundaryFactorSelection, seedValues []product.Value) (BoundaryFactorSelection, error) {
	return p.close(selection, seedValues, false)
}

func (p BoundaryReachabilityProgramSet) close(selection BoundaryFactorSelection, seedValues []product.Value, selectAll bool) (BoundaryFactorSelection, error) {
	if !p.Valid() || !selection.valid() || selection.keys != p.keys {
		return BoundaryFactorSelection{}, fmt.Errorf("state: reachability program set and selection have different ownership")
	}
	closure := cloneBoundaryFactorClosure(selection.closure)
	active := make([]bool, len(p.data.clauseRefs))
	queue := make([]int, 0, len(p.data.always)+len(closure.paths)+len(closure.identities))
	clauseAt := func(index int) boundaryReachabilityClause {
		ref := p.data.clauseRefs[index]
		return p.data.programs[ref.program].clauses[ref.clause]
	}
	enqueue := func(indices []int) {
		for _, index := range indices {
			if index >= 0 && index < len(active) && !active[index] {
				active[index] = true
				queue = append(queue, index)
			}
		}
	}
	enqueue(p.data.always)
	if selectAll {
		all := make([]int, len(p.data.clauseRefs))
		for index := range all {
			all[index] = index
		}
		enqueue(all)
	}
	activatePath := func(path keyspace.Key) {
		if root, ok := p.keys.StructuralRoot(path); ok {
			for _, index := range p.data.path[root] {
				if active[index] {
					continue
				}
				for _, endpoint := range clauseAt(index).paths {
					if closure.pathTouches(p.keys, endpoint) {
						enqueue([]int{index})
						break
					}
				}
			}
		}
		if path.Kind == keyspace.KindResolverSym && path.Segs == 0 {
			enqueue(p.data.version[path.Sym])
		}
	}
	for path := range closure.paths {
		activatePath(path)
	}
	for term := range closure.identities {
		enqueue(p.data.identities[term])
	}
	if closure.allIdentities {
		enqueue(p.data.identityAll)
		enqueue(p.data.allOnly)
	}
	for index, value := range seedValues {
		if !product.BelongsToRegistry(p.reg, value) {
			return BoundaryFactorSelection{}, fmt.Errorf("state: reachability seed value %d is foreign", index)
		}
		if term, exact := identityvalue.ExactTerm(p.reg, value); exact {
			if _, present := closure.identities[term]; !present {
				closure.identities[term] = struct{}{}
				enqueue(p.data.identities[term])
			}
			continue
		}
		if identity.Equal(product.Get(p.reg, value, identity.Key), identity.Top()) && !closure.allIdentities {
			closure.allIdentities = true
			enqueue(p.data.identityAll)
			enqueue(p.data.allOnly)
		}
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		clause := clauseAt(queue[cursor])
		for _, path := range clause.addPaths {
			if !closure.hasPath(path) {
				closure.paths[path] = struct{}{}
				activatePath(path)
			}
		}
		for _, term := range clause.addIdentities {
			if _, present := closure.identities[term]; !present {
				closure.identities[term] = struct{}{}
				enqueue(p.data.identities[term])
			}
		}
		if clause.addAllIdentities && !closure.allIdentities {
			closure.allIdentities = true
			enqueue(p.data.identityAll)
			enqueue(p.data.allOnly)
		}
		for _, suffix := range clause.addHeapSuffixes {
			closure.heapSuffixes[suffix] = struct{}{}
		}
	}
	return BoundaryFactorSelection{
		seal: &boundaryFactorSelectionSeal{}, keys: selection.keys, closure: closure,
		roots: append([]BoundaryFactorRoot(nil), selection.roots...),
	}, nil
}
