package core

import (
	"os"
	"strings"

	"github.com/wippyai/go-lua/types/typ"
)

var zzMethodDbg = os.Getenv("ZZMETHOD") != ""

// zzMethodDump renders a type including its metatable + __index surface, which
// FormatShort hides, so method-resolution failures can be diagnosed.
func zzMethodDump(t typ.Type) string {
	return zzMethodDumpDepth(t, 0)
}

func zzMethodDumpDepth(t typ.Type, depth int) string {
	if t == nil {
		return "<nil>"
	}
	if depth > 3 {
		return "..."
	}
	switch v := t.(type) {
	case *typ.Record:
		var b strings.Builder
		b.WriteString("{")
		for i, f := range v.Fields {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(f.Name)
			if f.Optional {
				b.WriteString("?")
			}
			b.WriteString(":")
			b.WriteString(typ.FormatShort(f.Type))
		}
		b.WriteString("}")
		if v.Metatable != nil {
			b.WriteString(" MT=")
			b.WriteString(zzMethodDumpDepth(v.Metatable, depth+1))
		}
		return b.String()
	case *typ.Recursive:
		if v.Body != nil && v.Body != typ.Type(v) {
			return "mu(" + zzMethodDumpDepth(v.Body, depth+1) + ")"
		}
		return typ.FormatShort(t)
	default:
		return typ.FormatShort(t)
	}
}
