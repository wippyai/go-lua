package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

func directCallResultOwner(result *body.Result, source sourceprovenance.ASTSource) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return false
	}
	return directCallPointResultOwner(result, source.CallPoint)
}

func directCallPointResultOwner(result *body.Result, point cfg.Point) bool {
	site, ok := result.CallSite(point)
	if !ok || site.CalleeSymbol() == 0 {
		return false
	}
	if site.CalleeMemberAccess() {
		if _, ok := result.CallSignatureType(site); !ok {
			return false
		}
	}
	return true
}
