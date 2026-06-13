package dynamicindex

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/standard"
)

func TestBottomAndTopFacts(t *testing.T) {
	reg := standard.Registry()
	bottom := Bottom(reg)
	if !presence.Equal(bottom.KeyPresence, presence.Bottom()) ||
		!product.Equal(reg, bottom.KeyValue, product.Bottom(reg)) ||
		!product.Equal(reg, bottom.Value, product.Bottom(reg)) ||
		bottom.Admission != AdmissionBottom {
		t.Fatalf("Bottom = %#v", bottom)
	}

	top := Top()
	if !presence.Equal(top.KeyPresence, presence.Top()) ||
		!product.Equal(reg, top.KeyValue, product.Top()) ||
		!product.Equal(reg, top.Value, product.Top()) ||
		top.Admission != AdmissionUnknown {
		t.Fatalf("Top = %#v", top)
	}
}

func TestDomainElementLatticeJoinsPointwise(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	absent := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())

	admitted := Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
		Admission:   AdmissionAdmitted,
	}
	rejected := Fact{
		KeyPresence: presence.Absent(),
		KeyValue:    absent,
		Value:       absent,
		Admission:   AdmissionRejected,
	}

	joined := domain.Join(admitted, rejected)
	if !presence.Equal(joined.KeyPresence, presence.Maybe()) ||
		!product.Equal(reg, joined.KeyValue, product.Top()) ||
		!product.Equal(reg, joined.Value, product.Top()) ||
		joined.Admission != AdmissionUnknown {
		t.Fatalf("Join = %#v, want pointwise top/unknown", joined)
	}
	if widened := domain.Widen(admitted, rejected); !domain.Equal(widened, joined) {
		t.Fatalf("Widen = %#v, want %#v", widened, joined)
	}
	if !domain.LessOrEq(admitted, joined) || !domain.LessOrEq(rejected, joined) {
		t.Fatalf("Join is not an upper bound")
	}
}

func TestAdmissionOrderAndJoin(t *testing.T) {
	reg := standard.Registry()
	domain := Domain(reg)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	base := Fact{
		KeyPresence: presence.Present(),
		KeyValue:    present,
		Value:       present,
	}

	bottom := base
	bottom.Admission = AdmissionBottom
	admitted := base
	admitted.Admission = AdmissionAdmitted
	rejected := base
	rejected.Admission = AdmissionRejected
	unknown := base
	unknown.Admission = AdmissionUnknown

	if !domain.LessOrEq(bottom, admitted) || !domain.LessOrEq(bottom, rejected) ||
		!domain.LessOrEq(admitted, unknown) || !domain.LessOrEq(rejected, unknown) {
		t.Fatalf("admission order is not bottom < admitted/rejected < unknown")
	}
	if domain.LessOrEq(admitted, rejected) || domain.LessOrEq(rejected, admitted) {
		t.Fatalf("admitted and rejected should be incomparable")
	}
	if got := domain.Join(admitted, rejected).Admission; got != AdmissionUnknown {
		t.Fatalf("admitted join rejected = %v, want unknown", got)
	}
	if got := domain.Join(bottom, admitted).Admission; got != AdmissionAdmitted {
		t.Fatalf("bottom join admitted = %v, want admitted", got)
	}
}

func TestMapDomainCloneAndDelete(t *testing.T) {
	reg := standard.Registry()
	domain := MapDomain(reg)
	key := Key{Table: pathdom.PathKey("sym1@1.table"), Site: "site"}
	other := Key{Table: pathdom.PathKey("sym1@1.table"), Site: "other"}
	fact := Fact{
		KeyPresence: presence.Present(),
		KeyValue:    product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		Value:       product.NewWithPresence(reg, product.ShapeTop, presence.Absent()),
		Admission:   AdmissionAdmitted,
	}
	m := map[Key]Fact{key: fact, other: Bottom(reg)}

	canonical := domain.Join(nil, m)
	if len(canonical) != 1 || !domain.Equal(canonical, map[Key]Fact{key: fact}) {
		t.Fatalf("map domain did not canonicalize bottom entries: %#v", canonical)
	}
	if !domain.LessOrEq(canonical, domain.Top()) {
		t.Fatalf("finite map should be <= top")
	}

	cloned := CloneMap(canonical)
	cloned[key] = Bottom(reg)
	if domain.Equal(cloned, canonical) {
		t.Fatalf("CloneMap did not isolate mutation")
	}
	if got := canonical[key]; !Domain(reg).Equal(got, fact) {
		t.Fatalf("original map changed after clone mutation: %#v", got)
	}

	deleted, changed := DeleteEntry(canonical, key)
	if !changed || len(deleted) != 0 {
		t.Fatalf("DeleteEntry = %#v/%v, want empty changed map", deleted, changed)
	}
	if got, changed := DeleteEntry(canonical, other); changed || got == nil {
		t.Fatalf("DeleteEntry missing key = %#v/%v, want original unchanged", got, changed)
	}
}
