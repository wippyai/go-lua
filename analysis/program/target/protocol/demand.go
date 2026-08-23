package protocol

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/target/operation"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	sealedrows "github.com/wippyai/go-lua/internal/rows"
)

// The callable-requirement authority.
//
// The protocol relations are declared per protocol: a protocol names the
// operations that acquire, require, move, and escape its resource. A consumer
// that decides one call site asks the opposite question - what does this
// callable demand of the resource it is handed - and the protocol-keyed
// relations answer it only by scanning every protocol.
//
// This directory is that opposite direction, sealed once beside the rows it
// projects. It is dense over the operation geometry, so an operation handle
// selects its own obligations by ordinal, and it stores no payload: a row
// addresses the protocol-local relation row that states it, and the reader
// takes the payload from the one relation that owns it. Nothing here can drift
// from the protocol table, because there is nothing here to drift.

// DemandKind is the closed vocabulary of typestate obligations one callable
// operation carries. It is the protocol surface's own relation set read by
// operation: every kind is one of the relations a ProtocolSpec declares, and
// no kind exists that a protocol cannot state.
type DemandKind uint8

const (
	// DemandInvalid is the zero value and is never a declared obligation.
	DemandInvalid DemandKind = iota
	// DemandAcquisition creates a resource at a fixed result slot in a
	// declared initial state.
	DemandAcquisition
	// DemandRequirement reads an input in a declared state and leaves it
	// there. It constrains every arm and discharges no obligation.
	DemandRequirement
	// DemandTransition moves an input out of a declared source state on the
	// operation's declared outcome arms.
	DemandTransition
	// DemandEscape hands an input somewhere the analysis does not follow, so
	// every proof about the resource is discharged there.
	DemandEscape
	demandKindLimit
)

// Available reports whether the kind is one of the declared relations.
func (kind DemandKind) Available() bool {
	return kind > DemandInvalid && kind < demandKindLimit
}

// Demand is one obligation of one callable operation: the protocol whose state
// machine states it, the relation it is stated in, and the protocol-local row
// that carries the payload. Row is the index this kind's own query accepts -
// ProtocolAcquisitionAt, ProtocolRequirementAt, TransitionAt, or EscapeAt - so
// the payload is read from its owning relation rather than copied here.
type Demand struct {
	Protocol vocabulary.Protocol
	Kind     DemandKind
	Row      int
}

// demandRow is the stored projection. It is exactly Demand with the protocol
// and row narrowed to their stored widths; the handle a reader receives is
// rebuilt from it.
type demandRow struct {
	protocol vocabulary.Protocol
	row      uint32
	kind     DemandKind
}

// sealDemands builds the operation-keyed directory from the already-ordered
// protocol drafts. It runs after appendProtocols, so the protocol ordinals and
// the protocol-local row indices it addresses are the sealed ones.
//
// The directory is dense over the operation geometry rather than over the
// operations that happen to declare something: an operation with no obligation
// is a sealed empty span, which is the answer "this callable demands nothing"
// rather than the absence of an answer.
func (c *Table) sealDemands(operations operation.Core) error {
	count := operations.OperationCount()
	if _, err := vocabulary.CheckedStoredLength("protocol demand operations", count); err != nil {
		return err
	}
	perOperation := make([][]demandRow, count)
	appendDemand := func(owner vocabulary.Operation, row demandRow) error {
		if owner == 0 || uint64(owner) > uint64(count) {
			return fmt.Errorf("target/protocol: demand names operation %d outside the sealed geometry", owner)
		}
		perOperation[owner-1] = append(perOperation[owner-1], row)
		return nil
	}
	for index := 0; index < c.ProtocolCount(); index++ {
		protocol, ok := c.ProtocolAt(index)
		if !ok {
			return fmt.Errorf("target/protocol: protocol %d has no handle", index)
		}
		if err := c.appendProtocolDemands(protocol, appendDemand); err != nil {
			return err
		}
	}
	var builder sealedrows.PoolBuilder[demandRow]
	spans := make([]sealedrows.Span, count)
	for index, rows := range perOperation {
		span, err := appendPool(&builder, rows, "protocol demand table")
		if err != nil {
			return err
		}
		spans[index] = span
	}
	c.demands = builder.Seal()
	c.operationDemands = sealedrows.NewRows(spans)
	return nil
}

// appendProtocolDemands projects one protocol's four relations in the order
// the kind vocabulary declares them, so the directory's per-operation order is
// a function of the sealed table alone.
func (c *Table) appendProtocolDemands(protocol vocabulary.Protocol, appendDemand func(vocabulary.Operation, demandRow) error) error {
	for row := 0; row < c.ProtocolAcquisitionCount(protocol); row++ {
		owner, _, _, _, ok := c.ProtocolAcquisitionAt(protocol, row)
		if !ok {
			return fmt.Errorf("target/protocol: protocol %d acquisition %d is unavailable", protocol, row)
		}
		if err := appendDemand(owner, demandRow{protocol: protocol, kind: DemandAcquisition, row: uint32(row)}); err != nil {
			return err
		}
	}
	for row := 0; row < c.ProtocolRequirementCount(protocol); row++ {
		owner, _, _, ok := c.ProtocolRequirementAt(protocol, row)
		if !ok {
			return fmt.Errorf("target/protocol: protocol %d requirement %d is unavailable", protocol, row)
		}
		if err := appendDemand(owner, demandRow{protocol: protocol, kind: DemandRequirement, row: uint32(row)}); err != nil {
			return err
		}
	}
	for row := 0; row < c.TransitionCount(protocol); row++ {
		owner, _, _, _, ok := c.TransitionAt(protocol, row)
		if !ok {
			return fmt.Errorf("target/protocol: protocol %d transition %d is unavailable", protocol, row)
		}
		if err := appendDemand(owner, demandRow{protocol: protocol, kind: DemandTransition, row: uint32(row)}); err != nil {
			return err
		}
	}
	// The escape relation includes the derived opaque escape. A consumer that
	// reads obligations by operation must see it at the opaque operation, or it
	// would have to re-derive the one escape that discharges every proof.
	for row := 0; row < c.EscapeCount(protocol); row++ {
		owner, _, _, ok := c.EscapeAt(protocol, row)
		if !ok {
			return fmt.Errorf("target/protocol: protocol %d escape %d is unavailable", protocol, row)
		}
		if err := appendDemand(owner, demandRow{protocol: protocol, kind: DemandEscape, row: uint32(row)}); err != nil {
			return err
		}
	}
	return nil
}

// DemandCount reports how many typestate obligations one mounted callable
// operation declares, across every protocol that states one.
func (c *Table) DemandCount(owner vocabulary.Operation) int {
	span, ok := c.operationDemandSpan(owner)
	if !ok {
		return 0
	}
	return c.demands.Count(span)
}

// DemandAt returns one obligation of one mounted callable operation by dense
// ordinal.
func (c *Table) DemandAt(owner vocabulary.Operation, index int) (Demand, bool) {
	span, ok := c.operationDemandSpan(owner)
	if !ok || index < 0 {
		return Demand{}, false
	}
	row, found := c.demands.At(span, index)
	if !found || !row.kind.Available() || row.protocol == 0 {
		return Demand{}, false
	}
	return Demand{Protocol: row.protocol, Kind: row.kind, Row: int(row.row)}, true
}

func (c *Table) operationDemandSpan(owner vocabulary.Operation) (sealedrows.Span, bool) {
	if c == nil || owner == 0 || uint64(owner) > uint64(c.operationDemands.Count()) {
		return sealedrows.Span{}, false
	}
	return c.operationDemands.At(int(owner) - 1)
}
