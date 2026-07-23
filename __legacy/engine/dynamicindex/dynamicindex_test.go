package dynamicindex

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
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

func TestNewFactDefaultsUnobservedReadbackToBottom(t *testing.T) {
	reg := standard.Registry()
	got := NewFact(reg, FactConfig{Admission: AdmissionRejected})

	if !presence.Equal(got.KeyPresence, presence.Bottom()) ||
		!product.Equal(reg, got.KeyValue, product.Bottom(reg)) ||
		!product.Equal(reg, got.Value, product.Bottom(reg)) ||
		got.Admission != AdmissionRejected {
		t.Fatalf("NewFact without observations = %#v, want bottom key/value with configured admission", got)
	}
}

func TestNewFactDerivesKeyPresenceFromObservedKeyValue(t *testing.T) {
	reg := standard.Registry()
	keyValue := product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())

	got := NewFact(reg, FactConfig{
		KeyValue:    keyValue,
		HasKeyValue: true,
		Value:       value,
		HasValue:    true,
		Admission:   AdmissionAdmitted,
	})

	if !presence.Equal(got.KeyPresence, presence.Absent()) ||
		!product.Equal(reg, got.KeyValue, keyValue) ||
		!product.Equal(reg, got.Value, value) ||
		got.Admission != AdmissionAdmitted {
		t.Fatalf("NewFact with observations = %#v, want observed key presence/value/admission", got)
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

func TestDomainExactMeetLaws(t *testing.T) {
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
	unknown := Fact{
		KeyPresence: presence.Maybe(),
		KeyValue:    product.Top(),
		Value:       product.Top(),
		Admission:   AdmissionUnknown,
	}
	if domain.Meet == nil {
		t.Fatal("dynamic-index fact domain has no exact Meet")
	}
	latticelaws.LawSuite[Fact]{
		Name:   "dynamicindex.Fact",
		Domain: domain,
		Sample: []Fact{domain.Bottom(), admitted, rejected, unknown, domain.Top()},
	}.Run(t)

	met := domain.Meet(admitted, rejected)
	if !presence.Equal(met.KeyPresence, presence.Bottom()) ||
		!product.Equal(reg, met.KeyValue, product.Bottom(reg)) ||
		!product.Equal(reg, met.Value, product.Bottom(reg)) ||
		met.Admission != AdmissionBottom {
		t.Fatalf("Meet(admitted, rejected) = %#v, want pointwise bottom", met)
	}
	if got := domain.Meet(domain.Top(), admitted); !domain.Equal(got, admitted) {
		t.Fatalf("Meet(top, admitted) = %#v, want admitted", got)
	}
}

func TestMapDomainExactMeetLaws(t *testing.T) {
	reg := standard.Registry()
	factDomain := Domain(reg)
	domain := MapDomain(reg)
	ks := keyspace.New()
	tableKey, ok := ks.FromStateKey(pathdom.PathKey("sym1@1.table"))
	if !ok {
		t.Fatal("FromStateKey failed")
	}
	first := Key{Table: tableKey, Site: "first"}
	second := Key{Table: tableKey, Site: "second"}
	fact := Fact{
		KeyPresence: presence.Present(),
		KeyValue:    product.NewWithPresence(reg, product.ShapeTop, presence.Present()),
		Value:       product.NewWithPresence(reg, product.ShapeTop, presence.Absent()),
		Admission:   AdmissionAdmitted,
	}
	left := map[Key]Fact{first: fact}
	right := map[Key]Fact{first: fact, second: fact}
	if domain.Meet == nil {
		t.Fatal("dynamic-index map domain has no exact Meet")
	}
	latticelaws.LawSuite[map[Key]Fact]{
		Name:   "dynamicindex.Map",
		Domain: domain,
		Sample: []map[Key]Fact{
			domain.Bottom(), left, right, domain.Top(),
		},
	}.Run(t)
	if got := domain.Meet(left, right); !domain.Equal(got, left) {
		t.Fatalf("Meet(left, right) = %#v, want shared pointwise facts %#v", got, left)
	}
	if got := domain.Meet(map[Key]Fact{first: factDomain.Bottom()}, right); !domain.Equal(got, domain.Bottom()) {
		t.Fatalf("Meet(explicit bottom, right) = %#v, want canonical map bottom", got)
	}
}

func TestMapDomainCloneAndCanonicalization(t *testing.T) {
	reg := standard.Registry()
	domain := MapDomain(reg)
	ks := keyspace.New()
	tableKey, ok := ks.FromStateKey(pathdom.PathKey("sym1@1.table"))
	if !ok {
		t.Fatal("FromStateKey failed")
	}
	key := Key{Table: tableKey, Site: "site"}
	other := Key{Table: tableKey, Site: "other"}
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

	if got := domain.Join(canonical, map[Key]Fact{key: Bottom(reg)}); !domain.Equal(got, canonical) {
		t.Fatalf("joining bottom entry changed canonical map: %#v", got)
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
