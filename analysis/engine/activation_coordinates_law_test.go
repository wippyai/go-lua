package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

func TestActivationCoordinatesKeepBindingWhenAxesCoincide(t *testing.T) {
	application, target, endpoint := coldKey(980_002), coldKey(980_003), coldKey(980_004)
	first := ActivationCoordinates{
		binding: coldKey(980_000), application: application, target: target, endpoint: endpoint,
	}
	second := ActivationCoordinates{
		binding: coldKey(980_001), application: application, target: target, endpoint: endpoint,
	}
	if !first.Available() || !second.Available() {
		t.Fatal("complete activation coordinates unavailable")
	}
	if first.Application() != second.Application() || first.Target() != second.Target() || first.Endpoint() != second.Endpoint() {
		t.Fatal("fixture axes differ")
	}
	if first.Binding() == second.Binding() || !first.Binding().Available() || !second.Binding().Available() {
		t.Fatal("equal activation axes collapsed distinct bindings")
	}

	missing := ActivationCoordinates{application: application, target: target, endpoint: endpoint}
	if missing.Available() || missing.Binding().Available() {
		t.Fatal("activation coordinates accepted an unavailable binding")
	}
}

func TestActivationReadPortBindingIssuesOnlyTypedExactRef(t *testing.T) {
	composition := NewComposition()
	value := coldFactor(composition, coldKey(980_101))
	foreign := coldFactor(composition, coldKey(980_102))
	if value == nil || foreign == nil {
		t.Fatal("factors")
	}
	declareRefFixtureMember(t, composition, value, 980_110)
	declareRefFixtureMember(t, composition, foreign, 980_120)
	if !composition.Seal() {
		t.Fatal("composition")
	}
	valueRef, valueOK := value.Ref(1)
	foreignRef, foreignOK := foreign.Ref(1)
	if !valueOK || !foreignOK {
		t.Fatal("refs")
	}
	role, valueSlot, foreignSlot := coldKey(980_105), coldKey(980_106), coldKey(980_107)
	build := NewSourceAssembly(composition)
	batch := build.state.batch
	scope, scopeOK := build.Scope()
	truth, truthOK := build.TrueExpr()
	site, siteOK := build.Site(coldKey(980_108), scope, truth, true)
	if !scopeOK || !truthOK || !siteOK || !build.Seal() {
		t.Fatal("activation base source")
	}
	assembly := newAssembly(composition, batch)
	point := admitPoint(assembly, site.value)
	base, baseOK := ActivationBaseAt(assembly, point)
	if !baseOK {
		t.Fatal("activation base capability")
	}
	binding, bound := activationPortBindingOf(role, base,
		activationPortReadOf(role, base, foreignSlot, foreign, foreignRef),
		activationPortReadOf(role, base, valueSlot, value, valueRef),
	)
	if !bound || binding.role != role.compositionKey() || binding.base != base || len(binding.reads) != 2 {
		t.Fatal("typed caller refs did not produce one multi-Factor import port")
	}
	if binding.reads[0].Role != valueSlot.compositionKey() || binding.reads[0].Surface.Factor != value.schema.semantic.compositionKey() || binding.reads[0].Surface.Form != equation.SurfaceReadExact || binding.reads[0].Surface.Local != 2 ||
		binding.reads[1].Role != foreignSlot.compositionKey() || binding.reads[1].Surface.Factor != foreign.schema.semantic.compositionKey() || binding.reads[1].Surface.Local != 2 {
		t.Fatal("collector did not canonicalize heterogeneous Factor slots")
	}
	if forged, accepted := activationPortBindingOf(role, base, activationPortReadOf(role, base, valueSlot, value, foreignRef)); accepted || forged.reads != nil {
		t.Fatal("foreign Factor ref crossed activation port")
	}
	if mismatched, accepted := activationPortBindingOf(role, base, activationPortReadOf(coldKey(980_109), base, valueSlot, value, valueRef)); accepted || mismatched.reads != nil {
		t.Fatal("collector accepted mismatched port role")
	}
}
