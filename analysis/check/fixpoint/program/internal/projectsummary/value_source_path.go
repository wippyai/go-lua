package projectsummary

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func valueSourcePath(result ResultReader, pathReader expressionPathRefReader, source factflow.ValueSource) (pathdom.Path, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr && pathReader != nil {
		p, ok := pathReader.ExpressionPathRef(source.ExprRef)
		return p, ok && !p.IsEmpty()
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" || result == nil {
		return pathdom.Path{}, false
	}
	return pathFromSourcePathKey(result.KeySpace(), source.PathKey)
}

func pathFromSourcePathKey(ks *keyspace.KeySpace, raw pathdom.PathKey) (pathdom.Path, bool) {
	if ks == nil || raw == "" {
		return pathdom.Path{}, false
	}
	key, ok := ks.FromStateKey(raw)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{
		Symbol:   key.Sym,
		Segments: ks.Segments(key),
	}, true
}
