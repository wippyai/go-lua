package diffreport

import (
	"strings"
	"testing"
)

// FuzzReadJSONL covers the external lint-harness baseline reader. JSONL files
// are supplied by tooling, so malformed or arbitrarily large-looking rows must
// be rejected without panicking.
func FuzzReadJSONL(f *testing.F) {
	for _, seed := range []string{
		`{"suite":"fixture","entry":"main","code":"type.call","severity":"error","file":"main.lua","span":{"start_line":1,"start_col":2},"message":"bad call"}` + "\n",
		`{"target":"external","entry_id":"pkg:main","code":"type.assignment","line":4,"column":8,"message":"bad assignment"}` + "\n",
		"\n\n",
		"{\n",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1<<20 {
			return
		}
		_, _ = ReadJSONL(strings.NewReader(input))
	})
}
