package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
)

func directCallResultOwner(result *body.Result, source sourceprovenance.ASTSource) bool {
	if result == nil || source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint {
		return false
	}
	fact, ok := result.Call(source.CallPoint)
	if !ok || fact.Call == nil {
		return false
	}
	return directCallPointResultOwner(result, source.CallPoint, fact)
}

func directCallPointResultOwner(result *body.Result, point cfg.Point, fact semantics.CallFact) bool {
	site, ok := result.CallSite(point)
	if !ok || site.CalleeSymbol() == 0 {
		return false
	}
	if directCallSiteUsesMemberAccess(result, site, fact) {
		if !hasTypedCallSignature(result, site) {
			return false
		}
	}
	return true
}

func hasTypedCallSignature(result *body.Result, site factflow.CallSite) bool {
	sig, ok := result.CallSignature(site)
	return ok && sig.Type != nil
}
