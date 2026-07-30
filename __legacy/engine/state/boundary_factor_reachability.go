package state

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

// PrepareBoundaryFactorReachability compiles one ordinary factor's registered
// reachability clauses against the exact source KeySpace. Callers freeze the
// returned program beside the factor terminal; Apply never scans the factor.
func (d ProductDomain) PrepareBoundaryFactorReachability(keys *keyspace.KeySpace, factor LaneFactor) (BoundaryReachabilityProgram, error) {
	runtime, err := d.validateFactor(factor)
	if err != nil || keys == nil || !keys.Valid() || runtime.ops.boundaryReachability == nil {
		return BoundaryReachabilityProgram{}, fmt.Errorf("%w: factor has no registered boundary reachability program", ErrInvalidLaneFactor)
	}
	return runtime.ops.boundaryReachability(d.reg, keys, factor.payload)
}
