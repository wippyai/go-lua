package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func callPlaceholderBindings(facts factflow.Facts, resolver *visibility.Resolver, site factflow.CallSiteView) []pathdom.Path {
	var bindings []pathdom.Path
	offset := 0
	if receiverPath, ok := site.ReceiverPath(); ok {
		bindings = bindPlaceholderPath(bindings, 0, receiverPath)
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := callSourcePath(facts, resolver, source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, i+offset, sourcePath)
		return true
	})
	return bindings
}

func callArgumentPlaceholderBindings(facts factflow.Facts, resolver *visibility.Resolver, site factflow.CallSiteView) []pathdom.Path {
	var bindings []pathdom.Path
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		sourcePath, ok := callSourcePath(facts, resolver, source)
		if !ok || sourcePath.IsEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, i, sourcePath)
		return true
	})
	return bindings
}

func callSourcePath(facts factflow.Facts, resolver *visibility.Resolver, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		return facts.ExpressionPathRef(source.ExprRef)
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" || resolver == nil {
		return pathdom.Path{}, false
	}
	ks := resolver.KeySpace()
	if ks == nil {
		return pathdom.Path{}, false
	}
	key, ok := ks.FromStateKey(source.PathKey)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{
		Symbol:   key.Sym,
		Segments: ks.Segments(key),
	}, true
}

func callReturnSlotBindings(site factflow.CallSiteView) []pathdom.Path {
	var bindings []pathdom.Path
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		if target.ResultIndex() < 0 || target.TargetPathEmpty() {
			return true
		}
		bindings = bindPlaceholderPath(bindings, target.ResultIndex(), target.TargetPathRef())
		return true
	})
	return bindings
}

func bindPlaceholderPath(bindings []pathdom.Path, index int, p pathdom.Path) []pathdom.Path {
	if index < 0 || p.IsEmpty() {
		return bindings
	}
	for len(bindings) <= index {
		bindings = append(bindings, pathdom.Path{})
	}
	bindings[index] = p
	return bindings
}
