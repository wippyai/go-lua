package userlattice

import (
	"fmt"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

// Runtime is the registry-local table used by the generic state lane.
type Runtime struct {
	axes []Axis
	byID map[AxisID]Axis
}

// Axis is one verified descriptor with its registry-local dense slot.
type Axis struct {
	slot AxisSlot
	spec *verifiedSpec
}

var runtimeCache = struct {
	mu    sync.Mutex
	byReg map[*axis.Registry]Runtime
}{
	byReg: make(map[*axis.Registry]Runtime),
}

// RuntimeFor returns the verified user-lattice descriptors registered on reg.
func RuntimeFor(reg *axis.Registry) Runtime {
	if reg == nil {
		return Runtime{}
	}
	if !reg.Frozen() {
		return buildRuntime(reg)
	}
	runtimeCache.mu.Lock()
	if rt, ok := runtimeCache.byReg[reg]; ok {
		runtimeCache.mu.Unlock()
		return rt
	}
	runtimeCache.mu.Unlock()

	rt := buildRuntime(reg)

	runtimeCache.mu.Lock()
	if existing, ok := runtimeCache.byReg[reg]; ok {
		runtimeCache.mu.Unlock()
		return existing
	}
	runtimeCache.byReg[reg] = rt
	runtimeCache.mu.Unlock()
	return rt
}

func buildRuntime(reg *axis.Registry) Runtime {
	view := reg.ExtensionsView(extensionKind)
	if view.Len() == 0 {
		return Runtime{}
	}
	verifiedSpecs := make([]*verifiedSpec, 0, view.Len())
	for i := 0; i < view.Len(); i++ {
		verified, ok := view.At(i).(*verifiedSpec)
		if !ok {
			panic(fmt.Sprintf("userlattice: extension %q has type %T, want verified user lattice", extensionKind, view.At(i)))
		}
		verifiedSpecs = append(verifiedSpecs, verified)
	}
	sort.Slice(verifiedSpecs, func(i, j int) bool { return verifiedSpecs[i].id < verifiedSpecs[j].id })
	axes := make([]Axis, 0, len(verifiedSpecs))
	byID := make(map[AxisID]Axis, view.Len())
	for i, verified := range verifiedSpecs {
		if i > int(^AxisSlot(0)) {
			panic("userlattice: too many registered axes")
		}
		axis := Axis{slot: AxisSlot(i), spec: verified}
		axes = append(axes, axis)
		byID[verified.id] = axis
	}
	return Runtime{axes: axes, byID: byID}
}

func (rt Runtime) Len() int {
	return len(rt.axes)
}

func (rt Runtime) AxisAt(i int) Axis {
	return rt.axes[i]
}

func (rt Runtime) AxisByID(id AxisID) (Axis, bool) {
	axis, ok := rt.byID[id]
	return axis, ok
}

func (rt Runtime) AxisBySlot(slot AxisSlot) (Axis, bool) {
	if int(slot) >= len(rt.axes) {
		return Axis{}, false
	}
	return rt.axes[slot], true
}

func (a Axis) Valid() bool { return a.spec != nil }

func (a Axis) ID() AxisID { return a.spec.id }

func (a Axis) Slot() AxisSlot { return a.slot }

func (a Axis) Bottom() Element { return a.spec.bottom }

func (a Axis) Top() Element { return a.spec.top }

func (a Axis) Element(name ElementID) (Element, bool) {
	elem, ok := a.spec.index[name]
	return elem, ok
}

func (a Axis) ElementName(elem Element) ElementID {
	if int(elem) >= len(a.spec.elements) {
		return ""
	}
	return a.spec.elements[elem]
}

func (a Axis) LessOrEq(left, right Element) bool {
	if int(left) >= len(a.spec.elements) || int(right) >= len(a.spec.elements) {
		return false
	}
	n := len(a.spec.elements)
	return a.spec.leq[int(left)*n+int(right)]
}

func (a Axis) Join(left, right Element) Element {
	if int(left) >= len(a.spec.elements) || int(right) >= len(a.spec.elements) {
		return a.spec.top
	}
	n := len(a.spec.elements)
	return a.spec.join[int(left)*n+int(right)]
}

func (a Axis) Assign(source Element) Element {
	if a.spec.assignMode != AssignPropagate || int(source) >= len(a.spec.assignMap) {
		return a.spec.bottom
	}
	return a.spec.assignMap[source]
}

func (a Axis) CallBoundary(source Element) Element {
	if a.spec.callBoundary != CallBoundaryKeep || int(source) >= len(a.spec.callBoundaryMap) {
		return a.spec.bottom
	}
	return a.spec.callBoundaryMap[source]
}

func (a Axis) Claim(name string) (Element, bool) {
	elem, ok := a.spec.claims[name]
	return elem, ok
}
