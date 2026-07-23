// Package diffharness compares two solves of the same analysis unit.
package diffharness

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"sort"

	"github.com/wippyai/go-lua/analysis/diagnostic"
)

// UnitResult is the observable output of one solve. Manifest is the exact
// canonical wire payload; Diagnostics are compared after rendering so the
// report follows the user-visible diagnostic contract.
type UnitResult struct {
	Diagnostics   []diagnostic.Diagnostic
	Manifest      []byte
	RenderOptions diagnostic.RenderOptions
}

type line struct {
	Kind     string `json:"kind"`
	Side     string `json:"side"`
	Rendered string `json:"rendered,omitempty"`
	Bytes    string `json:"bytes,omitempty"`
}

// Diff returns a deterministic JSONL report. Equal results return nil. Each
// diagnostic line contains its rendered form; manifest lines contain the exact
// bytes encoded as base64, avoiding newline or UTF-8 normalization.
func Diff(before, after UnitResult) []byte {
	var lines []line
	beforeDiagnostics := rendered(before)
	afterDiagnostics := rendered(after)
	i, j := 0, 0
	for i < len(beforeDiagnostics) || j < len(afterDiagnostics) {
		switch {
		case i == len(beforeDiagnostics):
			lines = append(lines, line{Kind: "diagnostic", Side: "after", Rendered: afterDiagnostics[j]})
			j++
		case j == len(afterDiagnostics):
			lines = append(lines, line{Kind: "diagnostic", Side: "before", Rendered: beforeDiagnostics[i]})
			i++
		case beforeDiagnostics[i] == afterDiagnostics[j]:
			i++
			j++
		case beforeDiagnostics[i] < afterDiagnostics[j]:
			lines = append(lines, line{Kind: "diagnostic", Side: "before", Rendered: beforeDiagnostics[i]})
			i++
		default:
			lines = append(lines, line{Kind: "diagnostic", Side: "after", Rendered: afterDiagnostics[j]})
			j++
		}
	}
	if !bytes.Equal(before.Manifest, after.Manifest) {
		lines = append(lines,
			line{Kind: "manifest", Side: "before", Bytes: base64.StdEncoding.EncodeToString(before.Manifest)},
			line{Kind: "manifest", Side: "after", Bytes: base64.StdEncoding.EncodeToString(after.Manifest)},
		)
	}
	if len(lines) == 0 {
		return nil
	}
	var out bytes.Buffer
	for _, entry := range lines {
		encoded, _ := json.Marshal(entry)
		out.Write(encoded)
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func rendered(result UnitResult) []string {
	out := make([]string, 0, len(result.Diagnostics))
	for _, item := range result.Diagnostics {
		out = append(out, diagnostic.Render(item, result.RenderOptions))
	}
	sort.Strings(out)
	return out
}
