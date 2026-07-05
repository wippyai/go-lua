package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func valueSourcePath(facts factflow.Facts, resolver *visibility.Resolver, source factflow.ValueSource) (pathdom.Path, bool) {
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

func (r *Result) valueSourcePath(source factflow.ValueSource) (pathdom.Path, bool) {
	if r == nil {
		return pathdom.Path{}, false
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		return r.ExpressionPathRef(source.ExprRef)
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" {
		return pathdom.Path{}, false
	}
	key, ok := r.KeySpace().FromStateKey(source.PathKey)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{
		Symbol:   key.Sym,
		Segments: r.KeySpace().Segments(key),
	}, true
}

func valueSourcePathStateKey(resolver *visibility.Resolver, point cfg.Point, p pathdom.Path) (pathaddr.StateKey, bool) {
	if rootlessSymbolRoot(p) {
		return visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
	}
	return visibility.AddressAt(resolver, point, p).VisibleStateKey()
}

func valueSourcePathKeyspaceKey(resolver *visibility.Resolver, point cfg.Point, p pathdom.Path) (keyspace.Key, bool) {
	if rootlessSymbolRoot(p) {
		return visibility.AddressAt(resolver, point, p).RootOrVisibleKeyspaceKey()
	}
	return visibility.AddressAt(resolver, point, p).VisibleKeyspaceKey()
}

func rootlessSymbolRoot(p pathdom.Path) bool {
	return p.Root == "" && p.Symbol != 0 && len(p.Segments) == 0
}
