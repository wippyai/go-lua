package arrangement

import "github.com/wippyai/go-lua/analysis/relation/mount/address"

// Inventory resolves one logical access to one private physical arrangement
// handle. The coordinate is retained by Plan for the future runtime, but no
// accessor on Handle exposes it: callers can only inspect logical
// requirements and validate the mount fence.
//
// Implementations must return the same Fence for the lifetime of one Derive
// call. Resolve is called exactly once per canonical logical Access.
type Inventory interface {
	Fence() address.Fence
	Resolve(Access) (Handle, bool)
}
