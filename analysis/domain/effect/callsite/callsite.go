// Package callsite owns the two ordinary Call-to-Effect judgments.
package callsite

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/domain/call"
	callowner "github.com/wippyai/go-lua/analysis/domain/call/owner"
	"github.com/wippyai/go-lua/analysis/domain/effect/factor"
	effectowner "github.com/wippyai/go-lua/analysis/domain/effect/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	linkproject "github.com/wippyai/go-lua/program/link/project"
)

// operand is one private exact ordinary Project Call/body witness.
type operand struct {
	root factor.Root
	key  call.Key
	app  linkproject.Application
	id   keyspace.ContentID
}

func newOperand(algebra *factor.Algebra, calls *call.Algebra, root factor.Root, app linkproject.Application) (operand, bool) {
	if algebra == nil || !algebra.Valid() || calls == nil || !calls.Valid() || calls.Link() != algebra.Link() || !algebra.ContainsCall(root, app) {
		return operand{}, false
	}
	key, ok := calls.KeyForApplication(app)
	if !ok {
		return operand{}, false
	}
	callID, ok := key.ContentID()
	if !ok || !callID.Available() {
		return operand{}, false
	}
	rootID, ok := algebra.RootID(root)
	if !ok {
		return operand{}, false
	}
	const prefix = "wippy.analysis.effect.callsite.v2\x00"
	var payload [len(prefix) + 2*sha256.Size]byte
	copy(payload[:], prefix)
	copy(payload[len(prefix):], callID[:])
	copy(payload[len(prefix)+sha256.Size:], rootID[:])
	id := keyspace.ContentID(sha256.Sum256(payload[:]))
	if !id.Available() {
		return operand{}, false
	}
	return operand{root: root, key: key, app: app, id: id}, true
}

// Rule is either the known selected-effect rule or the opaque-row rule.
type Rule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[factor.Value, operand]
	calls    *callowner.Owner
	effects  *effectowner.Owner
	read     engine.Read[engine.OrderedCells[call.Value]]
	write    engine.Write[factor.Value]
	opaque   bool
}

func DeclareSelected(c *engine.Composition, semantic, family, evidence engine.SemanticKey, calls *callowner.Owner, effects *effectowner.Owner) (*Rule, bool) {
	return declare(c, semantic, family, evidence, calls, effects, false)
}
func DeclareOpaque(c *engine.Composition, semantic, family, evidence engine.SemanticKey, calls *callowner.Owner, effects *effectowner.Owner) (*Rule, bool) {
	return declare(c, semantic, family, evidence, calls, effects, true)
}
func declare(c *engine.Composition, semantic, family, evidence engine.SemanticKey, calls *callowner.Owner, effects *effectowner.Owner, opaque bool) (*Rule, bool) {
	if c == nil || calls == nil || effects == nil || calls.Link() == nil || calls.Link() != effects.Link() || !semantic.Available() || !family.Available() || !evidence.Available() || semantic == family || semantic == evidence || family == evidence {
		return nil, false
	}
	d := &Rule{semantic: semantic, calls: calls, effects: effects, opaque: opaque}
	rule, ok := engine.DeclareRule(c, engine.RuleSpec[factor.Value, operand]{Semantic: semantic, OperandFamily: family, OperandContent: operandContent, Output: effects.Output(), Inputs: 1, Admission: engine.AdmitRuleByDerivation(evidence, d.check), Transfer: d.transfer}, func(rule *engine.Rule[factor.Value, operand]) bool {
		input, ok := rule.InputAt(0)
		if !ok {
			return false
		}
		read, readOK := engine.ReadFrom(rule, input, calls.ExactRead())
		write, writeOK := engine.WriteTo(rule, effects.ExactWrite())
		if !readOK || !writeOK {
			return false
		}
		d.rule, d.read, d.write = rule, read, write
		return true
	})
	if !ok || rule == nil || d.rule != rule {
		return nil, false
	}
	return d, true
}

func (d *Rule) Instance(app linkproject.Application) (*engine.RuleInstance[factor.Value, operand], bool) {
	if d == nil || d.rule == nil || d.calls == nil || d.effects == nil {
		return nil, false
	}
	root, ok := d.effects.Algebra().RootForCall(app)
	if !ok {
		return nil, false
	}
	o, ok := newOperand(d.effects.Algebra(), d.calls.Algebra(), root, app)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(d.rule, o, func(binding *engine.RuleBinding[factor.Value, operand]) bool {
		callRef, ok := d.calls.Locate(o.key)
		if !ok {
			return false
		}
		effectRef, ok := d.effects.Locate(o.root)
		if !ok {
			return false
		}
		return engine.InstanceRead(binding, d.read, callRef) && engine.InstanceWrite(binding, d.write, effectRef)
	})
}
func operandContent(o operand) (operand, [32]byte, bool) {
	if !o.id.Available() {
		return operand{}, [32]byte{}, false
	}
	return o, [32]byte(o.id), true
}
func (d *Rule) validOperand(o operand) bool {
	if d == nil || d.rule == nil || d.calls == nil || d.effects == nil || d.calls.Link() != d.effects.Link() || !o.id.Available() {
		return false
	}
	// Hot transfer trusts private constructor fields; cold derivation below
	// rebuilds the portable owner proof before accepting evidence.
	return o.key.Valid() && d.effects.Algebra().Admit(o.root, d.effects.Algebra().Bottom())
}

func (d *Rule) transfer(access engine.Access[factor.Value, operand]) bool {
	o, ok := engine.Operand(access)
	if !ok || !d.validOperand(o) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		cells, ok := engine.ReadValue(access, row, d.read)
		if !ok || cells.Count() != 1 {
			return false
		}
		value, present, ok := cells.At(0)
		if !ok {
			return false
		}
		if !present {
			return engine.NoCandidate(access, row)
		}
		next, ok := d.reduce(o, value)
		if !ok {
			return false
		}
		if d.effects.Algebra().Equal(next, d.effects.Algebra().Bottom()) {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, next)
	})
}

func (d *Rule) reduce(o operand, value call.Value) (factor.Value, bool) {
	if !d.calls.Algebra().Admits(o.key, value) {
		return factor.Value{}, false
	}
	effects := d.effects.Algebra()
	if value.IsTop() {
		return effects.Top(), true
	}
	// Each selected operation is reduced independently by Factor so it can
	// validate its exact Target witnesses.  Flatten their sparse values here
	// and canonicalize once below: repeated pairwise joins would copy the
	// growing prefix for every selected operation.
	atoms := make([]factor.Atom, 0)
	var unknownID keyspace.ContentID
	unknownKnown := false
	appendUnknown := func(atom factor.Atom) bool {
		id, ok := effects.AtomID(atom)
		if !ok {
			return false
		}
		if unknownKnown {
			return id == unknownID
		}
		if len(atoms) == int(^uint(0)>>1) {
			return false
		}
		atoms = append(atoms, atom)
		unknownID, unknownKnown = id, true
		return true
	}
	appendPart := func(part factor.Value) bool {
		if d.opaque {
			atom, present := effects.AtomAt(part, 0)
			if !present {
				return true
			}
			if _, more := effects.AtomAt(part, 1); more {
				return false
			}
			return appendUnknown(atom)
		}
		for index := 0; ; index++ {
			atom, present := effects.AtomAt(part, index)
			if !present {
				return true
			}
			if len(atoms) == int(^uint(0)>>1) {
				return false
			}
			atoms = append(atoms, atom)
		}
	}
	for i := 0; i < value.KnownTargetCount(); i++ {
		target, ok := value.KnownTargetAt(i)
		if !ok {
			return factor.Value{}, false
		}
		op, operation := target.Operation()
		if !operation {
			continue
		}
		var part factor.Value
		if d.opaque {
			part, ok = d.effects.Algebra().SelectedCallOpaque(o.root, o.app, op)
		} else {
			part, ok = d.effects.Algebra().SelectedCallEffects(o.root, o.app, op)
		}
		if !ok {
			return factor.Value{}, false
		}
		if !appendPart(part) {
			return factor.Value{}, false
		}
	}
	if d.opaque && value.HasOpaqueAlternative() {
		atom, ok := d.effects.Algebra().OpaqueCallUnknown(o.root, d.calls.Algebra(), o.app, value)
		if !ok {
			return factor.Value{}, false
		}
		if !appendUnknown(atom) {
			return factor.Value{}, false
		}
	}
	return effects.FromAtoms(atoms)
}

func (d *Rule) check(derivation engine.RuleDerivation[factor.Value, operand]) (engine.RuleEvidence, bool) {
	if d == nil || derivation.Rule() != d.semantic || derivation.InputCount() != 1 || derivation.ReadCount() != 1 || derivation.DispositionCount() > 1 {
		return engine.RuleEvidence{}, false
	}
	input, ok := derivation.InputAt(0)
	if !ok {
		return engine.RuleEvidence{}, false
	}
	o, ok := derivation.Operand()
	if !ok || !d.validOperand(o) || !derivation.OperandContentMatches([32]byte(o.id)) {
		return engine.RuleEvidence{}, false
	}
	rebuilt, rebuiltOK := newOperand(d.effects.Algebra(), d.calls.Algebra(), o.root, o.app)
	if !rebuiltOK || rebuilt.id != o.id || rebuilt.key != o.key {
		return engine.RuleEvidence{}, false
	}
	callRef, ok := d.calls.Locate(o.key)
	if !ok || !engine.DerivationReadMatchesRef(derivation, d.read, callRef) {
		return engine.RuleEvidence{}, false
	}
	effectRef, ok := d.effects.Locate(o.root)
	if !ok {
		return engine.RuleEvidence{}, false
	}
	if input.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	disposition, ok := derivation.DispositionAt(0)
	if !ok || !disposition.Guard().Same(input.Guard()) {
		return engine.RuleEvidence{}, false
	}
	if _, transformed := disposition.CarryTransform(); transformed || disposition.TransformOnly() {
		return engine.RuleEvidence{}, false
	}
	cells, ok := engine.DerivationDispositionReadValue(derivation, disposition, d.read)
	if !ok || cells.Count() != 1 {
		return engine.RuleEvidence{}, false
	}
	value, present, ok := cells.At(0)
	if !ok {
		return engine.RuleEvidence{}, false
	}
	if !present {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	expected, reduced := d.reduce(o, value)
	if !reduced {
		return engine.RuleEvidence{}, false
	}
	if d.effects.Algebra().Equal(expected, d.effects.Algebra().Bottom()) {
		if disposition.Kind() != engine.RuleDispositionNoCandidate || disposition.TargetCount() != 0 {
			return engine.RuleEvidence{}, false
		}
		return derivation.Accept()
	}
	actual, staged := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	if disposition.Kind() != engine.RuleDispositionStaged || !staged || disposition.TargetCount() != 1 || !targetOK || !d.effects.Algebra().Equal(actual, expected) || !engine.TargetMatchesRef(target, effectRef) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
