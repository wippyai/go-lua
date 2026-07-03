package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// boundaryPathProjector owns the path rebasing policy for facts that leave a
// function summary. It maps point-local state keys back to normal-return
// boundary paths: parameters become $N, return-derived values become ret[N], and
// persistent captured sinks remain concrete symbol paths.
type boundaryPathProjector struct {
	ks       *keyspace.KeySpace
	params   []path.Path
	returns  []exitFactReturnPath
	captured map[symbol.ID]struct{}
}

func newBoundaryPathProjector(
	ks *keyspace.KeySpace,
	params []path.Path,
	returns []exitFactReturnPath,
	captured map[symbol.ID]struct{},
) boundaryPathProjector {
	return boundaryPathProjector{
		ks:       ks,
		params:   params,
		returns:  returns,
		captured: captured,
	}
}

func (p boundaryPathProjector) Params() []path.Path {
	return p.params
}

func (p boundaryPathProjector) PlaceholderPath(pathKey path.PathKey) (path.Path, bool) {
	if pathKey == "" || len(p.params) == 0 {
		return path.Path{}, false
	}
	if placeholder, ok := pathaddr.PlaceholderPathFromKey(pathKey); ok {
		index := placeholder.PlaceholderIndex()
		if index < 0 || index >= len(p.params) || p.params[index].IsEmpty() {
			return path.Path{}, false
		}
		return placeholder, true
	}
	localPath, ok := pathaddr.LocalPathFromKey(pathKey)
	if !ok {
		return path.Path{}, false
	}
	return placeholderForParameterPath(p.params, localPath)
}

func (p boundaryPathProjector) StatePath(stateKey pathaddr.StateKey) (path.Path, bool) {
	if stateKey == "" || p.ks == nil {
		return path.Path{}, false
	}
	k, ok := p.ks.FromStateKey(stateKey.PathKey())
	if !ok {
		return path.Path{}, false
	}
	return p.statePathFromKey(k)
}

func (p boundaryPathProjector) KeyspacePlaceholderPath(stateKey keyspace.Key) (path.Path, bool) {
	if p.ks == nil {
		return path.Path{}, false
	}
	statePath, ok := p.ks.StatePath(stateKey)
	if !ok {
		return path.Path{}, false
	}
	if statePath.IsPlaceholder() {
		index := statePath.PlaceholderIndex()
		if index < 0 || index >= len(p.params) || p.params[index].IsEmpty() {
			return path.Path{}, false
		}
		return statePath, true
	}
	if statePath.Symbol == 0 || statePath.Version == 0 {
		return path.Path{}, false
	}
	return placeholderForParameterPath(p.params, statePath)
}

func (p boundaryPathProjector) KeyspaceStatePath(stateKey keyspace.Key) (path.Path, bool) {
	if p.ks == nil {
		return path.Path{}, false
	}
	return p.statePathFromKey(stateKey)
}

func (p boundaryPathProjector) PathKeyStatePath(pathKey path.PathKey) (path.Path, bool) {
	stateKey, ok := pathaddr.StateKeyFromPathKey(pathKey)
	if !ok {
		return path.Path{}, false
	}
	return p.StatePath(stateKey)
}

func (p boundaryPathProjector) RelConstraintFact(constraint state.RelConstraint) (callboundary.RelConstraintFact, bool) {
	a, ok := p.relConstraintOperand(constraint.A)
	if !ok {
		return callboundary.RelConstraintFact{}, false
	}
	c, ok := p.relConstraintOperand(constraint.C)
	if !ok {
		return callboundary.RelConstraintFact{}, false
	}
	out := callboundary.RelConstraintFact{
		CoA: constraint.CoA,
		A:   a,
		C:   c,
		K:   constraint.K,
	}
	if constraint.B.IsValid() && constraint.CoB != 0 {
		b, ok := p.relConstraintOperand(constraint.B)
		if !ok {
			return callboundary.RelConstraintFact{}, false
		}
		out.CoB = constraint.CoB
		out.B = b
	}
	return out, true
}

func (p boundaryPathProjector) relConstraintOperand(operand state.RelOperand) (callboundary.RelOperand, bool) {
	target, ok := p.StatePath(operand.StateKey())
	if !ok {
		return callboundary.RelOperand{}, false
	}
	return callboundary.RelOperand{Path: target, IsLength: operand.IsLength()}, true
}

func (p boundaryPathProjector) statePathFromKey(stateKey keyspace.Key) (path.Path, bool) {
	statePath, ok := p.ks.StatePath(stateKey)
	if !ok {
		return path.Path{}, false
	}
	if statePath.IsPlaceholder() {
		index := statePath.PlaceholderIndex()
		if index < 0 || index >= len(p.params) || p.params[index].IsEmpty() {
			return path.Path{}, false
		}
		return statePath, true
	}
	if statePath.ReturnSlotIndex() >= 0 {
		return statePath, true
	}
	if statePath.Symbol != 0 {
		if statePath.Version == 0 && len(statePath.Segments) != 0 {
			return path.Path{}, false
		}
		return p.localStatePath(statePath)
	}
	return path.Path{}, false
}

func (p boundaryPathProjector) localStatePath(localPath path.Path) (path.Path, bool) {
	if target, ok := boundaryPathForReturnPath(p.returns, localPath); ok {
		return target, true
	}
	if target, ok := placeholderForParameterPath(p.params, localPath); ok {
		return target, true
	}
	if _, ok := p.captured[localPath.Symbol]; ok {
		return localPath, true
	}
	return path.Path{}, false
}
