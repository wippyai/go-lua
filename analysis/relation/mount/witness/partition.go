package witness

import (
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// issuePartitionDirectories snapshots the raw partition evidence supplied by
// the mount inventory and turns it into the sole runtime directory value. A
// raw child posting is never allowed to escape this function as a bound
// witness: all rows are checked against the global child denominator and
// reissued under the solve-local fence.
func issuePartitionDirectories(cert certificate.Certificate, inventory Inventory, issuer binding.Issuer, runtime binding.Fence, witnesses map[model.DenominatorRef]binding.DenominatorWitness) ([]binding.PartitionDirectory, bool) {
	partitions := cert.CorrelationPartitions()
	if len(partitions) == 0 {
		// A certificate with no correlated Apply has no directory obligation.
		return []binding.PartitionDirectory{}, true
	}
	source, sourceOK := inventory.(PartitionInventory)
	if !sourceOK {
		return nil, false
	}
	result := make([]binding.PartitionDirectory, len(partitions))
	for index, partition := range partitions {
		if !partition.Available() || !projectionCertified(cert, partition) {
			return nil, false
		}
		evidenceByRow, evidenceOK := source.ResolvePartition(partition)
		if !evidenceOK || evidenceByRow == nil {
			return nil, false
		}
		populationRef := partition.Population()
		childRef := partition.Child()
		population, populationOK := witnesses[populationRef]
		child, childOK := witnesses[childRef]
		if !populationOK || !childOK || !population.ValidFor(runtime) || !population.Matches(populationRef) || !child.ValidFor(runtime) || !child.Matches(childRef) {
			return nil, false
		}
		if len(evidenceByRow) != population.Len() {
			return nil, false
		}
		entries := make(map[model.RowID]binding.DenominatorWitness, population.Len())
		for rowIndex := 0; rowIndex < population.Len(); rowIndex++ {
			row, rowOK := population.At(rowIndex)
			if !rowOK || !row.Available() || row.Relation() != populationRef.Relation() {
				return nil, false
			}
			evidence, evidencePresent := evidenceByRow[row]
			if !evidencePresent || !evidence.Available() {
				return nil, false
			}
			rows := evidence.Rows()
			if rows == nil {
				return nil, false
			}
			seen := make(map[model.RowID]struct{}, len(rows))
			for _, childRow := range rows {
				if !childRow.Available() || childRow.Relation() != childRef.Relation() || !child.Contains(childRow) {
					return nil, false
				}
				if _, duplicate := seen[childRow]; duplicate {
					return nil, false
				}
				seen[childRow] = struct{}{}
			}
			membership, membershipOK := binding.NewMembershipView(childRef.Relation(), rows)
			if !membershipOK {
				return nil, false
			}
			posting, postingOK := issuer.IssueDenominator(childRef, membership, evidence.Evidence())
			if !postingOK || !posting.Available() || !posting.ValidFor(runtime) || !posting.Matches(childRef) {
				return nil, false
			}
			entries[row] = posting
		}
		for row := range evidenceByRow {
			if !population.Contains(row) {
				return nil, false
			}
		}
		directory, directoryOK := issuer.IssuePartitionDirectory(partition.Digest(), populationRef, childRef, population, entries)
		if !directoryOK || !directory.Available() || !directory.ValidFor(runtime) || directory.Seal() != partition.Digest() || directory.Population() != populationRef || directory.Child() != childRef {
			return nil, false
		}
		result[index] = directory
	}
	return result, true
}

func projectionCertified(cert certificate.Certificate, partition certificate.CorrelationPartition) bool {
	projection := partition.Projection()
	if !projection.Available() || projection.Relation() != partition.Child().Relation() {
		return false
	}
	for _, column := range cert.Columns() {
		if column.Available() && column.ID() == projection && column.ID().Relation() == projection.Relation() {
			return true
		}
	}
	return false
}
