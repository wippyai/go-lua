// Package static owns Link's one detached namespace identity per concrete
// Project mount. ProgramArtifact owns every Program-internal static fact.
package static

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/identity"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// Input intentionally admits only Project's scalar mount projections.
type Input struct{ Project *linkproject.Component }

type Draft struct{ state *draftState }
type draftState struct {
	mu        sync.Mutex
	component *Component
	consumed  bool
	fence     *draftFence
}
type draftFence struct{ consumed bool }

// Component contains no Project, Shard, Program, TransformerInput, Flow, or
// source-term handle. Namespace IDs are exact scalar Link substitution keys.
type Component struct {
	contentID identity.ContentID
	schema    []identity.ContentID
}

// Cold is a portable scalar snapshot. Its fence only invalidates a Draft view.
type Cold struct {
	contentID identity.ContentID
	schema    []identity.ContentID
	fence     *draftFence
}

func (c *Component) Cold() Cold {
	if c == nil || !c.contentID.Available() {
		return Cold{}
	}
	return Cold{contentID: c.contentID, schema: append([]identity.ContentID(nil), c.schema...)}
}

func (d *Draft) Cold() Cold {
	if d == nil || d.state == nil {
		return Cold{}
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if d.state.consumed || d.state.component == nil {
		return Cold{}
	}
	cold := d.state.component.Cold()
	cold.fence = d.state.fence
	return cold
}

func (v Cold) live() bool { return v.contentID.Available() && (v.fence == nil || !v.fence.consumed) }
func (v Cold) ContentID() identity.ContentID {
	if !v.live() {
		return identity.ContentID{}
	}
	return v.contentID
}
func (v Cold) SchemaContentCount() int {
	if !v.live() {
		return 0
	}
	return len(v.schema)
}
func (v Cold) SchemaContentAt(i int) (identity.ContentID, bool) {
	if !v.live() || i < 0 || i >= len(v.schema) {
		return identity.ContentID{}, false
	}
	id := v.schema[i]
	return id, id.Available()
}
