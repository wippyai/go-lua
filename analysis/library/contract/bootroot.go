package contract

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/internal/framing"
)

// The boot-root payload format. It is owned here for the reason every payload in
// this package is: nothing it carries is a TYPE. A boot root states which
// aggregate the initial environment booted at one address and whether that
// aggregate's whole object is published sealed, and both are publication policy
// over one value rather than a proposition about which values inhabit a set.
//
// What the payload deliberately does NOT carry is a root NAME. The initial
// environment used to identify its roots by an authored string - "StringRoot",
// "ErrorMetatableRoot" - and a name is exactly what this surface refuses: it is
// rebindable, aliasable and shadowable, and a root reached under one name is
// reached under every alias of it. A boot root's identity is the export path the
// member is published at, so the environment root is the contract root, the table
// library is one step from it, and the error metatable is the value the errors
// aggregate publishes under Error. The name ABI dies at that address.
const (
	bootRootDomain  = "analysis/library/contract/boot-root"
	bootRootVersion = 1
)

// BootAggregate is which aggregate one boot root is. The distinction is
// structural: an ordinary table is reached as a value, and a metatable is reached
// as the dispatch table another value's members resolve through, so a path that
// continues past one continues differently than past the other.
type BootAggregate uint8

const (
	BootAggregateInvalid BootAggregate = iota
	// BootAggregateTable is a boot root that is an ordinary table.
	BootAggregateTable
	// BootAggregateMetatable is a boot root that is a metatable.
	BootAggregateMetatable
	bootAggregateLimit
)

func (aggregate BootAggregate) Available() bool {
	return aggregate > BootAggregateInvalid && aggregate < bootAggregateLimit
}

// BootRoot is one root of the initial environment: the aggregate it boots as,
// and the mutability its whole object is published with. The mutability here is
// the environment's own statement about the booted object, which is why only the
// environment class declares this form: a library that could publish its own
// aggregate sealed would be stating a fact about the host that boots it.
type BootRoot struct {
	Aggregate  BootAggregate
	Mutability Mutability
}

func (root BootRoot) Available() bool {
	return root.Aggregate.Available() && root.Mutability.Available()
}

// EncodeBootRoot writes one boot root as a member payload body. The result is a
// complete framed stream: a payload is decodable on its own, so a reader that
// holds a member and its declared format never needs the enclosing instance to
// interpret it.
func EncodeBootRoot(root BootRoot) ([]byte, error) {
	if !root.Available() {
		return nil, ErrMalformed
	}
	var buffer bytes.Buffer
	var writer framing.Writer
	if err := writer.Reset(&buffer, bootRootDomain, bootRootVersion); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(root.Aggregate)); err != nil {
		return nil, err
	}
	if err := writer.Uint(uint64(root.Mutability)); err != nil {
		return nil, err
	}
	if err := writer.Finish(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// DecodeBootRoot reads one boot-root payload body.
func DecodeBootRoot(data []byte) (BootRoot, error) {
	reader, err := framing.NewReader(data, len(data))
	if err != nil {
		return BootRoot{}, ErrMalformed
	}
	if err := reader.Header(bootRootDomain, bootRootVersion); err != nil {
		return BootRoot{}, ErrMalformed
	}
	aggregate, err := reader.Uint()
	if err != nil || aggregate > uint64(^uint8(0)) {
		return BootRoot{}, ErrMalformed
	}
	mutability, err := reader.Uint()
	if err != nil || mutability > uint64(^uint8(0)) {
		return BootRoot{}, ErrMalformed
	}
	if err := reader.Finish(); err != nil {
		return BootRoot{}, ErrMalformed
	}
	root := BootRoot{Aggregate: BootAggregate(aggregate), Mutability: Mutability(mutability)}
	if !root.Available() {
		return BootRoot{}, ErrMalformed
	}
	return root, nil
}
