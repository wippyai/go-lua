package flow

import (
	"fmt"
	"os"
	"sync"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// ZZSLOT diagnostic for the mutable-store presence-monotonicity work. Gated by the
// ZZSLOT env var; reports per-point slot deltas so a present/absent membership
// oscillation in the store join is visible. Retained until the abstract
// interpreter fully converges; removed only in the final de-scatter pass.
var zzSlotDbg = os.Getenv("ZZSLOT") != ""

var (
	zzFileOnce sync.Once
	zzFile     *os.File
)

func zzOut() *os.File {
	zzFileOnce.Do(func() {
		path := os.Getenv("ZZSLOT_FILE")
		if path == "" {
			path = "zz_slot_trace.txt"
		}
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644); err == nil {
			zzFile = f
		}
	})
	return zzFile
}

func zzLog(format string, args ...any) {
	if !zzSlotDbg {
		return
	}
	f := zzOut()
	if f == nil {
		return
	}
	fmt.Fprintf(f, format+"\n", args...)
	f.Sync()
}

func zzSlotDelta(p cfg.Point, key string, old map[string]product.AbstractValue, current map[string]product.AbstractValue, oldOK, curOK bool) {
	if !zzSlotDbg {
		return
	}
	zzLog("ZZSLOT point=%d key=%s oldOK=%v curOK=%v oldVal=%s curVal=%s",
		int(p), key, oldOK, curOK, projectFlowValueString(old[key]), projectFlowValueString(current[key]))
}

func projectFlowValueString(av product.AbstractValue) string {
	t := projectFlowValue(av)
	if t == nil {
		return "<nil>"
	}
	return t.String()
}
