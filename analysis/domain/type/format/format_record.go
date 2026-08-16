package format

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/type/internal/recursion"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

func (f *formatter) formatRecord(r *typ.Record, depth int, guard recursion.Guard) {
	f.write("{")
	limit := minInt(len(r.Fields), f.opts.MaxRecordFields)
	for i := 0; i < limit; i++ {
		if i > 0 {
			f.write(", ")
		}
		field := r.Fields[i]
		if field.Readonly {
			f.write("readonly ")
		}
		f.write(field.Name)
		if field.Optional {
			f.write("?")
		}
		f.write(": ")
		f.formatType(field.Type, depth+1, guard)
	}
	if limit < len(r.Fields) {
		f.write(", ...")
	}
	for i, member := range r.StaticMembers {
		if len(r.Fields) > 0 || i > 0 {
			f.write(", ")
		}
		if member.Readonly {
			f.write("readonly ")
		}
		var key strings.Builder
		typ.WriteStaticMemberKey(&key, member)
		f.write(key.String())
		if member.Optional {
			f.write("?")
		}
		f.write(": ")
		f.formatType(member.Type, depth+1, guard)
	}
	if r.HasMapComponent() {
		if len(r.Fields) > 0 || len(r.StaticMembers) > 0 {
			f.write(", ")
		}
		f.write("[")
		f.formatType(r.MapKey, depth+1, guard)
		f.write("]: ")
		f.formatType(r.MapValue, depth+1, guard)
	}
	if r.Open {
		if len(r.Fields) > 0 || r.HasMapComponent() {
			f.write(", ")
		}
		f.write("...")
	}
	f.write("}")
}
