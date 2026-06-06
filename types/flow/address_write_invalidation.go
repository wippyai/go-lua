package flow

import (
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

// ApplyAddressWriteInvalidation applies the shared write-kill law for address
// indexed point facts. StaticMembers is intentionally excluded: transfer owns
// its branch-specific prefix-bottoming rule before applying subtree kills.
func ApplyAddressWriteInvalidation(out *PointState, effect AddressWriteInvalidation) bool {
	if out == nil || effect.Write.Key() == "" {
		return false
	}
	beforeKeyPresence := out.KeyPresence
	beforeValueOrigins := out.ValueOrigins
	beforePathAliases := out.PathAliases
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
	out.ValueOrigins = out.ValueOrigins.KillAffectedByWriteAddress(effect.Write)
	out.PathAliases = out.PathAliases.KillAffectedByWriteAddress(effect.Write)
	return !KeyPresenceFactsDomain.Equal(beforeKeyPresence, out.KeyPresence) ||
		!ValueOriginFactsDomain.Equal(beforeValueOrigins, out.ValueOrigins) ||
		!PathAliasFactsDomain.Equal(beforePathAliases, out.PathAliases) ||
		!IndexWriteAdmissionFactsDomain.Equal(beforeIndexWrites, out.IndexWrites)
}
