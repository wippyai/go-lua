package gradual

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

const softPruneMemoMaxEntries = 4096

type softPruneState struct {
	memo     map[typ.Type]typ.Type
	visiting map[typ.Type]struct{}
	softMemo map[typ.Type]bool
}

var softPruneStatePool = sync.Pool{
	New: func() any {
		return &softPruneState{
			memo:     make(map[typ.Type]typ.Type, 64),
			visiting: make(map[typ.Type]struct{}, 64),
			softMemo: make(map[typ.Type]bool, 64),
		}
	},
}

func getSoftPruneState() *softPruneState {
	return softPruneStatePool.Get().(*softPruneState)
}

func putSoftPruneState(state *softPruneState) {
	if state == nil {
		return
	}
	if len(state.memo) > softPruneMemoMaxEntries {
		state.memo = make(map[typ.Type]typ.Type, 64)
	} else {
		clear(state.memo)
	}
	if len(state.visiting) > softPruneMemoMaxEntries {
		state.visiting = make(map[typ.Type]struct{}, 64)
	} else {
		clear(state.visiting)
	}
	if len(state.softMemo) > softPruneMemoMaxEntries {
		state.softMemo = make(map[typ.Type]bool, 64)
	} else {
		clear(state.softMemo)
	}
	softPruneStatePool.Put(state)
}
