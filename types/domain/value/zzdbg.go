package value

import (
	"os"

	"github.com/wippyai/go-lua/types/typ"
)

var zzMTDbg = os.Getenv("ZZMT") != ""

func zzMTlog(a, b, out typ.Type) {
	if !zzMTDbg {
		return
	}
	// Only log when a metatable surface is involved and something changes.
	println("ZZMT a=", zzShort(a), " b=", zzShort(b), " out=", zzShort(out))
}

func zzShort(t typ.Type) string {
	if t == nil {
		return "<nil>"
	}
	return typ.FormatShort(t)
}
