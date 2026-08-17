package composite

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/analysis/schema/axis"
	callactivation "github.com/wippyai/go-lua/domain/call/activation"
	heapindex "github.com/wippyai/go-lua/domain/heap/index"
	staticdomain "github.com/wippyai/go-lua/domain/static"
)

// The receiver-to-root topology and the mounted activation catalog are
// derivations over several sealed factors at once, so neither is any one axis's
// authority to mount. They are derived by the mount phase itself, after every
// declared mount has sealed, and the laws below state that placement: the
// derivation reads only what the mount phase produced, it names the derivation
// that rejected, and no caller can supply either one.

// TestPostMountDerivationRunsOnlyOverAMountedRecord states the ordering. Run
// over the phase's own neutral input half - the record every mount receives,
// before any factor authority has sealed - the derivation has nothing to derive
// from and rejects at the derivation that could not open, publishing no record.
func TestPostMountDerivationRunsOnlyOverAMountedRecord(t *testing.T) {
	unmounted := LinkInputs{
		Source:          &link.Link{},
		Artifacts:       []axis.MountedArtifact{{}},
		StaticAuthority: &staticdomain.Authority{},
	}.neutral()
	derived, failure := unmounted.derive()
	if !failure.Available() || failure.Stage != MountStageTopology {
		t.Fatalf("the derivation admitted a record no axis had mounted: %v", failure)
	}
	if derived.topology != nil || derived.activation != nil || derived.Source != nil {
		t.Fatalf("a rejected derivation published a partially derived record")
	}
}

// TestPostMountDerivationNamesTheDerivationThatRejected states the evidence a
// derivation failure carries: the phase names which derivation refused, so the
// two are not collapsed into one anonymous post-mount verdict.
func TestPostMountDerivationNamesTheDerivationThatRejected(t *testing.T) {
	stages := map[MountStage]string{
		MountStageTopology:   "topology",
		MountStageActivation: "activation",
	}
	for stage, name := range stages {
		failure := MountFailure{Stage: stage}
		if !failure.Available() || failure.String() != name {
			t.Fatalf("derivation stage %d renders as %q, not %q", stage, failure.String(), name)
		}
		if failure.Axis != DiagnosticAxisUnknown {
			t.Fatalf("derivation stage %q blamed axis %q; no axis owns a derivation", name, failure.Axis)
		}
	}
}

// TestDerivedAuthoritiesAreNotCallerSupplied states the ownership cut. The two
// derived authorities live in the Link input record the binding transaction
// consumes, and the mount phase is the only writer: neither is an exported
// field, so no caller can hand the composition a topology or an activation
// catalog it did not derive from the mounts it sealed.
func TestDerivedAuthoritiesAreNotCallerSupplied(t *testing.T) {
	derivations := map[reflect.Type]string{
		reflect.TypeOf((*heapindex.Topology)(nil)):                "the receiver-to-root topology",
		reflect.TypeOf((*callactivation.TargetBatchCatalog)(nil)): "the mounted activation catalog",
	}
	record := reflect.TypeOf(LinkInputs{})
	held := make(map[reflect.Type]struct{}, len(derivations))
	for index := 0; index < record.NumField(); index++ {
		field := record.Field(index)
		name, derived := derivations[field.Type]
		if !derived {
			continue
		}
		held[field.Type] = struct{}{}
		if field.PkgPath == "" {
			t.Fatalf("%s is the exported field %q; the mount phase derives it and no caller supplies it", name, field.Name)
		}
	}
	if len(held) != len(derivations) {
		t.Fatalf("the Link input record holds %d of the %d post-mount derivations", len(held), len(derivations))
	}
}
