package evidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
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

type evidenceFacts struct {
	current        map[constraint.PathKey]flow.TypedValue
	post           map[constraint.PathKey]flow.TypedValue
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
