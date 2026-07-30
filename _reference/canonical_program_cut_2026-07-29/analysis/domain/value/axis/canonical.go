package axis

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/internal/canonical"
)

// CanonicalStatus declares whether an axis has a complete portable canonical
// value codec. Pending is an explicit migration state, not optional authority:
// a registry containing any Pending axis cannot publish canonical authority.
type CanonicalStatus uint8

const (
	CanonicalUnspecified CanonicalStatus = iota
	CanonicalPending
	CanonicalReady
)

// CanonicalDescriptor is the mandatory canonical contract of one axis. Its
// fields are intentionally private so Ready/Pending metadata can only be
// created through validating constructors.
type CanonicalDescriptor[T any] struct {
	status        CanonicalStatus
	codecID       string
	version       uint64
	encode        func(*canonical.Writer, T) error
	decode        func(context.Context, *canonical.Reader) (T, error)
	pendingReason string
}

// ReadyCanonicalBidirectional declares the same canonical identity authority
// as ReadyCanonical and additionally supplies its exact materializer. Decode
// capability does not change the wire schema identity; it is a local ability
// to reconstruct that already-versioned semantic value.
func ReadyCanonicalBidirectional[T any](
	codecID string,
	version uint64,
	encode func(*canonical.Writer, T) error,
	decode func(context.Context, *canonical.Reader) (T, error),
) CanonicalDescriptor[T] {
	return CanonicalDescriptor[T]{
		status: CanonicalReady, codecID: codecID, version: version,
		encode: encode, decode: decode,
	}
}

// ReadyCanonical declares a complete portable codec. codecID identifies the
// semantic vocabulary; version changes whenever that vocabulary changes.
func ReadyCanonical[T any](codecID string, version uint64, encode func(*canonical.Writer, T) error) CanonicalDescriptor[T] {
	return CanonicalDescriptor[T]{
		status:  CanonicalReady,
		codecID: codecID,
		version: version,
		encode:  encode,
	}
}

// PendingCanonical declares that this axis deliberately withholds canonical
// authority until the named portability problem is solved.
func PendingCanonical[T any](reason string) CanonicalDescriptor[T] {
	return CanonicalDescriptor[T]{status: CanonicalPending, pendingReason: reason}
}

func validateCanonicalDescriptor[T any](axisID string, descriptor CanonicalDescriptor[T]) error {
	switch descriptor.status {
	case CanonicalReady:
		if descriptor.codecID == "" {
			return fmt.Errorf("axis %q: ready canonical descriptor has empty codec id", axisID)
		}
		if descriptor.version == 0 {
			return fmt.Errorf("axis %q: ready canonical descriptor has zero codec version", axisID)
		}
		if descriptor.encode == nil {
			return fmt.Errorf("axis %q: ready canonical descriptor has nil encoder", axisID)
		}
		if descriptor.pendingReason != "" {
			return fmt.Errorf("axis %q: ready canonical descriptor carries a pending reason", axisID)
		}
	case CanonicalPending:
		if descriptor.pendingReason == "" {
			return fmt.Errorf("axis %q: pending canonical descriptor has empty reason", axisID)
		}
		if descriptor.codecID != "" || descriptor.version != 0 || descriptor.encode != nil || descriptor.decode != nil {
			return fmt.Errorf("axis %q: pending canonical descriptor carries ready codec authority", axisID)
		}
	default:
		return fmt.Errorf("axis %q: canonical descriptor is unspecified", axisID)
	}
	return nil
}
