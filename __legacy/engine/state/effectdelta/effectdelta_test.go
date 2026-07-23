package effectdelta

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestExactLatticeLaws(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)
	latticelaws.LawSuite[Value]{
		Name:   "state.effectdelta.Value",
		Domain: domain,
		Sample: []Value{
			domain.Bottom(),
			domain.Top(),
			{Before: present, After: present, Change: ChangeNone},
			{Before: present, After: absent, Change: ChangeChanged},
			{Before: product.Top(), After: absent, Change: ChangeUnknown},
		},
		WideningBound: 8,
	}.Run(t)
}

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

func TestDomainMeetIsExactProductWithChangeDiamond(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	present := presentValue(reg)
	absent := absentValue(reg)
	bottom := Bottom(reg)

	unknown := Value{Before: product.Top(), After: absent, Change: ChangeUnknown}
	changed := Value{Before: present, After: product.Top(), Change: ChangeChanged}
	met := domain.Meet(unknown, changed)
	if met.Change != ChangeChanged ||
		!product.Equal(reg, met.Before, present) ||
		!product.Equal(reg, met.After, absent) {
		t.Fatalf("Meet(unknown, changed) = %#v, want exact component product", met)
	}
	if !domain.Equal(domain.Meet(Top(), changed), changed) {
		t.Fatal("Meet(Top, changed) must be changed")
	}
	if !domain.Equal(domain.Meet(bottom, changed), bottom) {
		t.Fatal("Meet(Bottom, changed) must be canonical Bottom")
	}

	none := Value{Before: present, After: present, Change: ChangeNone}
	if got := domain.Meet(none, changed); !domain.Equal(got, bottom) || got.Change != ChangeBottom {
		t.Fatalf("Meet(None, Changed) = %#v, want canonical Bottom", got)
	}
	joined := domain.Join(none, changed)
	if got := domain.Meet(none, joined); !domain.Equal(got, none) {
		t.Fatalf("Meet absorption = %#v, want %#v", got, none)
	}
	if got := domain.Join(none, domain.Meet(none, changed)); !domain.Equal(got, none) {
		t.Fatalf("Join absorption = %#v, want %#v", got, none)
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

	unknown := Value{Before: product.Top(), After: absent, Change: ChangeUnknown}
	met := domain.Meet(
		map[Key]Value{key: unknown, otherKey: delta},
		map[Key]Value{key: delta},
	)
	if len(met) != 1 || !Domain(reg).Equal(met[key], delta) {
		t.Fatalf("pointwise map Meet = %#v, want shared exact key only", met)
	}
	if got := domain.Meet(domain.Top(), left); !domain.Same(got, left) {
		t.Fatal("map Meet(Top, finite) did not reuse the finite operand")
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
