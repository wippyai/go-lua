package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// ModuleExportCallableIngress is the authenticated return of one exported
// closure allocation from its module-init context to the importing context.
// Origin and holder are explicit and distinct; neither endpoint is inferred
// later from a module name or from the currently executing point.
type ModuleExportCallableIngress struct {
	id, link                         identity.ContentID
	originID, callTransitionID       identity.ContentID
	returnTransitionID               identity.ContentID
	originContextID, holderContextID identity.ContentID
	sourceModuleKey, targetModuleKey identity.ContentID
	allocationID                     identity.ContentID
}

// NewModuleExportCallableIngress constructs the exact reverse value-transfer
// edge justified by an exported origin and its module-call transition.
func NewModuleExportCallableIngress(origin ModuleExportCallableOrigin, call ModuleCallTransition, directory executioncontext.Directory) (ModuleExportCallableIngress, bool) {
	if !origin.Available() || !call.Available() || !directory.Available() ||
		origin.LinkID() != call.LinkID() || origin.LinkID() != directory.LinkID() ||
		origin.TransitionID() != call.TransitionID() ||
		origin.FromContextID() != call.FromContextID() || origin.ToContextID() != call.ToContextID() ||
		origin.GenerationID() != call.GenerationID() {
		return ModuleExportCallableIngress{}, false
	}
	canonicalCall, callOK := directory.Transition(call.FromContextID(), call.ToContextID())
	returnEdge, returnOK := directory.ActivationEdge(call.ToContextID(), call.FromContextID())
	if !callOK || !returnOK || canonicalCall.ID() != call.TransitionID() {
		return ModuleExportCallableIngress{}, false
	}
	row := ModuleExportCallableIngress{
		link:               origin.LinkID(),
		originID:           origin.ID(),
		callTransitionID:   call.ID(),
		returnTransitionID: returnEdge.ID(),
		originContextID:    call.ToContextID(),
		holderContextID:    call.FromContextID(),
		sourceModuleKey:    call.SourceModuleKey(),
		targetModuleKey:    origin.ModuleKey(),
		allocationID:       origin.AllocationID(),
	}
	row.id = moduleExportCallableIngressID(row)
	return row, row.Available()
}

func (row ModuleExportCallableIngress) Available() bool {
	return row.id.Available() && row.link.Available() && row.originID.Available() && row.callTransitionID.Available() &&
		row.returnTransitionID.Available() && row.originContextID.Available() && row.holderContextID.Available() &&
		row.sourceModuleKey.Available() && row.targetModuleKey.Available() && row.allocationID.Available() &&
		row.id == moduleExportCallableIngressID(row)
}

func (row ModuleExportCallableIngress) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row ModuleExportCallableIngress) LinkID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.link
}
func (row ModuleExportCallableIngress) OriginID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.originID
}
func (row ModuleExportCallableIngress) CallTransitionID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.callTransitionID
}
func (row ModuleExportCallableIngress) ReturnTransitionID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.returnTransitionID
}
func (row ModuleExportCallableIngress) OriginContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.originContextID
}
func (row ModuleExportCallableIngress) HolderContextID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.holderContextID
}
func (row ModuleExportCallableIngress) SourceModuleKey() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.sourceModuleKey
}
func (row ModuleExportCallableIngress) TargetModuleKey() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.targetModuleKey
}
func (row ModuleExportCallableIngress) AllocationID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.allocationID
}
