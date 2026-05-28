package metatable

import (
	"os"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/types/typ"
)

var zzSealDbg = os.Getenv("ZZSEAL") != ""
var zzSealOff = os.Getenv("ZZSEALOFF") != ""

// zzSurfaceKey is an experimental distinct-per-class key: the metatable surface
// (or the record's own surface) field names, to test the family-name conflation
// hypothesis. Drifts with surface growth; for confirmation only.
func zzSurfaceKey(root *typ.Record) string {
	surface := root
	if meta, ok := unwrapRecord(root.Metatable); ok && meta != nil {
		surface = meta
	}
	names := make([]string, 0, len(surface.Fields))
	for _, f := range surface.Fields {
		if f.Name == indexField {
			continue
		}
		names = append(names, f.Name)
	}
	sort.Strings(names)
	return ":" + strings.Join(names, ",")
}

// zzDump renders a record including its metatable surface, which FormatShort hides.
func zzDump(t typ.Type, depth int) string {
	if t == nil {
		return "<nil>"
	}
	if depth > 4 {
		return "..."
	}
	rec, ok := t.(*typ.Record)
	if !ok {
		return typ.FormatShort(t)
	}
	var b strings.Builder
	b.WriteString("{")
	for i, f := range rec.Fields {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(f.Name)
		b.WriteString(":")
		b.WriteString(typ.FormatShort(f.Type))
	}
	b.WriteString("}")
	if rec.Metatable != nil {
		b.WriteString(" MT=")
		b.WriteString(zzDump(rec.Metatable, depth+1))
	}
	return b.String()
}
