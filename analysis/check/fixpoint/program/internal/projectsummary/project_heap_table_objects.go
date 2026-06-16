package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func projectHeapTableObjects(exit state.State) map[identity.ID]heapidentity.TableObject {
	snapshot := exit.HeapTableObjectsSnapshot()
	if snapshot.Top {
		return nil
	}
	return heapidentity.CloneMap(snapshot.Objects)
}
