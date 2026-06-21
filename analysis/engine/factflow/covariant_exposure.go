package factflow

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CovariantExposure records that a tracked object reachable at a source path is
// exposed at a CFG point through a wider mutable view (a covariant alias, cast,
// reassignment, or container store). The wide value carries the mutable view's
// declared contract type as its witness. Once exposed, a write through the wider
// view can launder a wide value into the object, so a later narrow read of the
// source path is no longer trustworthy; the engine eager-widens the source
// object's witness to the contract at this point to keep the read sound.
//
// The source path may be a bare root symbol (alias/cast/reassignment of a whole
// variable) or a sub-path (a field/element exposed individually, e.g. an alias of
// narrow.inner or a store of a sibling object into a container slot). A sub-path
// exposure requires ancestor repair: the object's witness is rebuilt so its
// ancestor cannot re-project the narrow field type.
// CovariantExposureKind distinguishes how the exposed object's witness widens.
type CovariantExposureKind uint8

const (
	// CovariantExposureRecord widens the exposed record object's fields, repairing
	// the ancestor witness for a sub-path exposure.
	CovariantExposureRecord CovariantExposureKind = iota
	// CovariantExposureArray widens an opaque array source's element witness to the
	// contract; a heap-tracked array stays precise through identity-keyed flow.
	CovariantExposureArray
)

type CovariantExposure struct {
	sourcePath path.Path
	wideValue  product.Value
	kind       CovariantExposureKind
}

// NewCovariantExposure creates a covariant-exposure fact for sourcePath toward
// the mutable contract carried by wideValue, of the given widening kind.
func NewCovariantExposure(sourcePath path.Path, wideValue product.Value, kind CovariantExposureKind) CovariantExposure {
	return CovariantExposure{
		sourcePath: sourcePath.Clone(),
		wideValue:  wideValue,
		kind:       kind,
	}
}

// SourcePath returns the exposed object's source path.
func (e CovariantExposure) SourcePath() path.Path { return e.sourcePath.Clone() }

// WideValue returns the mutable contract value to widen the exposed object
// toward.
func (e CovariantExposure) WideValue() product.Value { return e.wideValue }

// Kind returns how the exposed object's witness widens.
func (e CovariantExposure) Kind() CovariantExposureKind { return e.kind }

func (e CovariantExposure) copy() CovariantExposure {
	e.sourcePath = e.sourcePath.Clone()
	return e
}

func copyCovariantExposureMap(in map[cfg.Point][]CovariantExposure) map[cfg.Point][]CovariantExposure {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.Point][]CovariantExposure, len(in))
	for point, exposures := range in {
		copied := make([]CovariantExposure, len(exposures))
		for i := range exposures {
			copied[i] = exposures[i].copy()
		}
		out[point] = copied
	}
	return out
}
