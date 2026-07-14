package service

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestGeneratedRegistryMatchesPinnedStandardRetentionSchema(t *testing.T) {
	wantIDs := []string{
		"variantorigin", "identity", "runtimekind", "typewitness",
		"escape", "evidence", "assertion",
	}
	wantModes := []axis.RetentionMode{
		axis.RetentionImmutable, axis.RetentionImmutable, axis.RetentionImmutable,
		axis.RetentionValidated, axis.RetentionImmutable, axis.RetentionImmutable,
		axis.RetentionImmutable,
	}
	generated := checkerRegistry.SpecsView()
	reference := standard.Registry().SpecsView()
	if generated.Len() != len(wantIDs) || reference.Len() != len(wantIDs) {
		t.Fatalf("generated/standard axis counts = %d/%d, want %d", generated.Len(), reference.Len(), len(wantIDs))
	}
	for index, wantID := range wantIDs {
		gotGenerated, gotReference := generated.At(index), reference.At(index)
		if gotGenerated.ID() != wantID || gotReference.ID() != wantID {
			t.Fatalf("axis %d generated/standard IDs = %q/%q, want %q", index, gotGenerated.ID(), gotReference.ID(), wantID)
		}
		if gotGenerated.RetentionMode() != wantModes[index] || gotReference.RetentionMode() != wantModes[index] {
			t.Fatalf("axis %q generated/standard retention = %d/%d, want %d", wantID, gotGenerated.RetentionMode(), gotReference.RetentionMode(), wantModes[index])
		}
	}
	if got := presence.Spec().Erase().RetentionMode(); got != axis.RetentionImmutable {
		t.Fatalf("core presence retention = %d, want immutable", got)
	}
}
