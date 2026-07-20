package factflow

import (
	"context"
	"testing"
)

func TestCanonicalValueSourceContentIDTracksExactSource(t *testing.T) {
	shape, _ := NewValueSourceShape(true, false, false, false)
	base, _ := NewPathValueSource("7:module", 2, 0, 0, shape)
	first, err := CanonicalValueSourceContentID(context.Background(), base)
	if err != nil || !first.Available() {
		t.Fatalf("content identity = %x, err=%v", first, err)
	}
	again, err := CanonicalValueSourceContentID(context.Background(), base)
	if err != nil || again != first || !ValueSourceEqual(base, base) {
		t.Fatalf("equivalent source identity/equality drifted: %x/%x err=%v", first, again, err)
	}
	changed := base
	changed.SourcePoint = 3
	changed.HasSourcePoint = true
	different, err := CanonicalValueSourceContentID(context.Background(), changed)
	if err != nil || different == first || ValueSourceEqual(base, changed) {
		t.Fatalf("source-point change was invisible: %x/%x err=%v", first, different, err)
	}
}
