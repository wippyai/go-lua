// Package typepath projects structural paths through types using query-core
// field/index semantics.
package typepath

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/db"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// Options controls how a source path is projected through a type.
type Options struct {
	Ctx *db.QueryContext
	Ops querycore.TypeOps

	// MissingFieldAsNil enables Lua table read semantics for observation paths:
	// a field absent from a table-like type is nil. Strict projections leave
	// missing fields unresolved so diagnostics can still distinguish typos.
	MissingFieldAsNil bool
}

// Strict projects segments through base using query-core field/index rules and
// rejects unresolved segments.
func Strict(base typ.Type, segments []constraint.Segment) typ.Type {
	return TypeAtSegments(base, segments, Options{})
}

// TypeAtSegments returns the type denoted by applying path segments to base.
func TypeAtSegments(base typ.Type, segments []constraint.Segment, opts Options) typ.Type {
	if base == nil {
		return nil
	}
	if len(segments) == 0 {
		return base
	}
	current := base
	for _, segment := range segments {
		next := segmentType(current, segment, opts)
		if next == nil {
			if opts.MissingFieldAsNil && querycore.MissingFieldReadsNil(current) {
				next = typ.Nil
			} else {
				return nil
			}
		}
		current = next
	}
	return current
}

func segmentType(base typ.Type, segment constraint.Segment, opts Options) typ.Type {
	switch segment.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		next := field(base, segment.Name, opts)
		if next == nil {
			next = index(base, typ.LiteralString(segment.Name), opts)
		}
		return next
	case constraint.SegmentIndexInt:
		return index(base, typ.LiteralInt(int64(segment.Index)), opts)
	default:
		return nil
	}
}

func field(base typ.Type, name string, opts Options) typ.Type {
	if opts.Ops != nil {
		next, _ := opts.Ops.Field(opts.Ctx, base, name)
		return next
	}
	next, _ := querycore.Field(base, name)
	return next
}

func index(base typ.Type, key typ.Type, opts Options) typ.Type {
	if opts.Ops != nil {
		next, _ := opts.Ops.Index(opts.Ctx, base, key)
		return next
	}
	next, _ := querycore.Index(base, key)
	return next
}
