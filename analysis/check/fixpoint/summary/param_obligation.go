package summary

import (
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ParamMemberCallObligation records a pre-call obligation that depends on a
// callable member of another parameter. Call-boundary adaptation resolves the
// receiver argument's member type and materializes a plain ParamObligation for
// ArgParam.
type ParamMemberCallObligation struct {
	ReceiverParam    int
	ReceiverPath     pathaddr.SuffixKey
	Member           segment.Segment
	ArgParam         int
	MemberParamIndex int
}

// ParamMemberReturnSlot records that a function return slot delegates to a
// result slot from a callable member of one parameter. Call-boundary adaptation
// can then rehydrate effect-bearing return-presence relations from the concrete
// receiver argument's imported signature.
type ParamMemberReturnSlot struct {
	ReceiverParam     int
	Member            segment.Segment
	ReturnIndex       int
	MemberResultIndex int
}

// ReturnParamPathAlias records that a static member below one returned heap
// object aliases a parameter-relative source path. Call-boundary adaptation
// substitutes the concrete argument and writes the member into the returned heap
// object so nested factory DI keeps active provider evidence. An empty Member is
// the return-root alias (return o): the whole return slot, not a member below it,
// aliases the parameter.
type ReturnParamPathAlias struct {
	ReturnIndex int
	Member      pathaddr.SuffixKey
	Source      pathaddr.PlaceholderKey
}

// ParamSinkExposure records that the callee stores one parameter (Source, a bare
// placeholder path key) into a member slot of a persistent sink the caller cannot
// track writes back through: a captured upvalue or a global. Contract carries the
// sink's slot type, computed in-body where the sink's container type is available
// (the slot type, not the parameter's declared type, is the sound exposure type:
// a covariant store of a narrow parameter into a wider sink slot is well-typed,
// and a later write through the sink launders a wider value back into the
// argument). Call-boundary adaptation rebases Source onto the concrete argument
// and eager-widens it toward Contract.
type ParamSinkExposure struct {
	Source   pathaddr.RootPlaceholderKey
	Contract product.Value
}

// CapturedPathObligation records a pre-call obligation imposed on a stable
// symbol path captured from the caller. It lets a zero-argument closure publish
// "the captured path I read must satisfy this contract" without pretending the
// closure has a formal parameter. Call-boundary adaptation rehydrates Path into
// the caller's path vocabulary, and the caller may project it back through its
// own stable locals to parameter obligations.
type CapturedPathObligation struct {
	Path  pathaddr.StableKey
	Value product.Value
}
