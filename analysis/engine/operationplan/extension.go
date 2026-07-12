package operationplan

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// ExtensionKind identifies semantic operations owned above generic factflow.
// Extension payloads remain in their owning layer; this index makes their
// point/barrier ownership visible to semantic-program and executor compilers.
type ExtensionKind uint8

const (
	BodyGenericFor ExtensionKind = iota + 1
)

var extensionKinds = [...]ExtensionKind{BodyGenericFor}

// ExtensionKinds returns the immutable higher-layer operation catalog.
func ExtensionKinds() []ExtensionKind { return append([]ExtensionKind(nil), extensionKinds[:]...) }

func knownExtensionKind(kind ExtensionKind) bool {
	for _, candidate := range extensionKinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

// ExtensionInput registers one higher-layer semantic transaction.
type ExtensionInput struct {
	Point cfg.Point
	Kind  ExtensionKind
}

type ExtensionCell struct{ kind ExtensionKind }

func (c ExtensionCell) Kind() ExtensionKind { return c.kind }

func (c ExtensionCell) Metadata() Metadata {
	switch c.kind {
	case BodyGenericFor:
		return Metadata{Class: Composite, Phase: Node, Barrier: N7BodySemantics, Stages: barriers(N7BodySemantics)}
	default:
		return Metadata{}
	}
}

type extensionRow struct{ start, end uint32 }

// WithExtensions returns a plan sharing the immutable fact snapshot and dense
// fact index while owning a packed, deterministic higher-layer operation
// index. Unknown kinds and out-of-range points are rejected fail-closed by
// omission; ownership tests enumerate the public ExtensionKind catalog.
func (p *Plan) WithExtensions(input []ExtensionInput) *Plan {
	if p == nil || len(input) == 0 {
		return p
	}
	out := *p
	out.extensionRows = make([]extensionRow, len(p.rows))
	masks := make([]uint64, len(p.rows))
	for _, item := range input {
		if !knownExtensionKind(item.Kind) || uint64(item.Point) >= uint64(len(masks)) {
			continue
		}
		masks[item.Point] |= uint64(1) << (item.Kind - 1)
	}
	for point, mask := range masks {
		out.extensionRows[point].start = uint32(len(out.extensionCells))
		for _, kind := range extensionKinds {
			if mask&(uint64(1)<<(kind-1)) != 0 {
				out.extensionCells = append(out.extensionCells, ExtensionCell{kind: kind})
			}
		}
		out.extensionRows[point].end = uint32(len(out.extensionCells))
	}
	return &out
}

func (p *Plan) ExtensionCursor(point cfg.Point) ExtensionCursor {
	if p == nil || uint64(point) >= uint64(len(p.extensionRows)) {
		return ExtensionCursor{}
	}
	row := p.extensionRows[point]
	return ExtensionCursor{cells: p.extensionCells, next: row.start, end: row.end}
}

func (p *Plan) HasExtensions() bool { return p != nil && len(p.extensionCells) != 0 }

type ExtensionCursor struct {
	cells     []ExtensionCell
	next, end uint32
}

func (c *ExtensionCursor) Next() (ExtensionCell, bool) {
	if c == nil || c.next >= c.end {
		return ExtensionCell{}, false
	}
	cell := c.cells[c.next]
	c.next++
	return cell, true
}
