package typ

import (
	"strconv"
	"testing"
)

// benchmarkRecursiveFamily is deliberately shaped like exported recursive
// families: a mutually-recursive record ring, with a union retaining every
// member. It keeps setup out of the timed region so equality/hash work is the
// only thing measured.
func benchmarkRecursiveFamily(size int) Type {
	nodes := make([]*Recursive, size)
	for i := range nodes {
		nodes[i] = NewRecursivePlaceholder("Node")
	}
	for i, node := range nodes {
		node.SetBody(newRecord().
			Field("next", nodes[(i+1)%len(nodes)]).
			Field("payload", MaterializeUnion([]Type{String, Number, Boolean})).
			Build())
	}
	members := make([]Type, len(nodes))
	for i, node := range nodes {
		members[i] = node
	}
	return newRecord().
		Field("entry", nodes[0]).
		Field("members", MaterializeUnion(members)).
		Build()
}

func BenchmarkRecursiveFamilyEquality(b *testing.B) {
	left := benchmarkRecursiveFamily(48)
	right := benchmarkRecursiveFamily(48)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !TypeEquals(left, right) {
			b.Fatal("equivalent recursive families must compare equal")
		}
	}
}

func BenchmarkRecursiveFamilyEqualitySizes(b *testing.B) {
	for _, size := range []int{8, 32, 48, 96, 256} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			left := benchmarkRecursiveFamily(size)
			right := benchmarkRecursiveFamily(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !TypeEquals(left, right) {
					b.Fatal("equivalent recursive families must compare equal")
				}
			}
		})
	}
}

func BenchmarkRecursiveFamilyEqualityHash(b *testing.B) {
	typeFamily := benchmarkRecursiveFamily(48)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if EqualityHash(typeFamily) == 0 {
			b.Fatal("recursive family hash must be non-zero")
		}
	}
}

func BenchmarkRecursiveFamilyOpenRecursiveCache(b *testing.B) {
	typeFamily := benchmarkRecursiveFamily(48)
	if knownContainsOpenRecursive(typeFamily) {
		b.Fatal("closed recursive family must not be open")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if knownContainsOpenRecursive(typeFamily) {
			b.Fatal("closed recursive family must not be open")
		}
	}
}

// benchmarkLiveOpenRecursiveScan is the pre-cache traversal retained as a
// benchmark control. The production predicate must remain materially cheaper
// than this full graph walk after its first lookup.
type benchmarkOpenRecursiveScan struct {
	small    [64]recursiveTraversalMemoKey
	smallLen int
	entries  map[recursiveTraversalMemoKey]struct{}
}

func (s *benchmarkOpenRecursiveScan) contains(t Type) bool {
	t = unwrapAnnotatedOrNil(t)
	if t == nil {
		return false
	}
	if rec, ok := t.(*Recursive); ok {
		rec.ensureContainsClosedFlag()
		return !rec.containsFlagsClosed
	}
	if !knownContainsRecursive(t) || !s.enter(t) {
		return false
	}
	return WalkChildren(t, func(child Type) bool {
		return s.contains(child)
	})
}

func (s *benchmarkOpenRecursiveScan) enter(t Type) bool {
	key, ok := recursiveTraversalMemo(t)
	if !ok {
		return true
	}
	for i := 0; i < s.smallLen; i++ {
		if s.small[i] == key {
			return false
		}
	}
	if _, ok := s.entries[key]; ok {
		return false
	}
	if s.entries == nil && s.smallLen < len(s.small) {
		s.small[s.smallLen] = key
		s.smallLen++
		return true
	}
	if s.entries == nil {
		s.entries = make(map[recursiveTraversalMemoKey]struct{}, len(s.small)+1)
		for i := 0; i < s.smallLen; i++ {
			s.entries[s.small[i]] = struct{}{}
		}
		s.smallLen = 0
	}
	s.entries[key] = struct{}{}
	return true
}

func BenchmarkRecursiveFamilyOpenRecursiveLiveRescan(b *testing.B) {
	typeFamily := benchmarkRecursiveFamily(48)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var scan benchmarkOpenRecursiveScan
		if scan.contains(typeFamily) {
			b.Fatal("closed recursive family must not be open")
		}
	}
}
