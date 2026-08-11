package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
)

func TestActivationPortReadSlotsBindOnePointAndIgnoreOrder(t *testing.T) {
	value, pack := boundaryKey(230), boundaryKey(231)
	source, sealed := composition.Seal(composition.Candidate{Factors: []composition.Factor{{Key: value}, {Key: pack}}})
	if !sealed || source == nil {
		t.Fatal("read-slot source")
	}
	role, valueSlot, packSlot := boundaryKey(232), boundaryKey(233), boundaryKey(234)
	prototypeValue := Surface{Factor: value, Form: SurfaceReadExact, Local: 1}
	prototypePack := Surface{Factor: pack, Form: SurfaceReadExact, Local: 2}
	callerValue := Surface{Factor: value, Form: SurfaceReadExact, Local: 7}
	callerPack := Surface{Factor: pack, Form: SurfaceReadExact, Local: 9}
	plan := VariantPlan{data: &variantPlanData{
		source: source,
		ports:  map[composition.Key]PortMode{role: PortImport},
		portReads: map[composition.Key][]PortRead{role: {
			{Role: valueSlot, Surface: prototypeValue},
			{Role: packSlot, Surface: prototypePack},
		}},
	}}
	base := ambientPortPoints(t, EmptyScope())
	bindings := []PortBinding{{Role: role, Base: PointAt(0), Reads: []PortRead{
		{Role: packSlot, Surface: callerPack},
		{Role: valueSlot, Surface: callerValue},
	}}}
	ports, ambient, bound := sealPlanPortBindings(plan, bindings, base)
	if !bound || !sameScope(ambient, EmptyScope()) || len(ports) != 1 {
		t.Fatal("same-point multi-surface port did not bind")
	}
	port := ports[role]
	if len(port.reads) != 2 || port.reads[0].Role != valueSlot || port.reads[0].Surface != callerValue || port.reads[1].Role != packSlot || port.reads[1].Surface != callerPack {
		t.Fatal("read-slot binding retained declaration order")
	}

	row := RuleInstance{Reads: []ResolvedRead{
		{Index: 0, Surface: prototypeValue},
		{Index: 1, Surface: prototypePack},
		{Index: 2, Surface: Surface{Factor: value, Form: SurfaceReadExact, Local: 3}},
	}}
	substitutions, substitutionsOK := portReadSubstitutions(source, ports)
	template := sealedTemplate{source: source, ports: ports, substitutions: substitutions}
	if !substitutionsOK || !template.substitutePortReads(&row) || row.Reads[0].Surface != callerValue || row.Reads[1].Surface != callerPack || row.Reads[2].Surface.Local != 3 {
		t.Fatal("port substitution changed anything other than declared slots")
	}
	if weak, accepted := template.substituteWeakTargetCandidates(WeakTargetMapping{
		Surface:    Surface{Factor: value, Form: SurfaceWriteExact, Local: 1, Mode: TargetModeWeak},
		Candidates: []Surface{prototypeValue, callerValue},
	}); accepted || weak.Candidates != nil {
		t.Fatal("distinct prototype weak candidates collapsed into one caller surface")
	}

	wrongFactor := append([]PortRead(nil), bindings[0].Reads...)
	wrongFactor[0].Surface.Factor = value
	if ports, _, ok := sealPlanPortBindings(plan, []PortBinding{{Role: role, Base: PointAt(0), Reads: wrongFactor}}, base); ok || ports != nil {
		t.Fatal("port slot accepted a foreign Factor")
	}
}

func TestActivationTemplateAllowsDistinctRolesAtSharedEndpoint(t *testing.T) {
	source, sealed := composition.Seal(composition.Candidate{Factors: []composition.Factor{{Key: boundaryKey(234)}}})
	if !sealed || source == nil {
		t.Fatal("shared-endpoint source")
	}
	batch, _, _, _, _ := boundaryBatch(t, EmptyScope())
	points := ambientPortPoints(t, EmptyScope())
	point := points[PointAt(0)]
	left, right := boundaryKey(235), boundaryKey(236)
	prototype := templatePrototype{
		source: source,
		key:    boundaryKey(237),
		batch:  batch,
		value:  Template{FactorEdges: []FragmentFactorEdge{{Factor: boundaryKey(238), Provenance: boundaryKey(239)}}},
		ports:  map[composition.Key]PortMode{left: PortImport, right: PortExport},
	}
	bound, ok := prototype.bindPrototype(map[composition.Key]sealedPort{
		left:  {role: left, base: PointAt(0), point: point, mode: PortImport},
		right: {role: right, base: PointAt(0), point: point, mode: PortExport},
	}, EmptyScope())
	if !ok || len(bound.ports) != 2 || bound.ports[left].point.key != bound.ports[right].point.key {
		t.Fatal("distinct activation roles could not share one concrete endpoint")
	}
}
