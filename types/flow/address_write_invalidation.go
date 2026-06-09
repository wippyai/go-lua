package flow

import (
	"github.com/wippyai/go-lua/types/access"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// AddressWriteInvalidation is the reduced-product write footprint for facts
// whose truth depends on a stable storage address.
type AddressWriteInvalidation struct {
	Write                  StableAddress
	PresentElementWrite    bool
	PresentElementArray    StableAddress
	HasPresentElementArray bool
	PresentElementMember   []constraint.Segment
	Written                product.AbstractValue
}

// AddressWritePathInvalidation is the path-shaped write footprint accepted by
// producer layers. Flow owns the path-to-address normalization before applying
// the address-indexed fact laws below.
type AddressWritePathInvalidation struct {
	WritePath                  constraint.Path
	PresentElementWrite        bool
	PresentElementArrayPath    constraint.Path
	HasPresentElementArrayPath bool
	PresentElementMember       []constraint.Segment
	Written                    product.AbstractValue
}

// AccessMutation is the point-state transaction for facts whose truth is tied
// to one normalized source access write. It keeps transfer from routing the same
// write through separate path projections for each fact family.
type AccessMutation struct {
	Footprint     access.WriteFootprint
	StaticMembers bool
	Conditions    bool
	AddressFacts  bool
}

// ApplyAccessMutation applies all selected point-state consequences of one
// normalized access write. Flow owns the path-to-address lowering and the laws
// for each affected fact family.
func ApplyAccessMutation(out *PointState, mutation AccessMutation) bool {
	if out == nil || mutation.Footprint.WritePath.IsEmpty() {
		return false
	}
	changed := false
	if mutation.StaticMembers {
		if mutation.Footprint.HasExactWritePath {
			changed = InvalidateStaticMemberWritePath(out, mutation.Footprint.ExactWritePath) || changed
		}
		changed = KillStaticMemberSubtreePath(out, mutation.Footprint.WritePath) || changed
	}
	if mutation.Conditions {
		changed = ForgetConditionAffectedByWrite(out, mutation.Footprint.WritePath) || changed
	}
	if mutation.AddressFacts {
		changed = ApplyAccessWriteFootprint(out, mutation.Footprint) || changed
	}
	return changed
}

func ApplyAddressWritePathInvalidation(out *PointState, proof AddressWritePathInvalidation) bool {
	write, ok := StableAddressOfPath(proof.WritePath)
	if !ok {
		return false
	}
	effect := AddressWriteInvalidation{
		Write:               write,
		PresentElementWrite: proof.PresentElementWrite,
		Written:             proof.Written,
	}
	if len(proof.PresentElementMember) > 0 {
		if proof.HasPresentElementArrayPath {
			if array, ok := StableAddressOfPath(proof.PresentElementArrayPath); ok {
				effect.PresentElementArray = array
				effect.HasPresentElementArray = true
				effect.PresentElementMember = proof.PresentElementMember
			}
		} else {
			effect.PresentElementMember = proof.PresentElementMember
		}
	}
	return ApplyAddressWriteInvalidation(out, effect)
}

// ApplyAccessWriteFootprint applies a normalized access write footprint.
func ApplyAccessWriteFootprint(out *PointState, footprint access.WriteFootprint) bool {
	return ApplyAddressWritePathInvalidation(out, AddressWritePathInvalidation{
		WritePath:                  footprint.WritePath,
		PresentElementWrite:        footprint.PresentElementWrite,
		PresentElementArrayPath:    footprint.PresentElementArrayPath,
		HasPresentElementArrayPath: footprint.HasPresentElementArrayPath,
		PresentElementMember:       footprint.PresentElementMember,
		Written:                    footprint.Written,
	})
}

// ApplyAddressWriteInvalidation applies the shared write-kill law for address
// indexed point facts. StaticMembers is intentionally excluded from this generic
// reducer because it has a dedicated prefix-bottoming rule; see
// InvalidateStaticMemberWritePath.
func ApplyAddressWriteInvalidation(out *PointState, effect AddressWriteInvalidation) bool {
	if out == nil || effect.Write.Key() == "" {
		return false
	}
	beforeKeyPresence := out.KeyPresence
	beforeIndexWrites := out.IndexWrites
	if effect.PresentElementWrite {
		if len(effect.PresentElementMember) > 0 {
			array := effect.Write
			if effect.HasPresentElementArray {
				array = effect.PresentElementArray
			}
			out.KeyPresence = out.KeyPresence.KillAffectedByPresentElementMemberWriteAddress(array, effect.PresentElementMember)
		} else {
			out.KeyPresence = out.KeyPresence.KillAffectedByPresentElementWriteAddress(effect.Write)
		}
		written := effect.Written
		if written.IsZero() {
			written = product.PresentDynamic()
		}
		out.IndexWrites = out.IndexWrites.PreservePresentElementWriteAddress(effect.Write, written)
	} else {
		out.KeyPresence = out.KeyPresence.KillAffectedByWriteAddress(effect.Write)
		out.IndexWrites = out.IndexWrites.KillAffectedByWriteAddress(effect.Write)
	}
	valueOriginsChanged := KillValueOriginsAffectedByWrite(out, effect.Write)
	pathAliasesChanged := KillPathAliasesAffectedByWrite(out, effect.Write)
	return !KeyPresenceFactsDomain.Equal(beforeKeyPresence, out.KeyPresence) ||
		valueOriginsChanged ||
		pathAliasesChanged ||
		!IndexWriteAdmissionFactsDomain.Equal(beforeIndexWrites, out.IndexWrites)
}
