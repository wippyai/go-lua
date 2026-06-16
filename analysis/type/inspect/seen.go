package inspect

import "github.com/wippyai/go-lua/analysis/type/typ"

// ScanSeen is the cycle policy used by Scanner.
type ScanSeen interface {
	Contains(typ.Type) bool
	Remember(typ.Type)
}

type equalitySeen struct {
	small    [8]equalitySeenEntry
	smallLen int
	entries  map[uint64][]typ.Type
	key      func(typ.Type) uint64
}

type equalitySeenEntry struct {
	key uint64
	t   typ.Type
}

// NewEqualitySeen creates a seen set keyed by typ.EqualityHash and confirmed
// by typ.TypeEquals.
func NewEqualitySeen() ScanSeen {
	return NewEqualitySeenWithKey(nil)
}

// NewEqualitySeenWithKey creates an equality seen set with a custom hash key.
func NewEqualitySeenWithKey(key func(typ.Type) uint64) ScanSeen {
	if key == nil {
		key = typ.EqualityHash
	}
	return &equalitySeen{
		key: key,
	}
}

func (s *equalitySeen) Contains(t typ.Type) bool {
	if t == nil || s == nil {
		return false
	}
	key := s.key(t)
	for i := 0; i < s.smallLen; i++ {
		entry := s.small[i]
		if entry.key == key && typ.TypeEquals(entry.t, t) {
			return true
		}
	}
	for _, existing := range s.entries[key] {
		if typ.TypeEquals(existing, t) {
			return true
		}
	}
	return false
}

func (s *equalitySeen) Remember(t typ.Type) {
	if t == nil || s == nil {
		return
	}
	key := s.key(t)
	if s.entries == nil && s.smallLen < len(s.small) {
		s.small[s.smallLen] = equalitySeenEntry{key: key, t: t}
		s.smallLen++
		return
	}
	if s.entries == nil {
		s.entries = make(map[uint64][]typ.Type, len(s.small)+1)
		for i := 0; i < s.smallLen; i++ {
			entry := s.small[i]
			s.entries[entry.key] = append(s.entries[entry.key], entry.t)
			s.small[i] = equalitySeenEntry{}
		}
		s.smallLen = 0
	}
	s.entries[key] = append(s.entries[key], t)
}

func (s *equalitySeen) clear() {
	if s == nil {
		return
	}
	for i := 0; i < s.smallLen; i++ {
		s.small[i] = equalitySeenEntry{}
	}
	s.smallLen = 0
	if s.entries != nil {
		clear(s.entries)
	}
}

type identitySeen struct {
	small    [8]typ.Type
	smallLen int
	entries  map[typ.Type]struct{}
	track    func(typ.Type) bool
}

// NewIdentitySeen creates a comparable-node identity seen set. When track is
// nil, all nodes are tracked.
func NewIdentitySeen(track func(typ.Type) bool) ScanSeen {
	return &identitySeen{
		track: track,
	}
}

func (s *identitySeen) Contains(t typ.Type) bool {
	if t == nil || s == nil || !s.tracks(t) {
		return false
	}
	for i := 0; i < s.smallLen; i++ {
		if s.small[i] == t {
			return true
		}
	}
	_, ok := s.entries[t]
	return ok
}

func (s *identitySeen) Remember(t typ.Type) {
	if t == nil || s == nil || !s.tracks(t) {
		return
	}
	if s.entries == nil && s.smallLen < len(s.small) {
		s.small[s.smallLen] = t
		s.smallLen++
		return
	}
	if s.entries == nil {
		s.entries = make(map[typ.Type]struct{}, len(s.small)+1)
		for i := 0; i < s.smallLen; i++ {
			s.entries[s.small[i]] = struct{}{}
			s.small[i] = nil
		}
		s.smallLen = 0
	}
	s.entries[t] = struct{}{}
}

func (s *identitySeen) tracks(t typ.Type) bool {
	return s.track == nil || s.track(t)
}

func (s *identitySeen) clear() {
	if s == nil {
		return
	}
	for i := 0; i < s.smallLen; i++ {
		s.small[i] = nil
	}
	s.smallLen = 0
	if s.entries != nil {
		clear(s.entries)
	}
}

type pointerSeen struct {
	entries map[uintptr]struct{}
	key     func(typ.Type) uintptr
}

// NewPointerSeen creates a seen set from a caller-provided node pointer key.
func NewPointerSeen(key func(typ.Type) uintptr) ScanSeen {
	return &pointerSeen{
		entries: make(map[uintptr]struct{}),
		key:     key,
	}
}

func (s *pointerSeen) Contains(t typ.Type) bool {
	key := s.keyFor(t)
	if key == 0 {
		return false
	}
	_, ok := s.entries[key]
	return ok
}

func (s *pointerSeen) Remember(t typ.Type) {
	key := s.keyFor(t)
	if key == 0 {
		return
	}
	s.entries[key] = struct{}{}
}

func (s *pointerSeen) keyFor(t typ.Type) uintptr {
	if t == nil || s == nil || s.key == nil {
		return 0
	}
	return s.key(t)
}
