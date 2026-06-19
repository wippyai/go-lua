package typ

const (
	recursiveHashSmallVisitedCap = 8
	recursiveHashSmallMemoCap    = 16
	recursiveHashSmallActiveCap  = 16
)

type recursiveHashMemoEntry struct {
	t Type
	h uint64
}

type recursiveHashScratch struct {
	visited    [recursiveHashSmallVisitedCap]*Recursive
	visitedLen int
	visitedMap map[*Recursive]bool

	active    [recursiveHashSmallActiveCap]Type
	activeLen int
	activeMap map[Type]bool

	memo    [recursiveHashSmallMemoCap]recursiveHashMemoEntry
	memoLen int
	memoMap map[Type]uint64
}

func (s *recursiveHashScratch) visitedContains(r *Recursive) bool {
	if s.visitedMap != nil {
		return s.visitedMap[r]
	}
	for i := 0; i < s.visitedLen; i++ {
		if s.visited[i] == r {
			return true
		}
	}
	return false
}

func (s *recursiveHashScratch) visitedPush(r *Recursive) {
	if s.visitedMap != nil {
		s.visitedMap[r] = true
		return
	}
	if s.visitedLen < len(s.visited) {
		s.visited[s.visitedLen] = r
		s.visitedLen++
		return
	}
	s.visitedMap = make(map[*Recursive]bool, len(s.visited)+1)
	for i := 0; i < s.visitedLen; i++ {
		s.visitedMap[s.visited[i]] = true
	}
	s.visitedMap[r] = true
}

func (s *recursiveHashScratch) visitedPop(r *Recursive) {
	if s.visitedMap != nil {
		delete(s.visitedMap, r)
		return
	}
	if s.visitedLen == 0 {
		return
	}
	s.visitedLen--
	s.visited[s.visitedLen] = nil
}

func (s *recursiveHashScratch) activeContains(t Type) bool {
	if s.activeMap != nil {
		return s.activeMap[t]
	}
	for i := 0; i < s.activeLen; i++ {
		if s.active[i] == t {
			return true
		}
	}
	return false
}

func (s *recursiveHashScratch) activePush(t Type) {
	if s.activeMap != nil {
		s.activeMap[t] = true
		return
	}
	if s.activeLen < len(s.active) {
		s.active[s.activeLen] = t
		s.activeLen++
		return
	}
	s.activeMap = make(map[Type]bool, len(s.active)+1)
	for i := 0; i < s.activeLen; i++ {
		s.activeMap[s.active[i]] = true
	}
	s.activeMap[t] = true
}

func (s *recursiveHashScratch) activePop(t Type) {
	if s.activeMap != nil {
		delete(s.activeMap, t)
		return
	}
	if s.activeLen == 0 {
		return
	}
	s.activeLen--
	s.active[s.activeLen] = nil
}

func (s *recursiveHashScratch) memoGet(t Type) (uint64, bool) {
	if s.memoMap != nil {
		h, ok := s.memoMap[t]
		return h, ok
	}
	for i := 0; i < s.memoLen; i++ {
		if s.memo[i].t == t {
			return s.memo[i].h, true
		}
	}
	return 0, false
}

func (s *recursiveHashScratch) memoSet(t Type, h uint64) {
	if s.memoMap != nil {
		s.memoMap[t] = h
		return
	}
	for i := 0; i < s.memoLen; i++ {
		if s.memo[i].t == t {
			s.memo[i].h = h
			return
		}
	}
	if s.memoLen < len(s.memo) {
		s.memo[s.memoLen] = recursiveHashMemoEntry{t: t, h: h}
		s.memoLen++
		return
	}
	s.memoMap = make(map[Type]uint64, len(s.memo)+1)
	for i := 0; i < s.memoLen; i++ {
		s.memoMap[s.memo[i].t] = s.memo[i].h
	}
	s.memoMap[t] = h
}
