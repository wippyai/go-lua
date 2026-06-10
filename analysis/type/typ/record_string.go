package typ

import "strings"

func (r *Record) String() string {
	return r.strCache.get(func() string {
		var sb strings.Builder

		sb.WriteString("{")

		for i, f := range r.Fields {
			if i > 0 {
				sb.WriteString(", ")
			}

			if f.Readonly {
				sb.WriteString("readonly ")
			}

			sb.WriteString(f.Name)

			if f.Optional {
				sb.WriteString("?")
			}

			sb.WriteString(": ")
			if f.Type != nil {
				sb.WriteString(f.Type.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		for i, member := range r.StaticMembers {
			if len(r.Fields) > 0 || i > 0 {
				sb.WriteString(", ")
			}
			if member.Readonly {
				sb.WriteString("readonly ")
			}
			writeStaticMemberKey(&sb, member)
			if member.Optional {
				sb.WriteString("?")
			}
			sb.WriteString(": ")
			if member.Type != nil {
				sb.WriteString(member.Type.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		if r.HasMapComponent() {
			if len(r.Fields) > 0 || len(r.StaticMembers) > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("[")
			if r.MapKey != nil {
				sb.WriteString(r.MapKey.String())
			} else {
				sb.WriteString("unknown")
			}
			sb.WriteString("]: ")
			if r.MapValue != nil {
				sb.WriteString(r.MapValue.String())
			} else {
				sb.WriteString("unknown")
			}
		}

		if r.Open {
			if len(r.Fields) > 0 || len(r.StaticMembers) > 0 || r.HasMapComponent() {
				sb.WriteString(", ")
			}
			sb.WriteString("...")
		}

		sb.WriteString("}")

		return sb.String()
	})
}
