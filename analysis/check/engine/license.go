package engine

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

// familyReadLicense is the validity interval for one selected family
// publication. A proof is readable only while no declared revoker for the same
// semantic subject has published at or after it. The selected publication
// itself is not a revocation when a family declares self-replacement; later
// rows of that exact qualified family are.
type familyReadLicense struct {
	Family     factkey.Family
	Subject    factkey.Part
	Qualifiers []factkey.Part
	Proof      string
	EpochTerm  []byte
	AdmittedAt string
}

func (license familyReadLicense) Valid(partition equation.Partition) bool {
	if license.Family.ID == 0 || license.Proof == "" {
		return false
	}
	if len(license.EpochTerm) != 0 {
		if epoch, versioned := currentEpoch(license.EpochTerm, partition); versioned && epoch > license.Proof {
			return false
		}
	}
	for _, revokerID := range license.Family.RevocationSet {
		revoker, declared := factkey.FamilyByID(revokerID)
		if !declared {
			return false
		}
		subject, compatible := factkey.RebindSubject(license.Subject, revoker)
		if !compatible {
			// A revoker whose subject kind cannot name this semantic subject
			// cannot have published against it. Tagged identity proofs still
			// rebind to identity families; tagged term proofs deliberately do
			// not invent an allocation identity.
			continue
		}
		var partStorage [4]factkey.Part
		partStorage[0] = subject
		parts := partStorage[:1]
		if revoker.ID == license.Family.ID {
			parts = append(parts, license.Qualifiers...)
		}
		values := partition.FamilyValues(factkey.BuildKey(revoker, parts, ""))
		for fact, ok := values.Next(); ok; fact, ok = values.Next() {
			if fact.Occurrence < license.Proof {
				continue
			}
			if license.AdmittedAt != "" && license.AdmittedAt == fact.Occurrence {
				continue
			}
			if revoker.ID == license.Family.ID && fact.Occurrence == license.Proof {
				continue
			}
			return false
		}
	}
	return true
}
