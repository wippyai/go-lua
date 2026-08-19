package engine

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/identity"
)

// BindingTopology is the Binding-issued Link lowering witness. It authenticates
// concrete sealed Factor owners and the exact equation Topology before graph
// attachment is possible.
type BindingTopology struct {
	self              *BindingTopology
	topology          *equation.Topology
	state             *schemaBindingState
	authority         *schemaBindingAuthority
	factors           []schemaFactorBinding
	carrier           bindingTopologyCarrier
	directory         *semanticDirectory
	nativeCallStages  map[artifactMountedRuleOccurrence]artifactNativeCallStage
	artifactBacked    bool
	bootstrapOwner    identity.ContentID
	bootstrapPoint    identity.ContentID
	bootstrapSemantic identity.ContentID
}

// bindingTopologyCarrier is the sealed source value a committed
// BindingTopology owns: the Batch its rows were admitted into, the key that
// Batch sealed to, and the spec the topology was sealed from. It is finished
// at construction and only ever released, never reopened.
type bindingTopologyCarrier struct {
	batch     *equation.Batch
	sourceKey composition.Key
	spec      equation.TopologySpec
}

type bindingTopologyBuilderPhase uint8

const (
	bindingTopologyBuilderSourcesOpen bindingTopologyBuilderPhase = iota + 1
	bindingTopologyBuilderTopologyOpen
	bindingTopologyBuilderAborted
)

type bindingTopologyBuilderState struct {
	mu                  sync.Mutex
	state               *schemaBindingState
	batch               *equation.Batch
	sourceKey           composition.Key
	spec                equation.TopologySpec
	phase               bindingTopologyBuilderPhase
	authority           *schemaBindingAuthority
	semantic            *bindingSemanticRows
	directTransportSets map[directActivationTransportSetKey]equation.DirectActivationTransportSet
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

// BindingTopologyBuilder owns the disposable equation rows until Seal, plus
// the sealed admission ledger for one construction.
type BindingTopologyBuilder struct {
	inner   *bindingTopologyBuilderState
	binding *SchemaBinding
	// mountedRows is the immutable mounted-artifact input for this one
	// construction. It is deliberately outside the generic equation state:
	// the builder admits schema rows, while the mounted row table supplies
	// already-issued Program coordinates.
	mountedRows           *mountedArtifactRows
	selectedSurfaceMu     sync.Mutex
	selectedSurfaceAnchor map[equation.Surface]mountedSelectedSurfaceAnchor
	queuedRuleMu          sync.Mutex
	queuedRuleFinalizers  []queuedRuleFinalizer
	queuedQueryMu         sync.Mutex
	queuedQueryBatches    []func(*MountedQueryBatch) bool
	ruleSourceFailureMu   sync.Mutex
	ruleSourceFailure     RuleSourceSealFailure
	finalizerFailureMu    sync.Mutex
	finalizerFailure      RuleFinalizerFailure
	sealFailure           receiptSealFailure
}

func (receipt *BindingTopology) valid() bool {
	if receipt == nil || receipt.self != receipt || receipt.topology == nil || receipt.state == nil || receipt.authority == nil || !receipt.directory.ownedBy(receipt.topology, receipt.state, receipt.authority) || receipt.state.phase != schemaBindingSealed || receipt.state.authority != receipt.authority || receipt.state.schema == nil || !receipt.topology.OwnsComposition(receipt.state.schema.cold) || !receipt.carrier.sourceKey.Available() {
		return false
	}
	ownerAvailable, pointAvailable, semanticAvailable := receipt.bootstrapOwner.Available(), receipt.bootstrapPoint.Available(), receipt.bootstrapSemantic.Available()
	if receipt.artifactBacked {
		semantic := linkBootstrapPointSemanticID(receipt.bootstrapOwner, receipt.bootstrapPoint)
		if !ownerAvailable || !pointAvailable || !semanticAvailable || semantic != receipt.bootstrapSemantic {
			return false
		}
		if _, found := receipt.directory.point(receipt.bootstrapSemantic); !found {
			return false
		}
	} else if ownerAvailable || pointAvailable || semanticAvailable {
		return false
	}
	if receipt.carrier.batch == nil {
		return receipt.carrier.spec.Batch == nil
	}
	return receipt.carrier.batch.Sealed() && receipt.carrier.batch.Key() == receipt.carrier.sourceKey && receipt.carrier.spec.Batch == receipt.carrier.batch
}

func (topology *BindingTopology) releaseArtifact() bool {
	if topology == nil || !topology.valid() {
		return false
	}
	topology.factors = nil
	topology.carrier.batch = nil
	topology.carrier.spec = equation.TopologySpec{}
	return true
}
