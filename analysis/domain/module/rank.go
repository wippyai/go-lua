package module

import (
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// WidenRank is the Link-family witness. At is explicitly key-aware because
// each coordinate has a different finite subject support while the lattice
// and default remain constant across the Factor.
type WidenRank struct{ owner *schema }

func (schema Schema) WidenRank() (WidenRank, bool) {
	if !schema.Valid() {
		return WidenRank{}, false
	}
	return WidenRank{owner: schema.owner}, true
}

func (rank WidenRank) Width() int {
	if rank.owner == nil {
		return 0
	}
	return 1
}

func (rank WidenRank) At(key Key, value Value, component int) (uint64, bool) {
	owner, support, keyOK := key.support()
	if rank.owner == nil || component != 0 || !keyOK || owner != rank.owner || value.owner != rank.owner {
		return 0, false
	}
	if value.top {
		return 0, true
	}
	if !pendingSupported(rank.owner.source, value.pending, support.pending) || !readySupported(rank.owner.source, value.ready, support.ready) {
		return 0, false
	}
	used := uint64(len(value.pending) + len(value.ready))
	if value.cold {
		used++
	}
	readyCapacity := 0
	for _, ready := range support.ready {
		readyCapacity += len(ready.subjects)
	}
	capacity := uint64(1 + len(support.pending)*2 + readyCapacity)
	if used > capacity {
		return 0, false
	}
	return capacity - used + 1, true
}

func readySupported(source *link.Link, ready []readySite, support []readySupport) bool {
	for _, item := range ready {
		if !containsReadyForSite(source, support, item.site, item.subject) {
			return false
		}
	}
	return true
}

func pendingSupported(source *link.Link, pending []pendingSite, support []linkmodule.ModuleInitGeneration) bool {
	for _, item := range pending {
		if !pendingRoleValid(item.role) || !containsGeneration(source, support, item.site) {
			return false
		}
	}
	return true
}
