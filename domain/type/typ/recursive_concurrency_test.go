package typ

import (
	"sync"
	"testing"
)

func TestRecursiveDerivedMemosAreConcurrentReadSafe(t *testing.T) {
	left := NewRecursivePlaceholder("Left")
	right := NewRecursivePlaceholder("Right")
	left.SetBody(newRecord().Field("value", Any).OptField("right", right).Build())
	right.SetBody(newRecord().Field("bottom", Never).OptField("left", left).Build())
	wrapper := newRecord().Field("root", left).Build()

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				if !ContainsAny(wrapper) || !ContainsNever(wrapper) {
					t.Error("mutually recursive graph lost containment flags")
					return
				}
				if knownContainsOpenRecursive(wrapper) {
					t.Error("sealed mutually recursive graph reported open")
					return
				}
				if left.Hash() == 0 || EqualityHash(wrapper) == 0 {
					t.Error("recursive graph produced zero structural hash")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}

func TestRecursiveSetBodyPanicsOnSecondCallAfterMemosPublish(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	node.SetBody(newRecord().Field("value", Any).OptField("next", node).Build())
	firstHash := node.Hash()
	if !ContainsAny(node) || knownContainsOpenRecursive(node) {
		t.Fatal("initial recursive body did not derive expected properties")
	}
	if node.containsMemo.Load() == nil || node.closedMemo.Load() == nil || node.hashMemo.Load() == nil {
		t.Fatal("initial queries did not publish all derived memos")
	}

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("second SetBody on a sealed recursive node did not panic")
			}
		}()
		node.SetBody(newRecord().Field("value", String).Build())
	}()

	if got := node.Hash(); got != firstHash {
		t.Fatalf("rejected SetBody mutated the sealed hash: got %d, want %d", got, firstHash)
	}
}

func TestRecursiveProductCloneDoesNotCopyPublishedMemoSlot(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	fn := Func().Param("node", node).Returns(node).Build()
	if got, want := fn.String(), "fun(node: μNode. {next?: Node}) -> μNode. {next?: Node}"; got != want {
		t.Fatalf("function String() = %q, want %q", got, want)
	}
	if knownContainsOpenRecursive(fn) {
		t.Fatal("original closed function signature reported open")
	}
	originalMemo := fn.loadOpenRecursiveMemo()
	if originalMemo == nil || originalMemo.contains {
		t.Fatalf("original memo = %#v, want closed-graph proof", originalMemo)
	}
	initialClone := CloneFunction(fn)
	if initialClone == nil {
		t.Fatal("CloneFunction returned nil")
	}
	if cloneMemo := initialClone.loadOpenRecursiveMemo(); cloneMemo != nil {
		t.Fatalf("clone inherited original published memo slot: %#v", cloneMemo)
	}
	if got, want := initialClone.String(), fn.String(); got != want {
		t.Fatalf("clone String() = %q, want %q", got, want)
	}
	if knownContainsOpenRecursive(initialClone) {
		t.Fatal("clone recomputed an open graph for the same recursive signature")
	}
	cloneMemo := initialClone.loadOpenRecursiveMemo()
	if cloneMemo == nil || cloneMemo.contains {
		t.Fatalf("clone memo = %#v, want independently published closed-graph proof", cloneMemo)
	}
	if cloneMemo == originalMemo {
		t.Fatal("clone and original share the same published memo record")
	}
	if got := fn.loadOpenRecursiveMemo(); got != originalMemo {
		t.Fatal("computing the clone memo replaced or shared the original memo slot")
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				if knownContainsOpenRecursive(fn) {
					t.Error("closed function signature reported open")
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 100; j++ {
				clone := CloneFunction(fn)
				if clone == nil || knownContainsOpenRecursive(clone) {
					t.Error("cloned closed function signature reported open")
					return
				}
			}
		}()
	}
	close(start)
	wg.Wait()
}

func BenchmarkRecursiveDerivedMemoRead(b *testing.B) {
	node := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("value", Any).OptField("next", self).Build()
	})
	_ = ContainsAny(node)
	_ = knownContainsOpenRecursive(node)
	_ = node.Hash()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !knownContainsAny(node) || knownContainsOpenRecursive(node) || node.Hash() == 0 {
			b.Fatal("invalid derived memo")
		}
	}
}
