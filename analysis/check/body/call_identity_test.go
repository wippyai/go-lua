package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
)

func onlyCallSignature(t *testing.T, result *Result) (signature.Function, bool) {
	t.Helper()
	point, ok := onlySignatureCallPoint(t, result)
	if !ok {
		return signature.Function{}, false
	}
	return result.CallSignatureAtPoint(point)
}

func onlySignatureCallPoint(t *testing.T, result *Result) (cfg.Point, bool) {
	t.Helper()
	return onlySignatureCallPointNamed(t, result, "")
}

func onlySignatureCallPointNamed(t *testing.T, result *Result, wantName string) (cfg.Point, bool) {
	t.Helper()
	graph := result.Graph()
	if graph == nil {
		t.Fatalf("missing graph")
	}
	var out cfg.Point
	count := 0
	for _, point := range graph.RPO() {
		if _, ok := result.CallSignatureAtPoint(point); !ok {
			continue
		}
		if wantName != "" {
			name, ok := result.CallSignatureNameAtPoint(point)
			if !ok || name != wantName {
				continue
			}
		}
		out = point
		count++
	}
	if count > 1 {
		t.Fatalf("call sites = %d, want at most one", count)
	}
	return out, count == 1
}
