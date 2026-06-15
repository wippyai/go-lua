package inspect

import "github.com/wippyai/go-lua/analysis/type/typ"

// ScanSeen is the cycle policy used by Scanner.
type ScanSeen interface {
	Contains(typ.Type) bool
	Remember(typ.Type)
}

// ScanOptions configures a Scanner.
type ScanOptions struct {
	Seen ScanSeen

	// MaxSteps limits explicit Step calls. Use this for bounded scans where
	// nil leaves and already-seen nodes should still count against the budget.
	MaxSteps int

	// MaxEnters limits distinct nodes accepted by Enter after seen pruning.
	MaxEnters int
}

// Scanner holds reusable traversal state for recursive type scans.
type Scanner struct {
	seen      ScanSeen
	maxSteps  int
	maxEnters int
	steps     int
	enters    int
	exceeded  bool
}

// NewScanner creates a traversal scanner.
func NewScanner(opts ScanOptions) *Scanner {
	return &Scanner{
		seen:      opts.Seen,
		maxSteps:  opts.MaxSteps,
		maxEnters: opts.MaxEnters,
	}
}

// Step records a scan step and reports whether traversal may continue.
func (s *Scanner) Step() bool {
	if s == nil || s.maxSteps <= 0 {
		return true
	}
	s.steps++
	if s.steps <= s.maxSteps {
		return true
	}
	s.exceeded = true
	return false
}

// Enter applies the scanner's seen and unique-node budget policies.
func (s *Scanner) Enter(t typ.Type) bool {
	if s == nil {
		return true
	}
	if s.seen != nil {
		if s.seen.Contains(t) {
			return false
		}
	}
	if s.maxEnters > 0 && s.enters >= s.maxEnters {
		s.exceeded = true
		return false
	}
	if s.seen != nil {
		s.seen.Remember(t)
	}
	s.enters++
	return true
}

// Exceeded reports whether any scanner budget was exceeded.
func (s *Scanner) Exceeded() bool {
	return s != nil && s.exceeded
}

// Complete reports whether the scanner finished without exceeding a budget.
func (s *Scanner) Complete() bool {
	return s == nil || !s.exceeded
}

// WalkChildren visits the canonical child type slots of t in stable order.
func (s *Scanner) WalkChildren(t typ.Type, visit func(typ.Type) bool) bool {
	return WalkChildren(t, visit)
}

// WalkChildren visits the canonical child type slots of t in stable order.
// It returns true when visit reports a match and traversal should stop.
func WalkChildren(t typ.Type, visit func(typ.Type) bool) bool {
	if visit == nil {
		return false
	}
	return typ.Visit(t, typ.Visitor[bool]{
		Optional: func(o *typ.Optional) bool {
			return visit(o.Inner)
		},
		Union: func(u *typ.Union) bool {
			return walkEach(u.Members, visit)
		},
		Intersection: func(in *typ.Intersection) bool {
			return walkEach(in.Members, visit)
		},
		Array: func(a *typ.Array) bool {
			return visit(a.Element)
		},
		Map: func(m *typ.Map) bool {
			return visit(m.Key) || visit(m.Value)
		},
		ReadonlyMap: func(m *typ.ReadonlyMap) bool {
			return visit(m.Key) || visit(m.Value)
		},
		Tuple: func(tup *typ.Tuple) bool {
			return walkEach(tup.Elements, visit)
		},
		Function: func(fn *typ.Function) bool {
			for _, param := range fn.Params {
				if visit(param.Type) {
					return true
				}
			}
			if visit(fn.Variadic) {
				return true
			}
			for _, ret := range fn.Returns {
				if visit(ret) {
					return true
				}
			}
			for _, param := range fn.TypeParams {
				if param != nil && visit(param.Constraint) {
					return true
				}
			}
			return false
		},
		Record: func(r *typ.Record) bool {
			if visit(r.MapKey) || visit(r.MapValue) || visit(r.Metatable) {
				return true
			}
			for _, field := range r.Fields {
				if visit(field.Type) {
					return true
				}
			}
			for _, member := range r.StaticMembers {
				if visit(member.Type) {
					return true
				}
			}
			return false
		},
		Alias: func(a *typ.Alias) bool {
			return visit(a.Target)
		},
		Meta: func(m *typ.Meta) bool {
			return visit(m.Of)
		},
		Generic: func(g *typ.Generic) bool {
			for _, param := range g.TypeParams {
				if param != nil && visit(param.Constraint) {
					return true
				}
			}
			return visit(g.Body)
		},
		Instantiated: func(i *typ.Instantiated) bool {
			if i.Generic != nil && visit(i.Generic) {
				return true
			}
			return walkEach(i.TypeArgs, visit)
		},
		TypeParam: func(p *typ.TypeParam) bool {
			return visit(p.Constraint)
		},
		Interface: func(i *typ.Interface) bool {
			for _, method := range i.Methods {
				if method.Type != nil && visit(method.Type) {
					return true
				}
			}
			return false
		},
		Recursive: func(r *typ.Recursive) bool {
			return visit(r.Body)
		},
	})
}

func walkEach(types []typ.Type, visit func(typ.Type) bool) bool {
	for _, t := range types {
		if visit(t) {
			return true
		}
	}
	return false
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
