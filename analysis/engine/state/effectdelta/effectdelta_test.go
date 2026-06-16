package effectdelta

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestDomainBottomTopAndOrder(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	bottom := Bottom(reg)
	top := Top()
	present := presentValue(reg)
	absent := absentValue(reg)
	changed := Value{Before: present, After: absent, Change: ChangeChanged}
	none := Value{Before: present, After: present, Change: ChangeNone}

	if !domain.Equal(bottom, Value{}) {
		t.Fatalf("bottom = %#v, want equal to zero-value bottom", bottom)
	}
	if domain.Equal(bottom, top) {
		t.Fatalf("bottom and top should differ")
	}
	if !domain.LessOrEq(bottom, changed) {
		t.Fatalf("bottom should be below every value")
	}
	if domain.LessOrEq(changed, bottom) {
		t.Fatalf("non-bottom value should not be below bottom")
	}
	if !domain.Equal(domain.Join(bottom, changed), changed) {
		t.Fatalf("join(bottom, changed) should return changed")
	}
	if !domain.Equal(domain.Join(changed, bottom), changed) {
		t.Fatalf("join(changed, bottom) should return changed")
	}
	if got := domain.Join(changed, none); got.Change != ChangeUnknown {
		t.Fatalf("join should lift change to unknown, got %#v", got)
	}
	if got := domain.Widen(changed, none); got.Change != ChangeUnknown {
		t.Fatalf("widen should lift change to unknown, got %#v", got)
	}
	if !domain.Equal(domain.Join(top, changed), top) {
		t.Fatalf("top should absorb join")
	}
	if !domain.LessOrEq(changed, top) {
		t.Fatalf("every value should be below top")
	}
}

func TestMapDomainCloneDeleteAndCanonicalization(t *testing.T) {
	reg := standard.Registry()
	domain := MapDomain(reg)
	key := Key{Target: pathdom.PathKey("sym1@1.field"), Site: "effect", Kind: Mutation}
	otherKey := Key{Target: pathdom.PathKey("sym2@1.field"), Site: "effect", Kind: Escape}
	present := presentValue(reg)
	absent := absentValue(reg)
	delta := Value{Before: present, After: absent, Change: ChangeChanged}
	bottom := Bottom(reg)

	original := map[Key]Value{key: delta}
	clone := CloneMap(original)
	clone[key] = bottom
	if got := original[key]; got != delta {
		t.Fatalf("clone mutation changed original: %#v", got)
	}
	if got := domain.Equal(clone, map[Key]Value{key: bottom}); !got {
		t.Fatalf("clone should be independent and canonicalize bottom-like values")
	}

	left := map[Key]Value{key: delta}
	right := map[Key]Value{otherKey: Value{Before: absent, After: present, Change: ChangeNone}}
	joined := domain.Join(left, right)
	if len(joined) != 2 {
		t.Fatalf("join = %#v, want two keys", joined)
	}
	if !domain.LessOrEq(left, joined) || !domain.LessOrEq(right, joined) {
		t.Fatalf("join should be an upper bound")
	}
	if !domain.Equal(domain.Join(domain.Bottom(), left), left) {
		t.Fatalf("bottom should be join identity")
	}
	if !domain.Equal(domain.Join(domain.Top(), left), domain.Top()) {
		t.Fatalf("top should absorb join")
	}

	next, removed := DeleteEntry(joined, key)
	if !removed {
		t.Fatalf("delete should report removal")
	}
	if _, ok := next[key]; ok {
		t.Fatalf("delete retained removed key: %#v", next)
	}
	if _, removed := DeleteEntry(next, key); removed {
		t.Fatalf("deleting a missing key should report false")
	}
	if out, removed := DeleteEntry(map[Key]Value{key: delta}, key); !removed || out != nil {
		t.Fatalf("deleting last entry should return nil/true, got %#v/%v", out, removed)
	}
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}
