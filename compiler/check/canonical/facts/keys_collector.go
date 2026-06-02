package facts

import "github.com/wippyai/go-lua/compiler/check/domain/keyscoll"

// collectKeysCollectors derives structural keys-collector facts for module
// functions. The recognizer is deterministic over each graph's evidence and
// produces a finite fact consumed by indexed-iteration key provenance.
func collectKeysCollectors(p Program) []keysCollectorRow {
	if p.Evidence == nil {
		return nil
	}
	var out []keysCollectorRow
	for _, r := range p.Refs {
		g := graphOf(p, r)
		if g == nil {
			continue
		}
		info := keyscoll.DetectKeysCollector(g, p.Evidence(g))
		if info == nil {
			continue
		}
		out = append(out, keysCollectorRow{
			FuncRef: r,
			Info:    *info,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
