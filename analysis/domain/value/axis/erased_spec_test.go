package axis

import "testing"

// forgedErasedSpec models the old RegisterErased bypass: it claims immutable
// retention for a mutable payload without passing through typed Spec validation.
type forgedErasedSpec struct{}

func (forgedErasedSpec) erasedSpecSeal() erasedSpecToken { return erasedSpecToken{} }
func (forgedErasedSpec) ID() string                      { return "test.forged.mutable" }
func (forgedErasedSpec) BottomAny() any                  { return []int(nil) }
func (forgedErasedSpec) TopAny() any                     { return []int{1} }
func (forgedErasedSpec) IsTopAny(any) bool               { return false }
func (forgedErasedSpec) EqualAny(any, any) bool          { return false }
func (forgedErasedSpec) LessOrEqAny(any, any) bool       { return false }
func (forgedErasedSpec) JoinAny(any, any) any            { return []int(nil) }
func (forgedErasedSpec) HasMeet() bool                   { return true }
func (forgedErasedSpec) MeetAny(any, any) any            { return []int(nil) }
func (forgedErasedSpec) WidenAny(any, any) any           { return []int(nil) }
func (forgedErasedSpec) HashAny(any) uint64              { return 0 }
func (forgedErasedSpec) RetentionMode() RetentionMode    { return RetentionImmutable }
func (forgedErasedSpec) RetentionSafeAny(any) bool       { return true }
func (forgedErasedSpec) BoundaryPolicy() BoundaryPolicy  { return PortableIdentity }
func (forgedErasedSpec) ProjectBoundaryAny(v any) any    { return v }
func (forgedErasedSpec) ReducerHook() Reducer            { return nil }
func (forgedErasedSpec) ReducerReadsHook() []string      { return nil }

func TestRegisterErasedRejectsForgedMutableSpec(t *testing.T) {
	reg := NewRegistry()
	if err := reg.RegisterErased(forgedErasedSpec{}); err == nil {
		t.Fatal("RegisterErased accepted forged mutable spec without typed validation")
	}
}
