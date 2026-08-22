package formal

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// A formal parameter ordinal is owned by the callee declaration and is
// independent of any one call site's arity: Lua binds every parameter beyond
// the supplied actual prefix to nil. An ownership row naming an unsupplied
// ordinal therefore selects nothing at this mounted call. That reading is a
// proof, not a widening, because a closed mounted actual list authenticates
// the absence; nil holds no allocation, so the row contributes no demand and
// the selector is never malformed.
func TestFormalSelectorShortArityCallSelectsNothing(t *testing.T) {
	tests := []struct {
		name    string
		spec    vocabulary.FormalEffectSpec
		actuals int
		start   int
		end     int
		owns    bool
	}{
		{name: "store-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 2, HasInto: true, Into: 0}, actuals: 2, owns: true},
		{name: "retain-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectRetain, Param: 3}, actuals: 1, owns: true},
		{name: "send-param-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendParam, Param: 4}, actuals: 4, owns: true},
		{name: "export-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectExport, Param: 1}, actuals: 0, owns: true},
		{name: "opaque-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectOpaque, Param: 6}, actuals: 3, owns: true},
		{name: "borrow-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectBorrow, Param: 2}, actuals: 2},
		{name: "freeze-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectFreeze, Param: 5}, actuals: 0},
		{name: "trailing-selector-at-zero-actuals", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectRetain, Param: -1}, actuals: 0, owns: true},
		{name: "suffix-past-prefix", spec: vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 9}, actuals: 3, start: 3, end: 3, owns: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := resolveFormalSelectorRange(test.spec, test.actuals, false)
			if !got.valid || got.unknown || got.owns != test.owns || got.start != test.start || got.end != test.end {
				t.Fatalf("selector = %#v, want valid empty [%d,%d) owns=%t unknown=false", got, test.start, test.end, test.owns)
			}
		})
	}
}

// The same unsupplied ordinal stays conservative behind an authenticated Pack
// runtime tail: that boundary is the one authority that can place an actual
// there, so the selector widens rather than emptying.
func TestFormalSelectorShortArityRuntimeTailStaysUnknown(t *testing.T) {
	store := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectStore, Param: 2}, 2, true)
	if !store.valid || !store.unknown || !store.owns {
		t.Fatalf("open short-arity store selector = %#v, want conservative unknown", store)
	}
	suffix := resolveFormalSelectorRange(vocabulary.FormalEffectSpec{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 9}, 3, true)
	if !suffix.valid || !suffix.unknown || !suffix.owns || suffix.start != 3 || suffix.end != 3 {
		t.Fatalf("open short-arity suffix selector = %#v, want conservative unknown", suffix)
	}
}

// A negative authored ordinal other than the -1 trailing spelling remains the
// only malformed selector: it names no parameter at all.
func TestFormalSelectorNegativeOrdinalStaysMalformed(t *testing.T) {
	for _, spec := range []vocabulary.FormalEffectSpec{
		{Kind: vocabulary.FormalEffectStore, Param: -2},
		{Kind: vocabulary.FormalEffectBorrow, Param: -3},
		{Kind: vocabulary.FormalEffectSendSuffix, FromParam: -1},
	} {
		if got := resolveFormalSelectorRange(spec, 3, false); got.valid {
			t.Fatalf("negative authored ordinal %#v selected %#v, want invalid", spec, got)
		}
	}
}
