package effectdelta

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
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

func TestMapDomainCanonicalization(t *testing.T) {
	reg := standard.Registry()
	domain := MapDomain(reg)
	ks := keyspace.New()
	target1, ok := ks.FromStateKey(pathdom.PathKey("sym1@1.field"))
	if !ok {
		t.Fatal("FromStateKey failed")
	}
	target2, ok := ks.FromStateKey(pathdom.PathKey("sym2@1.field"))
	if !ok {
		t.Fatal("FromStateKey failed")
	}
	key := Key{Target: target1, Site: "effect", Kind: Mutation}
	otherKey := Key{Target: target2, Site: "effect", Kind: Escape}
	present := presentValue(reg)
	absent := absentValue(reg)
	delta := Value{Before: present, After: absent, Change: ChangeChanged}
	bottom := Bottom(reg)

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

	if got := domain.Join(joined, map[Key]Value{key: bottom}); !domain.Equal(got, joined) {
		t.Fatalf("joining bottom entry changed joined map: %#v", got)
	}
	if got := domain.Join(map[Key]Value{key: bottom}, nil); got != nil {
		t.Fatalf("map domain should canonicalize bottom-only maps to nil, got %#v", got)
	}
}

func TestMapDomainTopStableAcrossRepeatedConstruction(t *testing.T) {
	reg := standard.Registry()
	top := MapDomain(reg).Top()
	domain := MapDomain(reg)
	if !domain.Equal(top, domain.Top()) {
		t.Fatalf("reconstructed map domain did not recognize prior top sentinel")
	}
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}
