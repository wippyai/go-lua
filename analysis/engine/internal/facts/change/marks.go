package change

// Marks is the one dirty-set representation in the engine: a dense
// evidence-per-ordinal accumulator over a sealed plane, epoch-stamped so Reset
// is O(1), with exact enumeration in mark order.
//
// It carries two independent stamp layers over one flat plane because two
// consumers ask different questions of the same marks. The visit layer answers
// "what changed during this head visit" and is cleared by Reset. The since
// layer answers "what changed since this plane last remembered its interfaces"
// -- a longer epoch -- and is cleared only by Remember or Invalidate. A mark
// writes both; neither clear is more than a stamp increment.
type Marks struct {
	visitStamp []uint32
	visitSet   []Set
	sinceStamp []uint32
	sinceSet   []Set
	live       []Ord
	visitMark  uint32
	sinceMark  uint32
	width      int
}

// firstMark is the lowest live stamp. Zero is reserved for a never-written
// slot, so a grown plane reads as unmarked without a clear.
const firstMark uint32 = 1

// Reset opens one visit epoch over a plane of width ordinals. Widening keeps
// the accumulated since layer; the appended ordinals read as unmarked in both
// layers because their stamps are zero.
func (m *Marks) Reset(width int) {
	if m == nil || width < 0 {
		return
	}
	if width > len(m.visitStamp) {
		m.visitStamp = append(m.visitStamp, make([]uint32, width-len(m.visitStamp))...)
		m.visitSet = append(m.visitSet, make([]Set, width-len(m.visitSet))...)
		m.sinceStamp = append(m.sinceStamp, make([]uint32, width-len(m.sinceStamp))...)
		m.sinceSet = append(m.sinceSet, make([]Set, width-len(m.sinceSet))...)
	}
	m.width = width
	m.live = m.live[:0]
	m.visitMark = advance(m.visitMark, m.visitStamp)
	if m.sinceMark < firstMark {
		m.sinceMark = advance(m.sinceMark, m.sinceStamp)
	}
}

// Remember closes the since epoch: every ordinal reads as unmarked in the
// since layer until it is marked again.
func (m *Marks) Remember() {
	if m == nil {
		return
	}
	m.sinceMark = advance(m.sinceMark, m.sinceStamp)
}

// Invalidate is Remember under the name its caller uses: dropping the retained
// history of a plane is the same cut as recording it.
func (m *Marks) Invalidate() { m.Remember() }

// advance bumps a stamp epoch. A real clear happens only on wrap.
func advance(mark uint32, stamp []uint32) uint32 {
	if mark == ^uint32(0) {
		clear(stamp)
		return firstMark
	}
	mark++
	if mark < firstMark {
		return firstMark
	}
	return mark
}

// Mark accumulates evidence for one ordinal in both layers and records the
// ordinal's first-mark position in the visit layer's enumeration. It refuses
// an ordinal outside the plane rather than growing it.
func (m *Marks) Mark(o Ord, s Set) bool {
	if m == nil || int(o) >= m.width || o == NoOrd {
		return false
	}
	if m.visitStamp[o] != m.visitMark {
		m.visitStamp[o] = m.visitMark
		m.visitSet[o] = s
		m.live = append(m.live, o)
	} else {
		m.visitSet[o] = m.visitSet[o].Union(s)
	}
	if m.sinceStamp[o] != m.sinceMark {
		m.sinceStamp[o] = m.sinceMark
		m.sinceSet[o] = s
	} else {
		m.sinceSet[o] = m.sinceSet[o].Union(s)
	}
	return true
}

// At reads the visit layer. An unmarked ordinal reads as the zero Set, which
// Admits refuses.
func (m *Marks) At(o Ord) Set {
	if m == nil || int(o) >= m.width || o == NoOrd || m.visitStamp[o] != m.visitMark {
		return Set{}
	}
	return m.visitSet[o]
}

// Marked reports whether this ordinal already carries evidence in the current
// visit epoch. It separates a first mark from an accumulating one for owners
// that keep a payload plane alongside the evidence.
func (m *Marks) Marked(o Ord) bool {
	return m != nil && o != NoOrd && int(o) < m.width && m.visitStamp[o] == m.visitMark
}

// Since reads the since layer.
func (m *Marks) Since(o Ord) Set {
	if m == nil || int(o) >= m.width || o == NoOrd || m.sinceStamp[o] != m.sinceMark {
		return Set{}
	}
	return m.sinceSet[o]
}

// Dirty enumerates the ordinals marked in this visit epoch, in first-mark
// order. The slice is borrowed and is valid until the next Mark or Reset.
func (m *Marks) Dirty() []Ord {
	if m == nil {
		return nil
	}
	return m.live
}

// Empty reports that this visit epoch has no marks.
func (m *Marks) Empty() bool { return m == nil || len(m.live) == 0 }

// Width reports the plane this accumulator is currently open over.
func (m *Marks) Width() int {
	if m == nil {
		return 0
	}
	return m.width
}
