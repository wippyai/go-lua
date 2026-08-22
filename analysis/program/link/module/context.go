package module

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

const (
	actorIDTag        uint64 = 0x6d6f64756c652d61
	instanceIDTag     uint64 = 0x6d6f64756c652d69
	analysisRootIDTag uint64 = 0x6d6f64756c652d72
)

// BuildContextDirectory constructs the Link-owned execution-context
// directory from Module's already-sealed scalar roots.  Module is built before
// the enclosing Link identity exists, so the caller supplies that identity at
// the Link seal boundary.  The returned value is detached scalar data; this
// method does not mutate the Module authority and callers cannot ask it to
// retain or reopen Project, Boundary, or authored module input.
func (c *Component) BuildContextDirectory(linkID identity.ContentID) (executioncontext.Directory, bool) {
	if !live(c) || !linkID.Available() || c.authority.fence == nil || !c.authority.fence.sealed {
		return executioncontext.Directory{}, false
	}
	a := c.authority
	contexts := make([]executioncontext.Context, 0, len(a.roots))
	rootRows := make([]executioncontext.RootContext, 0, len(a.roots))
	contextByID := make(map[identity.ContentID]executioncontext.Context, len(a.roots))
	rootByID := make(map[identity.ContentID]executioncontext.RootContext, len(a.roots))
	for index, root := range a.roots {
		moduleKey, moduleOK := a.project.ModuleKey(root.shard)
		if !moduleOK || root.actor == 0 || uint64(root.actor) > uint64(len(a.actors)) || root.instance == 0 || uint64(root.instance) > uint64(len(a.instances)) {
			return executioncontext.Directory{}, false
		}
		representative := a.instances[root.instance-1].representative
		if representative == 0 || uint64(representative) > uint64(len(a.instances)) {
			return executioncontext.Directory{}, false
		}
		actorID := denseID(a.content, actorIDTag, uint64(root.actor))
		representativeID := denseID(a.content, instanceIDTag, uint64(representative))
		contextRow, contextOK := executioncontext.NewContext(linkID, moduleKey, actorID, representativeID)
		rootID := denseID(a.content, analysisRootIDTag, uint64(index+1))
		rootRow, rootOK := executioncontext.NewRootContext(linkID, rootID, contextRow.ID())
		if !contextOK || !rootOK {
			return executioncontext.Directory{}, false
		}
		if previous, present := contextByID[contextRow.ID()]; present {
			if previous != contextRow {
				return executioncontext.Directory{}, false
			}
		} else {
			contextByID[contextRow.ID()] = contextRow
			contexts = append(contexts, contextRow)
		}
		if _, duplicate := rootByID[rootRow.AnalysisRootID()]; duplicate {
			return executioncontext.Directory{}, false
		}
		rootByID[rootRow.AnalysisRootID()] = rootRow
		rootRows = append(rootRows, rootRow)
	}

	transitions := make([]executioncontext.Transition, 0, len(a.composition))
	transitionByID := make(map[identity.ContentID]executioncontext.Transition, len(a.composition))
	for _, entry := range a.composition {
		fromRootID := entry.fromRootID
		toRootID := entry.toRootID
		fromRoot, fromOK := rootByID[fromRootID]
		toRoot, toOK := rootByID[toRootID]
		if !fromOK || !toOK {
			return executioncontext.Directory{}, false
		}
		transition, transitionOK := executioncontext.NewTransition(linkID, fromRoot.ContextID(), toRoot.ContextID())
		if !transitionOK {
			return executioncontext.Directory{}, false
		}
		// Local module calls use the Directory's one canonical reflexive
		// transition.  They are not authored edges: retaining a second
		// same-context row here would create a duplicate authority and would be
		// rejected by executioncontext.Seal.  The composition entry itself is
		// still retained by the module relation; only this scalar edge is
		// quotient-ed at the Directory boundary.
		if transition.FromContextID() == transition.ToContextID() {
			continue
		}
		if previous, present := transitionByID[transition.ID()]; present {
			if previous.FromContextID() != transition.FromContextID() || previous.ToContextID() != transition.ToContextID() {
				return executioncontext.Directory{}, false
			}
			// Multiple authored imports may cross the same Context pair.  The
			// frozen transition relation is the pair quotient, so retain one
			// exact edge while each CacheIngress keeps its own import identity.
			continue
		}
		transitionByID[transition.ID()] = transition
		transitions = append(transitions, transition)
	}

	directory, directoryOK := executioncontext.Seal(linkID, contexts, rootRows, transitions)
	if !directoryOK {
		return executioncontext.Directory{}, false
	}
	return directory, true
}
