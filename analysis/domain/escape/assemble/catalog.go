// Package assemble assembles Escape's static transfer catalog. It intentionally
// contains no application coordinate: applications select these already-sealed
// target endpoints later through activation ports.
package assemble

import (
	"github.com/wippyai/go-lua/analysis/domain/escape/rule"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/link"
	"github.com/wippyai/go-lua/program/target"
)

const staticSemanticVersion uint64 = 1

// staticTransfer is one direct Pack-delivered transfer outcome.
// All fields are static Contract facts; operand is Escape Rule's checked
// representation of the same endpoint identity.
type staticTransfer struct {
	operation target.Operation
	transfer  target.TransferID
	outcome   uint32
	target    engine.SemanticKey
	endpoint  engine.SemanticKey
	operand   rule.TransferOperand
}

// staticCatalog is immutable-by-convention cold compiler input.  The flat
// order follows Contract operation, transfer, and outcome order.  byOperation
// is only a lookup index over that same static set; it deliberately has no
// Application dimension.
type staticCatalog struct {
	transfers   []staticTransfer
	byOperation map[target.Operation][]staticTransfer
}

// buildStaticCatalog collects every declared deliverable transfer outcome from
// source's one sealed Contract. The payload remains Target-owned and is
// projected from the application Pack later by Escape Rule, so this cold
// catalog has no scalar-formal or Application×operation table.
func buildStaticCatalog(source *link.Link) (staticCatalog, bool) {
	if source == nil || !source.ContentID().Available() {
		return staticCatalog{}, false
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil || !contract.ContentID().Available() {
		return staticCatalog{}, false
	}

	catalog := staticCatalog{byOperation: make(map[target.Operation][]staticTransfer)}
	for operationIndex := 0; operationIndex < contract.OperationCount(); operationIndex++ {
		operation, operationOK := contract.OperationAt(operationIndex)
		operationID, identityOK := contract.OperationContentID(operation)
		semantic, semanticOK := engine.NewSemanticKey([32]byte(operationID), staticSemanticVersion)
		if !operationOK || !identityOK || !operationID.Available() || !semanticOK {
			return staticCatalog{}, false
		}

		for transferIndex := 0; transferIndex < contract.TransferCount(operation); transferIndex++ {
			transfer, transferOK := contract.TransferIDAt(operation, transferIndex)
			if !transferOK {
				return staticCatalog{}, false
			}

			for outcomeIndex := 0; outcomeIndex < contract.TransferOutcomeCount(operation, transferIndex); outcomeIndex++ {
				outcome, disposition, outcomeOK := contract.TransferOutcomeAt(operation, transferIndex, outcomeIndex)
				if !outcomeOK {
					return staticCatalog{}, false
				}
				if disposition&target.TransferMayDeliver == 0 {
					continue
				}
				endpointID, endpointDisposition, endpointOK := contract.TransferOutcomeContentID(operation, transfer, outcomeIndex)
				endpoint, endpointSemanticOK := engine.NewSemanticKey([32]byte(endpointID), staticSemanticVersion)
				operand, operandOK := rule.NewTransferOperand(source, transfer, outcome)
				if !endpointOK || !endpointID.Available() || endpointDisposition != disposition || endpointDisposition&target.TransferMayDeliver == 0 ||
					!endpointSemanticOK || !operandOK || operand.ContentID() != endpointID {
					return staticCatalog{}, false
				}

				row := staticTransfer{
					operation: operation,
					transfer:  transfer,
					outcome:   outcome,
					target:    semantic,
					endpoint:  endpoint,
					operand:   operand,
				}
				catalog.transfers = append(catalog.transfers, row)
				catalog.byOperation[operation] = append(catalog.byOperation[operation], row)
			}
		}
	}
	return catalog, true
}
