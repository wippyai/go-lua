// Package workplan defines the type-independent execution view consumed by
// CFG schedulers. Semantic plans may retain arbitrarily rich payloads while
// exposing only which equation phases have work at each dense CFG point.
package workplan

import "github.com/wippyai/go-lua/analysis/ir/cfg"

// PointWork is the set of concrete equation phases active at one CFG point.
type PointWork uint8

const (
	Node PointWork = 1 << iota
	Edge
)

// Has reports whether the requested phase is active.
func (w PointWork) Has(phase PointWork) bool { return phase != 0 && w&phase == phase }

// Rows is the minimal, type-independent semantic-plan view required to
// certify a dense CFG schedule. PointWork must reject malformed or
// non-canonical semantic rows rather than allowing a scheduler to guess.
type Rows interface {
	PointCount() int
	PointWork(cfg.Point) (PointWork, error)
}
