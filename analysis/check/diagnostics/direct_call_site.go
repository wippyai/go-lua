package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/path"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
)

func callMemberAccessInfoForSite(site factflow.CallSite, fact semantics.CallFact) (callMemberAccessData, bool) {
	if fact.Call == nil {
		return callMemberAccessData{}, false
	}
	receiver, member, ok := site.CalleeMemberAccessPath()
	if !ok {
		return callMemberAccessData{}, false
	}
	if memberSegmentDisplay(member) == "" {
		return callMemberAccessData{}, false
	}
	return callMemberAccessData{receiver: receiver, member: member, call: fact.Call}, true
}

func directCallDisplayName(result *body.Result, site factflow.CallSite) string {
	if result != nil {
		if signatureName, ok := result.CallSignatureName(site); ok && signatureName != "" {
			return signatureName
		}
		if callPath := site.CalleePathRef(); !callPath.IsEmpty() {
			return displayPath(result, callPath)
		}
		if name := result.SymbolName(site.CalleeSymbol()); name != "" {
			return name
		}
	}
	return "call target"
}

func displayPath(result *body.Result, pth path.Path) string {
	if pth.IsEmpty() {
		return ""
	}
	display := pth.Clone()
	if result != nil {
		display.Root = pth.DisplayRoot(result.SymbolName)
	}
	return display.String()
}
