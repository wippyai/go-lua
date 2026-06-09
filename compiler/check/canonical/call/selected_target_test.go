package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/types/flow"
)

func TestSelectedTargetsFiniteClosureDominatesDirect(t *testing.T) {
	t.Parallel()

	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 9, ParentHash: 1}, flow.CaptureCellsDomain.Bottom(), nil)
	targets := NewTargetSet(
		[]summary.FuncRef{{GraphID: 1}, {GraphID: 2}},
		true,
		[]flow.ClosureRef{closure},
		true,
	)

	selected := targets.Select().Targets()
	if len(selected) != 1 {
		t.Fatalf("selected targets len = %d, want 1", len(selected))
	}
	if !selected[0].IsClosure() {
		t.Fatal("finite closure target was not selected as a closure")
	}
	if got := selected[0].Ref(); got.GraphID != 9 || got.ParentHash != 1 {
		t.Fatalf("selected closure ref = %#v, want graph 9/parent 1", got)
	}
	if closure, ok := selected[0].Closure(); !ok || closure.Ref.GraphID != 9 {
		t.Fatalf("Closure() = %#v/%v, want graph 9/true", closure, ok)
	}
}

func TestSelectedTargetsFallsBackToDirectWhenClosureTopHasNoFiniteTargets(t *testing.T) {
	t.Parallel()

	targets := NewTargetSet(
		[]summary.FuncRef{{GraphID: 2, ParentHash: 3}, {GraphID: 1}},
		true,
		nil,
		true,
	)

	selected := targets.Select().Targets()
	if len(selected) != 2 {
		t.Fatalf("selected targets len = %d, want 2", len(selected))
	}
	if selected[0].IsClosure() || selected[1].IsClosure() {
		t.Fatal("direct fallback targets should not be closure targets")
	}
	if got := selected[0].Ref(); got.GraphID != 1 || got.ParentHash != 0 {
		t.Fatalf("selected[0] = %#v, want graph 1", got)
	}
	if got := selected[1].Ref(); got.GraphID != 2 || got.ParentHash != 3 {
		t.Fatalf("selected[1] = %#v, want graph 2/parent 3", got)
	}
}

func TestTargetSelectionFallbackClassification(t *testing.T) {
	t.Parallel()

	direct := []summary.FuncRef{{GraphID: 1}}
	closure := flow.ClosureRefOf(flow.FunctionRef{GraphID: 2}, flow.CaptureCellsDomain.Bottom(), nil)

	cases := []struct {
		name                   string
		targets                TargetSet
		hasTargets             bool
		hasClosureTargets      bool
		blocksFallback         bool
		allowsCallbackFallback bool
	}{
		{
			name:                   "finite closure",
			targets:                NewTargetSet(direct, true, []flow.ClosureRef{closure}, true),
			hasTargets:             true,
			hasClosureTargets:      true,
			blocksFallback:         false,
			allowsCallbackFallback: false,
		},
		{
			name:                   "closure top with direct fallback",
			targets:                NewTargetSet(direct, true, nil, true),
			hasTargets:             true,
			hasClosureTargets:      false,
			blocksFallback:         false,
			allowsCallbackFallback: true,
		},
		{
			name:                   "closure top without direct fallback",
			targets:                NewTargetSet(nil, false, nil, true),
			hasTargets:             false,
			hasClosureTargets:      false,
			blocksFallback:         true,
			allowsCallbackFallback: false,
		},
		{
			name:                   "direct only",
			targets:                NewTargetSet(direct, true, nil, false),
			hasTargets:             true,
			hasClosureTargets:      false,
			blocksFallback:         false,
			allowsCallbackFallback: true,
		},
		{
			name:                   "no target evidence",
			targets:                NewTargetSet(nil, false, nil, false),
			hasTargets:             false,
			hasClosureTargets:      false,
			blocksFallback:         false,
			allowsCallbackFallback: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			selection := tc.targets.Select()
			if got := selection.HasTargets(); got != tc.hasTargets {
				t.Fatalf("HasTargets = %v, want %v", got, tc.hasTargets)
			}
			if got := selection.HasClosureTargets(); got != tc.hasClosureTargets {
				t.Fatalf("HasClosureTargets = %v, want %v", got, tc.hasClosureTargets)
			}
			if got := selection.BlocksTypeFallback(); got != tc.blocksFallback {
				t.Fatalf("BlocksTypeFallback = %v, want %v", got, tc.blocksFallback)
			}
			if got := selection.AllowsCallbackFallback(); got != tc.allowsCallbackFallback {
				t.Fatalf("AllowsCallbackFallback = %v, want %v", got, tc.allowsCallbackFallback)
			}
		})
	}
}

func TestSelectionNeverReturnsRequiresAllSelectedTargets(t *testing.T) {
	t.Parallel()

	noReturn := map[summary.FuncRef]bool{
		{GraphID: 1}: true,
		{GraphID: 2}: true,
	}

	cases := []struct {
		name    string
		targets TargetSet
		want    bool
	}{
		{
			name:    "all direct targets no-return",
			targets: NewTargetSet([]summary.FuncRef{{GraphID: 1}, {GraphID: 2}}, true, nil, false),
			want:    true,
		},
		{
			name:    "mixed direct targets do not prune",
			targets: NewTargetSet([]summary.FuncRef{{GraphID: 1}, {GraphID: 3}}, true, nil, false),
			want:    false,
		},
		{
			name:    "empty target set does not prune",
			targets: NewTargetSet(nil, false, nil, false),
			want:    false,
		},
		{
			name:    "closure-authoritative miss does not prune",
			targets: NewTargetSet(nil, false, nil, true),
			want:    false,
		},
		{
			name: "finite closure target uses closure ref",
			targets: NewTargetSet(
				[]summary.FuncRef{{GraphID: 3}},
				true,
				[]flow.ClosureRef{flow.ClosureRefOf(flow.FunctionRef{GraphID: 1}, flow.CaptureCellsDomain.Bottom(), nil)},
				true,
			),
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := selectionNeverReturns(tc.targets.Select(), func(ref summary.FuncRef) bool {
				return noReturn[ref]
			})
			if got != tc.want {
				t.Fatalf("selectionNeverReturns = %v, want %v", got, tc.want)
			}
		})
	}
}
