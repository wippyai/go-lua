package flow

import "github.com/wippyai/go-lua/types/domain/value/product"

// addressPredicate is the generic normalized-address predicate used inside
// fact domains that store compact keys but should compose through addresses.
type addressPredicate func(StableAddress) bool

// addressAbsentProof is an address-normalized predicate used when reduced
// products can preserve a one-sided fact because the other branch proves the
// fact's key/value path absent.
type addressAbsentProof func(StableAddress) bool

// addressPresentValueProof is the dual predicate for facts that can be
// preserved by reading a definitely-present value from the other branch.
type addressPresentValueProof func(StableAddress) (product.AbstractValue, bool)
