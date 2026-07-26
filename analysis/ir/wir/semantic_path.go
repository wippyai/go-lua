package wir

import "github.com/wippyai/go-lua/analysis/domain/path"

// ForEachValuePath visits every path whose runtime value participates in this
// body's semantics. It is live today at
// __legacy/check/body/run.go:symbolicBoundaryCaptureSymbols and is also the
// WIR design's planned runtime/JIT boundary-input traversal seam. A root-only
// direct-call callee is deliberately excluded:
// the direct-call catalog owns that function identity. Descendant callees and
// every other carrier are value paths. This is the closed traversal seam for
// boundary-input discovery; new instruction metadata must be added here.
func (b *Body) ForEachValuePath(visit func(path.Path) bool) bool {
	if b == nil || visit == nil {
		return true
	}
	operand := func(raw Operand) bool {
		if raw.Kind != OperandPath {
			return true
		}
		return visit(b.Path(PathRef(raw.Ref)))
	}
	check := func(raw Check) bool {
		if !raw.Path.IsEmpty() && !visit(raw.Path) {
			return false
		}
		return raw.OtherPath.IsEmpty() || visit(raw.OtherPath)
	}
	for i := 0; i < b.Len(); i++ {
		inst := b.Instr(i)
		if !operand(inst.Dst) || !operand(inst.A) || !operand(inst.B) {
			return false
		}
		for _, raw := range b.Operands(inst.List) {
			if !operand(raw) {
				return false
			}
		}
		for _, raw := range b.Operands(inst.Results) {
			if !operand(raw) {
				return false
			}
		}
		if !operand(inst.Call.Receiver) {
			return false
		}
		if inst.Call.Callee.Kind == OperandPath {
			callee := b.Path(PathRef(inst.Call.Callee.Ref))
			if len(callee.Segments) != 0 && !visit(callee) {
				return false
			}
		} else if !operand(inst.Call.Callee) {
			return false
		}
		if inst.Check != 0 && !check(b.Check(inst.Check)) {
			return false
		}
		for _, implied := range b.ImpliedChecks(inst.ImpliedChecks) {
			if !check(implied.Check) {
				return false
			}
		}
		for _, sufficient := range b.SufficientChecks(inst.SufficientChecks) {
			if !check(sufficient.Check) {
				return false
			}
		}
		for _, constraint := range b.BranchDiffConstraints(inst.DiffConstraints) {
			if (!constraint.HiPath.IsEmpty() && !visit(constraint.HiPath)) ||
				(constraint.HasHi2 && !constraint.Hi2Path.IsEmpty() && !visit(constraint.Hi2Path)) ||
				(!constraint.LoPath.IsEmpty() && !visit(constraint.LoPath)) {
				return false
			}
		}
		for _, entry := range b.TableEntries(inst.TableEntries) {
			if !operand(entry.Value) || (!entry.Suffix.IsEmpty() && !visit(entry.Suffix)) {
				return false
			}
		}
	}
	for _, target := range b.callTargets {
		if !target.Path.IsEmpty() && !visit(target.Path) {
			return false
		}
	}
	return true
}
