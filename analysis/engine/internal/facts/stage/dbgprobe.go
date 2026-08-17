package stage

import (
	"fmt"
	"strings"
	"sync/atomic"
)

var (
	DbgStageBegin            atomic.Int64
	DbgStageSet              atomic.Int64
	DbgStageWeakJoin         atomic.Int64
	DbgStageWeakJoinMany     atomic.Int64
	DbgStageWeakJoinManyKeys atomic.Int64
	DbgStageTransform        atomic.Int64
	DbgStageTransformKeys    atomic.Int64
	DbgStageRewrite          atomic.Int64
	DbgStageAccept           atomic.Int64
	DbgStageDiscard          atomic.Int64
)

var dbgAll = []struct {
	name    string
	counter *atomic.Int64
}{
	{"Begin", &DbgStageBegin},
	{"Set", &DbgStageSet},
	{"WeakJoin", &DbgStageWeakJoin},
	{"WeakJoinMany", &DbgStageWeakJoinMany},
	{"WeakJoinMany_keys", &DbgStageWeakJoinManyKeys},
	{"Transform", &DbgStageTransform},
	{"Transform_keys", &DbgStageTransformKeys},
	{"rewrite_key_writes", &DbgStageRewrite},
	{"Accept", &DbgStageAccept},
	{"Discard", &DbgStageDiscard},
}

func DbgReset() {
	for _, row := range dbgAll {
		row.counter.Store(0)
	}
}

func DbgReport() string {
	var out strings.Builder
	for _, row := range dbgAll {
		fmt.Fprintf(&out, "  stage.%-24s %12d\n", row.name, row.counter.Load())
	}
	return out.String()
}
