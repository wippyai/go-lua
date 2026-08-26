package binding

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

const partitionDirectoryDigestDomain = "analysis/relation/semantic/binding/partition-directory/v1"

// PartitionDirectory is the immutable runtime posting directory for one
// certificate-owned correlated Apply child.  Its domain is the population's
// authenticated Q RowID set; each exact Q row redeems one already-issued
// child DenominatorWitness.  An empty child witness is a real authenticated
// posting and is retained in the directory rather than omitted.
//
// The directory owns no relation copy, scope key, scan, cache, or physical
// ordinal.  It is only the owner-neutral binding between an opaque
// certificate/mount seal and existing runtime-fenced denominator witnesses.
type PartitionDirectory struct {
	seal          identity.ContentID
	fence         Fence
	populationRef model.DenominatorRef
	childRef      model.DenominatorRef
	population    DenominatorWitness
	entries       map[model.RowID]DenominatorWitness
	digest        identity.ContentID
	sealed        bool
}

// IssuePartitionDirectory authenticates one complete population-to-child
// posting directory under this issuer's exact binding fence.  Entries must
// contain every population member exactly once; a non-nil empty map is the
// authenticated empty directory for an empty population.
func (issuer Issuer) IssuePartitionDirectory(
	seal identity.ContentID,
	populationRef model.DenominatorRef,
	childRef model.DenominatorRef,
	population DenominatorWitness,
	entries map[model.RowID]DenominatorWitness,
) (PartitionDirectory, bool) {
	if !issuer.Available() || !seal.Available() || !populationRef.Available() || !childRef.Available() || entries == nil {
		return PartitionDirectory{}, false
	}
	if populationRef.Relation() != population.Relation() ||
		!population.ValidFor(issuer.fence) || !population.Matches(populationRef) {
		return PartitionDirectory{}, false
	}

	// The population witness is the sole Q keyset authority.  Require a total
	// one-to-one entry set, including authenticated empty child postings.
	if len(entries) != population.Len() {
		return PartitionDirectory{}, false
	}
	copyOf := make(map[model.RowID]DenominatorWitness, len(entries))
	for index := 0; index < population.Len(); index++ {
		row, rowOK := population.At(index)
		if !rowOK || !row.Available() || row.Relation() != populationRef.Relation() {
			return PartitionDirectory{}, false
		}
		child, childOK := entries[row]
		if !childOK || !child.Available() || !child.ValidFor(issuer.fence) || !child.Matches(childRef) {
			return PartitionDirectory{}, false
		}
		copyOf[row] = child
	}
	// A map can carry an extra foreign row even when its cardinality matches
	// the population.  Exact membership, rather than map cardinality alone,
	// closes that hostile substitution.
	for row := range entries {
		if !population.Contains(row) {
			return PartitionDirectory{}, false
		}
	}

	value := PartitionDirectory{
		seal:          seal,
		fence:         issuer.fence,
		populationRef: populationRef,
		childRef:      childRef,
		population:    population,
		entries:       copyOf,
		sealed:        true,
	}
	digest, digestOK := value.recomputeDigest()
	if !digestOK {
		return PartitionDirectory{}, false
	}
	value.digest = digest
	return value, value.Available()
}

// Available reports whether this is a complete sealed directory.  It repeats
// only immutable structural checks; no inventory or runtime resolver is
// consulted.
func (directory PartitionDirectory) Available() bool {
	if !directory.sealed || !directory.seal.Available() || !directory.fence.Available() ||
		!directory.populationRef.Available() || !directory.childRef.Available() ||
		!directory.entriesAvailable() || !directory.digest.Available() {
		return false
	}
	return true
}

// entriesAvailable is intentionally a constant-time post-seal check. The
// issuer performs totality, row validity, subset, and digest checks once;
// runtime lookup must not re-prove the whole Q directory for every row.
func (directory PartitionDirectory) entriesAvailable() bool {
	return directory.population.fence.Same(directory.fence) &&
		directory.population.relation == directory.populationRef.Relation() &&
		directory.population.key == directory.populationRef.Key() &&
		directory.population.membership.relation == directory.populationRef.Relation() &&
		directory.population.membership.rows != nil &&
		directory.population.opaque.Available() &&
		directory.entries != nil && len(directory.entries) == len(directory.population.membership.rows)
}

// Seal returns the opaque certificate/mount identity that names this
// directory. Binding does not interpret the seal or depend on checker types.
func (directory PartitionDirectory) Seal() identity.ContentID {
	if !directory.Available() {
		return identity.ContentID{}
	}
	return directory.seal
}

// Fence returns the exact binding fence that authenticated every posting.
func (directory PartitionDirectory) Fence() Fence {
	if !directory.Available() {
		return Fence{}
	}
	return directory.fence
}

// Population returns the existing logical denominator reference whose
// authenticated RowID set is the directory domain.
func (directory PartitionDirectory) Population() model.DenominatorRef {
	if !directory.Available() {
		return model.DenominatorRef{}
	}
	return directory.populationRef
}

// Child returns the existing child denominator reference authenticated by
// every posting witness.
func (directory PartitionDirectory) Child() model.DenominatorRef {
	if !directory.Available() {
		return model.DenominatorRef{}
	}
	return directory.childRef
}

// Digest returns the immutable identity of the partition keyset and all
// authenticated child postings, including the binding generation fence.
func (directory PartitionDirectory) Digest() identity.ContentID {
	if !directory.Available() {
		return identity.ContentID{}
	}
	return directory.digest
}

// ValidFor reports whether the complete directory belongs to the exact
// solve-local binding fence.
func (directory PartitionDirectory) ValidFor(fence Fence) bool {
	return directory.Available() && fence.Available() && directory.fence.Same(fence)
}

// Lookup redeems exactly one authenticated Q RowID.  No scan, fallback,
// inferred key, or relation-wide enumeration is exposed by this ABI.
func (directory PartitionDirectory) Lookup(row model.RowID) (DenominatorWitness, bool) {
	if !directory.Available() || !row.Available() || !row.Relation().Available() || row.Relation() != directory.populationRef.Relation() {
		return DenominatorWitness{}, false
	}
	value, ok := directory.entries[row]
	if !ok || !witnessHeaderMatches(value, directory.fence, directory.childRef) {
		return DenominatorWitness{}, false
	}
	return value, true
}

func witnessHeaderMatches(witness DenominatorWitness, fence Fence, ref model.DenominatorRef) bool {
	return witness.fence.Same(fence) && witness.relation == ref.Relation() && witness.key == ref.Key() && witness.membership.relation == ref.Relation() && witness.membership.rows != nil && witness.opaque.Available()
}

func (directory PartitionDirectory) recomputeDigest() (identity.ContentID, bool) {
	if !directory.sealed || !directory.seal.Available() || !directory.fence.Available() || !directory.populationRef.Available() || !directory.childRef.Available() || !directory.population.Available() || directory.entries == nil {
		return identity.ContentID{}, false
	}
	parts := make([][]byte, 0, 5+directory.population.Len())
	parts = append(parts,
		contentBytes(directory.seal),
		nominalBytes(directory.fence.Schema().Owner().Content(), directory.fence.Schema().Content()),
		mountBytes(directory.fence.Mount()),
		generationBytes(directory.fence.Generation()),
		denominatorRefBytes(directory.populationRef),
		denominatorRefBytes(directory.childRef),
		denominatorWitnessBytes(directory.population),
	)
	for index := 0; index < directory.population.Len(); index++ {
		row, rowOK := directory.population.At(index)
		if !rowOK {
			return identity.ContentID{}, false
		}
		child, childOK := directory.entries[row]
		if !childOK || !child.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, rowBytes(row), denominatorWitnessBytes(child))
	}
	return identity.DeriveContentID(partitionDirectoryDigestDomain, parts...)
}

func denominatorRefBytes(ref model.DenominatorRef) []byte {
	if !ref.Available() {
		return nil
	}
	relation := ref.Relation()
	key := ref.Key()
	relationOwner, relationContent := relation.Owner().Content(), relation.Content()
	keyOwner, keyContent := key.Owner().Content(), key.Content()
	result := nominalBytes(relationOwner, relationContent)
	result = append(result, keyOwner[:]...)
	return append(result, keyContent[:]...)
}

func denominatorWitnessBytes(witness DenominatorWitness) []byte {
	if !witness.Available() {
		return nil
	}
	parts := make([]byte, 0, 96+16*witness.Len())
	parts = append(parts, nominalBytes(witness.relation.Owner().Content(), witness.relation.Content())...)
	parts = append(parts, nominalBytes(witness.key.Relation().Owner().Content(), witness.key.Content())...)
	evidence, _ := witness.Evidence()
	parts = append(parts, contentBytes(evidence)...)
	for index := 0; index < witness.Len(); index++ {
		row, ok := witness.At(index)
		if !ok {
			return nil
		}
		parts = append(parts, rowBytes(row)...)
	}
	return parts
}

func rowBytes(row model.RowID) []byte {
	if !row.Available() {
		return nil
	}
	content := row.Content()
	result := make([]byte, 0, len(row.Relation().Owner().Content())+len(row.Relation().Content())+len(content))
	result = append(result, nominalBytes(row.Relation().Owner().Content(), row.Relation().Content())...)
	return append(result, content[:]...)
}

func nominalBytes(owner, content identity.ContentID) []byte {
	result := make([]byte, 0, len(owner)+len(content))
	result = append(result, owner[:]...)
	return append(result, content[:]...)
}

func contentBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

func mountBytes(value identity.MountID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

func generationBytes(value identity.Generation) []byte {
	result := make([]byte, 8)
	binary.BigEndian.PutUint64(result, uint64(value))
	return result
}
