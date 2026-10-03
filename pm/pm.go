// Package pm implements Lua's bounded pattern-matching VM.
//
// The package name is historical. It is the pattern-machine backend used by the
// string library, not a general-purpose regular expression engine.
package pm

import (
	"bytes"
	"container/list"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unsafe"
)

// pool is a typed sync.Pool.
type pool[T any] struct{ p sync.Pool }

func newPool[T any](create func() T) *pool[T] {
	return &pool[T]{p: sync.Pool{New: func() any { return create() }}}
}

func (p *pool[T]) Get() T  { return p.p.Get().(T) }
func (p *pool[T]) Put(v T) { p.p.Put(v) }

// Drop discards v without retaining it.
func (p *pool[T]) Drop(T) {}

const (
	eos         = -1
	unknownPos  = -2
	beforeStart = -3
)

const (
	// inlineCaptureCap is the threshold for inline vs heap capture storage.
	// 16 covers most patterns (up to 8 capture groups with start/end pairs).
	inlineCaptureCap = 16

	// maxPooledCapacity limits pooled slice sizes to prevent memory bloat.
	maxPooledCapacity = 128

	// initialStackCap is the initial capacity for the VM backtrack stack.
	initialStackCap = 8

	// maxPatternDepth limits recursion depth during parsing to prevent stack overflow.
	maxPatternDepth = 200

	// MaxPatternBytes rejects pathological user patterns before parse/compile.
	MaxPatternBytes = 4096

	// MaxProgramInstructions bounds compiled pattern VM code.
	MaxProgramInstructions = 8192

	// MaxCaptureSlots bounds capture storage for one match. Slots are start/end
	// pairs, so this allows up to 63 explicit captures plus the whole match.
	MaxCaptureSlots = 128

	// MaxBacktracks limits backtracking for one attempted start position.
	MaxBacktracks = 100000

	// MaxBacktrackStack bounds pending forks for greedy/lazy repetitions.
	MaxBacktrackStack = 16384

	// MaxVMSteps bounds VM dispatch iterations for one attempted start position.
	MaxVMSteps = 4000000

	// MaxVMByteScans bounds byte-by-byte work inside VM helpers that can scan
	// more than one byte per instruction, such as %b and backreferences. This
	// budget is shared across all attempted start positions in one search.
	MaxVMByteScans = 16 << 20

	// MaxSearchPositions bounds how many input offsets one Find call may try.
	MaxSearchPositions = 1000000

	// MaxMatches bounds unbounded Find calls. Callers that need more should
	// page work explicitly instead of materializing every match at once.
	MaxMatches = 100000
)

// Error represents a pattern matching error.
type Error struct {
	Pos     int
	Message string
}

func newError(pos int, message string, args ...any) *Error {
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	return &Error{Pos: pos, Message: message}
}

func (e *Error) Error() string {
	switch e.Pos {
	case eos:
		return fmt.Sprintf("%s at EOS", e.Message)
	case unknownPos:
		return e.Message
	default:
		return fmt.Sprintf("%s at %d", e.Message, e.Pos)
	}
}

func (e *Error) String() string {
	return e.Message
}

// MatchData holds captured positions from a match.
// Layout: bit 0 indicates position capture, bits 1+ hold the position value.
type MatchData struct {
	inline   [inlineCaptureCap]uint32
	captures []uint32
	released bool
}

var matchDataPool = newPool(func() *MatchData {
	md := &MatchData{}
	md.captures = md.inline[:0]
	return md
})

func newMatchData(size int) *MatchData {
	md := matchDataPool.Get()
	md.released = false
	md.reset(size)
	return md
}

func (md *MatchData) reset(size int) {
	if size <= inlineCaptureCap {
		md.captures = md.inline[:0]
		return
	}
	if cap(md.captures) < size {
		md.captures = make([]uint32, 0, size)
		return
	}
	md.captures = md.captures[:0]
}

func (md *MatchData) release() {
	if md.released {
		return
	}
	md.released = true
	if cap(md.captures) <= maxPooledCapacity {
		matchDataPool.Put(md)
		return
	}
	matchDataPool.Drop(md)
}

func (md *MatchData) addPosCapture(slot, pos int) {
	md.ensureSlot(slot + 1)
	md.captures[slot] = uint32(pos)<<1 | 1
	md.captures[slot+1] = uint32(pos)<<1 | 1
}

func (md *MatchData) setCapture(slot, pos int) {
	md.ensureSlot(slot)
	md.captures[slot] = uint32(pos) << 1
}

func (md *MatchData) ensureSlot(slot int) {
	for slot >= len(md.captures) {
		md.captures = append(md.captures, 0)
	}
}

func (md *MatchData) restoreFrom(t *vmThread) {
	if t.captureLen <= inlineCaptureCap {
		md.captures = md.inline[:t.captureLen]
	} else if cap(md.captures) < t.captureLen {
		md.captures = make([]uint32, t.captureLen)
	} else {
		md.captures = md.captures[:t.captureLen]
	}
	if t.useInline {
		copy(md.captures, t.inlineCaptures[:t.captureLen])
	} else {
		copy(md.captures, t.heapCaptures[:t.captureLen])
	}
}

// CaptureLength returns the number of capture slots.
func (md *MatchData) CaptureLength() int { return len(md.captures) }

// IsPosCapture returns true if the capture at idx is a position capture.
func (md *MatchData) IsPosCapture(idx int) bool {
	if idx < 0 || idx >= len(md.captures) {
		return false
	}
	return (md.captures[idx] & 1) == 1
}

// Capture returns the captured position at idx.
func (md *MatchData) Capture(idx int) int {
	if idx < 0 || idx >= len(md.captures) {
		return 0
	}
	return int(md.captures[idx] >> 1)
}

// scanner tokenizes pattern input.
type scanner struct {
	src      []byte
	pos      int
	savedPos int
}

func newScanner(src []byte) *scanner {
	return &scanner{src: src, pos: beforeStart, savedPos: beforeStart}
}

func (sc *scanner) next() int {
	switch sc.pos {
	case beforeStart:
		sc.pos = 0
	case eos:
		return eos
	default:
		sc.pos++
	}
	if sc.pos >= len(sc.src) {
		sc.pos = eos
		return eos
	}
	return int(sc.src[sc.pos])
}

func (sc *scanner) currentPos() int {
	return sc.pos
}

func (sc *scanner) peek() int {
	if sc.pos == eos {
		return eos
	}
	var next int
	if sc.pos == beforeStart {
		next = 0
	} else {
		next = sc.pos + 1
	}
	if next >= len(sc.src) {
		return eos
	}
	return int(sc.src[next])
}

func (sc *scanner) atEnd() bool {
	if sc.pos == eos {
		return true
	}
	if sc.pos == beforeStart {
		return len(sc.src) == 0
	}
	return sc.pos+1 >= len(sc.src)
}

func (sc *scanner) save() {
	sc.savedPos = sc.pos
}

func (sc *scanner) restore() {
	sc.pos = sc.savedPos
}

// opCode represents a VM instruction type.
type opCode int

const (
	opChar      opCode = iota // match character against class
	opMatch                   // successful match
	opTailMatch               // match only if at end of input
	opJmp                     // unconditional jump
	opSplit                   // fork execution (backtrack point)
	opSave                    // save position to capture slot
	opPSave                   // save position capture (1-indexed)
	opBrace                   // balanced brace matching
	opNumber                  // backreference to capture group
	opRepeat                  // single-item Lua repetition with compact fallbacks
	opFrontier                // zero-width frontier assertion
)

type instruction struct {
	op       opCode
	class    charClass
	operand1 int
	operand2 int
}

// charClass matches a character against a pattern class.
type charClass interface {
	matches(ch int) bool
}

// dotClass matches any character (singleton).
var theDotClass = &dotClass{}

type dotClass struct{}

func (dc *dotClass) matches(_ int) bool { return true }

type literalClass struct {
	char int
}

func (lc *literalClass) matches(ch int) bool { return lc.char == ch }

type singleClass struct {
	code int
}

func (sc *singleClass) matches(ch int) bool {
	return matchCharClass(sc.code, ch)
}

// matchCharClass matches Lua character classes.
func matchCharClass(code, ch int) bool {
	var matched bool
	switch code {
	case 'a', 'A':
		matched = ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
	case 'c', 'C':
		matched = (0x00 <= ch && ch <= 0x1F) || ch == 0x7F
	case 'd', 'D':
		matched = '0' <= ch && ch <= '9'
	case 'l', 'L':
		matched = 'a' <= ch && ch <= 'z'
	case 'p', 'P':
		matched = (0x21 <= ch && ch <= 0x2f) || (0x3a <= ch && ch <= 0x40) ||
			(0x5b <= ch && ch <= 0x60) || (0x7b <= ch && ch <= 0x7e)
	case 's', 'S':
		switch ch {
		case ' ', '\f', '\n', '\r', '\t', '\v':
			matched = true
		}
	case 'u', 'U':
		matched = 'A' <= ch && ch <= 'Z'
	case 'w', 'W':
		matched = ('0' <= ch && ch <= '9') || ('A' <= ch && ch <= 'Z') || ('a' <= ch && ch <= 'z')
	case 'x', 'X':
		matched = ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
	case 'z', 'Z':
		matched = ch == 0
	default:
		return ch == code
	}
	if 'A' <= code && code <= 'Z' {
		return !matched
	}
	return matched
}

type setClass struct {
	negated bool
	classes []charClass
}

func (sc *setClass) matches(ch int) bool {
	for _, cls := range sc.classes {
		if cls.matches(ch) {
			return !sc.negated
		}
	}
	return sc.negated
}

type rangeClass struct {
	begin int
	end   int
}

func (rc *rangeClass) matches(ch int) bool {
	return rc.begin <= ch && ch <= rc.end
}

// pattern is a parsed pattern node.
type pattern interface {
	patternNode()
}

type singlePattern struct {
	class charClass
}

func (*singlePattern) patternNode() {}

type seqPattern struct {
	mustHead bool
	mustTail bool
	patterns []pattern
}

func (*seqPattern) patternNode() {}

type repeatPattern struct {
	repeatType int
	class      charClass
}

func (*repeatPattern) patternNode() {}

type posCapPattern struct{}

func (*posCapPattern) patternNode() {}

type capPattern struct {
	inner pattern
}

func (*capPattern) patternNode() {}

type numberPattern struct {
	index int
}

func (*numberPattern) patternNode() {}

type bracePattern struct {
	begin int
	end   int
}

func (*bracePattern) patternNode() {}

type frontierPattern struct {
	class charClass
}

func (*frontierPattern) patternNode() {}

type captureValidationState struct {
	closed []bool
	next   int
}

func parseClass(sc *scanner, allowSet bool) (charClass, error) {
	ch := sc.next()
	switch ch {
	case '%':
		code := sc.next()
		if code == eos {
			return nil, newError(sc.currentPos(), "unexpected EOS")
		}
		return &singleClass{code}, nil
	case '.':
		if allowSet {
			return theDotClass, nil
		}
		return &literalClass{ch}, nil
	case '[':
		if allowSet {
			return parseClassSet(sc)
		}
		return &literalClass{ch}, nil
	case eos:
		return nil, newError(sc.currentPos(), "unexpected EOS")
	default:
		return &literalClass{ch}, nil
	}
}

func parseClassSet(sc *scanner) (charClass, error) {
	set := &setClass{}
	if sc.peek() == '^' {
		set.negated = true
		sc.next()
	}

	pendingRange := false
	for {
		ch := sc.peek()

		// End of set
		if ch == ']' && len(set.classes) > 0 {
			sc.next()
			if pendingRange {
				set.classes = append(set.classes, &literalClass{'-'})
			}
			return set, nil
		}

		// EOS without closing bracket
		if ch == eos {
			return nil, newError(sc.currentPos(), "unexpected EOS")
		}

		// Range operator
		if ch == '-' && len(set.classes) > 0 && !pendingRange {
			sc.next()
			pendingRange = true
			continue
		}

		cls, err := parseClass(sc, false)
		if err != nil {
			return nil, err
		}

		if pendingRange {
			prev := set.classes[len(set.classes)-1]
			set.classes = set.classes[:len(set.classes)-1]
			begin, end := extractRangeBounds(prev, cls)
			set.classes = append(set.classes, &rangeClass{begin, end})
			pendingRange = false
		} else {
			set.classes = append(set.classes, cls)
		}
	}
}

func extractRangeBounds(begin, end charClass) (int, int) {
	b, e := 0, 0
	if lit, ok := begin.(*literalClass); ok {
		b = lit.char
	}
	if lit, ok := end.(*literalClass); ok {
		e = lit.char
	}
	return b, e
}

func parsePattern(sc *scanner, topLevel bool) (*seqPattern, error) {
	return parsePatternDepth(sc, topLevel, 0)
}

func parsePatternDepth(sc *scanner, topLevel bool, depth int) (*seqPattern, error) {
	if depth > maxPatternDepth {
		return nil, newError(sc.currentPos(), "pattern too complex")
	}
	pat := &seqPattern{}
	if topLevel && sc.peek() == '^' {
		sc.next()
		pat.mustHead = true
	}
	for {
		ch := sc.peek()
		switch ch {
		case '%':
			if err := parseEscape(sc, pat); err != nil {
				return nil, err
			}
		case '.', '[', ']':
			cls, err := parseClass(sc, true)
			if err != nil {
				return nil, err
			}
			pat.patterns = append(pat.patterns, &singlePattern{cls})
		case ')':
			if topLevel {
				return nil, newError(sc.currentPos(), "invalid ')'")
			}
			return pat, nil
		case '(':
			if err := parseCaptureDepth(sc, pat, depth+1); err != nil {
				return nil, err
			}
		case '*', '+', '-', '?':
			parseRepeat(sc, pat, ch)
		case '$':
			sc.next()
			if topLevel && sc.atEnd() {
				pat.mustTail = true
			} else {
				pat.patterns = append(pat.patterns, &singlePattern{&literalClass{ch}})
			}
		case eos:
			return pat, nil
		default:
			sc.next()
			pat.patterns = append(pat.patterns, &singlePattern{&literalClass{ch}})
		}
	}
}

func parseEscape(sc *scanner, pat *seqPattern) error {
	sc.save()
	sc.next()
	switch sc.peek() {
	case '0':
		return newError(sc.currentPos(), "invalid capture index")
	case '1', '2', '3', '4', '5', '6', '7', '8', '9':
		pat.patterns = append(pat.patterns, &numberPattern{sc.next() - '0'})
	case 'b':
		sc.next()
		begin, end := sc.next(), sc.next()
		if begin == eos || end == eos {
			return newError(sc.currentPos(), "unfinished balanced pattern")
		}
		pat.patterns = append(pat.patterns, &bracePattern{begin, end})
	case 'f':
		sc.next()
		if sc.peek() != '[' {
			return newError(sc.currentPos(), "missing '[' after '%f'")
		}
		cls, err := parseClass(sc, true)
		if err != nil {
			return err
		}
		pat.patterns = append(pat.patterns, &frontierPattern{cls})
	default:
		sc.restore()
		cls, err := parseClass(sc, true)
		if err != nil {
			return err
		}
		pat.patterns = append(pat.patterns, &singlePattern{cls})
	}
	return nil
}

func parseCaptureDepth(sc *scanner, pat *seqPattern, depth int) error {
	sc.next()
	if sc.peek() == ')' {
		sc.next()
		pat.patterns = append(pat.patterns, &posCapPattern{})
		return nil
	}
	inner, err := parsePatternDepth(sc, false, depth)
	if err != nil {
		return err
	}
	if sc.peek() != ')' {
		return newError(sc.currentPos(), "unfinished capture")
	}
	sc.next()
	pat.patterns = append(pat.patterns, &capPattern{inner})
	return nil
}

func parseRepeat(sc *scanner, pat *seqPattern, ch int) {
	sc.next()
	if len(pat.patterns) > 0 {
		if single, ok := pat.patterns[len(pat.patterns)-1].(*singlePattern); ok {
			pat.patterns[len(pat.patterns)-1] = &repeatPattern{ch, single.class}
			return
		}
	}
	pat.patterns = append(pat.patterns, &singlePattern{&literalClass{ch}})
}

type instructionBuilder struct {
	instructions []instruction
	captureSlot  int
}

func compilePattern(p pattern, builder *instructionBuilder) []instruction {
	topLevel := builder == nil
	if topLevel {
		builder = &instructionBuilder{
			instructions: []instruction{{opSave, nil, 0, -1}},
			captureSlot:  2,
		}
	}

	switch pat := p.(type) {
	case *singlePattern:
		builder.instructions = append(builder.instructions, instruction{opChar, pat.class, -1, -1})

	case *seqPattern:
		for _, child := range pat.patterns {
			compilePattern(child, builder)
		}
		if topLevel {
			if pat.mustTail {
				builder.instructions = append(builder.instructions,
					instruction{opSave, nil, 1, -1},
					instruction{opTailMatch, nil, -1, -1})
			} else {
				builder.instructions = append(builder.instructions,
					instruction{opSave, nil, 1, -1},
					instruction{opMatch, nil, -1, -1})
			}
		}

	case *repeatPattern:
		compileRepeat(builder, pat)

	case *posCapPattern:
		builder.instructions = append(builder.instructions, instruction{opPSave, nil, builder.captureSlot, -1})
		builder.captureSlot += 2

	case *capPattern:
		startSlot := builder.captureSlot
		endSlot := builder.captureSlot + 1
		builder.captureSlot += 2
		builder.instructions = append(builder.instructions, instruction{opSave, nil, startSlot, -1})
		compilePattern(pat.inner, builder)
		builder.instructions = append(builder.instructions, instruction{opSave, nil, endSlot, -1})

	case *bracePattern:
		builder.instructions = append(builder.instructions, instruction{opBrace, nil, pat.begin, pat.end})

	case *numberPattern:
		builder.instructions = append(builder.instructions, instruction{opNumber, nil, pat.index, -1})

	case *frontierPattern:
		builder.instructions = append(builder.instructions, instruction{opFrontier, pat.class, -1, -1})
	}

	return builder.instructions
}

func compileRepeat(builder *instructionBuilder, pat *repeatPattern) {
	builder.instructions = append(builder.instructions, instruction{opRepeat, pat.class, pat.repeatType, -1})
}

// LRU pattern cache with bounded size.
type patternCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type cacheEntry struct {
	key     string
	insts   []instruction
	pat     *seqPattern
	capSize int
}

func newPatternCache(capacity int) *patternCache {
	return &patternCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (pc *patternCache) get(pattern string) ([]instruction, *seqPattern, int, bool) {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	elem, ok := pc.items[pattern]
	if !ok {
		return nil, nil, 0, false
	}
	pc.order.MoveToFront(elem)
	entry := elem.Value.(*cacheEntry)
	return entry.insts, entry.pat, entry.capSize, true
}

func (pc *patternCache) put(pattern string, insts []instruction, pat *seqPattern, capSize int) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if elem, ok := pc.items[pattern]; ok {
		pc.order.MoveToFront(elem)
		return
	}

	if pc.order.Len() >= pc.capacity {
		oldest := pc.order.Back()
		if oldest != nil {
			entry := oldest.Value.(*cacheEntry)
			delete(pc.items, entry.key)
			pc.order.Remove(oldest)
		}
	}

	pattern = strings.Clone(pattern)
	entry := &cacheEntry{key: pattern, insts: insts, pat: pat, capSize: capSize}
	elem := pc.order.PushFront(entry)
	pc.items[pattern] = elem
}

const maxCachedPatterns = 256

var globalPatternCache = newPatternCache(maxCachedPatterns)

// Program is a compiled Lua pattern program. It separates parse/cache lookup
// from execution so callers that scan repeatedly, such as gsub, do not re-enter
// the global pattern cache on every match.
type Program struct {
	insts   []instruction
	pat     *seqPattern
	capSize int
	done    <-chan struct{}
}

// ErrCanceled is returned by a search whose done channel was closed.
var ErrCanceled = errors.New("pattern match canceled")

// cancelCheckMask spaces cancellation checks: one per cancelCheckMask+1 VM
// steps or search positions.
const cancelCheckMask = 4095

// WithDone returns a Program whose searches stop with ErrCanceled once done is
// closed. A nil channel never cancels.
func (p Program) WithDone(done <-chan struct{}) Program {
	p.done = done
	return p
}

func canceled(done <-chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// Compile parses or retrieves a cached pattern program.
func Compile(pattern string) (Program, error) {
	if len(pattern) > MaxPatternBytes {
		return Program{}, newError(unknownPos, "pattern too large")
	}
	if insts, pat, capSize, ok := globalPatternCache.get(pattern); ok {
		return Program{insts: insts, pat: pat, capSize: capSize}, nil
	}

	pat, err := parsePattern(newScanner([]byte(pattern)), true)
	if err != nil {
		return Program{}, err
	}
	if err := validateCaptures(pat); err != nil {
		return Program{}, err
	}
	insts := compilePattern(pat, nil)
	if len(insts) > MaxProgramInstructions {
		return Program{}, newError(unknownPos, "compiled pattern too large")
	}
	capSize := calcMaxCaptureSlot(insts)
	if capSize > MaxCaptureSlots {
		return Program{}, newError(unknownPos, "too many captures")
	}
	globalPatternCache.put(pattern, insts, pat, capSize)
	return Program{insts: insts, pat: pat, capSize: capSize}, nil
}

func validateCaptures(pat pattern) error {
	st := &captureValidationState{
		closed: []bool{false},
		next:   1,
	}
	return validateCapturePattern(pat, st)
}

func validateCapturePattern(pat pattern, st *captureValidationState) error {
	switch p := pat.(type) {
	case *seqPattern:
		for _, child := range p.patterns {
			if err := validateCapturePattern(child, st); err != nil {
				return err
			}
		}
	case *posCapPattern:
		idx := st.next
		st.next++
		st.closed = append(st.closed, true)
		if idx >= MaxCaptureSlots/2 {
			return newError(unknownPos, "too many captures")
		}
	case *capPattern:
		idx := st.next
		st.next++
		st.closed = append(st.closed, false)
		if idx >= MaxCaptureSlots/2 {
			return newError(unknownPos, "too many captures")
		}
		if err := validateCapturePattern(p.inner, st); err != nil {
			return err
		}
		st.closed[idx] = true
	case *numberPattern:
		if p.index <= 0 || p.index >= st.next || p.index >= len(st.closed) || !st.closed[p.index] {
			return newError(unknownPos, "invalid capture index")
		}
	}
	return nil
}

func calcMaxCaptureSlot(insts []instruction) int {
	maxSlot := 0
	for _, inst := range insts {
		if inst.op == opSave || inst.op == opPSave {
			if inst.operand1 > maxSlot {
				maxSlot = inst.operand1
			}
		}
	}
	return maxSlot + 2
}

// vmThread represents a backtrack point in the VM.
type vmThread struct {
	programCounter int
	sourcePos      int
	rangeActive    bool
	rangeNext      int
	rangeLimit     int
	rangeStep      int
	inlineCaptures [inlineCaptureCap]uint32
	heapCaptures   []uint32
	captureLen     int
	useInline      bool
}

var threadPool = newPool(func() *vmThread { return &vmThread{} })

func newVMThread(programCounter, sourcePos int, captures []uint32) *vmThread {
	t := threadPool.Get()
	t.programCounter = programCounter
	t.sourcePos = sourcePos
	t.rangeActive = false
	t.rangeNext = 0
	t.rangeLimit = 0
	t.rangeStep = 0
	t.setCaptures(captures)
	return t
}

func newVMRangeThread(programCounter, next, limit, step int, captures []uint32) *vmThread {
	t := threadPool.Get()
	t.programCounter = programCounter
	t.sourcePos = next
	t.rangeActive = true
	t.rangeNext = next
	t.rangeLimit = limit
	t.rangeStep = step
	t.setCaptures(captures)
	return t
}

func (t *vmThread) setCaptures(captures []uint32) {
	t.captureLen = len(captures)
	if len(captures) <= inlineCaptureCap {
		t.useInline = true
		copy(t.inlineCaptures[:], captures)
	} else {
		t.useInline = false
		if cap(t.heapCaptures) < len(captures) {
			t.heapCaptures = make([]uint32, len(captures))
		} else {
			t.heapCaptures = t.heapCaptures[:len(captures)]
		}
		copy(t.heapCaptures, captures)
	}
}

func (t *vmThread) rangeHasNext() bool {
	if !t.rangeActive {
		return false
	}
	if t.rangeStep < 0 {
		return t.rangeNext >= t.rangeLimit
	}
	return t.rangeNext <= t.rangeLimit
}

func (t *vmThread) release() {
	if !t.useInline && cap(t.heapCaptures) > maxPooledCapacity {
		t.heapCaptures = nil
	}
	threadPool.Put(t)
}

// matchClass dispatches to the appropriate matcher with inlined hot paths.
func matchClass(cls charClass, ch int) bool {
	switch c := cls.(type) {
	case *literalClass:
		return c.char == ch
	case *singleClass:
		return matchCharClass(c.code, ch)
	case *dotClass:
		return true
	case *setClass:
		return c.matches(ch)
	case *rangeClass:
		return c.matches(ch)
	default:
		return cls.matches(ch)
	}
}

type vm struct {
	src       []byte
	insts     []instruction
	matchData *MatchData
	stack     []*vmThread
	byteScans int
	done      <-chan struct{}
}

var vmPool = newPool(func() *vm {
	return &vm{stack: make([]*vmThread, 0, initialStackCap)}
})

func newVM(src []byte, insts []instruction, done <-chan struct{}) *vm {
	v := vmPool.Get()
	v.done = done
	v.src = src
	v.insts = insts
	v.matchData = nil
	v.stack = v.stack[:0]
	v.byteScans = 0
	return v
}

func (v *vm) release() {
	v.done = nil
	v.src = nil
	v.insts = nil
	v.matchData = nil
	if cap(v.stack) <= maxPooledCapacity {
		vmPool.Put(v)
		return
	}
	vmPool.Drop(v)
}

func (v *vm) releaseStack() {
	for i := len(v.stack) - 1; i >= 0; i-- {
		v.stack[i].release()
	}
	v.stack = v.stack[:0]
}

func (v *vm) run(programCounter, sourcePos int) (bool, int, error) {
	backtracks := 0
	steps := 0

	for {
		steps++
		if steps > MaxVMSteps {
			v.releaseStack()
			return false, sourcePos, newError(unknownPos, "pattern match step limit exceeded")
		}
		if steps&cancelCheckMask == 0 && canceled(v.done) {
			v.releaseStack()
			return false, sourcePos, ErrCanceled
		}
		inst := v.insts[programCounter]
		switch inst.op {
		case opChar:
			if sourcePos >= len(v.src) || !matchClass(inst.class, int(v.src[sourcePos])) {
				ok, err := v.backtrack(&programCounter, &sourcePos, &backtracks)
				if err != nil {
					return false, sourcePos, err
				}
				if !ok {
					return false, sourcePos, nil
				}
				continue
			}
			programCounter++
			sourcePos++

		case opMatch:
			v.releaseStack()
			return true, sourcePos, nil

		case opTailMatch:
			if sourcePos >= len(v.src) {
				v.releaseStack()
				return true, sourcePos, nil
			}
			ok, err := v.backtrack(&programCounter, &sourcePos, &backtracks)
			if err != nil {
				return false, sourcePos, err
			}
			if !ok {
				return false, sourcePos, nil
			}

		case opJmp:
			programCounter = inst.operand1

		case opSplit:
			if len(v.stack) >= MaxBacktrackStack {
				v.releaseStack()
				return false, sourcePos, newError(unknownPos, "pattern match stack limit exceeded")
			}
			t := newVMThread(inst.operand2, sourcePos, v.matchData.captures)
			v.stack = append(v.stack, t)
			programCounter = inst.operand1

		case opSave:
			v.matchData.setCapture(inst.operand1, sourcePos)
			programCounter++

		case opPSave:
			v.matchData.addPosCapture(inst.operand1, sourcePos+1)
			programCounter++

		case opBrace:
			ok, newPos, err := v.matchBrace(inst.operand1, inst.operand2, sourcePos)
			if err != nil {
				return false, sourcePos, err
			}
			if !ok {
				ok, err := v.backtrack(&programCounter, &sourcePos, &backtracks)
				if err != nil {
					return false, sourcePos, err
				}
				if !ok {
					return false, sourcePos, nil
				}
				continue
			}
			sourcePos = newPos
			programCounter++

		case opNumber:
			ok, newPos, err := v.matchBackref(inst.operand1, sourcePos)
			if err != nil {
				return false, sourcePos, err
			}
			if !ok {
				ok, err := v.backtrack(&programCounter, &sourcePos, &backtracks)
				if err != nil {
					return false, sourcePos, err
				}
				if !ok {
					return false, sourcePos, nil
				}
				continue
			}
			programCounter++
			sourcePos = newPos

		case opRepeat:
			minPos, maxPos, ok, err := v.repeatBounds(inst.class, inst.operand1, sourcePos)
			if err != nil {
				return false, sourcePos, err
			}
			if !ok {
				ok, err := v.backtrack(&programCounter, &sourcePos, &backtracks)
				if err != nil {
					return false, sourcePos, err
				}
				if !ok {
					return false, sourcePos, nil
				}
				continue
			}
			nextPC := programCounter + 1
			switch inst.operand1 {
			case '*', '+', '?':
				sourcePos = maxPos
				if maxPos > minPos {
					if err := v.pushRepeatFallback(nextPC, maxPos-1, minPos, -1); err != nil {
						return false, sourcePos, err
					}
				}
			case '-':
				sourcePos = minPos
				if maxPos > minPos {
					if err := v.pushRepeatFallback(nextPC, minPos+1, maxPos, 1); err != nil {
						return false, sourcePos, err
					}
				}
			default:
				v.releaseStack()
				return false, sourcePos, nil
			}
			programCounter = nextPC

		case opFrontier:
			if !v.matchFrontier(inst.class, sourcePos) {
				ok, err := v.backtrack(&programCounter, &sourcePos, &backtracks)
				if err != nil {
					return false, sourcePos, err
				}
				if !ok {
					return false, sourcePos, nil
				}
				continue
			}
			programCounter++

		default:
			v.releaseStack()
			return false, sourcePos, nil
		}
	}
}

func (v *vm) backtrack(pc, sp, backtracks *int) (bool, error) {
	for len(v.stack) != 0 {
		t := v.stack[len(v.stack)-1]
		if t.rangeActive {
			if !t.rangeHasNext() {
				v.stack = v.stack[:len(v.stack)-1]
				t.release()
				continue
			}
			*pc = t.programCounter
			*sp = t.rangeNext
			v.matchData.restoreFrom(t)
			t.rangeNext += t.rangeStep
			if !t.rangeHasNext() {
				v.stack = v.stack[:len(v.stack)-1]
				t.release()
			}
			return true, nil
		}

		v.stack = v.stack[:len(v.stack)-1]
		(*backtracks)++
		if *backtracks > MaxBacktracks {
			t.release()
			v.releaseStack()
			return false, newError(unknownPos, "pattern match backtrack limit exceeded")
		}
		*pc = t.programCounter
		*sp = t.sourcePos
		v.matchData.restoreFrom(t)
		t.release()
		return true, nil
	}
	v.releaseStack()
	return false, nil
}

func (v *vm) pushRepeatFallback(programCounter, next, limit, step int) error {
	if len(v.stack) >= MaxBacktrackStack {
		v.releaseStack()
		return newError(unknownPos, "pattern match stack limit exceeded")
	}
	t := newVMRangeThread(programCounter, next, limit, step, v.matchData.captures)
	if !t.rangeHasNext() {
		t.release()
		return nil
	}
	v.stack = append(v.stack, t)
	return nil
}

func (v *vm) repeatBounds(cls charClass, repeatType int, sourcePos int) (int, int, bool, error) {
	switch repeatType {
	case '?':
		if sourcePos < len(v.src) && matchClass(cls, int(v.src[sourcePos])) {
			if err := v.chargeByteScans(1); err != nil {
				return 0, 0, false, err
			}
			return sourcePos, sourcePos + 1, true, nil
		}
		return sourcePos, sourcePos, true, nil
	case '*', '-':
		maxPos, err := v.scanRepeat(cls, sourcePos)
		if err != nil {
			return 0, 0, false, err
		}
		return sourcePos, maxPos, true, nil
	case '+':
		if sourcePos >= len(v.src) || !matchClass(cls, int(v.src[sourcePos])) {
			return 0, 0, false, nil
		}
		maxPos, err := v.scanRepeat(cls, sourcePos+1)
		if err != nil {
			return 0, 0, false, err
		}
		if err := v.chargeByteScans(1); err != nil {
			return 0, 0, false, err
		}
		return sourcePos + 1, maxPos, true, nil
	default:
		return 0, 0, false, nil
	}
}

func (v *vm) scanRepeat(cls charClass, sourcePos int) (int, error) {
	pos := sourcePos
	for pos < len(v.src) && matchClass(cls, int(v.src[pos])) {
		pos++
	}
	if err := v.chargeByteScans(pos - sourcePos); err != nil {
		return sourcePos, err
	}
	return pos, nil
}

func (v *vm) matchFrontier(cls charClass, sourcePos int) bool {
	previous := 0
	if sourcePos > 0 {
		previous = int(v.src[sourcePos-1])
	}
	current := 0
	if sourcePos < len(v.src) {
		current = int(v.src[sourcePos])
	}
	return !matchClass(cls, previous) && matchClass(cls, current)
}

func (v *vm) chargeByteScans(n int) error {
	if n <= 0 {
		return nil
	}
	if n > MaxVMByteScans-v.byteScans {
		v.releaseStack()
		return newError(unknownPos, "pattern match byte scan limit exceeded")
	}
	v.byteScans += n
	return nil
}

func (v *vm) matchBrace(open, close, sourcePos int) (bool, int, error) {
	if sourcePos >= len(v.src) || int(v.src[sourcePos]) != open {
		return false, sourcePos, nil
	}
	count := 1
	sourcePos++
	for ; sourcePos < len(v.src); sourcePos++ {
		if err := v.chargeByteScans(1); err != nil {
			return false, sourcePos, err
		}
		ch := int(v.src[sourcePos])
		switch ch {
		case close:
			count--
			if count == 0 {
				return true, sourcePos + 1, nil
			}
		case open:
			count++
		}
	}
	return false, sourcePos, nil
}

func (v *vm) matchBackref(index, sourcePos int) (bool, int, error) {
	slot := index * 2
	capLen := v.matchData.CaptureLength()
	if slot+1 >= capLen {
		return false, sourcePos, nil
	}
	start := v.matchData.Capture(slot)
	end := v.matchData.Capture(slot + 1)
	if start > end || end > len(v.src) {
		return false, sourcePos, nil
	}
	capture := v.src[start:end]
	if sourcePos+len(capture) > len(v.src) {
		return false, sourcePos, nil
	}
	if err := v.chargeByteScans(len(capture)); err != nil {
		return false, sourcePos, err
	}
	if !bytes.Equal(capture, v.src[sourcePos:sourcePos+len(capture)]) {
		return false, sourcePos, nil
	}
	return true, sourcePos + len(capture), nil
}

// Find searches for pattern matches in src starting at offset.
// Returns up to limit matches (-1 for unlimited).
func Find(pattern string, src []byte, offset, limit int) ([]*MatchData, error) {
	program, err := Compile(pattern)
	if err != nil {
		return nil, err
	}
	return program.Find(src, offset, limit)
}

// Find searches for pattern matches in src using a compiled Program.
func (p Program) Find(src []byte, offset, limit int) ([]*MatchData, error) {
	if !p.Valid() {
		return nil, newError(unknownPos, "uncompiled pattern")
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) || limit == 0 {
		return nil, nil
	}
	if p.pat.mustHead && offset != 0 {
		return nil, nil
	}

	var matches []*MatchData
	maxMatches := MaxMatches
	if limit > 0 && limit < maxMatches {
		maxMatches = limit
	}
	if limit > 0 {
		matches = make([]*MatchData, 0, boundedMatchCap(limit))
	}

	v := newVM(src, p.insts, p.done)
	defer v.release()

	scratch := newMatchData(p.capSize)
	scratchOwned := true
	positions := 0
	for sourcePos := offset; sourcePos <= len(src); {
		positions++
		if positions&cancelCheckMask == 1 && canceled(p.done) {
			if scratchOwned {
				scratch.release()
			}
			releaseMatches(matches)
			return nil, ErrCanceled
		}
		if positions > MaxSearchPositions {
			if scratchOwned {
				scratch.release()
			}
			releaseMatches(matches)
			return nil, newError(unknownPos, "pattern search position limit exceeded")
		}
		md := scratch
		md.reset(p.capSize)
		v.matchData = md
		ok, newPos, err := v.run(0, sourcePos)
		sourcePos++
		if err != nil {
			if scratchOwned {
				scratch.release()
			}
			releaseMatches(matches)
			return nil, err
		}
		if ok {
			if sourcePos < newPos {
				sourcePos = newPos
			}
			matches = append(matches, md)
			scratchOwned = false
			if len(matches) >= maxMatches {
				if limit < 0 {
					releaseMatches(matches)
					return nil, newError(unknownPos, "pattern match count limit exceeded")
				}
				break
			}
			scratch = newMatchData(p.capSize)
			scratchOwned = true
		} else {
			scratchOwned = true
		}
		if p.pat.mustHead {
			break
		}
	}
	if scratchOwned {
		scratch.release()
	}
	return matches, nil
}

func boundedMatchCap(limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit > MaxMatches {
		return MaxMatches
	}
	return limit
}

func releaseMatches(matches []*MatchData) {
	for _, md := range matches {
		if md != nil {
			md.release()
		}
	}
}

// ReleaseMatch returns one match storage object to the internal pool after the
// caller has finished reading captures.
func ReleaseMatch(md *MatchData) {
	if md != nil {
		md.release()
	}
}

// ReleaseMatches returns match storage obtained from Find/FindString to the
// internal pools after the caller has finished reading captures.
func ReleaseMatches(matches []*MatchData) {
	releaseMatches(matches)
}

// FindOne returns the first match at or after offset without materializing the
// rest of the search result set. The returned MatchData belongs to the caller
// until ReleaseMatch is called.
func FindOne(pattern string, src []byte, offset int) (*MatchData, error) {
	program, err := Compile(pattern)
	if err != nil {
		return nil, err
	}
	return program.FindOne(src, offset)
}

// FindOne returns the first match at or after offset using a compiled Program.
func (p Program) FindOne(src []byte, offset int) (*MatchData, error) {
	if !p.Valid() {
		return nil, newError(unknownPos, "uncompiled pattern")
	}
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		return nil, nil
	}
	if p.pat.mustHead && offset != 0 {
		return nil, nil
	}

	v := newVM(src, p.insts, p.done)
	defer v.release()

	scratch := newMatchData(p.capSize)
	scratchOwned := true
	positions := 0
	for sourcePos := offset; sourcePos <= len(src); sourcePos++ {
		positions++
		if positions&cancelCheckMask == 1 && canceled(p.done) {
			if scratchOwned {
				scratch.release()
			}
			return nil, ErrCanceled
		}
		if positions > MaxSearchPositions {
			if scratchOwned {
				scratch.release()
			}
			return nil, newError(unknownPos, "pattern search position limit exceeded")
		}
		md := scratch
		md.reset(p.capSize)
		v.matchData = md
		ok, _, err := v.run(0, sourcePos)
		if err != nil {
			if scratchOwned {
				scratch.release()
			}
			return nil, err
		}
		if ok {
			scratchOwned = false
			return md, nil
		}
		scratchOwned = true
		if p.pat.mustHead {
			break
		}
	}
	if scratchOwned {
		scratch.release()
	}
	return nil, nil
}

// FindString is Find over an immutable string source without forcing a copy to
// a new []byte first. The matcher treats src as read-only.
func FindString(pattern, src string, offset, limit int) ([]*MatchData, error) {
	program, err := Compile(pattern)
	if err != nil {
		return nil, err
	}
	return program.FindString(src, offset, limit)
}

// FindString searches an immutable string source using a compiled Program.
func (p Program) FindString(src string, offset, limit int) ([]*MatchData, error) {
	if len(src) == 0 {
		return p.Find(nil, offset, limit)
	}
	return p.Find(unsafe.Slice(unsafe.StringData(src), len(src)), offset, limit)
}

// FindStringOne is FindOne over an immutable string source without forcing a
// copy to a new []byte first. The returned MatchData belongs to the caller until
// ReleaseMatch is called.
func FindStringOne(pattern, src string, offset int) (*MatchData, error) {
	program, err := Compile(pattern)
	if err != nil {
		return nil, err
	}
	return program.FindStringOne(src, offset)
}

// FindStringOne returns the first string-source match using a compiled Program.
func (p Program) FindStringOne(src string, offset int) (*MatchData, error) {
	if len(src) == 0 {
		return p.FindOne(nil, offset)
	}
	return p.FindOne(unsafe.Slice(unsafe.StringData(src), len(src)), offset)
}

// Valid reports whether this Program came from Compile.
func (p Program) Valid() bool {
	return p.insts != nil && p.pat != nil && p.capSize > 0
}
