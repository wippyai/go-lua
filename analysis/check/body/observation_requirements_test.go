package body

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPreparedObservationRequirementsMatchCurrentConsumerPlan(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{"is_str", `function is_str(value: any): boolean return type(value) == "string" and (value :: string) ~= "" end`},
		{"trim", `function trim(value: any): string if type(value) ~= "string" then return "" end return (value :: string):gsub("^%s*(.-)%s*$", "%1") end`},
		{"plain_normal", `function plain(value: string): string return value end`},
		{"recursive", `function recurse(value: number): number if value <= 0 then return 0 end return recurse(value - 1) end`},
		{"abnormal", `function checked(value: any): any if value == nil then error("missing") end return value end`},
		{"unconditional_no_return", `function stop(): never error("stop") end`},
		{"terminal", `function terminal(value: string): string local out = value return out end`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fn := parseFunction(t, test.source)
			prepared, err := PrepareFunction(fn, Config{
				Registry: standard.Registry(), TypeValues: typevalue.NewCache(),
				Signatures: signaturelookup.Source{IncludeStdlib: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			requirements, sealed := prepared.operationPlan.ObservationRequirements()
			if !sealed || requirements.SchemaID() == (operationplan.ObservationSchemaID{}) || requirements.ConsumerInventoryID() == (operationplan.ObservationConsumerInventoryID{}) {
				t.Fatal("prepared observation requirements are not sealed and schema-bound")
			}
			assertObservationRequirementCensus(t, prepared, requirements)
		})
	}
}

func assertObservationRequirementCensus(t *testing.T, prepared *Static, requirements operationplan.ObservationRequirements) {
	t.Helper()
	legacy := compileObservationPlan(prepared.cfg.Graph, prepared.facts)
	pointSet := make(map[cfg.Point]struct{})
	boundarySet := make(map[cfg.Point]struct{})
	edgeSet := make(map[observationEdge]struct{})
	anchors := make(map[string]struct{})
	for _, requirement := range requirements.Entries(true) {
		switch requirement.Stage() {
		case operationplan.RequirementPoint:
			pointSet[requirement.Point()] = struct{}{}
		case operationplan.RequirementBoundary:
			boundarySet[requirement.Point()] = struct{}{}
		case operationplan.RequirementEdge:
			to, ok := requirement.EdgeTarget()
			if !ok {
				t.Fatal("edge requirement has no target")
			}
			edgeSet[observationEdge{from: requirement.Point(), to: to}] = struct{}{}
		case operationplan.RequirementObservation, operationplan.RequirementRoute:
			anchor, ok := requirement.Anchor()
			if !ok {
				t.Fatal("observation requirement has no durable anchor")
			}
			key := fmt.Sprintf("%v:%s", anchor, requirement.Projection())
			if _, duplicate := anchors[key]; duplicate {
				t.Fatalf("duplicate observation anchor %s", key)
			}
			anchors[key] = struct{}{}
		}
	}
	wantAnchors := make(map[string]struct{})
	for _, point := range cfg.RPOReadOnly(prepared.cfg.Graph) {
		if _, ok := prepared.facts.RootAssignment(point); ok {
			anchor, durable := prepared.operationPlan.AssignmentObservationAnchor(point)
			if !durable {
				t.Fatalf("assignment point %d has no durable anchor", point)
			}
			wantAnchors[fmt.Sprintf("%v:%s", anchor, operationplan.ProjectionObservationAssignment)] = struct{}{}
		}
		if site, ok := prepared.facts.CallSiteView(point); ok {
			for index := 0; index < site.ArgumentSourceCount(); index++ {
				anchor, durable := prepared.operationPlan.CallArgumentObservationAnchor(point, uint32(index))
				if !durable {
					t.Fatalf("call argument %d:%d has no durable anchor", point, index)
				}
				wantAnchors[fmt.Sprintf("%v:%s", anchor, operationplan.ProjectionObservationCallArgument)] = struct{}{}
			}
			invocation, durable := prepared.operationPlan.CallInvocationObservationAnchor(point)
			if !durable {
				t.Fatalf("call point %d has no invocation route", point)
			}
			wantAnchors[fmt.Sprintf("%v:%s", invocation, operationplan.ProjectionObservationCallInvocation)] = struct{}{}
			for index := 0; index < site.ResultTargetCount(); index++ {
				target, found := site.ResultTargetAt(index)
				if !found || target.Kind() != factflow.CallResultTargetLocalAssignment || target.TargetSymbol() == 0 {
					continue
				}
				anchor, durable := prepared.operationPlan.CallResultObservationAnchor(point, uint32(index))
				if !durable {
					t.Fatalf("call result %d:%d has no durable anchor", point, index)
				}
				projection := operationplan.ProjectionObservationCallResult
				wantAnchors[fmt.Sprintf("%v:%s", anchor, projection)] = struct{}{}
			}
		}
	}
	if len(anchors) != len(wantAnchors) {
		t.Fatalf("observation/route requirements=%v, want %v", anchors, wantAnchors)
	}
	for key := range wantAnchors {
		if _, ok := anchors[key]; !ok {
			t.Fatalf("missing observation/route requirement %s", key)
		}
	}
	wantPoints := append([]cfg.Point(nil), cfg.RPOReadOnly(prepared.cfg.Graph)...)
	sort.Slice(wantPoints, func(i, j int) bool { return wantPoints[i] < wantPoints[j] })
	gotPoints := sortedObservationPoints(pointSet)
	if !equalObservationPoints(gotPoints, wantPoints) {
		t.Fatalf("point requirements=%v, want reachable RPO=%v", gotPoints, wantPoints)
	}
	wantBoundary := append([]cfg.Point(nil), legacy.boundaryPoints...)
	sort.Slice(wantBoundary, func(i, j int) bool { return wantBoundary[i] < wantBoundary[j] })
	if got, want := sortedObservationPoints(boundarySet), wantBoundary; !equalObservationPoints(got, want) {
		t.Fatalf("boundary requirements=%v, want legacy=%v", got, want)
	}
	gotEdges := make([]observationEdge, 0, len(edgeSet))
	for edge := range edgeSet {
		gotEdges = append(gotEdges, edge)
	}
	sort.Slice(gotEdges, func(i, j int) bool {
		if gotEdges[i].from != gotEdges[j].from {
			return gotEdges[i].from < gotEdges[j].from
		}
		return gotEdges[i].to < gotEdges[j].to
	})
	wantEdges := append([]observationEdge(nil), legacy.edgeReachability...)
	sort.Slice(wantEdges, func(i, j int) bool {
		if wantEdges[i].from != wantEdges[j].from {
			return wantEdges[i].from < wantEdges[j].from
		}
		return wantEdges[i].to < wantEdges[j].to
	})
	if len(gotEdges) != len(wantEdges) {
		t.Fatalf("edge requirements=%v, want legacy=%v", gotEdges, legacy.edgeReachability)
	}
	for i := range gotEdges {
		if gotEdges[i] != wantEdges[i] {
			t.Fatalf("edge requirement[%d]=%v, want %v", i, gotEdges[i], wantEdges[i])
		}
	}
	nodeSet := make(map[cfg.Point]struct{}, len(boundarySet)+len(edgeSet))
	for point := range boundarySet {
		nodeSet[point] = struct{}{}
	}
	for edge := range edgeSet {
		nodeSet[edge.from] = struct{}{}
	}
	wantNodes := append([]cfg.Point(nil), legacy.nodePoints...)
	sort.Slice(wantNodes, func(i, j int) bool { return wantNodes[i] < wantNodes[j] })
	if got, want := sortedObservationPoints(nodeSet), wantNodes; !equalObservationPoints(got, want) {
		t.Fatalf("node requirements=%v, want legacy=%v", got, want)
	}
}

func sortedObservationPoints(set map[cfg.Point]struct{}) []cfg.Point {
	out := make([]cfg.Point, 0, len(set))
	for point := range set {
		out = append(out, point)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equalObservationPoints(a, b []cfg.Point) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
