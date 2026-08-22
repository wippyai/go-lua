package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

type bindingSemanticRowKind uint8

const (
	bindingSemanticPoint bindingSemanticRowKind = iota + 1
	bindingSemanticMember
	bindingSemanticQuery
	bindingSemanticActivation
)

type bindingSemanticRows struct {
	// Only the four published address planes cross into the directory sealer.
	// Candidate/materialization rows remain owned by equation.Topology and
	// are not mirrored in this engine address value.
	points      map[identity.ContentID]equation.PointRef
	members     map[identity.ContentID]equation.RuleRef
	queries     map[identity.ContentID]uint64
	activations map[identity.ContentID]equation.RuleRef
}
