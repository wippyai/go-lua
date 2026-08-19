package target

import (
	"github.com/wippyai/go-lua/analysis/identity"
	bootvalue "github.com/wippyai/go-lua/analysis/program/target/boot"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	protocolvalue "github.com/wippyai/go-lua/analysis/program/target/protocol"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// operationRow is the Target aggregate's cross-subsystem relation index. The
// operation package owns operation handles, outcomes, callbacks, and every
// continuation/output relation; this row retains only Target-owned invocation
// and effect projections.
type operationRow struct {
	subedges        indexRange
	subedgeRelation uint32
	releases        indexRange
	effects         indexRange
	effectTail      vocabulary.RowTail
	effectVar       vocabulary.RowVar
}

type subedgeRelationRow struct {
	operand       vocabulary.ValueFormal
	selector      uint32
	subedge       vocabulary.SubedgeID
	resultOutcome uint32
	result        uint32
	effects       indexRange
}

type callbackRow struct {
	function   vocabulary.InputSource
	admission  schematype.CallableAdmission
	arguments  vocabulary.Values
	outcomes   [5]vocabulary.Values
	subedge    vocabulary.SubedgeID
	effects    indexRange
	effectTail vocabulary.RowTail
	effectVar  vocabulary.RowVar
	release    uint32
}

type subedgeRow struct {
	owner            vocabulary.Operation
	role             uint32
	family           vocabulary.SubedgeFamily
	callee           vocabulary.SubedgeCalleeKind
	callback         vocabulary.CallbackID
	readRoot         vocabulary.InitialRoot
	readKey          vocabulary.ExactKey
	metaKey          vocabulary.ExactKey
	admission        schematype.CallableAdmission
	arguments        vocabulary.Values
	ruleEntry        bool
	argumentOrigins  indexRange
	outcomes         [5]vocabulary.Values
	admissionFailure vocabulary.Values
	admissionRoute   subedgeRouteRow
	routes           [5]subedgeRouteRow
}

type subedgeArgumentOriginRow struct {
	segment vocabulary.ArgumentSegment
	index   uint32
	kind    vocabulary.ArgumentSource
	source  vocabulary.InputSource
}

type subedgeRouteRow struct {
	route       vocabulary.SubedgeRoute
	adjustment  vocabulary.Adjustment
	result      vocabulary.Values
	placement   vocabulary.Placement
	offset      uint32
	outcome     uint32
	subedge     vocabulary.SubedgeID
	destination vocabulary.Values
}

type callbackReleaseRow struct {
	callback     vocabulary.CallbackID
	operation    vocabulary.Operation
	input        vocabulary.ValueFormal
	outcome      uint32
	mode         vocabulary.CallbackReleaseMode
	zeroBehavior vocabulary.CallbackReleaseZeroBehavior
	zeroOutcome  uint32
}

// effectOwner discriminates the owner of one row in the flat effect table. The
// table is indexed by both operationRow.effects and callbackRow.effects, so the
// owner is what tells the two populations apart without walking either index.
type effectOwner uint8

const (
	effectOwnerOperation effectOwner = iota + 1
	effectOwnerCallback
)

type effectRow struct {
	owner          effectOwner
	target         vocabulary.Operation
	values         indexRange
	types          indexRange
	valuesVar      indexRange
	rows           indexRange
	publication    PublicationEffectDescriptor
	hasPublication bool
}

type indexRange struct{ start, end uint32 }

func (r indexRange) len() int { return int(r.end - r.start) }

// operation resolves one Target-owned invocation/effect row. Operation.Core
// owns the canonical operation handle and all operation-owned outcome,
// continuation, and output relations.
func (c *Contract) operation(op vocabulary.Operation) (operationRow, bool) {
	if c == nil || op == 0 || uint64(op) > uint64(len(c.operations)) {
		return operationRow{}, false
	}
	if _, ok := c.Operations.OperationAt(int(op) - 1); !ok {
		return operationRow{}, false
	}
	return c.operations[uint32(op)-1], true
}

// Contract is immutable after Seal. Every slice is private and every public
// hot query returns only scalar handles or values.
type Contract struct {
	bootvalue.Table
	Operations             operationvalue.Core
	operations             []operationRow
	effects                []effectRow
	effectVals             []vocabulary.ValueFormal
	effectType             []vocabulary.TypeFormal
	effectVars             []vocabulary.ValuesVar
	effectRows             []vocabulary.RowVar
	callbacks              []callbackRow
	subedges               []subedgeRow
	subedgeOrigins         []subedgeArgumentOriginRow
	subedgeRelations       []subedgeRelationRow
	subedgeRelationEffects []uint32
	callbackReleases       []callbackReleaseRow
	protocols              protocolvalue.Table
	exactKeys              exactkey.Table
	counts                 denominator.CountRows
	// identityColumns carries the identity plane's own columns. The layout is
	// declared with the rest of the model; the values are written and read only
	// by the identity altitude.
	identityColumns
	sealed bool
}

// identityColumns are the content identities the identity altitude computes
// over the published read surface and seals with the contract. They are not a
// second graph authority: each row is a cached canonical descriptor indexed
// only by the existing dense Target tables.
type identityColumns struct {
	// Effect identity columns are projections of the existing
	// operation/callback/effect tables. Effect descriptors intentionally have
	// no inverse index: duplicate authored occurrences are distinct evidence,
	// while their descriptor identity is the shared semantic quotient.
	effectOperationIDs      []identity.ContentID
	effectDescriptorIDs     []identity.ContentID
	effectOccurrenceIDs     []identity.ContentID
	operationEffectFamilies []identity.ContentID
	callbackEffectFamilies  []identity.ContentID
	operationContentIDs     []identity.ContentID
	callbackSelectors       []identity.ContentID
	callbackContentIDs      []identity.ContentID
	callbackContentIndex    []callbackContentIDRow
	outcomeSelectors        []identity.ContentID
	outcomeContentIDs       []identity.ContentID
	transferContentIDs      []identity.ContentID
	transferOutcomeIDs      []identity.ContentID
	resumeContentIDs        []identity.ContentID
	resumeContentIndex      []resumeContentIDRow
	inputFormalRanges       []indexRange
	inputFormalIDs          []identity.ContentID
	inputFormalIndex        []inputFormalIDRow
	outcomeResultRanges     []indexRange
	outcomeResultIDs        []identity.ContentID
	outcomeResultIndex      []outcomeResultIDRow
}

type inputFormalIDRow struct {
	id     identity.ContentID
	op     vocabulary.Operation
	formal vocabulary.ValueFormal
}

type outcomeResultIDRow struct {
	id      identity.ContentID
	op      vocabulary.Operation
	outcome uint32
	result  uint32
}

// callbackContentIDRow and resumeContentIDRow are the immutable sorted
// reverse columns for the Target-owned portable relation identities. The
// forward columns remain dense by their existing sealed handles; these rows
// retain only the existing sealed relation handle. Both callback and resume
// owners are issued by operation.Core.
type callbackContentIDRow struct {
	id       identity.ContentID
	callback vocabulary.CallbackID
}

type resumeContentIDRow struct {
	id     identity.ContentID
	resume vocabulary.ResumeID
}
