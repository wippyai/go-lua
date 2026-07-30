package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func TestCallFrameInterningUsesCompleteBoundaryAndOccurrence(t *testing.T) {
	arena := NewArena(standard.Registry())
	shape := Shape{Params: 1}
	root := Root{Kind: RootParam}
	value, path := arena.Root(root), arena.Path(root)
	base := arena.callFrame(CellRef{Function: 20}, 7, 1, shape, []ValueTerm{value}, []PathTerm{path}, 2)
	same := arena.callFrame(CellRef{Function: 20}, 7, 1, shape, []ValueTerm{value}, []PathTerm{path}, 2)
	otherOccurrence := arena.callFrame(CellRef{Function: 20}, 7, 2, shape, []ValueTerm{value}, []PathTerm{path}, 2)
	otherWidth := arena.callFrame(CellRef{Function: 20}, 7, 1, shape, []ValueTerm{value}, []PathTerm{path}, 1)
	if base == 0 || base != same || base == otherOccurrence || base == otherWidth {
		t.Fatalf("call-frame interning identity = base:%d same:%d occurrence:%d width:%d", base, same, otherOccurrence, otherWidth)
	}
}

func TestDirectCallWithIgnoredResultsStillOwnsWorldFrame(t *testing.T) {
	targets, err := exactDirectCallTargets(factflow.NewCallSite(factflow.CallSiteConfig{
		Point: 9, HasPoint: true, Final: true, Expanded: true,
	}).View())
	if err != nil || len(targets) != 0 {
		t.Fatalf("ignored-result target inventory = %#v, err=%v", targets, err)
	}
	arena := NewArena(standard.Registry())
	frame := arena.callFrame(CellRef{Function: 77}, 9, 0, Shape{}, nil, nil, 0)
	if frame == 0 || arena.callFrames[frame].resultCount != 0 {
		t.Fatalf("ignored-result call frame = %d/%#v", frame, arena.callFrames[frame])
	}
}

func TestDirectCallReturnTargetUsesExactTypedResultRoot(t *testing.T) {
	site := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextReturnSource,
		Point:   9, HasPoint: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 2, 0, 0, pathdom.Path{}),
		},
	}).View()
	targets, err := exactDirectCallTargets(site)
	if err != nil || len(targets) != 1 || targets[0].symbol != 0 || targets[0].slot != 0 {
		t.Fatalf("return target inventory = %#v, err=%v", targets, err)
	}

	wrongContext := factflow.NewCallSite(factflow.CallSiteConfig{
		Context: factflow.CallSiteContextAssignmentSource,
		Point:   9, HasPoint: true,
		ResultTargets: []factflow.CallResultTarget{
			factflow.NewCallResultTarget(factflow.CallResultTargetReturn, 2, 0, 0, pathdom.Path{}),
		},
	}).View()
	if _, err := exactDirectCallTargets(wrongContext); err == nil {
		t.Fatal("return target without return-source context was admitted")
	}
}

func TestCallFrameScopesCalleeResultsAndHeapTemplatesAsExistentials(t *testing.T) {
	arena := NewArena(standard.Registry())
	shape := Shape{Params: 1, Results: 2, HeapTemplates: 3}
	input := arena.Constant(product.Top())
	first := arena.callFrame(CellRef{Function: 80}, 11, 0, shape, []ValueTerm{input}, []PathTerm{0}, 1)
	second := arena.callFrame(CellRef{Function: 80}, 19, 0, shape, []ValueTerm{input}, []PathTerm{0}, 1)
	if first == 0 || second == 0 || first == second {
		t.Fatalf("lexically scoped existential frames = %d/%d", first, second)
	}
	for _, frame := range []callFrameTerm{first, second} {
		node := arena.callFrames[frame]
		if len(node.values) != shape.InputCount() || node.shape.ExistentialCount() != 5 {
			t.Fatalf("frame %d caller inputs/existentials = %d/%d", frame, len(node.values), node.shape.ExistentialCount())
		}
	}
}
