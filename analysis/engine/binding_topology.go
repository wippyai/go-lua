package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// committedSourceCarrier is the immutable source value a committed program
// owns: the sealed Batch its rows were admitted into, the key that Batch sealed
// to, and the spec the topology was sealed from.
type committedSourceCarrier struct {
	batch     *equation.Batch
	sourceKey composition.Key
	spec      equation.TopologySpec
}

func (carrier *committedSourceCarrier) valid() bool {
	if carrier == nil || !carrier.sourceKey.Available() {
		return false
	}
	return carrier.batch != nil && carrier.batch.Sealed() && carrier.batch.Key() == carrier.sourceKey && carrier.spec.Batch == carrier.batch
}

type bindingSemanticRowKind uint8

const (
	bindingSemanticPoint bindingSemanticRowKind = iota + 1
	bindingSemanticMember
	bindingSemanticQuery
	bindingSemanticActivation
)

type bindingSemanticRows struct {
	ids                   map[identity.ContentID]bindingSemanticRowKind
	points                map[identity.ContentID]equation.PointRef
	pointAt               map[equation.Site]identity.ContentID
	members               map[identity.ContentID]equation.RuleRef
	memberAt              map[equation.RuleRef]identity.ContentID
	queries               map[identity.ContentID]uint64
	activations           map[identity.ContentID]equation.RuleRef
	activationAt          map[equation.RuleRef]identity.ContentID
	materializationAt     map[equation.TemplateMaterialization]equation.RuleRef
	directCandidateAt     map[equation.DirectActivationCandidate]equation.RuleRef
	directCandidateKey    map[composition.Key]equation.RuleRef
	activationCandidates  map[equation.RuleRef]uint64
	activationExpected    map[equation.RuleRef]uint64
	activationApplication map[equation.RuleRef]composition.Key
}
