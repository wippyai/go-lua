package axis

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// forgedErasedSpec models the old RegisterErased bypass: it claims immutable
// retention for a mutable payload without passing through typed Spec validation.
type forgedErasedSpec struct{}

func (forgedErasedSpec) erasedSpecSeal() erasedSpecToken                 { return erasedSpecToken{} }
func (forgedErasedSpec) ID() string                                      { return "test.forged.mutable" }
func (forgedErasedSpec) BottomAny() any                                  { return []int(nil) }
func (forgedErasedSpec) TopAny() any                                     { return []int{1} }
func (forgedErasedSpec) IsTopAny(any) bool                               { return false }
func (forgedErasedSpec) EqualAny(any, any) bool                          { return false }
func (forgedErasedSpec) LessOrEqAny(any, any) bool                       { return false }
func (forgedErasedSpec) JoinAny(any, any) any                            { return []int(nil) }
func (forgedErasedSpec) HasMeet() bool                                   { return true }
func (forgedErasedSpec) MeetAny(any, any) any                            { return []int(nil) }
func (forgedErasedSpec) WidenAny(any, any) any                           { return []int(nil) }
func (forgedErasedSpec) WidenRankWidth() int                             { return 0 }
func (forgedErasedSpec) WidenRankAtAny(any, int) uint64                  { return 0 }
func (forgedErasedSpec) ReductionRankWidth() int                         { return 0 }
func (forgedErasedSpec) ReductionRankAtAny(any, int) uint64              { return 0 }
func (forgedErasedSpec) HashAny(any) uint64                              { return 0 }
func (forgedErasedSpec) RetentionMode() RetentionMode                    { return RetentionImmutable }
func (forgedErasedSpec) RetentionSafeAny(any) bool                       { return true }
func (forgedErasedSpec) CanonicalStatus() CanonicalStatus                { return CanonicalReady }
func (forgedErasedSpec) CanonicalCodecID() string                        { return "forged" }
func (forgedErasedSpec) CanonicalCodecVersion() uint64                   { return 1 }
func (forgedErasedSpec) CanonicalPendingReason() string                  { return "" }
func (forgedErasedSpec) EncodeCanonicalAny(*canonical.Writer, any) error { return nil }
func (forgedErasedSpec) CanonicalDecodeReady() bool                      { return false }
func (forgedErasedSpec) DecodeCanonicalAny(context.Context, *canonical.Reader) (any, error) {
	return nil, nil
}
func (forgedErasedSpec) BoundaryPolicy() BoundaryPolicy { return PortableIdentity }
func (forgedErasedSpec) ProjectBoundaryAny(v any) any   { return v }
func (forgedErasedSpec) ReducerHook() Reducer           { return nil }
func (forgedErasedSpec) ReducerReadsHook() []string     { return nil }
func (forgedErasedSpec) ReducerWritesHook() []string    { return nil }

// staleReadyErasedSpec models descriptor metadata that passed an older typed
// boundary but arrives at Registry without a usable schema version.
type staleReadyErasedSpec struct{ forgedErasedSpec }

func (staleReadyErasedSpec) erasedSpecSeal() erasedSpecToken { return erasedSpecToken{validated: true} }
func (staleReadyErasedSpec) CanonicalCodecVersion() uint64   { return 0 }

func TestRegisterErasedRejectsForgedMutableSpec(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterErased(forgedErasedSpec{}); err == nil {
		t.Fatal("RegisterErased accepted forged mutable spec without typed validation")
	}
}

func TestRegisterErasedRejectsStaleZeroCanonicalVersion(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterErased(staleReadyErasedSpec{}); err == nil {
		t.Fatal("RegisterErased accepted stale Ready metadata with zero codec version")
	}
}

func TestCanonicalDescriptorRejectsPendingEncoderAuthority(t *testing.T) {
	descriptor := CanonicalDescriptor[int]{
		status:        CanonicalPending,
		encode:        func(*canonical.Writer, int) error { return nil },
		pendingReason: "not portable",
	}
	if err := validateCanonicalDescriptor("test.pending.encoder", descriptor); err == nil {
		t.Fatal("pending canonical descriptor accepted an encoder")
	}
}
