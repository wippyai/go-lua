package call

// MaySuspend projects the suspension capability of one admitted Call fact.
// Call owns the dynamic target set; Target owns each operation's sealed
// suspension denominator. Consumers receive only the joined capability and
// never repeat target classification or interpret Target vocabulary.
//
// An opaque alternative and a body/non-operation alternative remain capable
// of suspension. A closed set containing only operations with an empty sealed
// suspension denominator is proven synchronous.
func (algebra *Algebra) MaySuspend(key Key, value Value) (bool, bool) {
	if algebra == nil || !algebra.Valid() || algebra.contract == nil || !algebra.Admits(key, value) {
		return false, false
	}
	if value.HasOpaqueAlternative() {
		return true, true
	}
	for index := 0; index < value.KnownTargetCount(); index++ {
		candidate, candidateOK := value.KnownTargetAt(index)
		if !candidateOK {
			return false, false
		}
		operation, kind := algebra.ClassifyTargetOperation(candidate)
		switch kind {
		case TargetOperationInvalid:
			return false, false
		case TargetOperationNone:
			return true, true
		case TargetOperationPresent:
			declared, declaredOK := algebra.contract.Operations.OperationAt(int(operation) - 1)
			if !declaredOK || declared != operation {
				return false, false
			}
			if algebra.contract.Operations.SuspensionCount(operation) != 0 {
				return true, true
			}
		}
	}
	return false, true
}
