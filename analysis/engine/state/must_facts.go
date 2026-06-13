package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func (s State) ReadPathStaticMember(pathKey pathdom.PathKey) (product.Value, bool) {
	return s.pathEvidence.ReadPathStaticMember(pathKey)
}

func (s State) WritePathStaticMember(pathKey pathdom.PathKey, value product.Value) State {
	pathEvidence, reachable := s.pathEvidence.WritePathStaticMember(pathKey, value)
	if !reachable {
		return s
	}
	out := s.reachable()
	out.pathEvidence = pathEvidence
	return out
}
