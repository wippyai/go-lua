package placement

import (
	"sync"
	"testing"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

func TestStaticContainmentCacheEntryAuthenticatesPresenceAndValueEquality(t *testing.T) {
	// A positive empty-vector match exercises the immutable entry path without
	// manufacturing a Heap Value. Non-empty zero Values are deliberately not
	// admitted: heap.Equal must reject them, even when the hash agrees.
	entry := &staticContainmentCacheEntry{schema: Schema{}, hash: 17}
	if !entry.matches(Schema{}, 17, nil, nil) {
		t.Fatal("equal empty authenticated vector missed cache entry")
	}
	value := heapdomain.Value{}
	entry.values = []heapdomain.Value{value}
	entry.present = []bool{false}
	if entry.matches(Schema{}, 17, []heapdomain.Value{value}, []bool{true}) {
		t.Fatal("presence-only change hit cache entry")
	}
	if entry.matches(Schema{}, 17, []heapdomain.Value{value}, []bool{false}) {
		t.Fatal("invalid Heap Value bypassed heap.Equal authentication")
	}
	if entry.matches(Schema{}, 18, []heapdomain.Value{value}, []bool{false}) {
		t.Fatal("hash mismatch hit cache entry")
	}
}

func TestStaticContainmentCacheAtomicEntryPublicationIsComplete(t *testing.T) {
	cache := &StaticContainmentCache{}
	left := &staticContainmentCacheEntry{schema: Schema{}, hash: 1}
	right := &staticContainmentCacheEntry{schema: Schema{}, hash: 2}
	cache.entry.Store(left)

	const readers = 8
	const rounds = 500
	var group sync.WaitGroup
	group.Add(readers + 1)
	for reader := 0; reader < readers; reader++ {
		go func() {
			defer group.Done()
			for round := 0; round < rounds; round++ {
				entry := cache.entry.Load()
				if entry == nil || entry.schema != (Schema{}) || entry.values != nil || entry.present != nil {
					t.Errorf("reader observed incomplete cache entry")
					return
				}
			}
		}()
	}
	go func() {
		defer group.Done()
		for round := 0; round < rounds; round++ {
			if round&1 == 0 {
				cache.entry.Store(left)
			} else {
				cache.entry.Store(right)
			}
		}
	}()
	group.Wait()
}

func TestNewStaticContainmentCacheRejectsUnavailableSchema(t *testing.T) {
	if NewStaticContainmentCache(Schema{}) != nil {
		t.Fatal("unavailable schema issued containment cache")
	}
}
