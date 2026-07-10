package typ

import "testing"

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
