package cold

import "github.com/wippyai/go-lua/analysis/identity"

// CallTarget is the exact closure-allocation-to-callable-body proof: which
// allocation is called, which body it enters, and the context and formal that
// body was compiled with.
//
// Every field is an artifact-local identity. None of them is mount-qualified,
// which is why one compiled program's call targets are shared unchanged by
// every Link that mounts it, and why a consumer that needs a mount-qualified
// address derives it at the read site from the module key it already holds.
type CallTarget struct {
	Allocation identity.ContentID
	Body       identity.ContentID
	Context    identity.ContentID
	Function   identity.ContentID
	Formal     identity.ContentID
}

// Available reports whether row names a proof. A target that is missing any
// of its five identities proves nothing, so it is never a row a consumer can
// read as one.
func (row CallTarget) Available() bool {
	return row.Allocation.Available() && row.Body.Available() && row.Context.Available() &&
		row.Function.Available() && row.Formal.Available()
}
