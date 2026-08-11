// Package sequence owns the handle-only Lua result-pack carrier.
//
// A Value is Bottom, PackTop, or a finite unordered disjunction of one-tail
// modes. A mode is either a closed word, P·h*·S, or P·opaque*·S. The finite
// disjunction is necessary: a single one-tail form has no least upper bound
// for ordinary correlated Lua result packs. Modes retain the correlation; no
// operation here joins handles or consults a subtype relation.
package sequence

import (
	"math/bits"
	"sort"

	"github.com/wippyai/go-lua/analysis/internal/hash"
)

// Handle is a dense Table-local type coordinate. owner is an in-process fence
// against accidental cross-Table use; ordinal is the compact hot coordinate.
// Neither field is a portable identity.
type Handle struct{ raw uint64 }

// NewHandle is usable only by the owning domain package because sequence is
// internal. External callers cannot forge a nonzero Handle.
func NewHandle(owner, ordinal uint32) Handle {
	return Handle{raw: uint64(owner)<<32 | uint64(ordinal)}
}

func (h Handle) ValidFor(owner uint32) bool {
	return owner != 0 && uint32(h.raw>>32) == owner && uint32(h.raw) != 0
}

func (h Handle) Ordinal() uint32 { return uint32(h.raw) }

// Labels is deliberately narrower than a type lattice. Pack labels are flat:
// an exact handle is comparable only with itself and the Table's TypeTop.
// This prevents result-pack operations from silently performing subtype or
// union algebra and thereby destroying positional correlations.
type Labels interface {
	Equal(Handle, Handle) bool
	Hash(Handle) uint64
	Nil() Handle
	Never() Handle
	TypeTop() Handle
}

// ModeKind describes the one optional repeated middle of a result pack.
type ModeKind uint8

const (
	ModeClosed ModeKind = iota + 1
	ModeKnown
	ModeOpaque
)

// Mode is an immutable one-tail language. Its slices are never exposed
// directly; constructors and accessors copy them. A Mode is only admitted to
// a Value by the owning Table, which validates every handle first.
type Mode struct {
	kind   ModeKind
	prefix []Handle
	tail   Handle // meaningful only for ModeKnown
	suffix []Handle
	closed closedWord
}

func ClosedMode(values ...Handle) Mode {
	return Mode{kind: ModeClosed, closed: closedWordFromFlat(values)}
}

func KnownMode(prefix []Handle, element Handle, suffix []Handle) Mode {
	return Mode{kind: ModeKnown, prefix: copyHandles(prefix), tail: element, suffix: copyHandles(suffix)}
}

func OpaqueMode(prefix, suffix []Handle) Mode {
	return Mode{kind: ModeOpaque, prefix: copyHandles(prefix), suffix: copyHandles(suffix)}
}

func (m Mode) Kind() ModeKind { return m.kind }

// Prefix materializes the closed word only for a cold query. Hot lattice
// operations use ClosedAt/ClosedIterator instead, so retained rope words
// never flatten merely to compare, hash, or iterate them.
func (m Mode) Prefix() []Handle {
	if m.kind == ModeClosed {
		return m.closed.Materialize()
	}
	return copyHandles(m.prefix)
}
func (m Mode) Suffix() []Handle     { return copyHandles(m.suffix) }
func (m Mode) Tail() (Handle, bool) { return m.tail, m.kind == ModeKnown }

// ClosedLen reports the exact closed-word width. It is zero for an open mode.
func (m Mode) ClosedLen() int {
	if m.kind != ModeClosed {
		return 0
	}
	return m.closed.length
}

// ClosedAt returns one closed-word label without materializing its retained
// backing. It is allocation-free and is the hot random-access API.
func (m Mode) ClosedAt(index int) (Handle, bool) {
	if m.kind != ModeClosed {
		return Handle{}, false
	}
	return m.closed.At(index)
}

// ClosedIterator returns an allocation-free exact closed-word iterator.
func (m Mode) ClosedIterator() ClosedIterator {
	if m.kind != ModeClosed {
		return ClosedIterator{}
	}
	return newClosedIterator(m.closed)
}

// ValidMode checks only representation and local-coordinate ownership. It
// deliberately has no type algebra: the Table remains the sole authority.
func ValidMode(mode Mode, valid func(Handle) bool) bool {
	if valid == nil {
		return false
	}
	switch mode.kind {
	case ModeClosed, ModeKnown, ModeOpaque:
	default:
		return false
	}
	if mode.kind == ModeClosed {
		for index := 0; index < mode.closed.length; index++ {
			handle, _ := mode.closed.At(index)
			if !valid(handle) {
				return false
			}
		}
		return true
	}
	for _, handle := range mode.prefix {
		if !valid(handle) {
			return false
		}
	}
	if mode.kind == ModeKnown && !valid(mode.tail) {
		return false
	}
	for _, handle := range mode.suffix {
		if !valid(handle) {
			return false
		}
	}
	return true
}

type valueState uint8

const (
	stateBottom valueState = iota
	stateModes
	stateTop
)

// Value is a persistent immutable pack fact. A finite alternative set is a
// semantic set, never a trace or provenance list: its physical order has no
// effect on Equal, Hash, LessEqual, Join, or Widen.
type Value struct {
	state valueState
	modes []Mode
}

// Inputs is the private hot-path view of fixed Values operands.  Assemble
// needs indexed access only; requiring callers to first materialize []Value
// makes every outer carrier pay an avoidable conversion allocation.  A slice
// of an outer immutable carrier can implement this view directly while the
// sequence package stays independent of that carrier and of its ownership
// fence.
//
// Implementations must be stable for the duration of one call.  Assemble
// never retains Inputs or a Value returned from it.
type Inputs interface {
	Len() int
	At(int) Value
}

type valueInputs []Value

func (input valueInputs) Len() int           { return len(input) }
func (input valueInputs) At(index int) Value { return input[index] }

func Bottom() Value { return Value{state: stateBottom} }
func Top() Value    { return Value{state: stateTop} }

func FromModes(labels Labels, modes ...Mode) Value {
	return normalize(labels, modes)
}

func (v Value) IsBottom() bool { return v.state == stateBottom }
func (v Value) IsTop() bool    { return v.state == stateTop }

func (v Value) Modes() []Mode {
	if v.state != stateModes {
		return nil
	}
	return copyModes(v.modes)
}

func (v Value) ModeCount() int {
	if v.state != stateModes {
		return 0
	}
	return len(v.modes)
}

func copyHandles(in []Handle) []Handle {
	if len(in) == 0 {
		return nil
	}
	return append([]Handle(nil), in...)
}

func copyMode(mode Mode) Mode {
	return Mode{kind: mode.kind, prefix: copyHandles(mode.prefix), tail: mode.tail, suffix: copyHandles(mode.suffix), closed: mode.closed.copy()}
}

func copyModes(modes []Mode) []Mode {
	if len(modes) == 0 {
		return nil
	}
	out := make([]Mode, len(modes))
	for index, mode := range modes {
		out[index] = copyMode(mode)
	}
	return out
}

// normalize applies only proved language-preserving rewrites. In particular,
// it never merges a pair of alternatives merely because their labels could be
// joined in the Table: that would fabricate cross-product correlations.
func normalize(labels Labels, input []Mode) Value {
	if labels == nil {
		return Bottom()
	}
	modes := make([]Mode, 0, len(input))
	for _, source := range input {
		mode, keep, top := normalizeMode(labels, source)
		if top {
			return Top()
		}
		if keep {
			modes = append(modes, mode)
		}
	}
	if len(modes) == 0 {
		return Bottom()
	}
	modes = deduplicate(labels, modes)
	modes = removeDominated(labels, modes)
	if len(modes) == 0 {
		return Bottom()
	}
	sortModes(modes)
	return Value{state: stateModes, modes: modes}
}

func normalizeMode(labels Labels, source Mode) (mode Mode, keep, top bool) {
	mode = copyMode(source)
	switch mode.kind {
	case ModeClosed:
		if len(mode.suffix) != 0 {
			mode.closed = concatClosedWords(mode.closed, closedWordFromFlat(mode.suffix))
		}
		mode.prefix = nil
		mode.suffix = nil
	case ModeKnown, ModeOpaque:
	default:
		return Mode{}, false, false
	}
	if mode.kind == ModeClosed && closedContains(labels, mode.closed, labels.Never()) {
		return Mode{}, false, false
	}
	if mode.kind != ModeClosed && (contains(labels, mode.prefix, labels.Never()) || contains(labels, mode.suffix, labels.Never())) {
		return Mode{}, false, false
	}
	if mode.kind == ModeKnown && labels.Equal(mode.tail, labels.Never()) {
		// Never* has only the zero-length repetition.
		mode.kind = ModeClosed
		mode.closed = concatClosedWords(closedWordFromSharedFlat(mode.prefix), closedWordFromSharedFlat(mode.suffix))
		mode.prefix = nil
		mode.suffix = nil
	}
	if mode.kind == ModeKnown {
		// P·h*·h^k·S = P·h^k·h*·S. This is an equivalence, not a
		// widening, and makes a+ have one representation.
		for len(mode.suffix) != 0 && labels.Equal(mode.suffix[0], mode.tail) {
			mode.prefix = append(mode.prefix, mode.suffix[0])
			mode.suffix = mode.suffix[1:]
		}
	}
	if mode.kind == ModeOpaque && len(mode.prefix) == 0 && len(mode.suffix) == 0 {
		return Mode{}, false, true
	}
	return mode, true, false
}

func closedContains(labels Labels, word closedWord, want Handle) bool {
	for index := 0; index < word.length; index++ {
		value, _ := word.At(index)
		if labels.Equal(value, want) {
			return true
		}
	}
	return false
}

func contains(labels Labels, values []Handle, want Handle) bool {
	for _, value := range values {
		if labels.Equal(value, want) {
			return true
		}
	}
	return false
}

func deduplicate(labels Labels, modes []Mode) []Mode {
	out := make([]Mode, 0, len(modes))
	buckets := make(map[uint64][]int, len(modes))
	for _, mode := range modes {
		duplicate := false
		modeHash := hashMode(labels, mode)
		for _, index := range buckets[modeHash] {
			if modeEqual(labels, mode, out[index]) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			buckets[modeHash] = append(buckets[modeHash], len(out))
			out = append(out, mode)
		}
	}
	return out
}

// frame is the fixed portion of an open one-tail language.  The tail kind is
// deliberately not part of the key: a Known mode may be covered by an
// Opaque mode with the same fixed frame.
type frame struct{ prefix, suffix int }

func removeDominated(labels Labels, modes []Mode) []Mode {
	// Containment is much narrower than the all-pairs representation made it
	// appear.  An open mode can only be covered by an open mode with the same
	// fixed frame.  A closed word can only be covered by another closed word
	// of its own length, or an open mode whose fixed frame fits inside that
	// word.  Indexing those necessary candidates removes the historical
	// quadratic scan across unrelated skeletons without changing the language
	// relation (closed-versus-open remains deliberately exact).
	closed := make(map[int][]int, len(modes))
	opens := make(map[frame][]int, len(modes))
	frames := make([]frame, 0, len(modes))
	for index, mode := range modes {
		if mode.kind == ModeClosed {
			closed[mode.closed.length] = append(closed[mode.closed.length], index)
			continue
		}
		key := frame{prefix: len(mode.prefix), suffix: len(mode.suffix)}
		if _, present := opens[key]; !present {
			frames = append(frames, key)
		}
		opens[key] = append(opens[key], index)
	}
	sort.Slice(frames, func(left, right int) bool {
		if frames[left].prefix != frames[right].prefix {
			return frames[left].prefix < frames[right].prefix
		}
		return frames[left].suffix < frames[right].suffix
	})

	out := make([]Mode, 0, len(modes))
	for index, mode := range modes {
		dominated := false
		var candidates []int
		if mode.kind == ModeClosed {
			candidates = closed[mode.closed.length]
		} else {
			candidates = opens[frame{prefix: len(mode.prefix), suffix: len(mode.suffix)}]
		}
		for _, otherIndex := range candidates {
			if index != otherIndex && modeLessEqual(labels, mode, modes[otherIndex]) {
				dominated = true
				break
			}
		}
		if !dominated && mode.kind == ModeClosed {
			for _, key := range frames {
				if key.prefix+key.suffix > mode.closed.length {
					continue
				}
				for _, otherIndex := range opens[key] {
					if modeLessEqual(labels, mode, modes[otherIndex]) {
						dominated = true
						break
					}
				}
				if dominated {
					break
				}
			}
		}
		if !dominated {
			out = append(out, mode)
		}
	}
	return out
}

func sortModes(modes []Mode) {
	sort.Slice(modes, func(left, right int) bool {
		return compareMode(modes[left], modes[right]) < 0
	})
}

func compareMode(left, right Mode) int {
	if left.kind < right.kind {
		return -1
	}
	if left.kind > right.kind {
		return 1
	}
	if left.kind == ModeClosed {
		if left.closed.length < right.closed.length {
			return -1
		}
		if left.closed.length > right.closed.length {
			return 1
		}
		for index := 0; index < left.closed.length; index++ {
			leftHandle, _ := left.closed.At(index)
			rightHandle, _ := right.closed.At(index)
			if leftHandle.raw < rightHandle.raw {
				return -1
			}
			if leftHandle.raw > rightHandle.raw {
				return 1
			}
		}
		return 0
	}
	// Shape comes before labels so Widen can scan one contiguous group per
	// skeleton. This order is operational only; Equal and Hash remain setwise.
	if len(left.prefix) < len(right.prefix) {
		return -1
	}
	if len(left.prefix) > len(right.prefix) {
		return 1
	}
	if len(left.suffix) < len(right.suffix) {
		return -1
	}
	if len(left.suffix) > len(right.suffix) {
		return 1
	}
	if result := compareHandles(left.prefix, right.prefix); result != 0 {
		return result
	}
	if left.kind == ModeKnown {
		if left.tail.raw < right.tail.raw {
			return -1
		}
		if left.tail.raw > right.tail.raw {
			return 1
		}
	}
	return compareHandles(left.suffix, right.suffix)
}

func compareHandles(left, right []Handle) int {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if left[index].raw < right[index].raw {
			return -1
		}
		if left[index].raw > right[index].raw {
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func Equal(labels Labels, left, right Value) bool {
	if left.state != right.state {
		return false
	}
	if left.state != stateModes {
		return true
	}
	if len(left.modes) != len(right.modes) {
		return false
	}
	// normalize sorts, but retain set semantics even if a future internal
	// constructor supplies a different physical order.
	for _, leftMode := range left.modes {
		found := false
		for _, rightMode := range right.modes {
			if modeEqual(labels, leftMode, rightMode) {
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

func modeEqual(labels Labels, left, right Mode) bool {
	if left.kind != right.kind {
		return false
	}
	if left.kind == ModeClosed {
		return left.closed.equal(labels, right.closed)
	}
	return handlesEqual(labels, left.prefix, right.prefix) &&
		(left.kind != ModeKnown || labels.Equal(left.tail, right.tail)) &&
		handlesEqual(labels, left.suffix, right.suffix)
}

func handlesEqual(labels Labels, left, right []Handle) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !labels.Equal(left[index], right[index]) {
			return false
		}
	}
	return true
}

// Hash is order independent by construction. Hash collisions only feed the
// Factor's candidate bucket; Equal remains the authority.
func Hash(labels Labels, value Value) uint64 {
	h := uint64(0x9b91a671f37a429d)
	h = hash.MixHash(h, uint64(value.state))
	if value.state != stateModes {
		return h
	}
	var sum, folded uint64
	for _, mode := range value.modes {
		modeHash := hashMode(labels, mode)
		sum += modeHash * 0x9e3779b97f4a7c15
		folded ^= bits.RotateLeft64(modeHash, int(modeHash&63))
	}
	h = hash.MixHash(h, uint64(len(value.modes)))
	h = hash.MixHash(h, sum)
	return hash.MixHash(h, folded)
}

func hashMode(labels Labels, mode Mode) uint64 {
	h := uint64(0x6d0f27bd45c1a29b)
	h = hash.MixHash(h, uint64(mode.kind))
	if mode.kind == ModeClosed {
		return mode.closed.hash(labels, h)
	}
	h = hashHandles(labels, h, mode.prefix)
	if mode.kind == ModeKnown {
		h = hash.MixHash(h, labels.Hash(mode.tail))
	}
	return hashHandles(labels, h, mode.suffix)
}

func hashHandles(labels Labels, current uint64, values []Handle) uint64 {
	current = hash.MixHash(current, uint64(len(values)))
	for _, value := range values {
		current = hash.MixHash(current, labels.Hash(value))
	}
	return current
}

// LessEqual is the finite-disjunction order: every left alternative must be
// structurally covered by one right alternative. It intentionally does not
// attempt arbitrary regular-language inclusion across an alternative union.
func LessEqual(labels Labels, left, right Value) bool {
	if left.IsBottom() || right.IsTop() || Equal(labels, left, right) {
		return true
	}
	if left.IsTop() || right.IsBottom() || left.state != stateModes || right.state != stateModes {
		return false
	}
	for _, leftMode := range left.modes {
		covered := false
		for _, rightMode := range right.modes {
			if modeLessEqual(labels, leftMode, rightMode) {
				covered = true
				break
			}
		}
		if !covered {
			return false
		}
	}
	return true
}

func modeLessEqual(labels Labels, left, right Mode) bool {
	if left.kind == ModeClosed {
		return closedLessEqual(labels, left.closed, right)
	}
	switch right.kind {
	case ModeKnown:
		return left.kind == ModeKnown && sameSkeleton(left, right) &&
			handlesLessEqual(labels, left.prefix, right.prefix) &&
			labelLessEqual(labels, left.tail, right.tail) &&
			handlesLessEqual(labels, left.suffix, right.suffix)
	case ModeOpaque:
		return (left.kind == ModeKnown || left.kind == ModeOpaque) && sameFrame(left, right) &&
			handlesLessEqual(labels, left.prefix, right.prefix) &&
			handlesLessEqual(labels, left.suffix, right.suffix)
	default:
		return false
	}
}

func closedLessEqual(labels Labels, word closedWord, right Mode) bool {
	switch right.kind {
	case ModeClosed:
		return word.lessEqual(labels, right.closed)
	case ModeKnown, ModeOpaque:
		frame := len(right.prefix) + len(right.suffix)
		if word.length < frame || !closedPrefixLessEqual(labels, word, right.prefix) {
			return false
		}
		middleEnd := word.length - len(right.suffix)
		if right.kind == ModeKnown {
			for index := len(right.prefix); index < middleEnd; index++ {
				label, _ := word.At(index)
				if !labelLessEqual(labels, label, right.tail) {
					return false
				}
			}
		}
		for index, want := range right.suffix {
			got, _ := word.At(middleEnd + index)
			if !labelLessEqual(labels, got, want) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func closedPrefixLessEqual(labels Labels, word closedWord, prefix []Handle) bool {
	for index, want := range prefix {
		got, _ := word.At(index)
		if !labelLessEqual(labels, got, want) {
			return false
		}
	}
	return true
}

func sameSkeleton(left, right Mode) bool {
	return left.kind == right.kind && sameFrame(left, right)
}

func sameFrame(left, right Mode) bool {
	if left.kind == ModeClosed || right.kind == ModeClosed {
		return left.kind == ModeClosed && right.kind == ModeClosed && left.closed.length == right.closed.length
	}
	return len(left.prefix) == len(right.prefix) && len(left.suffix) == len(right.suffix)
}

func handlesLessEqual(labels Labels, left, right []Handle) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !labelLessEqual(labels, left[index], right[index]) {
			return false
		}
	}
	return true
}

func labelLessEqual(labels Labels, left, right Handle) bool {
	return labels.Equal(left, right) || labels.Equal(right, labels.TypeTop())
}

// Join is exact finite alternative union. It never joins two labels in the
// Table and therefore retains tuple/pack correlations across control flow.
func Join(labels Labels, left, right Value) Value {
	if left.IsBottom() {
		return right
	}
	if right.IsBottom() || Equal(labels, left, right) {
		return left
	}
	if left.IsTop() || right.IsTop() {
		return Top()
	}
	modes := make([]Mode, 0, len(left.modes)+len(right.modes))
	modes = append(modes, left.modes...)
	modes = append(modes, right.modes...)
	return normalize(labels, modes)
}

// Rank is the exact two-component lexicographic termination measure for
// Widen. ShapeClass descends Bottom (2) -> finite modes (1) -> PackTop (0).
// Within one stable skeleton set Widen only changes an exact label to TypeTop,
// so ExactLabels strictly descends. Counts are exact for every realizable Go
// value: a Value cannot contain more labels than an int-sized slice.
type Rank struct {
	ShapeClass  uint64
	ExactLabels uint64
}

func WidenRank(labels Labels, value Value) Rank {
	switch value.state {
	case stateBottom:
		return Rank{ShapeClass: 2}
	case stateTop:
		return Rank{}
	case stateModes:
		var exact uint64
		for _, mode := range value.modes {
			exact += uint64(exactLabels(labels, mode))
		}
		return Rank{ShapeClass: 1, ExactLabels: exact}
	default:
		return Rank{}
	}
}

func exactLabels(labels Labels, mode Mode) int {
	count := 0
	if mode.kind == ModeClosed {
		for index := 0; index < mode.closed.length; index++ {
			label, _ := mode.closed.At(index)
			if !labels.Equal(label, labels.TypeTop()) {
				count++
			}
		}
		return count
	}
	for _, label := range mode.prefix {
		if !labels.Equal(label, labels.TypeTop()) {
			count++
		}
	}
	if mode.kind == ModeKnown && !labels.Equal(mode.tail, labels.TypeTop()) {
		count++
	}
	for _, label := range mode.suffix {
		if !labels.Equal(label, labels.TypeTop()) {
			count++
		}
	}
	return count
}

// Widen is only for an explicit compiled Mu boundary. Acyclic propagation
// uses Join. With equal skeleton sets it removes only distinctions that vary
// across incoming alternatives; a changed skeleton set reaches PackTop. This
// provides a cap-free, executable strict descent proof through WidenRank.
func Widen(labels Labels, previous, next Value) Value {
	if LessEqual(labels, next, previous) {
		return previous
	}
	if previous.IsBottom() {
		return next
	}
	if previous.IsTop() || next.IsTop() || previous.state != stateModes || next.state != stateModes {
		return Top()
	}
	if !sameSkeletonSet(previous.modes, next.modes) {
		return Top()
	}
	all := make([]Mode, 0, len(previous.modes)+len(next.modes))
	all = append(all, previous.modes...)
	all = append(all, next.modes...)
	sortModes(all)
	generalized := make([]Mode, 0, len(all))
	for start := 0; start < len(all); {
		end := start + 1
		for end < len(all) && sameSkeleton(all[start], all[end]) {
			end++
		}
		generalized = append(generalized, generalize(labels, all[start:end]))
		start = end
	}
	return normalize(labels, generalized)
}

func sameSkeletonSet(left, right []Mode) bool {
	leftShapes := uniqueSkeletons(left)
	rightShapes := uniqueSkeletons(right)
	if len(leftShapes) != len(rightShapes) {
		return false
	}
	for index := range leftShapes {
		if leftShapes[index] != rightShapes[index] {
			return false
		}
	}
	return true
}

type skeleton struct {
	kind           ModeKind
	prefix, suffix int
}

func uniqueSkeletons(modes []Mode) []skeleton {
	if len(modes) == 0 {
		return nil
	}
	out := make([]skeleton, 0, len(modes))
	for _, mode := range modes {
		prefix, suffix := len(mode.prefix), len(mode.suffix)
		if mode.kind == ModeClosed {
			prefix, suffix = mode.closed.length, 0
		}
		shape := skeleton{kind: mode.kind, prefix: prefix, suffix: suffix}
		seen := false
		for _, prior := range out {
			if prior == shape {
				seen = true
				break
			}
		}
		if !seen {
			out = append(out, shape)
		}
	}
	sort.Slice(out, func(left, right int) bool {
		if out[left].kind != out[right].kind {
			return out[left].kind < out[right].kind
		}
		if out[left].prefix != out[right].prefix {
			return out[left].prefix < out[right].prefix
		}
		return out[left].suffix < out[right].suffix
	})
	return out
}

func generalize(labels Labels, modes []Mode) Mode {
	result := copyMode(modes[0])
	if result.kind == ModeClosed {
		values := make([]Handle, result.closed.length)
		for index := range values {
			label, _ := result.closed.At(index)
			if !unanimous(labels, modes, label, func(mode Mode) Handle {
				value, _ := mode.closed.At(index)
				return value
			}) {
				values[index] = labels.TypeTop()
			} else {
				values[index] = label
			}
		}
		result.closed = closedWordFromFlat(values)
		return result
	}
	for index, label := range result.prefix {
		if !unanimous(labels, modes, label, func(mode Mode) Handle { return mode.prefix[index] }) {
			result.prefix[index] = labels.TypeTop()
		}
	}
	if result.kind == ModeKnown && !unanimous(labels, modes, result.tail, func(mode Mode) Handle { return mode.tail }) {
		result.tail = labels.TypeTop()
	}
	for index, label := range result.suffix {
		if !unanimous(labels, modes, label, func(mode Mode) Handle { return mode.suffix[index] }) {
			result.suffix[index] = labels.TypeTop()
		}
	}
	return result
}

func unanimous(labels Labels, modes []Mode, first Handle, at func(Mode) Handle) bool {
	for _, mode := range modes[1:] {
		if !labels.Equal(first, at(mode)) {
			return false
		}
	}
	return true
}

// Scalar applies Lua's non-final-expression rule. It returns a closed
// one-value pack per feasible length alignment rather than joining possible
// labels. A final expression remains a Pack and is framed separately.
func Scalar(labels Labels, value Value) Value {
	return scalarValue(labels, scalarLabels(labels, value))
}

// Assemble is the exact result-pack law for one Program Values relation. Each
// fixed input is adjusted to Lua's first result and final is forwarded whole:
//
//	{ scalar(x1) · ... · scalar(xn) · y | xi in fixed[i], y in final }
//
// There is intentionally no other concatenation operation in this carrier.
// In particular, Assemble cannot append one arbitrary open Pack to another:
// its only unbounded component is final, which is precisely the source
// language's final-expression rule.
//
// The finite Cartesian expansion is required by this carrier's exact
// alternative semantics. A caller supplies facts from one compatible guarded
// engine tuple, so guards retain cross-term control correlation; this function
// retains the pack correlation within each fact. No cardinality cap, label
// join, or hidden widening is lawful on this acyclic transfer.
func Assemble(labels Labels, fixed []Value, final Value) Value {
	return AssembleInputs(labels, valueInputs(fixed), final)
}

// AssembleInputs is Assemble's non-materializing operand route.  It is used
// by outer fact carriers whose immutable values contain a sequence.Value but
// are not themselves []Value.  It has exactly the same result law as
// Assemble; the interface is consumed synchronously and is never retained.
func AssembleInputs[I Inputs](labels Labels, fixed I, final Value) Value {
	if final.IsBottom() {
		return final
	}
	if fixed.Len() == 0 {
		return final
	}

	// Every prefix is an exact immutable closed word. Extending it copies only
	// an AVL join spine, so a long deterministic fixed list stays near-linear
	// rather than repeatedly copying its full historical prefix.
	prefixes := []closedWord{{}}
	for index := 0; index < fixed.Len(); {
		// A source list is overwhelmingly a run of exact scalar values. Batch
		// that run into one privately owned immutable leaf before framing the
		// current alternatives. This is O(run width), rather than O(width log
		// width) persistent joins, while preserving the same word and sharing
		// it across every current alternative.
		var run []Handle
		for index < fixed.Len() {
			label, singleton := scalarSingleton(labels, fixed.At(index))
			if !singleton {
				break
			}
			run = append(run, label)
			index++
		}
		if len(run) != 0 {
			word := closedWordFromSharedFlat(run)
			for prefixIndex, prefix := range prefixes {
				prefixes[prefixIndex] = concatClosedWords(prefix, word)
			}
			continue
		}

		input := fixed.At(index)
		alternatives := scalarLabels(labels, input)
		if len(alternatives) == 0 {
			return Bottom()
		}
		words := make([]closedWord, len(alternatives))
		for index, label := range alternatives {
			words[index] = closedWordFromLeaf(closedRepeatLeaf(label, 1))
		}
		if len(words) == 1 {
			for index, prefix := range prefixes {
				prefixes[index] = concatClosedWords(prefix, words[0])
			}
			continue
		}
		next := make([]closedWord, 0, len(prefixes))
		for _, prefix := range prefixes {
			for _, word := range words {
				next = append(next, concatClosedWords(prefix, word))
			}
		}
		prefixes = next
		index++
	}

	if final.IsTop() {
		modes := make([]Mode, 0, len(prefixes))
		for _, prefix := range prefixes {
			modes = append(modes, Mode{kind: ModeOpaque, prefix: prefix.Materialize()})
		}
		sortModes(modes)
		return Value{state: stateModes, modes: modes}
	}

	// final is normalized. scalarLabels returns an exact, TypeTop-exclusive
	// label set for every position. Thus different generated prefixes are
	// incomparable; applying the same prefix is injective and containment
	// preserving for every final mode. No duplicate or dominated mode can be
	// introduced, so sorting establishes the canonical representation without
	// re-running normalize's pairwise dominance scan over the product.
	modes := make([]Mode, 0, len(final.modes))
	for _, tail := range final.modes {
		for _, prefix := range prefixes {
			switch tail.kind {
			case ModeClosed:
				modes = append(modes, Mode{kind: ModeClosed, closed: concatClosedWords(prefix, tail.closed)})
			case ModeKnown:
				modes = append(modes, Mode{
					kind:   ModeKnown,
					prefix: appendClosedPrefix(prefix, tail.prefix),
					tail:   tail.tail,
					suffix: tail.suffix,
				})
			case ModeOpaque:
				modes = append(modes, Mode{
					kind:   ModeOpaque,
					prefix: appendClosedPrefix(prefix, tail.prefix),
					suffix: tail.suffix,
				})
			default:
				return Bottom()
			}
		}
	}
	sortModes(modes)
	return Value{state: stateModes, modes: modes}
}

// scalarSingleton recognizes the common exact one-label adjustment without
// allocating a transient slice or hash set. It is deliberately an optimization
// only: scalarLabels remains the complete semantic implementation for every
// non-singleton case.
func scalarSingleton(labels Labels, value Value) (Handle, bool) {
	if value.IsTop() {
		return labels.TypeTop(), true
	}
	if value.IsBottom() || len(value.modes) != 1 {
		return Handle{}, false
	}
	mode := value.modes[0]
	if mode.kind == ModeClosed {
		if mode.closed.length == 0 {
			return labels.Nil(), true
		}
		label, _ := mode.closed.At(0)
		return label, true
	}
	if len(mode.prefix) != 0 {
		return mode.prefix[0], true
	}
	if mode.kind == ModeOpaque || mode.kind == ModeKnown && labels.Equal(mode.tail, labels.TypeTop()) {
		return labels.TypeTop(), true
	}
	return Handle{}, false
}

// appendClosedPrefix is the only flattening Assemble needs. Closed words keep
// their persistent rope representation; open modes currently own a flat
// prefix, so one exact materialization per emitted open alternative is the
// representation-space lower bound. It never rescans an already-built prefix.
func appendClosedPrefix(prefix closedWord, suffix []Handle) []Handle {
	if prefix.length == 0 {
		return suffix
	}
	result := make([]Handle, prefix.length+len(suffix))
	iterator := newClosedIterator(prefix)
	for index := 0; index < prefix.length; index++ {
		result[index], _ = iterator.Next()
	}
	copy(result[prefix.length:], suffix)
	return result
}

// scalarLabels returns the exact finite marginal of Lua's first-value
// adjustment. It is intentionally shared with Assemble so Program Values
// construction does not allocate an intermediate public Mode view or
// normalize each fixed operand independently.
//
// A scalar marginal is a flat finite label set. TypeTop covers every other
// label in that order, so it is the only retained candidate whenever present;
// all other canonical Table handles are exact and can be deduplicated by
// coordinate identity. The output is ordinal-sorted solely to let Assemble
// construct a deterministic normal form without rescanning its own product.
func scalarLabels(labels Labels, value Value) []Handle {
	if value.IsBottom() {
		return nil
	}
	if value.IsTop() {
		return []Handle{labels.TypeTop()}
	}
	candidates := make([]Handle, 0, len(value.modes)*2)
	for _, mode := range value.modes {
		candidates = append(candidates, firstLabels(labels, mode)...)
	}
	if len(candidates) == 0 {
		return nil
	}
	top := labels.TypeTop()
	seen := make(map[Handle]struct{}, len(candidates))
	out := make([]Handle, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == top {
			return []Handle{top}
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	sort.Slice(out, func(left, right int) bool { return out[left].raw < out[right].raw })
	return out
}

func firstLabels(labels Labels, mode Mode) []Handle {
	if mode.kind == ModeClosed {
		if mode.closed.length == 0 {
			return []Handle{labels.Nil()}
		}
		value, _ := mode.closed.At(0)
		return []Handle{value}
	}
	if len(mode.prefix) != 0 {
		return []Handle{mode.prefix[0]}
	}
	switch mode.kind {
	case ModeClosed:
		return []Handle{labels.Nil()}
	case ModeKnown:
		out := []Handle{mode.tail}
		if len(mode.suffix) != 0 {
			out = append(out, mode.suffix[0])
		} else {
			out = append(out, labels.Nil())
		}
		return out
	case ModeOpaque:
		out := []Handle{labels.TypeTop()}
		if len(mode.suffix) != 0 {
			out = append(out, mode.suffix[0])
		} else {
			out = append(out, labels.Nil())
		}
		return out
	default:
		return nil
	}
}

// FixedAt computes one demanded scalar from a Lua assignment/parameter/result
// pack.  It intentionally does not construct a synthetic fixed-width Pack:
// the original Pack remains the only carrier of cross-position correlation.
//
// For P·h*·S at offset k after P, h is feasible (take more than k tail
// elements), every S[j] with j<=k is feasible (take k-j tail elements), and
// nil is feasible once k is beyond S (take no tail elements). Opaque tails
// use TypeTop for h. This is the exact scalar marginal, not a joined label.
func FixedAt(labels Labels, value Value, index int) Value {
	if index < 0 || value.IsBottom() {
		return Bottom()
	}
	if value.IsTop() {
		return scalarValue(labels, []Handle{labels.TypeTop()})
	}
	candidates := make([]Handle, 0, len(value.modes)*3)
	for _, mode := range value.modes {
		if mode.kind == ModeClosed {
			label, ok := mode.closed.At(index)
			if !ok {
				label = labels.Nil()
			}
			candidates = append(candidates, label)
			continue
		}
		if index < len(mode.prefix) {
			candidates = append(candidates, mode.prefix[index])
			continue
		}
		offset := index - len(mode.prefix)
		tail := labels.TypeTop()
		if mode.kind == ModeKnown {
			tail = mode.tail
		}
		candidates = append(candidates, tail)
		limit := offset + 1
		if limit > len(mode.suffix) {
			limit = len(mode.suffix)
		}
		for suffix := 0; suffix < limit; suffix++ {
			candidates = append(candidates, mode.suffix[suffix])
		}
		if offset >= len(mode.suffix) {
			candidates = append(candidates, labels.Nil())
		}
	}
	return scalarValue(labels, candidates)
}

// scalarValue normalizes only scalar marginals. Their sole nontrivial order
// fact is that a TypeTop label covers every exact label; otherwise distinct
// exact handles are incomparable. Sealed Table handles canonicalize semantic
// equality to Handle identity, so a direct Handle set is both collision-free
// and linear; this intentionally does not route through arbitrary Labels
// hashes. Source mode and suffix order are deterministic, so first occurrence
// gives a stable result without sorting or pairwise dominance.
func scalarValue(labels Labels, candidates []Handle) Value {
	if len(candidates) == 0 {
		return Bottom()
	}
	top := labels.TypeTop()
	out := make([]Mode, 0, len(candidates))
	seen := make(map[Handle]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate == top {
			return Value{state: stateModes, modes: []Mode{ClosedMode(candidate)}}
		}
		if _, duplicate := seen[candidate]; duplicate {
			continue
		}
		seen[candidate] = struct{}{}
		out = append(out, ClosedMode(candidate))
	}
	if len(out) == 0 {
		return Bottom()
	}
	return Value{state: stateModes, modes: out}
}
