package link

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// Literal values must enter the one Link key universe even when authored only
// as values. Dynamic access keeps its Value occurrence; a later Rule may prove
// that occurrence exact without introducing another key coordinate.
func TestLinkLiteralValueKeyUniverseAndInitialValueProjection(t *testing.T) {
	main := source(t, `
local falseKey = false
local integerKey = 1
local floatKey = 1.0
local stringKey = "value-only"
local zeroKey = 0.0
local tab = {}
return tab[falseKey], tab[integerKey], tab[floatKey], tab[stringKey], tab[zeroKey]
`)
	other := source(t, `local key = "other-only"; local tab = {}; return tab[key]`)
	contract := literalProjectionContract(t)
	first := linked(t, contract, linkproject.Module{Name: "main", Program: main}, linkproject.Module{Name: "other", Program: other})
	second := linked(t, contract, linkproject.Module{Name: "other", Program: other}, linkproject.Module{Name: "main", Program: main})

	if first.ContentID() != second.ContentID() || first.Project().Keys().Count() != second.Project().Keys().Count() {
		t.Fatal("module declaration order changed the sealed literal key universe")
	}
	for index := 0; index < first.Project().Keys().Count(); index++ {
		left, leftOK := first.Project().Keys().At(index)
		right, rightOK := second.Project().Keys().At(index)
		leftValue, leftValueOK := first.Project().Keys().Exact(left)
		rightValue, rightValueOK := second.Project().Keys().Exact(right)
		if !leftOK || !rightOK || !leftValueOK || !rightValueOK || leftValue != rightValue {
			t.Fatalf("key universe differs at %d: %#v/%v %#v/%v", index, leftValue, leftValueOK, rightValue, rightValueOK)
		}
	}

	keys := map[string]linkproject.Key{}
	for name, literal := range map[string]keyspace.LiteralValue{
		"false":   {Kind: keyspace.LiteralBool, Bool: false},
		"integer": {Kind: keyspace.LiteralInteger, Integer: 1},
		"float":   {Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1)},
		"string":  {Kind: keyspace.LiteralString, String: "value-only"},
		"zero":    {Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Copysign(0, -1))},
	} {
		key, ok := literalKeyFor(first.Project().Keys(), literal)
		if !ok {
			t.Fatalf("literal authored only as a Program value lacks Link key: %s", name)
		}
		keys[name] = key
	}
	if keys["integer"] != keys["float"] || keys["zero"] != mustLiteralKey(t, first, keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: 0}) {
		t.Fatal("Lua numeric equality did not retain its canonical Link key")
	}
	if _, ok := literalKeyFor(first.Project().Keys(), keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())}); ok {
		t.Fatal("NaN acquired a storable Link key")
	}
	for _, want := range []struct {
		name string
		key  linkproject.Key
	}{
		{"primitive_false", keys["false"]},
		{"primitive_integer", keys["integer"]},
		{"primitive_float", keys["float"]},
		{"primitive_string", keys["string"]},
		{"primitive_zero", keys["zero"]},
	} {
		_, value, _, _, ok := contract.InitialBinding(want.name)
		got, found := first.Project().Keys().ForInitial(contract, value)
		if !ok || !found || got != want.key {
			t.Fatalf("Target initial %s exact key = %v/%v, want %v", want.name, got, found, want.key)
		}
	}
	for _, name := range []string{"primitive_nan", "primitive_absent", "primitive_root", "primitive_callable"} {
		_, value, _, _, ok := contract.InitialBinding(name)
		if !ok {
			t.Fatalf("fixture lacks %s binding", name)
		}
		if key, found := first.Project().Keys().ForInitial(contract, value); found || key != (linkproject.Key{}) {
			t.Fatalf("non-storable Target initial %s acquired Link key %v/%v", name, key, found)
		}
	}

	replayed := artifactAssertProjectionRoundTrip(t, first, contract, main, other)
	for name, want := range keys {
		got, ok := literalKeyFor(replayed.Project().Keys(), map[string]keyspace.LiteralValue{
			"false":   {Kind: keyspace.LiteralBool, Bool: false},
			"integer": {Kind: keyspace.LiteralInteger, Integer: 1},
			"float":   {Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1)},
			"string":  {Kind: keyspace.LiteralString, String: "value-only"},
			"zero":    {Kind: keyspace.LiteralInteger, Integer: 0},
		}[name])
		wantIndex, wantOK := first.Project().Keys().Index(want)
		gotIndex, gotOK := replayed.Project().Keys().Index(got)
		if !ok || !wantOK || !gotOK || gotIndex != wantIndex {
			t.Fatalf("artifact replay changed literal key %s = %v/%v, want %v", name, got, ok, want)
		}
	}
}

func mustLiteralKey(t *testing.T, link *Link, literal keyspace.LiteralValue) linkproject.Key {
	t.Helper()
	key, ok := literalKeyFor(link.Project().Keys(), literal)
	if !ok {
		t.Fatalf("literal %#v lacks Link key", literal)
	}
	return key
}

func literalKeyFor(keys linkproject.Keys, literal keyspace.LiteralValue) (linkproject.Key, bool) {
	want, ok := scalar.Normalize(literal)
	if !ok {
		return linkproject.Key{}, false
	}
	for index := 0; index < keys.Count(); index++ {
		key, keyOK := keys.At(index)
		value, valueOK := keys.Exact(key)
		got, normalized := scalar.Normalize(value)
		order, ordered := scalar.Compare(got, want)
		if keyOK && valueOK && normalized && ordered && order == 0 {
			return key, true
		}
	}
	return linkproject.Key{}, false
}

func literalProjectionContract(t *testing.T) *target.Contract {
	t.Helper()
	operation := actorBootOperation("callable")
	return actorBootContract(t, []target.OperationSpec{operation}, []target.InitialEntrySpec{
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_false"), Value: target.InitialValueSpec{Kind: target.InitialValueBoolean, Boolean: false}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_integer"), Value: target.InitialValueSpec{Kind: target.InitialValueInteger, Integer: 1}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_float"), Value: target.InitialValueSpec{Kind: target.InitialValueFloat, FloatBits: math.Float64bits(1)}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_string"), Value: target.InitialValueSpec{Kind: target.InitialValueString, String: "value-only"}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_zero"), Value: target.InitialValueSpec{Kind: target.InitialValueFloat, FloatBits: math.Float64bits(math.Copysign(0, -1))}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_nan"), Value: target.InitialValueSpec{Kind: target.InitialValueFloat, FloatBits: math.Float64bits(math.NaN())}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_absent"), Value: target.InitialValueSpec{Kind: target.InitialValueAbsent}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_root"), Value: target.InitialValueSpec{Kind: target.InitialValueRoot, Root: "GlobalEnvRoot"}, Mutability: target.InitialMutable},
		{Root: "GlobalEnvRoot", Key: targetStringKey("primitive_callable"), Value: target.InitialValueSpec{Kind: target.InitialValueOperation, Operation: target.BindingSpec{Namespace: target.BindingBuiltin, Member: []string{"callable"}}}, Mutability: target.InitialMutable},
	}, []target.InitialBindingSpec{
		{Name: "primitive_false", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_false")},
		{Name: "primitive_integer", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_integer")},
		{Name: "primitive_float", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_float")},
		{Name: "primitive_string", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_string")},
		{Name: "primitive_zero", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_zero")},
		{Name: "primitive_nan", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_nan")},
		{Name: "primitive_root", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_root")},
		{Name: "primitive_callable", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_callable")},
		{Name: "primitive_absent", Root: "GlobalEnvRoot", Key: targetStringKey("primitive_absent")},
	})
}
