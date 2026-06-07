package evidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDirectPathCandidateUsesRequestedView(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(7), "record").Field("id")
	facts := evidenceFacts{
		current: map[constraint.PathKey]flow.TypedValue{
			path.Key(): {Type: typ.Number, State: flow.StateResolved},
		},
		post: map[constraint.PathKey]flow.TypedValue{
			path.Key(): {Type: typ.String, State: flow.StateResolved},
		},
	}
	projection := New(Config{Facts: facts})

	current := projection.DirectPathCandidate(cfg.Point(10), path, flow.PathReadCurrent)
	if !current.OK || current.Source != flow.PathObservationDirectPath || !typ.TypeEquals(current.Type, typ.Number) {
		t.Fatalf("current direct candidate = %#v, want number direct path", current)
	}

	post := projection.DirectPathCandidate(cfg.Point(10), path, flow.PathReadPost)
	if !post.OK || post.Source != flow.PathObservationDirectPath || !typ.TypeEquals(post.Type, typ.String) {
		t.Fatalf("post direct candidate = %#v, want string direct path", post)
	}
}

func TestIndexReadFlowAdaptsPointFacts(t *testing.T) {
	table := constraint.NewPath(cfg.SymbolID(11), "records")
	key := constraint.NewPath(cfg.SymbolID(12), "id")
	member := table.Field("items")
	facts := evidenceFacts{
		hasKeyOf:       true,
		lengthPath:     member,
		lengthLower:    3,
		mapReadback:    typ.String,
		indexAdmission: typ.Number,
	}
	flowProof := New(Config{Facts: facts}).IndexReadFlow()
	if flowProof == nil {
		t.Fatal("IndexReadFlow returned nil")
	}
	if !flowProof.HasKeyOf(cfg.Point(20), table, key) {
		t.Fatal("IndexReadFlow did not expose key-presence proof")
	}
	if lower, _, ok := flowProof.LengthBoundsAt(cfg.Point(20), member); !ok || lower != 3 {
		t.Fatalf("LengthBoundsAt = %d/%v, want 3/true", lower, ok)
	}
	readback, ok := flowProof.(interface {
		MapReadback(flow.IndexWriteReadQuery) (typ.Type, bool)
	})
	if !ok {
		t.Fatal("IndexReadFlow did not expose map readback")
	}
	got, ok := readback.MapReadback(flow.IndexWriteReadQuery{Point: cfg.Point(20)})
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("MapReadback = %v/%v, want string/true", got, ok)
	}
}

func TestProjectionExposesAuxiliaryObservationEvidence(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(21), "rows").Field("items")
	call := &ast.FuncCallExpr{}
	contract := paramevidence.ParamContractDomain.Top()
	route := flow.ProvenanceRoute{Source: constraint.NewPath(cfg.SymbolID(22), "source")}
	facts := evidenceFacts{
		effective:    flow.TypedValue{Type: typ.Number, State: flow.StateResolved},
		postSymbol:   flow.TypedValue{Type: typ.String, State: flow.StateResolved},
		contracts:    paramevidence.Contracts{1: contract},
		routePath:    path,
		routes:       []flow.ProvenanceRoute{route},
		appendRoutes: []flow.ProvenanceRoute{route},
		callReturns:  []typ.Type{typ.Boolean},
	}
	projection := New(Config{Facts: facts})

	if got := projection.EffectiveTypeAt(cfg.Point(1), cfg.SymbolID(21), false); !typ.TypeEquals(got.Type, typ.Number) {
		t.Fatalf("EffectiveTypeAt current = %v, want number", got.Type)
	}
	if got := projection.EffectiveTypeAt(cfg.Point(1), cfg.SymbolID(21), true); !typ.TypeEquals(got.Type, typ.String) {
		t.Fatalf("EffectiveTypeAt post = %v, want string", got.Type)
	}
	if got := projection.BodyContracts()[1]; !paramevidence.ParamContractDomain.Equal(got, contract) {
		t.Fatalf("BodyContracts[1] = %#v, want contract", got)
	}
	if got := projection.ProvenanceRoutesAt(cfg.Point(1), path); len(got) != 1 || !got[0].Source.Equal(route.Source) {
		t.Fatalf("ProvenanceRoutesAt = %#v, want source route", got)
	}
	if got := projection.AppendElementFieldSourceRoutesAt(cfg.Point(1), flow.AppendElementFieldRouteQuery{ArrayPath: path}); len(got) != 1 || !got[0].Source.Equal(route.Source) {
		t.Fatalf("AppendElementFieldSourceRoutesAt = %#v, want source route", got)
	}
	if got, ok := projection.CallReturnTypesAt(cfg.Point(1), call, typ.Unknown); !ok || len(got) != 1 || !typ.TypeEquals(got[0], typ.Boolean) {
		t.Fatalf("CallReturnTypesAt = %v/%v, want boolean/true", got, ok)
	}
}

type evidenceFacts struct {
	current        map[constraint.PathKey]flow.TypedValue
	post           map[constraint.PathKey]flow.TypedValue
	effective      flow.TypedValue
	postSymbol     flow.TypedValue
	contracts      paramevidence.Contracts
	routePath      constraint.Path
	routes         []flow.ProvenanceRoute
	appendRoutes   []flow.ProvenanceRoute
	callReturns    []typ.Type
	hasKeyOf       bool
	lengthPath     constraint.Path
	lengthLower    int64
	mapReadback    typ.Type
	indexAdmission typ.Type
}

func (f evidenceFacts) DeclaredAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f evidenceFacts) RefinedAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f evidenceFacts) EffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	if f.effective.Type != nil {
		return f.effective
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f evidenceFacts) IsAnnotated(cfg.SymbolID) bool { return false }

func (f evidenceFacts) RefinedPathAt(_ cfg.Point, path constraint.Path) flow.TypedValue {
	if f.current == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return f.current[path.Key()]
}

func (f evidenceFacts) PostRefinedPathAt(_ cfg.Point, path constraint.Path) flow.TypedValue {
	if f.post == nil {
		return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
	}
	return f.post[path.Key()]
}

func (f evidenceFacts) PostEffectiveTypeAt(cfg.Point, cfg.SymbolID) flow.TypedValue {
	if f.postSymbol.Type != nil {
		return f.postSymbol
	}
	return flow.TypedValue{Type: typ.Unknown, State: flow.StateUnknown}
}

func (f evidenceFacts) BodyContracts() paramevidence.Contracts {
	return f.contracts
}

func (f evidenceFacts) ProvenanceRoutesAt(_ cfg.Point, path constraint.Path) []flow.ProvenanceRoute {
	if !f.routePath.Equal(path) {
		return nil
	}
	return f.routes
}

func (f evidenceFacts) AppendElementFieldSourceRoutesAt(cfg.Point, flow.AppendElementFieldRouteQuery) []flow.ProvenanceRoute {
	return f.appendRoutes
}

func (f evidenceFacts) CallReturnTypesAt(cfg.Point, *ast.FuncCallExpr, typ.Type) ([]typ.Type, bool) {
	return f.callReturns, len(f.callReturns) > 0
}

func (f evidenceFacts) HasKeyOf(cfg.Point, constraint.Path, constraint.Path) bool {
	return f.hasKeyOf
}

func (f evidenceFacts) LengthLowerBoundAt(cfg.Point, cfg.SymbolID) (int64, bool) {
	return 0, false
}

func (f evidenceFacts) LengthLowerBoundForPathAt(_ cfg.Point, path constraint.Path) (int64, bool) {
	if f.lengthPath.Equal(path) {
		return f.lengthLower, true
	}
	return 0, false
}

func (f evidenceFacts) IndexWriteAdmission(flow.IndexWriteReadQuery) (typ.Type, bool) {
	return f.indexAdmission, f.indexAdmission != nil
}

func (f evidenceFacts) MapReadback(flow.IndexWriteReadQuery) (typ.Type, bool) {
	return f.mapReadback, f.mapReadback != nil
}
