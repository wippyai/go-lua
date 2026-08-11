package generator

// Closed typed vocabulary for the Link ownership manifests.  There is one
// parser/model path: every final query, reference, identity, and structural
// storage row names scanner facts directly and carries one atomic pattern ID.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

type StorageDisposition string

const (
	StoragePublicSurface StorageDisposition = "v1:public-surface"
)

func (value StorageDisposition) valid() bool {
	return value == StoragePublicSurface
}

// ResidueDeleteDestination is the sole closed destination for deletion. Move
// destinations are exact Catalog OwnerRow IDs. Split rows name a derived
// split-plan ID whose recipients are joined during population.
const ResidueDeleteDestination = "v1:private-representation"

// splitPlanID is the closed identity of one sorted recipient OwnerID set. It
// prevents two arbitrary aliases from describing the same split recipients.
func splitPlanID(ownerIDs []string) string {
	hash := sha256.New()
	write := func(value string) {
		var size [8]byte
		valueSize := uint64(len(value))
		for index := range size {
			size[index] = byte(valueSize >> (56 - 8*index))
		}
		_, _ = hash.Write(size[:])
		_, _ = hash.Write([]byte(value))
	}
	write("link-split-plan-v1")
	write(fmt.Sprintf("%d", len(ownerIDs)))
	for _, ownerID := range ownerIDs {
		write(ownerID)
	}
	return "split-plan-v1-" + hex.EncodeToString(hash.Sum(nil))
}

type IdentityRelationKind string

const (
	IdentityRelationDirect    IdentityRelationKind = "v1:direct"
	IdentityRelationComposite IdentityRelationKind = "v1:composite"
)

func (value IdentityRelationKind) valid() bool {
	return value == IdentityRelationDirect || value == IdentityRelationComposite
}

type StorageRow struct {
	FactID       string
	OwnerSurface string
	Disposition  StorageDisposition
}

// benchmarkReceiptDigest is optional empirical evidence. It is deliberately
// only a canonical SHA-256 receipt and never a formal law/proof input.
func canonicalBenchmarkReceiptDigest(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

type IndexPlanRow struct {
	ID, Owner, DeclarationFactID    string
	SourceFactIDs, CallerUseFactIDs []string
}

type ReferencePlanRow struct {
	ID, Issuer, Consumer, DeclarationFactID string
	SourceFactIDs, CallerUseFactIDs         []string
}

type IdentityPlanRow struct {
	ID, Owner, DeclarationFactID     string
	RelationKind                     IdentityRelationKind
	DirectFactIDs, ParentIdentityIDs []string
}

func factIDsCanonical(values []string) bool {
	for index, value := range values {
		if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\t\n\r") || forbiddenClassification(value) {
			return false
		}
		if index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func typedFactList(name, raw string, allowEmpty bool) ([]string, error) {
	if raw == "" {
		if allowEmpty {
			return nil, nil
		}
		return nil, fmt.Errorf("%s is empty", name)
	}
	values := strings.Split(raw, ",")
	if !factIDsCanonical(values) {
		return nil, fmt.Errorf("%s is not a sorted unique FactID list", name)
	}
	return append([]string(nil), values...), nil
}

// identityDigest commits exactly the authored identity relation inputs and
// recursively commits each parent's computed digest.  The authored ID is
// checked by population against this result; it is never treated as input.
func identityDigest(row IdentityRow, all []IdentityRow) (string, error) {
	byID := make(map[string]IdentityRow, len(all))
	for _, candidate := range all {
		if candidate.ID == "" {
			return "", fmt.Errorf("identity row has empty ID")
		}
		if _, exists := byID[candidate.ID]; exists {
			return "", fmt.Errorf("duplicate identity row %q", candidate.ID)
		}
		byID[candidate.ID] = candidate
	}
	state := make(map[string]uint8, len(all))
	cache := make(map[string]string, len(all))
	var visit func(IdentityRow) (string, error)
	visit = func(current IdentityRow) (string, error) {
		if cached, ok := cache[current.ID]; ok {
			return cached, nil
		}
		if state[current.ID] == 1 {
			return "", fmt.Errorf("identity DAG cycle at %q", current.ID)
		}
		state[current.ID] = 1
		if !current.RelationKind.valid() || !validPopulationAtom(current.Owner) || !validPopulationAtom(current.DeclarationFactID) || !validPopulationAtom(current.PatternID) || !factIDsCanonical(current.DirectFactIDs) || !factIDsCanonical(current.ParentIdentityIDs) {
			return "", fmt.Errorf("identity %q has malformed relation inputs", current.ID)
		}
		if (current.RelationKind == IdentityRelationDirect && len(current.ParentIdentityIDs) != 0) || (current.RelationKind == IdentityRelationComposite && len(current.ParentIdentityIDs) == 0) {
			return "", fmt.Errorf("identity %q has malformed relation inputs", current.ID)
		}
		hash := sha256.New()
		write := func(value string) {
			var size [8]byte
			valueSize := uint64(len(value))
			for index := range size {
				size[index] = byte(valueSize >> (56 - 8*index))
			}
			_, _ = hash.Write(size[:])
			_, _ = hash.Write([]byte(value))
		}
		write("link-identity-relation-v2")
		write(current.Owner)
		write(current.DeclarationFactID)
		write(string(current.RelationKind))
		write("formal-v1")
		write(current.PatternID)
		write(fmt.Sprintf("%d", len(current.DirectFactIDs)))
		for _, fact := range current.DirectFactIDs {
			write(fact)
		}
		write(fmt.Sprintf("%d", len(current.ParentIdentityIDs)))
		for _, parentID := range current.ParentIdentityIDs {
			parent, exists := byID[parentID]
			if !exists {
				return "", fmt.Errorf("identity %q references unknown parent %q", current.ID, parentID)
			}
			parentDigest, err := visit(parent)
			if err != nil {
				return "", err
			}
			write(parentID)
			write(parentDigest)
		}
		result := "identity-v2-" + hex.EncodeToString(hash.Sum(nil))
		state[current.ID] = 2
		cache[current.ID] = result
		return result, nil
	}
	return visit(row)
}
