package flow

import (
	"cmp"
	"slices"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

// ReturnRelations is the finite caller-visible relation component of a function
// summary. It records return-slot relations the function proves for every normal
// return path, such as Lua's `(value, err)` inverse convention and length
// postconditions like `len(ret_i) >= len(param_j)`.
//
// The carrier is a must-fact lattice: a relation may be consumed by callers only
// when every incoming summary path proves it. Join is therefore intersection. The
// bottom element is represented by a sentinel (`bottom=true`) because the
// unreachable function vacuously proves every relation; all reachable finite states
// are represented by a sorted compact slice.
type ReturnRelations struct {
	bottom      bool
	errorReturn []ReturnCorrelation
	lengthParam []ReturnLengthParamRelation
}

// ReturnRelationsDomain is the abstract domain of finite return relations.
var ReturnRelationsDomain = lattice.Lattice[ReturnRelations]{
	Bottom: func() ReturnRelations {
		return ReturnRelations{bottom: true}
	},
	Top: func() ReturnRelations {
		return ReturnRelations{}
	},
	Equal: func(a, b ReturnRelations) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return slices.Equal(a.errorReturn, b.errorReturn) &&
			slices.Equal(a.lengthParam, b.lengthParam)
	},
	LessOrEq: func(a, b ReturnRelations) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return returnCorrelationsContainAll(a.errorReturn, b.errorReturn) &&
			returnLengthParamsContainAll(a.lengthParam, b.lengthParam)
	},
	Join: joinReturnRelations,
	Meet: nil,
	Widen: func(prev, next ReturnRelations) ReturnRelations {
		return joinReturnRelations(prev, next)
	},
}

// ReturnRelationsOfErrorReturns builds a canonical finite return-relation value.
func ReturnRelationsOfErrorReturns(xs []ReturnCorrelation) ReturnRelations {
	return ReturnRelations{errorReturn: compactReturnCorrelations(xs)}
}

// ReturnRelationsOfLengthParams builds a canonical finite return-length relation
// value. Each relation means len(return[ReturnIndex]) >= len(param[ParamIndex]).
func ReturnRelationsOfLengthParams(xs []ReturnLengthParamRelation) ReturnRelations {
	return ReturnRelations{lengthParam: compactReturnLengthParams(xs)}
}

// MergeReturnRelationProofs combines independently-proven finite relation facts.
// It is a proof builder, not the lattice Join: if two derivations both hold on
// the same path, callers may consume the union of their facts. Top contributes no
// finite proof; Bottom is treated as unreachable/no consumable proof here.
func MergeReturnRelationProofs(a, b ReturnRelations) ReturnRelations {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return ReturnRelations{
		errorReturn: compactReturnCorrelations(append(a.ErrorReturns(), b.ErrorReturns()...)),
		lengthParam: compactReturnLengthParams(append(a.LengthParams(), b.LengthParams()...)),
	}
}

// IsBottom reports the unreachable relation sentinel.
func (r ReturnRelations) IsBottom() bool { return r.bottom }

// HasProof reports whether this reachable relation value carries any finite
// caller-visible fact. Top has no finite proof; Bottom is a recursive/unreachable
// seed and is deliberately not consumable.
func (r ReturnRelations) HasProof() bool {
	return !r.bottom && (len(r.errorReturn) > 0 || len(r.lengthParam) > 0)
}

// ErrorReturns returns a defensive copy of the proven ErrorReturn relations.
func (r ReturnRelations) ErrorReturns() []ReturnCorrelation {
	if r.bottom || len(r.errorReturn) == 0 {
		return nil
	}
	return append([]ReturnCorrelation(nil), r.errorReturn...)
}

// HasErrorReturn reports whether the reachable finite carrier proves relation c.
// Bottom deliberately reports false to avoid treating a recursive query seed as
// consumable proof while the summary fixed point is still climbing.
func (r ReturnRelations) HasErrorReturn(c ReturnCorrelation) bool {
	if r.bottom {
		return false
	}
	_, ok := slices.BinarySearchFunc(r.errorReturn, c, compareReturnCorrelation)
	return ok
}

// ReturnLengthParamRelation records a caller-visible length postcondition:
// len(return[ReturnIndex]) >= len(param[ParamIndex]).
type ReturnLengthParamRelation struct {
	ReturnIndex int
	ParamIndex  int
}

// LengthParams returns a defensive copy of the proven return/parameter length
// relations.
func (r ReturnRelations) LengthParams() []ReturnLengthParamRelation {
	if r.bottom || len(r.lengthParam) == 0 {
		return nil
	}
	return append([]ReturnLengthParamRelation(nil), r.lengthParam...)
}

// HasLengthParam reports whether the reachable finite carrier proves relation c.
func (r ReturnRelations) HasLengthParam(c ReturnLengthParamRelation) bool {
	if r.bottom {
		return false
	}
	_, ok := slices.BinarySearchFunc(r.lengthParam, c, compareReturnLengthParam)
	return ok
}

// PointRelations is the finite point-local relation component of a PointState.
// SiblingNil facts are must-correlations between variables assigned from one
// multi-return call. Join intersects them so a branch may consume a relation only
// when every predecessor proves it.
type PointRelations struct {
	bottom      bool
	siblingNil  []SiblingNilRelation
	lengthParam []PointLengthParamRelation
	sizeLower   []PointContainerLowerBound
}

// SiblingNilRelation records that ErrSym being nil proves each ValueSym is
// present. The relation is symbol-local to one function and is killed on writes to
// either side.
type SiblingNilRelation struct {
	ErrSym    cfg.SymbolID
	ValueSyms []cfg.SymbolID
}

// PointLengthParamRelation records a point-local proof that the sequence at
// TargetKey has length/cardinality at least the function parameter ParamIndex.
// TargetRoot is the root symbol used for invalidation; TargetKey is the
// versioned path key used for exact summary projection.
type PointLengthParamRelation struct {
	TargetRoot cfg.SymbolID
	TargetKey  constraint.PathKey
	ParamIndex int
}

// PointContainerLowerBound records a point-local proof that a container has at
// least Lower entries under iteration/cardinality semantics. It is deliberately
// separate from numeric len-bounds: a string-keyed map literal has cardinality
// for `pairs`, but it does not prove anything about Lua's `#table` sequence
// border. Join keeps only must lower bounds by taking the minimum lower bound
// proven by all predecessors for the same target.
type PointContainerLowerBound struct {
	TargetRoot cfg.SymbolID
	TargetKey  constraint.PathKey
	Lower      int64
}

// PointRelationsDomain is the abstract domain of point-local relations.
var PointRelationsDomain = lattice.Lattice[PointRelations]{
	Bottom: func() PointRelations {
		return PointRelations{bottom: true}
	},
	Top: func() PointRelations {
		return PointRelations{}
	},
	Equal: func(a, b PointRelations) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return slices.EqualFunc(a.siblingNil, b.siblingNil, siblingNilEqual) &&
			slices.Equal(a.lengthParam, b.lengthParam) &&
			slices.Equal(a.sizeLower, b.sizeLower)
	},
	LessOrEq: func(a, b PointRelations) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return siblingNilContainsAll(a.siblingNil, b.siblingNil) &&
			pointLengthParamsContainAll(a.lengthParam, b.lengthParam) &&
			pointContainerLowerBoundsImplyAll(a.sizeLower, b.sizeLower)
	},
	Join: joinPointRelations,
	Meet: nil,
	Widen: func(prev, next PointRelations) PointRelations {
		return joinPointRelations(prev, next)
	},
}

func joinReturnRelations(a, b ReturnRelations) ReturnRelations {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return ReturnRelations{
		errorReturn: intersectReturnCorrelations(a.errorReturn, b.errorReturn),
		lengthParam: intersectReturnLengthParams(a.lengthParam, b.lengthParam),
	}
}

func joinPointRelations(a, b PointRelations) PointRelations {
	if a.bottom {
		return b
	}
	if b.bottom {
		return a
	}
	return PointRelations{
		siblingNil:  intersectSiblingNil(a.siblingNil, b.siblingNil),
		lengthParam: intersectPointLengthParams(a.lengthParam, b.lengthParam),
		sizeLower:   joinPointContainerLowerBounds(a.sizeLower, b.sizeLower),
	}
}

// IsBottom reports the unreachable relation sentinel.
func (r PointRelations) IsBottom() bool { return r.bottom }

// WithSiblingNil returns r plus the err -> values must relation.
func (r PointRelations) WithSiblingNil(err cfg.SymbolID, values []cfg.SymbolID) PointRelations {
	if err == 0 || len(values) == 0 {
		return r
	}
	if r.bottom {
		r = PointRelations{}
	}
	entries := append([]SiblingNilRelation(nil), r.siblingNil...)
	entries = append(entries, SiblingNilRelation{ErrSym: err, ValueSyms: compactSymbolIDs(values)})
	return PointRelations{
		siblingNil:  compactSiblingNil(entries),
		lengthParam: append([]PointLengthParamRelation(nil), r.lengthParam...),
		sizeLower:   append([]PointContainerLowerBound(nil), r.sizeLower...),
	}
}

// KillSymbols removes relations whose error or value side was overwritten.
func (r PointRelations) KillSymbols(symbols ...cfg.SymbolID) PointRelations {
	if r.bottom || len(symbols) == 0 || (len(r.siblingNil) == 0 && len(r.lengthParam) == 0 && len(r.sizeLower) == 0) {
		return r
	}
	killed := make(map[cfg.SymbolID]bool, len(symbols))
	for _, sym := range symbols {
		if sym != 0 {
			killed[sym] = true
		}
	}
	if len(killed) == 0 {
		return r
	}
	siblings := make([]SiblingNilRelation, 0, len(r.siblingNil))
	for _, rel := range r.siblingNil {
		if killed[rel.ErrSym] {
			continue
		}
		values := rel.ValueSyms[:0]
		for _, sym := range rel.ValueSyms {
			if !killed[sym] {
				values = append(values, sym)
			}
		}
		if len(values) == 0 {
			continue
		}
		siblings = append(siblings, SiblingNilRelation{ErrSym: rel.ErrSym, ValueSyms: append([]cfg.SymbolID(nil), values...)})
	}
	lengths := make([]PointLengthParamRelation, 0, len(r.lengthParam))
	for _, rel := range r.lengthParam {
		if !killed[rel.TargetRoot] {
			lengths = append(lengths, rel)
		}
	}
	return PointRelations{
		siblingNil:  compactSiblingNil(siblings),
		lengthParam: compactPointLengthParams(lengths),
		sizeLower:   compactPointContainerLowerBounds(filterContainerLowerBoundsByKilledRoots(r.sizeLower, killed)),
	}
}

// SiblingNil returns the relation for err, if it is proven in a reachable state.
func (r PointRelations) SiblingNil(err cfg.SymbolID) (SiblingNilRelation, bool) {
	if r.bottom || err == 0 || len(r.siblingNil) == 0 {
		return SiblingNilRelation{}, false
	}
	idx, ok := slices.BinarySearchFunc(r.siblingNil, SiblingNilRelation{ErrSym: err}, compareSiblingNilErrOnly)
	if !ok {
		return SiblingNilRelation{}, false
	}
	rel := r.siblingNil[idx]
	rel.ValueSyms = append([]cfg.SymbolID(nil), rel.ValueSyms...)
	return rel, true
}

// WithTargetLengthParam returns r plus the target-length >= parameter-length
// must relation. The target key is versioned, so summary projection can reject
// stale proofs after an intervening reassignment.
func (r PointRelations) WithTargetLengthParam(root cfg.SymbolID, target constraint.PathKey, paramIndex int) PointRelations {
	if root == 0 || target == "" || paramIndex < 0 {
		return r
	}
	if r.bottom {
		r = PointRelations{}
	}
	entries := append([]PointLengthParamRelation(nil), r.lengthParam...)
	entries = append(entries, PointLengthParamRelation{
		TargetRoot: root,
		TargetKey:  target,
		ParamIndex: paramIndex,
	})
	return PointRelations{
		siblingNil:  append([]SiblingNilRelation(nil), r.siblingNil...),
		lengthParam: compactPointLengthParams(entries),
		sizeLower:   append([]PointContainerLowerBound(nil), r.sizeLower...),
	}
}

// WithContainerLowerBound returns r plus a must lower-bound proof for a
// container's iteration/cardinality size.
func (r PointRelations) WithContainerLowerBound(root cfg.SymbolID, target constraint.PathKey, lower int64) PointRelations {
	if root == 0 || target == "" || lower <= 0 {
		return r
	}
	if r.bottom {
		r = PointRelations{}
	}
	entries := append([]PointContainerLowerBound(nil), r.sizeLower...)
	entries = append(entries, PointContainerLowerBound{
		TargetRoot: root,
		TargetKey:  target,
		Lower:      lower,
	})
	return PointRelations{
		siblingNil:  append([]SiblingNilRelation(nil), r.siblingNil...),
		lengthParam: append([]PointLengthParamRelation(nil), r.lengthParam...),
		sizeLower:   compactPointContainerLowerBounds(entries),
	}
}

// KillLengthTargets removes target-length relations rooted at overwritten or
// shape-mutated symbols without touching unrelated point relations.
func (r PointRelations) KillLengthTargets(symbols ...cfg.SymbolID) PointRelations {
	if r.bottom || (len(r.lengthParam) == 0 && len(r.sizeLower) == 0) || len(symbols) == 0 {
		return r
	}
	killed := make(map[cfg.SymbolID]bool, len(symbols))
	for _, sym := range symbols {
		if sym != 0 {
			killed[sym] = true
		}
	}
	if len(killed) == 0 {
		return r
	}
	out := make([]PointLengthParamRelation, 0, len(r.lengthParam))
	for _, rel := range r.lengthParam {
		if !killed[rel.TargetRoot] {
			out = append(out, rel)
		}
	}
	return PointRelations{
		siblingNil:  append([]SiblingNilRelation(nil), r.siblingNil...),
		lengthParam: compactPointLengthParams(out),
		sizeLower:   compactPointContainerLowerBounds(filterContainerLowerBoundsByKilledRoots(r.sizeLower, killed)),
	}
}

// LengthParamsForTarget returns every proven target-length relation for target.
func (r PointRelations) LengthParamsForTarget(target constraint.PathKey) []PointLengthParamRelation {
	if r.bottom || target == "" || len(r.lengthParam) == 0 {
		return nil
	}
	var out []PointLengthParamRelation
	for _, rel := range r.lengthParam {
		if rel.TargetKey == target {
			out = append(out, rel)
		}
	}
	return out
}

// ContainerLowerBoundFor returns the proven container cardinality lower bound
// for target, if every predecessor preserves one.
func (r PointRelations) ContainerLowerBoundFor(target constraint.PathKey) (int64, bool) {
	if r.bottom || target == "" || len(r.sizeLower) == 0 {
		return 0, false
	}
	for _, rel := range r.sizeLower {
		if rel.TargetKey == target {
			return rel.Lower, true
		}
	}
	return 0, false
}

// HasContainerLowerBound reports whether r proves target has at least lower
// entries.
func (r PointRelations) HasContainerLowerBound(root cfg.SymbolID, target constraint.PathKey, lower int64) bool {
	if root == 0 || target == "" || lower <= 0 {
		return false
	}
	got, ok := r.ContainerLowerBoundFor(target)
	if !ok {
		return false
	}
	for _, rel := range r.sizeLower {
		if rel.TargetRoot == root && rel.TargetKey == target {
			return got >= lower
		}
	}
	return false
}

// HasTargetLengthParam reports whether r proves one exact target/parameter
// length relation in a reachable state.
func (r PointRelations) HasTargetLengthParam(root cfg.SymbolID, target constraint.PathKey, paramIndex int) bool {
	if r.bottom || root == 0 || target == "" || paramIndex < 0 {
		return false
	}
	_, ok := slices.BinarySearchFunc(r.lengthParam, PointLengthParamRelation{
		TargetRoot: root,
		TargetKey:  target,
		ParamIndex: paramIndex,
	}, comparePointLengthParam)
	return ok
}

func compactReturnCorrelations(xs []ReturnCorrelation) []ReturnCorrelation {
	if len(xs) == 0 {
		return nil
	}
	out := append([]ReturnCorrelation(nil), xs...)
	slices.SortFunc(out, compareReturnCorrelation)
	out = slices.CompactFunc(out, func(a, b ReturnCorrelation) bool {
		return compareReturnCorrelation(a, b) == 0
	})
	return out
}

func compareReturnCorrelation(a, b ReturnCorrelation) int {
	if c := cmp.Compare(a.ValueIndex, b.ValueIndex); c != 0 {
		return c
	}
	return cmp.Compare(a.ErrorIndex, b.ErrorIndex)
}

func compactReturnLengthParams(xs []ReturnLengthParamRelation) []ReturnLengthParamRelation {
	if len(xs) == 0 {
		return nil
	}
	out := append([]ReturnLengthParamRelation(nil), xs...)
	slices.SortFunc(out, compareReturnLengthParam)
	out = slices.CompactFunc(out, func(a, b ReturnLengthParamRelation) bool {
		return compareReturnLengthParam(a, b) == 0
	})
	return out
}

func compareReturnLengthParam(a, b ReturnLengthParamRelation) int {
	if c := cmp.Compare(a.ReturnIndex, b.ReturnIndex); c != 0 {
		return c
	}
	return cmp.Compare(a.ParamIndex, b.ParamIndex)
}

func returnCorrelationsContainAll(have, want []ReturnCorrelation) bool {
	for _, w := range want {
		if _, ok := slices.BinarySearchFunc(have, w, compareReturnCorrelation); !ok {
			return false
		}
	}
	return true
}

func returnLengthParamsContainAll(have, want []ReturnLengthParamRelation) bool {
	for _, w := range want {
		if _, ok := slices.BinarySearchFunc(have, w, compareReturnLengthParam); !ok {
			return false
		}
	}
	return true
}

func intersectReturnCorrelations(a, b []ReturnCorrelation) []ReturnCorrelation {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []ReturnCorrelation
	for _, x := range a {
		if _, ok := slices.BinarySearchFunc(b, x, compareReturnCorrelation); ok {
			out = append(out, x)
		}
	}
	return out
}

func intersectReturnLengthParams(a, b []ReturnLengthParamRelation) []ReturnLengthParamRelation {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []ReturnLengthParamRelation
	for _, x := range a {
		if _, ok := slices.BinarySearchFunc(b, x, compareReturnLengthParam); ok {
			out = append(out, x)
		}
	}
	return out
}

func compactPointLengthParams(xs []PointLengthParamRelation) []PointLengthParamRelation {
	if len(xs) == 0 {
		return nil
	}
	out := make([]PointLengthParamRelation, 0, len(xs))
	for _, rel := range xs {
		if rel.TargetRoot == 0 || rel.TargetKey == "" || rel.ParamIndex < 0 {
			continue
		}
		out = append(out, rel)
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, comparePointLengthParam)
	out = slices.CompactFunc(out, func(a, b PointLengthParamRelation) bool {
		return comparePointLengthParam(a, b) == 0
	})
	return out
}

func comparePointLengthParam(a, b PointLengthParamRelation) int {
	if c := cmp.Compare(a.TargetRoot, b.TargetRoot); c != 0 {
		return c
	}
	if c := cmp.Compare(a.TargetKey, b.TargetKey); c != 0 {
		return c
	}
	return cmp.Compare(a.ParamIndex, b.ParamIndex)
}

func pointLengthParamsContainAll(have, want []PointLengthParamRelation) bool {
	for _, w := range want {
		if _, ok := slices.BinarySearchFunc(have, w, comparePointLengthParam); !ok {
			return false
		}
	}
	return true
}

func intersectPointLengthParams(a, b []PointLengthParamRelation) []PointLengthParamRelation {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []PointLengthParamRelation
	for _, x := range a {
		if _, ok := slices.BinarySearchFunc(b, x, comparePointLengthParam); ok {
			out = append(out, x)
		}
	}
	return out
}

func compactPointContainerLowerBounds(xs []PointContainerLowerBound) []PointContainerLowerBound {
	if len(xs) == 0 {
		return nil
	}
	best := make(map[containerLowerKey]int64, len(xs))
	for _, rel := range xs {
		if rel.TargetRoot == 0 || rel.TargetKey == "" || rel.Lower <= 0 {
			continue
		}
		key := containerLowerKey{root: rel.TargetRoot, target: rel.TargetKey}
		if cur, ok := best[key]; !ok || rel.Lower > cur {
			best[key] = rel.Lower
		}
	}
	if len(best) == 0 {
		return nil
	}
	out := make([]PointContainerLowerBound, 0, len(best))
	for key, lower := range best {
		out = append(out, PointContainerLowerBound{
			TargetRoot: key.root,
			TargetKey:  key.target,
			Lower:      lower,
		})
	}
	slices.SortFunc(out, comparePointContainerLowerBound)
	return out
}

type containerLowerKey struct {
	root   cfg.SymbolID
	target constraint.PathKey
}

func comparePointContainerLowerBound(a, b PointContainerLowerBound) int {
	if c := cmp.Compare(a.TargetRoot, b.TargetRoot); c != 0 {
		return c
	}
	if c := cmp.Compare(a.TargetKey, b.TargetKey); c != 0 {
		return c
	}
	return cmp.Compare(a.Lower, b.Lower)
}

func pointContainerLowerBoundsImplyAll(have, want []PointContainerLowerBound) bool {
	for _, w := range want {
		found := false
		for _, h := range have {
			if h.TargetRoot == w.TargetRoot && h.TargetKey == w.TargetKey && h.Lower >= w.Lower {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func joinPointContainerLowerBounds(a, b []PointContainerLowerBound) []PointContainerLowerBound {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []PointContainerLowerBound
	for _, x := range a {
		for _, y := range b {
			if x.TargetRoot != y.TargetRoot || x.TargetKey != y.TargetKey {
				continue
			}
			lower := x.Lower
			if y.Lower < lower {
				lower = y.Lower
			}
			out = append(out, PointContainerLowerBound{
				TargetRoot: x.TargetRoot,
				TargetKey:  x.TargetKey,
				Lower:      lower,
			})
			break
		}
	}
	return compactPointContainerLowerBounds(out)
}

func filterContainerLowerBoundsByKilledRoots(xs []PointContainerLowerBound, killed map[cfg.SymbolID]bool) []PointContainerLowerBound {
	if len(xs) == 0 || len(killed) == 0 {
		return append([]PointContainerLowerBound(nil), xs...)
	}
	out := make([]PointContainerLowerBound, 0, len(xs))
	for _, rel := range xs {
		if !killed[rel.TargetRoot] {
			out = append(out, rel)
		}
	}
	return out
}

func compactSiblingNil(xs []SiblingNilRelation) []SiblingNilRelation {
	if len(xs) == 0 {
		return nil
	}
	byErr := make(map[cfg.SymbolID][]cfg.SymbolID, len(xs))
	for _, rel := range xs {
		if rel.ErrSym == 0 || len(rel.ValueSyms) == 0 {
			continue
		}
		byErr[rel.ErrSym] = append(byErr[rel.ErrSym], rel.ValueSyms...)
	}
	if len(byErr) == 0 {
		return nil
	}
	out := make([]SiblingNilRelation, 0, len(byErr))
	for err, values := range byErr {
		values = compactSymbolIDs(values)
		if len(values) == 0 {
			continue
		}
		out = append(out, SiblingNilRelation{ErrSym: err, ValueSyms: values})
	}
	slices.SortFunc(out, compareSiblingNilErrOnly)
	return out
}

func compactSymbolIDs(xs []cfg.SymbolID) []cfg.SymbolID {
	out := append([]cfg.SymbolID(nil), xs...)
	slices.Sort(out)
	out = slices.Compact(out)
	return out
}

func compareSiblingNilErrOnly(a, b SiblingNilRelation) int {
	return cmp.Compare(a.ErrSym, b.ErrSym)
}

func siblingNilEqual(a, b SiblingNilRelation) bool {
	return a.ErrSym == b.ErrSym && slices.Equal(a.ValueSyms, b.ValueSyms)
}

func siblingNilContainsAll(have, want []SiblingNilRelation) bool {
	for _, w := range want {
		idx, ok := slices.BinarySearchFunc(have, w, compareSiblingNilErrOnly)
		if !ok {
			return false
		}
		if !symbolIDsContainAll(have[idx].ValueSyms, w.ValueSyms) {
			return false
		}
	}
	return true
}

func symbolIDsContainAll(have, want []cfg.SymbolID) bool {
	for _, w := range want {
		if _, ok := slices.BinarySearch(have, w); !ok {
			return false
		}
	}
	return true
}

func intersectSiblingNil(a, b []SiblingNilRelation) []SiblingNilRelation {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []SiblingNilRelation
	for _, ar := range a {
		idx, ok := slices.BinarySearchFunc(b, ar, compareSiblingNilErrOnly)
		if !ok {
			continue
		}
		values := intersectSymbolIDs(ar.ValueSyms, b[idx].ValueSyms)
		if len(values) == 0 {
			continue
		}
		out = append(out, SiblingNilRelation{ErrSym: ar.ErrSym, ValueSyms: values})
	}
	return out
}

func intersectSymbolIDs(a, b []cfg.SymbolID) []cfg.SymbolID {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}
	var out []cfg.SymbolID
	for _, x := range a {
		if _, ok := slices.BinarySearch(b, x); ok {
			out = append(out, x)
		}
	}
	return out
}
