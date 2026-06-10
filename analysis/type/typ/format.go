package typ

import "strings"

// FormatOptions controls budgeted type rendering for diagnostics.
// Limits are best-effort; rendering may truncate with "..." when exceeded.
type FormatOptions struct {
	MaxDepth        int
	MaxNodes        int
	MaxUnionMembers int
	MaxRecordFields int
	MaxTupleElems   int
	MaxTypeParams   int
	MaxParams       int
	MaxReturns      int
	MaxBytes        int
}

// DefaultFormatOptions keeps diagnostics readable while avoiding huge output.
var DefaultFormatOptions = FormatOptions{
	MaxDepth:        6,
	MaxNodes:        200,
	MaxUnionMembers: 6,
	MaxRecordFields: 8,
	MaxTupleElems:   8,
	MaxTypeParams:   6,
	MaxParams:       8,
	MaxReturns:      6,
	MaxBytes:        800,
}

// FormatShort renders a type for diagnostics with bounded output size.
func FormatShort(t Type) string {
	return Format(t, DefaultFormatOptions)
}

// Format renders a type using the provided options.
func Format(t Type, opts FormatOptions) string {
	f := formatter{
		opts: opts,
	}
	f.formatType(t, 0, NewGuard())
	return f.string()
}

type formatter struct {
	opts      FormatOptions
	nodes     int
	bytes     int
	truncated bool
	sb        strings.Builder
}

func (f *formatter) string() string {
	s := f.sb.String()
	if f.truncated && !strings.HasSuffix(s, "...") {
		if f.bytes+3 <= f.opts.MaxBytes {
			f.sb.WriteString("...")
			s = f.sb.String()
		}
	}
	return s
}

func (f *formatter) write(s string) {
	if f.truncated {
		return
	}
	if f.opts.MaxBytes > 0 && f.bytes >= f.opts.MaxBytes {
		f.truncated = true
		return
	}
	if f.opts.MaxBytes > 0 {
		remaining := f.opts.MaxBytes - f.bytes
		if remaining <= 0 {
			f.truncated = true
			return
		}
		if len(s) > remaining {
			f.sb.WriteString(s[:remaining])
			f.bytes += remaining
			f.truncated = true
			return
		}
	}
	f.sb.WriteString(s)
	f.bytes += len(s)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
