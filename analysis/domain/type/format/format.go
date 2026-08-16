package format

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/domain/type/internal/recursion"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// formatterSafetyDepth bounds presentation only when a caller disables every
// explicit output budget. It does not participate in semantic type reasoning.
const formatterSafetyDepth = 64

// Options controls budgeted type rendering for diagnostics.
// Limits are best-effort; rendering may truncate with "..." when exceeded.
type Options struct {
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

// DefaultOptions keeps diagnostics readable while avoiding huge output.
var DefaultOptions = Options{
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

// Short renders a type for diagnostics with bounded output size.
func Short(t typ.Type) string {
	return Type(t, DefaultOptions)
}

// Type renders a type using the provided options.
func Type(t typ.Type, opts Options) string {
	f := formatter{
		opts: opts,
	}
	guardDepth := opts.MaxDepth
	if guardDepth <= 0 {
		guardDepth = opts.MaxNodes
	}
	if guardDepth <= 0 {
		guardDepth = formatterSafetyDepth
	}
	f.formatType(t, 0, recursion.NewGuard(guardDepth))
	return f.string()
}

type formatter struct {
	opts      Options
	nodes     int
	bytes     int
	truncated bool
	sb        strings.Builder
}

func (f *formatter) string() string {
	s := f.sb.String()
	if f.truncated && !strings.HasSuffix(s, "...") {
		if f.opts.MaxBytes <= 0 || f.bytes+3 <= f.opts.MaxBytes {
			f.sb.WriteString("...")
			s = f.sb.String()
		} else if f.opts.MaxBytes > 0 && len(s) >= 3 {
			s = s[:len(s)-3] + "..."
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
