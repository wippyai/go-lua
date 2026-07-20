package body

func deadAssignmentProofByName(proofs []DeadAssignmentProof, name string) (DeadAssignmentProof, bool) {
	for _, proof := range proofs {
		if proof.Write.Name == name {
			return proof, true
		}
	}
	return DeadAssignmentProof{}, false
}
