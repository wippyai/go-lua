package contract

import (
	"github.com/wippyai/go-lua/analysis/identity"
	bootvalue "github.com/wippyai/go-lua/analysis/program/target/boot"
	"github.com/wippyai/go-lua/analysis/program/target/exactkey"
	operationvalue "github.com/wippyai/go-lua/analysis/program/target/operation"
	protocolvalue "github.com/wippyai/go-lua/analysis/program/target/protocol"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

type indexRange struct{ start, end uint32 }

func (r indexRange) len() int { return int(r.end - r.start) }

// Contract is immutable after Seal. Every slice is private and every public
// hot query returns only scalar handles or values.
type Contract struct {
	bootvalue.Table
	Operations operationvalue.Core
	protocols  protocolvalue.Table
	exactKeys  exactkey.Table
	counts     denominator.CountRows
	// identityColumns carries the identity plane's own columns. The layout is
	// declared with the rest of the model; the values are written and read only
	// by the identity altitude.
	identityColumns
	sealed bool
}

// Protocols returns the immutable protocol query table owned by the Contract.
// The table exposes read-only queries; its storage remains private to this
// package and cannot be replaced through the returned value.
func (c *Contract) Protocols() protocolvalue.Table {
	if c == nil {
		return protocolvalue.Table{}
	}
	return c.protocols
}

// identityColumns are the content identities the identity altitude computes
// over the published read surface and seals with the contract. They are not a
// second graph authority: each row is a cached canonical descriptor indexed
// only by the existing dense Target tables.
type identityColumns struct {
	operationContentIDs  []identity.ContentID
	callbackSelectors    []identity.ContentID
	callbackContentIDs   []identity.ContentID
	callbackContentIndex []callbackContentIDRow
	outcomeSelectors     []identity.ContentID
	outcomeContentIDs    []identity.ContentID
	transferContentIDs   []identity.ContentID
	transferOutcomeIDs   []identity.ContentID
	resumeContentIDs     []identity.ContentID
	resumeContentIndex   []resumeContentIDRow
	inputFormalRanges    []indexRange
	inputFormalIDs       []identity.ContentID
	inputFormalIndex     []inputFormalIDRow
	outcomeResultRanges  []indexRange
	outcomeResultIDs     []identity.ContentID
	outcomeResultIndex   []outcomeResultIDRow
}

type inputFormalIDRow struct {
	index  uint32
	op     vocabulary.Operation
	formal vocabulary.ValueFormal
}

type outcomeResultIDRow struct {
	index   uint32
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
