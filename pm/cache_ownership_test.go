package pm

import (
	"testing"
	"unsafe"
)

func TestPatternCacheOwnsPatternKey(t *testing.T) {
	cache := newPatternCache(4)
	buf := []byte("abc")
	borrowed := unsafe.String(&buf[0], len(buf))

	cache.put(borrowed, nil, nil, 0)
	copy(buf, "xyz")

	front := cache.order.Front()
	if front == nil {
		t.Fatal("cache entry missing")
	}
	entry := front.Value.(*cacheEntry)
	if entry.key != "abc" {
		t.Fatalf("cache retained borrowed key storage: %q", entry.key)
	}
	if _, _, _, ok := cache.get("abc"); !ok {
		t.Fatal("cache lookup by original pattern failed")
	}
}
