// Package numeric owns the finite Numeric/equality Factor algebra.
//
// It is intentionally not the historical constraint/numeric theory API.  Its
// coordinates are sealed Program atoms and pair templates, never paths,
// textual variables, dynamically admitted constraints, or an SMT service.
package numeric

import (
	"math"
	"math/big"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// Root is Numeric's exact executable Program body coordinate. It is issued
// only by the algebra that owns the Link and is never a Link projection.
type Root struct {
	source *link.Link
	shard  linkproject.Shard
	body   keyspace.Term
}

func (root Root) Shard() linkproject.Shard { return root.shard }
func (root Root) Body() keyspace.Term      { return root.body }

// RootRef is Root's portable replay identity. It carries the Link identity and
// canonical Project mount ordinal, never a hot owner-fenced Project handle.
// Rebinding resolves that ordinal through the receiving Algebra's exact
// Project authority.
type RootRef struct {
	linkID       keyspace.ContentID
	shardOrdinal uint32
	body         keyspace.Term
}

func (ref RootRef) LinkID() keyspace.ContentID { return ref.linkID }
func (ref RootRef) Shard() uint32              { return ref.shardOrdinal }
func (ref RootRef) ShardOrdinal() uint32       { return ref.shardOrdinal }
func (ref RootRef) Body() keyspace.Term        { return ref.body }

// Scalar is Numeric's exact Program value occurrence in one executable body.
type Scalar struct {
	source *link.Link
	shard  linkproject.Shard
	body   keyspace.Term
	term   keyspace.Term
}

func (scalar Scalar) Shard() linkproject.Shard { return scalar.shard }
func (scalar Scalar) Body() keyspace.Term      { return scalar.body }
func (scalar Scalar) Term() keyspace.Term      { return scalar.term }

// Key is one private dense Numeric Root coordinate.
type Key struct {
	owner *Algebra
	slot  uint32
}

func (key Key) Valid() bool { return key.owner != nil && key.owner.validKey(key) }

// Atom is one private dense Numeric Scalar coordinate or the algebra's single
// mathematical zero. Its fields are opaque outside this package. A scalar is
// an occurrence, so equal Link Values at different program locations never
// collapse into a Numeric coordinate.
type Atom struct {
	owner *Algebra
	slot  uint32
}

func (atom Atom) Valid() bool {
	return atom.owner != nil && atom.slot != 0 && uint64(atom.slot) <= uint64(len(atom.owner.atoms))
}

// Pair is one opaque Algebra-local selector for an oriented difference
// template. Callers cannot construct topology from Atom handles; Pair values
// are issued only by Algebra.Pair after the Link-derived table is sealed.
type Pair struct {
	owner *Algebra
	slot  uint32
}

func (pair Pair) Valid() bool {
	return pair.owner != nil && pair.slot != 0 && uint64(pair.slot) <= uint64(len(pair.owner.pairs))
}

// Eligibility is the finite numeric/runtime mask for one atom. It does not
// identify concrete literals or erase integer-versus-float representation.
// Integral equality is a separate relation because a finite float may be
// mathematically integral without having the runtime integer representation.
type Eligibility uint8

const (
	MayInteger Eligibility = 1 << iota
	MayFiniteFloat
	MayInfinity
	MayNaN
	MayOther

	allEligibility             = MayInteger | MayFiniteFloat | MayInfinity | MayNaN | MayOther
	numericIntegralEligibility = MayInteger | MayFiniteFloat
)

func (mask Eligibility) Valid() bool { return mask != 0 && mask&^allEligibility == 0 }

func (mask Eligibility) DefinitelyNonNaN() bool { return mask.Valid() && mask&MayNaN == 0 }

func (mask Eligibility) MayContainNaN() bool { return mask.Valid() && mask&MayNaN != 0 }

func (mask Eligibility) OnlyNaN() bool { return mask == MayNaN }

func (mask Eligibility) MustInteger() bool {
	return mask.Valid() && mask&^MayInteger == 0
}

type template struct {
	Left       Atom
	Right      Atom
	Thresholds []int64
}

type pairRow struct {
	left  Atom
	right Atom
}

type pairIndex struct{ left, right Atom }

// component is a cold, sealed connected component of the declared pair
// graph.  It is topology only: relation images never materialize a dense
// closure table.  Sources contains the declared left endpoints whose exact
// observable bounds are emitted by a closure pass.
type component struct {
	atoms   []int
	pairs   []int
	sources []source
}

type source struct {
	atom    int
	outputs []int
}

// scratch is pooled owner-local workspace.  It is not part of Value and is
// returned before an operation returns, so relation images remain immutable.
// Its size is linear in the sealed owner vocabulary.
type scratch struct {
	masks            []Eligibility
	equal            []uint64
	unequal          []uint64
	integral         []uint64
	bounds           []uint16
	parents          []int
	integralClass    []bool
	literalClass     []int
	originalEqual    []uint64
	originalUnequal  []uint64
	originalIntegral []uint64
	originalBounds   []uint16
	classPairs       []classPair
	distance         []int64
	potential        []int64
	edges            []edge
	activeRoots      []int
	heap             []heapEntry
	saturated        bool
}

// edge and heapEntry are operation-local sparse graph machinery.  They are
// never retained by Value or exposed as another constraint representation.
type edge struct {
	from, to int
	weight   int64
}

type heapEntry struct {
	atom     int
	distance int64
}

// Algebra is one sealed Numeric/equality carrier vocabulary. Construction is
// cold and projects only Link-issued identities; all returned Values are
// immutable and owner-fenced. The hot algebra knows no Rule, engine, path, or
// generic constraint-solver type.
type Algebra struct {
	source         *link.Link
	linkID         keyspace.ContentID
	content        keyspace.ContentID
	keys           []keyRow
	keyIndex       map[RootRef]uint32
	keyAtoms       [][]uint32
	keyPairs       [][]uint32
	atomScalars    []Scalar
	atomScalarRefs []scalarRef
	atomLiterals   []numericLiteral
	literalAtoms   []uint32
	scalarAtoms    map[Scalar]Atom
	atoms          []Atom
	atomIndex      map[Atom]int
	pairs          []pairRow
	pairIndex      map[pairIndex]int
	thresholds     [][]int64
	components     []component
	fingerprint    uint64
	scratch        sync.Pool
	bottom         Value
	defaultValue   Value
}

type numericLiteral struct {
	kind    uint8
	integer int64
	float   float64
}

const (
	literalNone uint8 = iota
	literalInteger
	literalFloat
)

// Value is Bottom or one normalized finite Numeric/equality relation.
// Unexported slices are immutable after construction. A missing bound is the
// implicit positive-infinity element of that template's threshold chain.
type Value struct {
	algebra  *Algebra
	bottom   bool
	masks    []atomFact
	equal    []uint32
	unequal  []uint32
	integral []uint32
	bounds   []boundFact
}

type atomFact struct {
	slot uint32
	mask Eligibility
}

type boundFact struct {
	slot  uint32
	level uint16
}

type denseValue struct {
	masks    []Eligibility
	equal    []uint64
	unequal  []uint64
	integral []uint64
	bounds   []uint16
}

// Seal creates the sole finite vocabulary for one owner declaration.  Its
// result is canonical: permutations of equivalent atom/template input yield
// the same atom and pair ordinals, threshold chains, topology fingerprint,
// and later relation hashes.  Pair templates themselves are observations, so
// duplicate pairs are rejected rather than silently changing the language.
//
// Seal also precomputes the sparse pair components and the observable source
// groups used by normalization.  No operation constructs an n*n matrix.
func seal(owner *Algebra, atomCount int) bool {
	if owner == nil || atomCount == 0 || uint64(atomCount) > uint64(math.MaxUint32) {
		return false
	}
	owner.atoms = make([]Atom, atomCount)
	owner.atomIndex = make(map[Atom]int, atomCount)
	for index := range owner.atoms {
		atom := Atom{owner: owner, slot: uint32(index + 1)}
		owner.atoms[index] = atom
		owner.atomIndex[atom] = index
	}
	return true
}

func sealThresholds(input []int64) ([]int64, bool) {
	chain := append([]int64(nil), input...)
	for _, threshold := range chain {
		// MaxInt64 is the internal infinity sentinel and cannot be a finite
		// Program threshold. This makes closure arithmetic unambiguous.
		if threshold == math.MaxInt64 {
			return nil, false
		}
	}
	sort.Slice(chain, func(left, right int) bool { return chain[left] < chain[right] })
	unique := chain[:0]
	for _, threshold := range chain {
		if len(unique) == 0 || unique[len(unique)-1] != threshold {
			unique = append(unique, threshold)
		}
	}
	if len(unique) >= math.MaxUint16 {
		return nil, false
	}
	return unique, true
}

// Default is the sparse no-assumption value: every atom has the full
// eligibility mask and every pair/bound is absent. It is Top of this Factor's
// information order, not an unreachable control-flow state.
func (algebra *Algebra) Default() Value {
	if !algebra.Valid() {
		return Value{}
	}
	return algebra.defaultValue
}

func (algebra *Algebra) Bottom() Value {
	if !algebra.Valid() {
		return Value{}
	}
	return algebra.bottom
}

func (algebra *Algebra) Top() Value {
	return algebra.Default()
}

// Relation constructs and normalizes one finite relation. Every supplied
// fact must already use a sealed atom/pair/template. It is a cold domain API;
// Rules will later construct it from typed Program operation laws.
func (algebra *Algebra) AdmitAt(key Key, masks map[Atom]Eligibility, equal, unequal, integral []Pair, bounds map[Pair]int64) (Value, bool) {
	if !algebra.validKey(key) {
		return Value{}, false
	}
	value := Value{algebra: algebra}
	for atom, mask := range masks {
		index, ok := algebra.atomIndex[atom]
		if !ok || !mask.Valid() {
			return Value{}, false
		}
		mask &= algebra.baseEligibility(index)
		if !mask.Valid() {
			return Value{}, false
		}
		if mask != algebra.baseEligibility(index) {
			value.masks = append(value.masks, atomFact{slot: atom.slot, mask: mask})
		}
	}
	for _, pair := range equal {
		index, ok := algebra.index(pair)
		if !ok {
			return Value{}, false
		}
		value.equal = append(value.equal, uint32(index+1))
	}
	for _, pair := range unequal {
		index, ok := algebra.index(pair)
		if !ok {
			return Value{}, false
		}
		value.unequal = append(value.unequal, uint32(index+1))
	}
	for _, pair := range integral {
		index, ok := algebra.index(pair)
		if !ok {
			return Value{}, false
		}
		value.integral = append(value.integral, uint32(index+1))
	}
	for pair, threshold := range bounds {
		index, ok := algebra.index(pair)
		if !ok {
			return Value{}, false
		}
		level, ok := algebra.exactLevel(index, threshold)
		if !ok {
			return Value{}, false
		}
		value.bounds = append(value.bounds, boundFact{slot: uint32(index + 1), level: level})
	}
	sort.Slice(value.masks, func(i, j int) bool { return value.masks[i].slot < value.masks[j].slot })
	sort.Slice(value.equal, func(i, j int) bool { return value.equal[i] < value.equal[j] })
	sort.Slice(value.unequal, func(i, j int) bool { return value.unequal[i] < value.unequal[j] })
	sort.Slice(value.integral, func(i, j int) bool { return value.integral[i] < value.integral[j] })
	sort.Slice(value.bounds, func(i, j int) bool { return value.bounds[i].slot < value.bounds[j].slot })
	if !uniqueAtomFacts(value.masks) || !strictUint32(value.equal) || !strictUint32(value.unequal) ||
		!strictUint32(value.integral) || !uniqueBoundFacts(value.bounds) {
		return Value{}, false
	}
	return algebra.admitImage(key, value)
}

// admitImage is the common exact relation boundary for the generic cold
// constructor and the fixed-arity transfer kernels. Its caller must provide
// sorted, duplicate-free fact slices. No caller may round thresholds or add
// atoms/pairs outside the sealed algebra.
func (algebra *Algebra) admitImage(key Key, value Value) (Value, bool) {
	if !algebra.validKey(key) || !algebra.owns(value) || value.bottom {
		return Value{}, false
	}
	normalized, ok := algebra.normalize(value)
	if !ok || !algebra.Admits(key, normalized) {
		return Value{}, false
	}
	return normalized, true
}

// Lattice exposes the owner-fenced finite lattice used by a later Factor
// declaration. Cross-owner Values are invalid inputs and normalize to Bottom;
// a Factor only receives Values constructed by its own Algebra.
func (algebra *Algebra) Lattice() lattice.Lattice[Value] {
	return lattice.Lattice[Value]{
		Bottom:   algebra.Bottom,
		Top:      algebra.Top,
		Equal:    algebra.Equal,
		Same:     algebra.Same,
		LessOrEq: algebra.LessOrEq,
		Join:     algebra.Join,
		Meet:     algebra.Meet,
		Widen:    algebra.Widen,
	}
}

func (algebra *Algebra) Same(left, right Value) bool {
	if !algebra.owns(left) || !algebra.owns(right) || left.bottom != right.bottom ||
		len(left.masks) != len(right.masks) || len(left.equal) != len(right.equal) ||
		len(left.unequal) != len(right.unequal) || len(left.integral) != len(right.integral) || len(left.bounds) != len(right.bounds) {
		return false
	}
	if left.bottom || len(left.masks) == 0 && len(left.equal) == 0 && len(left.unequal) == 0 && len(left.integral) == 0 && len(left.bounds) == 0 {
		return true
	}
	return sameAtomFacts(left.masks, right.masks) && sameUint32(left.equal, right.equal) &&
		sameUint32(left.unequal, right.unequal) && sameUint32(left.integral, right.integral) && sameBoundFacts(left.bounds, right.bounds)
}

func (algebra *Algebra) Equal(left, right Value) bool {
	if !algebra.owns(left) || !algebra.owns(right) || left.bottom != right.bottom {
		return false
	}
	if left.bottom {
		return true
	}
	return equalAtomFacts(left.masks, right.masks) && equalUint32(left.equal, right.equal) &&
		equalUint32(left.unequal, right.unequal) && equalUint32(left.integral, right.integral) && equalBoundFacts(left.bounds, right.bounds)
}

func (algebra *Algebra) LessOrEq(left, right Value) bool {
	if !algebra.owns(left) || !algebra.owns(right) {
		return false
	}
	if left.bottom {
		return true
	}
	if right.bottom {
		return false
	}
	for _, fact := range right.masks {
		if algebra.mask(left, fact.slot)&^fact.mask != 0 {
			return false
		}
	}
	// Equality/disequality are must facts: every fact retained by the less
	// precise right operand must be present in left.
	if !containsUint32Set(left.equal, right.equal) || !containsUint32Set(left.unequal, right.unequal) ||
		!containsUint32Set(left.integral, right.integral) {
		return false
	}
	for _, fact := range right.bounds {
		level, present := boundLevel(left.bounds, fact.slot)
		if !present || level > fact.level {
			return false
		}
	}
	return true
}

func (algebra *Algebra) Join(left, right Value) Value {
	if !algebra.owns(left) || !algebra.owns(right) {
		return Value{}
	}
	if left.bottom {
		return right
	}
	if right.bottom {
		return left
	}
	if left.isDefault() || right.isDefault() {
		return algebra.defaultValue
	}
	if algebra.Same(left, right) {
		return left
	}
	value := Value{algebra: algebra}
	value.masks = joinAtomFacts(left.masks, right.masks)
	value.equal = intersectUint32(left.equal, right.equal)
	value.unequal = intersectUint32(left.unequal, right.unequal)
	value.integral = intersectUint32(left.integral, right.integral)
	value.bounds = joinBoundFacts(left.bounds, right.bounds)
	result, ok := algebra.normalize(value)
	if !ok {
		return algebra.Bottom()
	}
	return result
}

func (algebra *Algebra) Meet(left, right Value) Value {
	if !algebra.owns(left) || !algebra.owns(right) {
		return Value{}
	}
	if left.bottom || right.bottom {
		return algebra.Bottom()
	}
	if left.isDefault() {
		return right
	}
	if right.isDefault() || algebra.Same(left, right) {
		return left
	}
	value := Value{algebra: algebra}
	value.masks = meetAtomFacts(left.masks, right.masks)
	value.equal = unionUint32(left.equal, right.equal)
	value.unequal = unionUint32(left.unequal, right.unequal)
	value.integral = unionUint32(left.integral, right.integral)
	value.bounds = meetBoundFacts(left.bounds, right.bounds)
	result, ok := algebra.normalize(value)
	if !ok {
		return algebra.Bottom()
	}
	return result
}

// Widen equals Join: the sealed atoms, pair templates, and threshold chains
// make the carrier finite height. No budget or exhaustion fallback exists.
func (algebra *Algebra) Widen(previous, next Value) Value { return algebra.Join(previous, next) }

func (value Value) IsBottom() bool  { return value.valid() && value.bottom }
func (value Value) IsDefault() bool { return value.valid() && value.isDefault() }

func (value Value) isDefault() bool {
	if value.bottom || value.algebra == nil {
		return false
	}
	defaultValue := value.algebra.defaultValue
	if defaultValue.algebra == nil {
		return len(value.masks) == 0 && len(value.equal) == 0 && len(value.unequal) == 0 && len(value.integral) == 0 && len(value.bounds) == 0
	}
	return equalAtomFacts(value.masks, defaultValue.masks) && equalUint32(value.equal, defaultValue.equal) &&
		equalUint32(value.unequal, defaultValue.unequal) && equalUint32(value.integral, defaultValue.integral) &&
		equalBoundFacts(value.bounds, defaultValue.bounds)
}

func (value Value) Eligibility(atom Atom) (Eligibility, bool) {
	if !value.valid() {
		return 0, false
	}
	index, ok := value.algebra.atomIndex[atom]
	if !ok {
		return 0, false
	}
	return value.algebra.mask(value, uint32(index+1)), true
}

func (value Value) MustEqual(pair Pair) bool {
	if value.has(value.equal, pair) {
		return true
	}
	left, right, ok := pair.Atoms()
	if !ok {
		return false
	}
	leftIndex, rightIndex := int(left.slot-1), int(right.slot-1)
	if equal, known := value.algebra.literalEqual(leftIndex, rightIndex); known {
		return equal
	}
	if left != right {
		return false
	}
	mask, ok := value.Eligibility(left)
	return ok && mask.DefinitelyNonNaN()
}

func (value Value) MustUnequal(pair Pair) bool {
	if value.has(value.unequal, pair) {
		return true
	}
	left, right, ok := pair.Atoms()
	if !ok {
		return false
	}
	if equal, known := value.algebra.literalEqual(int(left.slot-1), int(right.slot-1)); known {
		return !equal
	}
	if left != right {
		return false
	}
	mask, ok := value.Eligibility(left)
	return ok && mask.OnlyNaN()
}

func (value Value) MustIntegralEqual(pair Pair) bool {
	if value.has(value.integral, pair) {
		return true
	}
	left, right, ok := pair.Atoms()
	if !ok {
		return false
	}
	leftIndex, rightIndex := int(left.slot-1), int(right.slot-1)
	leftIntegral, leftKnown := value.algebra.literalIntegral(leftIndex)
	rightIntegral, rightKnown := value.algebra.literalIntegral(rightIndex)
	if leftKnown && rightKnown {
		equal, _ := value.algebra.literalEqual(leftIndex, rightIndex)
		return leftIntegral && rightIntegral && equal
	}
	if left != right {
		return false
	}
	mask, ok := value.Eligibility(left)
	if !ok {
		return false
	}
	return mask.MustInteger()
}

// Bound returns the canonical upper bound for a declared pair. Infinite is
// true for the implicit positive-infinity default.
func (value Value) Bound(pair Pair) (bound int64, infinite bool, ok bool) {
	if !value.valid() {
		return 0, false, false
	}
	index, ok := value.algebra.index(pair)
	if !ok {
		return 0, false, false
	}
	levelValue, present := boundLevel(value.bounds, uint32(index+1))
	level := len(value.algebra.thresholds[index])
	if present {
		level = int(levelValue)
	}
	if level == len(value.algebra.thresholds[index]) {
		if value.MustIntegralEqual(pair) {
			return 0, false, true
		}
		return 0, true, true
	}
	return value.algebra.thresholds[index][level], false, true
}

func (value Value) Hash() uint64 {
	if !value.valid() {
		return 0
	}
	writer := hash.NewWriter()
	_, _ = writer.WriteString("numeric.factor")
	writer.WriteUintHex(value.algebra.fingerprint)
	if value.bottom {
		_ = writer.WriteByte(0)
		return writer.Sum64()
	}
	_ = writer.WriteByte(1)
	_ = writer.WriteByte(1)
	writer.WriteUintDecimal(uint64(len(value.masks)))
	for _, fact := range value.masks {
		writer.WriteUintDecimal(uint64(fact.slot))
		_ = writer.WriteByte(byte(fact.mask))
	}
	_ = writer.WriteByte(2)
	writer.WriteUintDecimal(uint64(len(value.equal)))
	for _, slot := range value.equal {
		writer.WriteUintDecimal(uint64(slot))
	}
	_ = writer.WriteByte(3)
	writer.WriteUintDecimal(uint64(len(value.unequal)))
	for _, slot := range value.unequal {
		writer.WriteUintDecimal(uint64(slot))
	}
	_ = writer.WriteByte(4)
	writer.WriteUintDecimal(uint64(len(value.integral)))
	for _, slot := range value.integral {
		writer.WriteUintDecimal(uint64(slot))
	}
	_ = writer.WriteByte(5)
	writer.WriteUintDecimal(uint64(len(value.bounds)))
	for _, fact := range value.bounds {
		writer.WriteUintDecimal(uint64(fact.slot))
		writer.WriteUintDecimal(uint64(fact.level))
	}
	return writer.Sum64()
}

func (algebra *Algebra) normalize(value Value) (Value, bool) {
	if !algebra.owns(value) {
		return Value{}, false
	}
	if value.bottom {
		return value, true
	}
	work := algebra.borrowScratch()
	defer algebra.releaseScratch(work)
	image, ok := algebra.expand(value, work)
	if !ok {
		return Value{}, false
	}
	for index := range work.parents {
		work.parents[index] = index
		work.integralClass[index] = false
		work.literalClass[index] = -1
	}
	for index, pair := range algebra.pairs {
		if bitAt(image.integral, index) == 0 {
			continue
		}
		left := algebra.atomIndex[pair.left]
		right := algebra.atomIndex[pair.right]
		if integral, known := algebra.literalIntegral(left); known && !integral {
			return Value{}, false
		}
		if integral, known := algebra.literalIntegral(right); known && !integral {
			return Value{}, false
		}
		if equal, known := algebra.literalEqual(left, right); known && !equal {
			return Value{}, false
		}
		image.masks[left] &= numericIntegralEligibility
		image.masks[right] &= numericIntegralEligibility
		if !image.masks[left].Valid() || !image.masks[right].Valid() {
			return Value{}, false
		}
		work.integralClass[left] = true
		work.integralClass[right] = true
		union(work.parents, left, right)
	}
	for index, pair := range algebra.pairs {
		if bitAt(image.equal, index) == 0 {
			continue
		}
		left := algebra.atomIndex[pair.left]
		right := algebra.atomIndex[pair.right]
		if equal, known := algebra.literalEqual(left, right); known && !equal {
			return Value{}, false
		}
		if !image.masks[left].DefinitelyNonNaN() || !image.masks[right].DefinitelyNonNaN() {
			return Value{}, false
		}
		union(work.parents, left, right)
	}
	for atom, mask := range image.masks {
		integralLiteral, literal := algebra.literalIntegral(atom)
		if mask.MustInteger() || literal && integralLiteral {
			work.integralClass[atom] = true
		}
	}
	for atom, witnessed := range work.integralClass {
		if witnessed {
			work.integralClass[find(work.parents, atom)] = true
		}
	}
	// Pairwise checks above catch direct literal collisions. This class pass
	// also catches the transitive case literal-a == x == literal-b without
	// allocating a map on the normalization path.
	for _, slot := range algebra.literalAtoms {
		atom := int(slot - 1)
		root := find(work.parents, atom)
		previous := work.literalClass[root]
		if previous >= 0 {
			if equal, known := algebra.literalEqual(previous, atom); !known || !equal {
				return Value{}, false
			}
			continue
		}
		work.literalClass[root] = atom
	}
	for atom := range image.masks {
		root := find(work.parents, atom)
		if !work.integralClass[root] {
			continue
		}
		image.masks[atom] &= numericIntegralEligibility
		if !image.masks[atom].Valid() {
			return Value{}, false
		}
	}
	// Equality is a full primitive equality congruence over the existing pair
	// templates; its representation remains separate from eligibility masks.
	copy(work.originalEqual, image.equal)
	copy(work.originalUnequal, image.unequal)
	copy(work.originalIntegral, image.integral)
	copy(work.originalBounds, image.bounds)
	clearWords(image.equal)
	for index, pair := range algebra.pairs {
		left, right := algebra.atomIndex[pair.left], algebra.atomIndex[pair.right]
		// A self pair is not reflexively equal when its atom may be NaN. It
		// becomes an equality fact only when an explicit non-NaN equality law
		// supplied it. Distinct atoms become equal only through such a law.
		literalEqual, literalKnown := algebra.literalEqual(left, right)
		structuralEqual := literalKnown && literalEqual
		structuralSelf := pair.left == pair.right && algebra.baseEligibility(left).DefinitelyNonNaN()
		explicitSelf := pair.left == pair.right && !structuralSelf &&
			(bitAt(work.originalEqual, index) != 0 || bitAt(work.originalIntegral, index) != 0 ||
				hasAtomFact(value.masks, pair.left.slot) && image.masks[left].DefinitelyNonNaN())
		if !structuralEqual && (pair.left != pair.right || explicitSelf) && find(work.parents, left) == find(work.parents, right) {
			setBit(image.equal, index)
		}
	}
	clearWords(image.integral)
	for index, pair := range algebra.pairs {
		leftAtom := algebra.atomIndex[pair.left]
		rightAtom := algebra.atomIndex[pair.right]
		left := find(work.parents, leftAtom)
		right := find(work.parents, rightAtom)
		integralLiteral, literal := algebra.literalIntegral(leftAtom)
		rightIntegral, rightLiteral := algebra.literalIntegral(rightAtom)
		literalEqual, equalityKnown := algebra.literalEqual(leftAtom, rightAtom)
		structuralEquality := literal && rightLiteral && integralLiteral && rightIntegral && equalityKnown && literalEqual
		structuralSelf := pair.left == pair.right && (algebra.baseEligibility(leftAtom).MustInteger() || literal && integralLiteral)
		explicitSelf := pair.left == pair.right && !structuralSelf &&
			(bitAt(work.originalIntegral, index) != 0 ||
				(bitAt(work.originalEqual, index) != 0 || hasAtomFact(value.masks, pair.left.slot)) && image.masks[leftAtom].MustInteger())
		if !structuralEquality && left == right && work.integralClass[left] && (pair.left != pair.right || explicitSelf) {
			setBit(image.integral, index)
		}
	}
	work.classPairs = work.classPairs[:0]
	for index, pair := range algebra.pairs {
		if bitAt(work.originalUnequal, index) == 0 {
			continue
		}
		if pair.left == pair.right {
			atom := algebra.atomIndex[pair.left]
			// IEEE/Lua primitive self-disequality is exactly the NaN case.
			// Retain it as a finite eligibility refinement instead of declaring
			// every x~=x branch contradictory.
			image.masks[atom] &= MayNaN
			if !image.masks[atom].Valid() {
				return Value{}, false
			}
			continue
		}
		leftAtom, rightAtom := algebra.atomIndex[pair.left], algebra.atomIndex[pair.right]
		if equal, known := algebra.literalEqual(leftAtom, rightAtom); known {
			if equal {
				return Value{}, false
			}
			continue // structurally entailed literal disequality is not stored
		}
		left, right := find(work.parents, leftAtom), find(work.parents, rightAtom)
		if left == right {
			return Value{}, false
		}
		work.classPairs = append(work.classPairs, newClassPair(left, right))
	}
	sortClassPairs(work.classPairs)
	work.classPairs = uniqueClassPairs(work.classPairs)
	// Disequality is symmetric and follows equality congruence too. It is
	// re-emitted only for the existing observed pair templates.
	clearWords(image.unequal)
	for index, pair := range algebra.pairs {
		if pair.left == pair.right {
			// Self-disequality has already become the exact MayNaN mask above.
			// NaN is deliberately never retained as a reflexive disequality edge.
			continue
		}
		left, right := find(work.parents, algebra.atomIndex[pair.left]), find(work.parents, algebra.atomIndex[pair.right])
		if classPairContains(work.classPairs, newClassPair(left, right)) {
			setBit(image.unequal, index)
		}
	}
	if !algebra.normalizeBounds(image, work) {
		return Value{}, false
	}
	return algebra.compact(image), true
}

// normalizeBounds computes exact difference closure only through the finite
// declared pair graph, and writes only declared pair observations back to the
// Value.  Its workspace is O(atoms + pairs), never O(atoms squared).
func (algebra *Algebra) normalizeBounds(value *denseValue, work *scratch) bool {
	for index, pair := range algebra.pairs {
		level := int(work.originalBounds[index])
		if level > len(algebra.thresholds[index]) {
			return false
		}
		left, right := algebra.atomIndex[pair.left], algebra.atomIndex[pair.right]
		if level != len(algebra.thresholds[index]) && !algebra.literalBoundHolds(left, right, algebra.thresholds[index][level]) {
			return false
		}
		if level != len(algebra.thresholds[index]) &&
			(!work.integralClass[find(work.parents, left)] || !work.integralClass[find(work.parents, right)]) {
			return false
		}
	}
	for _, component := range algebra.components {
		if !algebra.buildEdges(component, value, work) {
			return false
		}
		// Bounds are consequences of the active sparse graph, not permanent
		// entries in a dense closure image. Reset only this component's declared
		// observations before deriving the ones reachable from an active source.
		for _, index := range component.pairs {
			value.bounds[index] = uint16(len(algebra.thresholds[index]))
		}
		if len(work.edges) == 0 {
			continue
		}
		if algebra.derivePotentials(component, work) {
			return false
		}
		if !work.saturated && !reweightable(work.edges, work.potential) {
			work.saturated = true
		}
		// Edges are sorted by quotient source. Run closure once per active source
		// class, never once per sealed observation source. This makes the common
		// zero-edge image O(V+E), and a one-edge image one shortest-path pass even
		// when the sealed component declares many possible observations.
		work.activeRoots = activeBoundRoots(work.activeRoots[:0], work.edges)
		for _, activeRoot := range work.activeRoots {
			if work.saturated {
				algebra.shortestFromSaturated(component, activeRoot, work)
			} else {
				algebra.shortestFrom(component, activeRoot, work)
			}
			for _, source := range component.sources {
				if find(work.parents, source.atom) != activeRoot {
					continue
				}
				for _, index := range source.outputs {
					pair := algebra.pairs[index]
					left, right := algebra.atomIndex[pair.left], algebra.atomIndex[pair.right]
					if !work.integralClass[find(work.parents, left)] || !work.integralClass[find(work.parents, right)] {
						continue
					}
					if find(work.parents, left) == find(work.parents, right) {
						// Integral equality already entails the exact zero bound.
						// Keep it derived rather than storing one redundant fact per
						// self/equal pair in every image.
						continue
					}
					// x-y<=c is the graph edge y->x. The source group is the
					// declared right endpoint and the observed distance is left.
					distance := work.distance[find(work.parents, left)]
					if !work.saturated {
						distance = restoreDistance(distance, work.potential[activeRoot], work.potential[find(work.parents, left)])
					}
					if distance == math.MaxInt64 {
						continue
					}
					level, ok := algebra.ceilLevel(index, distance)
					if !ok {
						return false
					}
					value.bounds[index] = level
				}
			}
		}
	}
	return true
}

// activeBoundRoots projects the unique quotient sources that can produce a
// non-reflexive finite bound. edges are already sorted by source.
func activeBoundRoots(output []int, edges []edge) []int {
	for _, item := range edges {
		if len(output) == 0 || output[len(output)-1] != item.from {
			output = append(output, item.from)
		}
	}
	return output
}

// buildEdges maps the active declared bounds through equality classes once.
// The resulting edge image is sparse and operation-local.  Sorting by source
// lets the exact shortest-path pass touch only outgoing declared edges.
func (algebra *Algebra) buildEdges(component component, value *denseValue, work *scratch) bool {
	work.edges = work.edges[:0]
	work.saturated = false
	for _, index := range component.pairs {
		pair := algebra.pairs[index]
		left, right := algebra.atomIndex[pair.left], algebra.atomIndex[pair.right]
		level := int(work.originalBounds[index])
		if level == len(algebra.thresholds[index]) {
			continue
		}
		if !work.integralClass[find(work.parents, left)] || !work.integralClass[find(work.parents, right)] {
			return false
		}
		// Difference logic uses the conventional graph encoding:
		// left-right<=weight becomes right->left with that weight.
		from, to := find(work.parents, right), find(work.parents, left)
		weight := algebra.thresholds[index][level]
		if weight == math.MinInt64 {
			work.saturated = true
		}
		if from == to {
			if weight < 0 {
				return false
			}
			continue
		}
		work.edges = append(work.edges, edge{from: from, to: to, weight: weight})
	}
	sortEdges(work.edges)
	return true
}

// derivePotentials is Bellman-Ford over one sealed sparse component with all
// vertices initially zero.  It both rejects any negative cycle and produces
// Johnson potentials, so later exact source closures use non-negative sparse
// edges rather than a dense all-pairs matrix.
func (algebra *Algebra) derivePotentials(component component, work *scratch) bool {
	for _, atom := range component.atoms {
		work.potential[atom] = 0
	}
	for pass := 0; pass < len(component.atoms); pass++ {
		changed := false
		for _, edge := range work.edges {
			candidate, saturated := saturatingAdd(work.potential[edge.from], edge.weight)
			if saturated || candidate == math.MinInt64 {
				work.saturated = true
			}
			if candidate < work.potential[edge.to] {
				work.potential[edge.to] = candidate
				changed = true
			}
		}
		if !changed {
			if work.saturated {
				return hasNegativeCycleExact(component, work.edges)
			}
			return false
		}
		if pass == len(component.atoms)-1 {
			if work.saturated {
				return hasNegativeCycleExact(component, work.edges)
			}
			return true
		}
	}
	return false
}

// hasNegativeCycleExact is the rare overflow path for the domain-specific
// difference graph. Saturating int64 arithmetic is sound for representable
// output bounds, but cannot prove cycle absence after a negative underflow.
// Exact integers are therefore used only for that proof; they never enter the
// carrier, threshold vocabulary, or ordinary hot closure path.
func hasNegativeCycleExact(component component, edges []edge) bool {
	distance := make([]big.Int, len(component.atoms))
	position := make(map[int]int, len(component.atoms))
	for index, atom := range component.atoms {
		position[atom] = index
	}
	var candidate, weight big.Int
	for pass := 0; pass < len(component.atoms); pass++ {
		changed := false
		for _, item := range edges {
			from, fromOK := position[item.from]
			to, toOK := position[item.to]
			if !fromOK || !toOK {
				continue
			}
			weight.SetInt64(item.weight)
			candidate.Add(&distance[from], &weight)
			if candidate.Cmp(&distance[to]) < 0 {
				distance[to].Set(&candidate)
				changed = true
			}
		}
		if !changed {
			return false
		}
		if pass == len(component.atoms)-1 {
			return true
		}
	}
	return false
}

func reweightable(edges []edge, potential []int64) bool {
	for _, edge := range edges {
		if potential[edge.to] == math.MinInt64 {
			return false
		}
		first, overflow := saturatingAdd(edge.weight, potential[edge.from])
		if overflow {
			return false
		}
		weight, overflow := saturatingAdd(first, -potential[edge.to])
		if overflow || weight < 0 {
			return false
		}
	}
	return true
}

// shortestFromSaturated preserves the carrier's established saturating-int64
// semantics for the rare boundary case where Johnson reweighting itself is
// not representable. It remains sparse and exact for that semantics; only its
// operation count falls back to the finite Bellman-Ford formula.
func (algebra *Algebra) shortestFromSaturated(component component, source int, work *scratch) {
	for _, atom := range component.atoms {
		work.distance[atom] = math.MaxInt64
	}
	work.distance[find(work.parents, source)] = 0
	for pass := 0; pass < len(component.atoms)-1; pass++ {
		changed := false
		for _, edge := range work.edges {
			if work.distance[edge.from] == math.MaxInt64 {
				continue
			}
			candidate := safeAdd(work.distance[edge.from], edge.weight)
			if candidate < work.distance[edge.to] {
				work.distance[edge.to] = candidate
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

// shortestFrom is deterministic Dijkstra over Johnson-reweighted active
// declared edges.  It has no iteration budget: finite sealed topology and a
// prior negative-cycle proof are its only convergence argument.
func (algebra *Algebra) shortestFrom(component component, source int, work *scratch) {
	for _, atom := range component.atoms {
		work.distance[atom] = math.MaxInt64
	}
	root := find(work.parents, source)
	work.distance[root] = 0
	work.heap = work.heap[:0]
	pushHeap(&work.heap, heapEntry{atom: root, distance: 0})
	for len(work.heap) != 0 {
		current := popHeap(&work.heap)
		if current.distance != work.distance[current.atom] {
			continue
		}
		first := sort.Search(len(work.edges), func(index int) bool { return work.edges[index].from >= current.atom })
		for index := first; index < len(work.edges) && work.edges[index].from == current.atom; index++ {
			edge := work.edges[index]
			weight := reweight(edge.weight, work.potential[edge.from], work.potential[edge.to])
			candidate := safeAdd(current.distance, weight)
			if candidate >= work.distance[edge.to] {
				continue
			}
			work.distance[edge.to] = candidate
			pushHeap(&work.heap, heapEntry{atom: edge.to, distance: candidate})
		}
	}
}

func (algebra *Algebra) borrowScratch() *scratch {
	return algebra.scratch.Get().(*scratch)
}

func (algebra *Algebra) releaseScratch(work *scratch) {
	work.classPairs = work.classPairs[:0]
	algebra.scratch.Put(work)
}

// sealComponents precomputes the undirected pair components and output source
// groups.  They are immutable schema metadata, not a second constraint graph.
func sealComponents(algebra *Algebra) []component {
	parents := make([]int, len(algebra.atoms))
	for index := range parents {
		parents[index] = index
	}
	for _, pair := range algebra.pairs {
		union(parents, algebra.atomIndex[pair.left], algebra.atomIndex[pair.right])
	}
	rootIndex := make(map[int]int, len(algebra.atoms))
	atomComponent := make([]int, len(algebra.atoms))
	components := make([]component, 0, len(algebra.atoms))
	for atom := range algebra.atoms {
		root := find(parents, atom)
		index, ok := rootIndex[root]
		if !ok {
			index = len(components)
			rootIndex[root] = index
			components = append(components, component{})
		}
		atomComponent[atom] = index
		components[index].atoms = append(components[index].atoms, atom)
	}
	for index, pair := range algebra.pairs {
		componentIndex := atomComponent[algebra.atomIndex[pair.left]]
		components[componentIndex].pairs = append(components[componentIndex].pairs, index)
	}
	sourceOutputs := make([]map[int][]int, len(components))
	for index, pair := range algebra.pairs {
		componentIndex := atomComponent[algebra.atomIndex[pair.left]]
		if sourceOutputs[componentIndex] == nil {
			sourceOutputs[componentIndex] = make(map[int][]int)
		}
		right := algebra.atomIndex[pair.right]
		sourceOutputs[componentIndex][right] = append(sourceOutputs[componentIndex][right], index)
	}
	for componentIndex := range components {
		component := &components[componentIndex]
		for _, atom := range component.atoms {
			if outputs := sourceOutputs[componentIndex][atom]; len(outputs) != 0 {
				component.sources = append(component.sources, source{atom: atom, outputs: outputs})
			}
		}
	}
	return components
}

func (algebra *Algebra) schemaHash() uint64 {
	writer := hash.NewWriter()
	_, _ = writer.WriteString("numeric.schema.v2")
	for _, atom := range algebra.atoms {
		writer.WriteUintDecimal(uint64(atom.slot))
	}
	for index, pair := range algebra.pairs {
		writer.WriteUintDecimal(uint64(pair.left.slot))
		writer.WriteUintDecimal(uint64(pair.right.slot))
		for _, threshold := range algebra.thresholds[index] {
			writer.WriteIntDecimal(threshold)
		}
	}
	return writer.Sum64()
}

func (algebra *Algebra) owns(value Value) bool {
	return algebra != nil && value.algebra == algebra
}

func (value Value) valid() bool { return value.algebra != nil && value.algebra.owns(value) }

func (algebra *Algebra) index(pair Pair) (int, bool) {
	if !pair.Valid() || pair.owner != algebra {
		return 0, false
	}
	return int(pair.slot - 1), true
}

func (algebra *Algebra) ceilLevel(index int, bound int64) (uint16, bool) {
	chain := algebra.thresholds[index]
	level := sort.Search(len(chain), func(candidate int) bool { return chain[candidate] >= bound })
	if level == len(chain) {
		return uint16(level), true
	}
	return uint16(level), true
}

// exactLevel is the admission-side operation. Rules may select only a bound
// already frozen into the pair's finite threshold chain. ceilLevel remains
// private to closure, where rounding a derived distance upward is the lawful
// abstraction into that chain.
func (algebra *Algebra) exactLevel(index int, bound int64) (uint16, bool) {
	chain := algebra.thresholds[index]
	level := sort.Search(len(chain), func(candidate int) bool { return chain[candidate] >= bound })
	if level == len(chain) || chain[level] != bound {
		return 0, false
	}
	return uint16(level), true
}

func (algebra *Algebra) literalEqual(left, right int) (bool, bool) {
	if left < 0 || right < 0 || left >= len(algebra.atomLiterals) || right >= len(algebra.atomLiterals) {
		return false, false
	}
	l, r := algebra.atomLiterals[left], algebra.atomLiterals[right]
	if l.kind == literalNone || r.kind == literalNone {
		return false, false
	}
	switch {
	case l.kind == literalInteger && r.kind == literalInteger:
		return l.integer == r.integer, true
	case l.kind == literalFloat && r.kind == literalFloat:
		return !math.IsNaN(l.float) && !math.IsNaN(r.float) && l.float == r.float, true
	case l.kind == literalInteger && r.kind == literalFloat:
		return integerFloatEqual(l.integer, r.float), true
	case l.kind == literalFloat && r.kind == literalInteger:
		return integerFloatEqual(r.integer, l.float), true
	default:
		return false, true
	}
}

func integerFloatEqual(integer int64, number float64) bool {
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number ||
		number < -9223372036854775808.0 || number >= 9223372036854775808.0 {
		return false
	}
	return int64(number) == integer
}

func (algebra *Algebra) literalEligibility(atom int) (Eligibility, bool) {
	if atom < 0 || atom >= len(algebra.atomLiterals) {
		return 0, false
	}
	literal := algebra.atomLiterals[atom]
	switch literal.kind {
	case literalInteger:
		return MayInteger, true
	case literalFloat:
		switch {
		case math.IsNaN(literal.float):
			return MayNaN, true
		case math.IsInf(literal.float, 0):
			return MayInfinity, true
		default:
			return MayFiniteFloat, true
		}
	default:
		return 0, false
	}
}

func (algebra *Algebra) baseEligibility(atom int) Eligibility {
	if exact, known := algebra.literalEligibility(atom); known {
		return exact
	}
	return allEligibility
}

func (algebra *Algebra) literalIntegral(atom int) (bool, bool) {
	if atom < 0 || atom >= len(algebra.atomLiterals) {
		return false, false
	}
	literal := algebra.atomLiterals[atom]
	switch literal.kind {
	case literalInteger:
		return true, true
	case literalFloat:
		return !math.IsNaN(literal.float) && !math.IsInf(literal.float, 0) && math.Trunc(literal.float) == literal.float, true
	default:
		return false, false
	}
}

func (algebra *Algebra) literalBoundHolds(left, right int, bound int64) bool {
	var leftValue, rightValue big.Int
	if !algebra.literalIntegerValue(left, &leftValue) || !algebra.literalIntegerValue(right, &rightValue) {
		return true
	}
	var difference, threshold big.Int
	difference.Sub(&leftValue, &rightValue)
	threshold.SetInt64(bound)
	return difference.Cmp(&threshold) <= 0
}

func (algebra *Algebra) literalIntegerValue(atom int, output *big.Int) bool {
	if output == nil {
		return false
	}
	if integral, known := algebra.literalIntegral(atom); !known || !integral {
		return false
	}
	literal := algebra.atomLiterals[atom]
	if literal.kind == literalInteger {
		output.SetInt64(literal.integer)
		return true
	}
	var floating big.Float
	floating.SetFloat64(literal.float)
	_, accuracy := floating.Int(output)
	return accuracy == big.Exact
}

func (value Value) has(slots []uint32, pair Pair) bool {
	if !value.valid() {
		return false
	}
	index, ok := value.algebra.index(pair)
	return ok && containsUint32(slots, uint32(index+1))
}

func wordCount(count int) int                { return (count + 63) / 64 }
func setBit(words []uint64, index int)       { words[index/64] |= uint64(1) << uint(index%64) }
func bitAt(words []uint64, index int) uint64 { return (words[index/64] >> uint(index%64)) & 1 }
func clearWords(words []uint64) {
	for index := range words {
		words[index] = 0
	}
}

func maxUint16(left, right uint16) uint16 {
	if left > right {
		return left
	}
	return right
}
func minUint16(left, right uint16) uint16 {
	if left < right {
		return left
	}
	return right
}
func safeAdd(left, right int64) int64 {
	result, _ := saturatingAdd(left, right)
	return result
}

func saturatingAdd(left, right int64) (int64, bool) {
	if right > 0 && left > math.MaxInt64-right {
		return math.MaxInt64, true
	}
	if right < 0 && left < math.MinInt64-right {
		return math.MinInt64, true
	}
	return left + right, false
}

// reweight is the sealed int64 counterpart of w + h[from] - h[to]. It is
// called only after reweightable established that this exact arithmetic fits.
func reweight(weight, fromPotential, toPotential int64) int64 {
	return weight + fromPotential - toPotential
}

func restoreDistance(distance, sourcePotential, targetPotential int64) int64 {
	if distance == math.MaxInt64 || sourcePotential == math.MinInt64 {
		return math.MaxInt64
	}
	return safeAdd(safeAdd(distance, -sourcePotential), targetPotential)
}

func find(parents []int, node int) int {
	for parents[node] != node {
		parents[node] = parents[parents[node]]
		node = parents[node]
	}
	return node
}
func union(parents []int, left, right int) {
	left, right = find(parents, left), find(parents, right)
	if left < right {
		parents[right] = left
	} else if right < left {
		parents[left] = right
	}
}

type classPair struct{ left, right int }

func sortEdges(items []edge) {
	for root := len(items)/2 - 1; root >= 0; root-- {
		siftEdge(items, root, len(items))
	}
	for end := len(items) - 1; end > 0; end-- {
		items[0], items[end] = items[end], items[0]
		siftEdge(items, 0, end)
	}
}

func siftEdge(items []edge, root, end int) {
	for child := root*2 + 1; child < end; child = root*2 + 1 {
		if child+1 < end && edgeLess(items[child], items[child+1]) {
			child++
		}
		if !edgeLess(items[root], items[child]) {
			return
		}
		items[root], items[child] = items[child], items[root]
		root = child
	}
}

func edgeLess(left, right edge) bool {
	if left.from != right.from {
		return left.from < right.from
	}
	if left.to != right.to {
		return left.to < right.to
	}
	return left.weight < right.weight
}

func sortClassPairs(items []classPair) {
	for root := len(items)/2 - 1; root >= 0; root-- {
		siftClassPair(items, root, len(items))
	}
	for end := len(items) - 1; end > 0; end-- {
		items[0], items[end] = items[end], items[0]
		siftClassPair(items, 0, end)
	}
}

func siftClassPair(items []classPair, root, end int) {
	for child := root*2 + 1; child < end; child = root*2 + 1 {
		if child+1 < end && classPairLess(items[child], items[child+1]) {
			child++
		}
		if !classPairLess(items[root], items[child]) {
			return
		}
		items[root], items[child] = items[child], items[root]
		root = child
	}
}

func classPairLess(left, right classPair) bool {
	if left.left != right.left {
		return left.left < right.left
	}
	return left.right < right.right
}

func newClassPair(left, right int) classPair {
	if left > right {
		left, right = right, left
	}
	return classPair{left: left, right: right}
}

func uniqueClassPairs(pairs []classPair) []classPair {
	if len(pairs) == 0 {
		return pairs
	}
	write := 1
	for read := 1; read < len(pairs); read++ {
		if pairs[read] == pairs[write-1] {
			continue
		}
		pairs[write] = pairs[read]
		write++
	}
	return pairs[:write]
}

func classPairContains(pairs []classPair, wanted classPair) bool {
	index := sort.Search(len(pairs), func(index int) bool {
		if pairs[index].left != wanted.left {
			return pairs[index].left >= wanted.left
		}
		return pairs[index].right >= wanted.right
	})
	return index < len(pairs) && pairs[index] == wanted
}

func pushHeap(heap *[]heapEntry, entry heapEntry) {
	items := append(*heap, entry)
	index := len(items) - 1
	for index > 0 {
		parent := (index - 1) / 2
		if !heapLess(items[index], items[parent]) {
			break
		}
		items[index], items[parent] = items[parent], items[index]
		index = parent
	}
	*heap = items
}

func popHeap(heap *[]heapEntry) heapEntry {
	items := *heap
	result := items[0]
	last := len(items) - 1
	items[0] = items[last]
	items = items[:last]
	for index := 0; ; {
		left := index*2 + 1
		if left >= len(items) {
			break
		}
		right := left + 1
		smallest := left
		if right < len(items) && heapLess(items[right], items[left]) {
			smallest = right
		}
		if !heapLess(items[smallest], items[index]) {
			break
		}
		items[index], items[smallest] = items[smallest], items[index]
		index = smallest
	}
	*heap = items
	return result
}

func heapLess(left, right heapEntry) bool {
	if left.distance != right.distance {
		return left.distance < right.distance
	}
	return left.atom < right.atom
}
