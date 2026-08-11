package factbinding

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"
)

// pendingRoot holds one sealed typed output that has not received a root-store
// identity. It is the Factor-owned half of carrier's all-slot publication
// preflight; no raw plane escapes this package.
type pendingRoot[K scalar.Key, V any] struct {
	work        *bindingWork[K, V]
	plane       semantic.Plane[planeFactor, K, V]
	reservation *rootReservation[planeFactor, K, V]
	preview     carrier.RootHandle
	published   bool
}

func (pending *pendingRoot[K, V]) Ready() bool {
	return pending != nil && !pending.published && pending.work != nil && pending.work.live() && pending.work.binding != nil && pending.work.binding.plane != nil && pending.work.roots != nil && pending.work.binding.plane.domain.Valid(pending.plane)
}

func (pending *pendingRoot[K, V]) Reserve() bool {
	if !pending.Ready() {
		return false
	}
	if pending.reservation != nil {
		return true
	}
	reservation, ok := pending.work.roots.reserve(pending.plane)
	if !ok {
		return false
	}
	pending.reservation = reservation
	return true
}

func (pending *pendingRoot[K, V]) Publish() carrier.RootHandle {
	if pending == nil || !pending.Ready() || pending.reservation == nil || pending.preview != (carrier.RootHandle{}) {
		panic("unreserved pending root publication")
	}
	id := pending.reservation.Publish()
	handle, ok := pending.work.epoch.IssueRoot(pending.work.binding.issuer, id)
	if !ok {
		panic("bound pending root lost issuer")
	}
	pending.published = true
	pending.reservation = nil
	pending.plane = semantic.Plane[planeFactor, K, V]{}
	return handle
}

// PreviewRoot exposes this already sealed typed plane only to carrier's
// ephemeral Preview transaction.  It deliberately skips rootStore.reserve:
// the Binding owns the temporary token-to-plane entry and Drop revokes it.
func (pending *pendingRoot[K, V]) PreviewRoot() (carrier.RootHandle, bool) {
	if pending == nil || !pending.Ready() || pending.reservation != nil {
		return carrier.RootHandle{}, false
	}
	if pending.preview != (carrier.RootHandle{}) {
		return pending.preview, true
	}
	handle, ok := pending.work.issuePreview(pending.plane)
	if !ok {
		return carrier.RootHandle{}, false
	}
	pending.preview = handle
	return handle, true
}

// OwnsPreviewRoot is the typed half of carrier's temporary-root admission.
// Equality alone is insufficient: resolve proves the Binding's local token
// still names this sealed plane and therefore cannot outlive Drop/Abort.
func (pending *pendingRoot[K, V]) OwnsPreviewRoot(handle carrier.RootHandle) bool {
	if pending == nil || pending.work == nil || pending.preview != handle {
		return false
	}
	plane, ok := pending.work.resolve(handle)
	return ok && pending.work.binding != nil && pending.work.binding.plane != nil && pending.work.binding.plane.domain.Valid(plane)
}

func (pending *pendingRoot[K, V]) Drop() {
	if pending == nil || pending.published {
		return
	}
	if pending.preview != (carrier.RootHandle{}) && pending.work != nil {
		pending.work.dropPreview(pending.preview)
	}
	pending.work = nil
	pending.plane = semantic.Plane[planeFactor, K, V]{}
	pending.reservation = nil
	pending.preview = carrier.RootHandle{}
}
