// Package literal declares Numeric's exact authored-integer source judgment.
// It owns neither a second numeric vocabulary nor a broad Program/Link scan:
// callers pass one already-issued Numeric scalar to NewOperand.
package literal

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/domain/numeric"
	numericowner "github.com/wippyai/go-lua/analysis/domain/numeric/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
)

// Operand is the exact owner-fenced projection of one authored numeric
// literal. Its key and atom are derived together from the scalar's existing
// Link root, so a caller cannot pair a literal identity with another Numeric
// output coordinate.
type Operand struct {
	source  *link.Link
	algebra *numeric.Algebra
	scalar  numeric.Scalar
	key     numeric.Key
	atom    numeric.Atom
	integer int64
	content [32]byte
}

// Key returns the exact Numeric root written by this authored literal.
func (operand Operand) Key() (numeric.Key, bool) {
	if !operand.valid() {
		return numeric.Key{}, false
	}
	return operand.key, true
}

// NewOperand admits an integer scalar only when it occurs in Link's complete
// authored number-literal range and is represented by this exact Numeric
// Algebra. Float literals remain a separate equality/coercion consumer;
// the finite difference carrier must not erase their representation. Source's
// typed literal view is the cold lookup authority; no Rule evaluator discovers
// or rescans Program structure.
func NewOperand(source *link.Link, algebra *numeric.Algebra, scalar numeric.Scalar) (Operand, bool) {
	if source == nil || algebra == nil || !algebra.Valid() || algebra.Link() != source || source.ContentID() != algebra.LinkID() {
		return Operand{}, false
	}
	project := source.Project()
	if project == nil {
		return Operand{}, false
	}
	mounts := project.Mounts()
	shardIndex, shardOK := mounts.Index(scalar.Shard())
	if !shardOK || uint64(shardIndex+1) > uint64(^uint32(0)) {
		return Operand{}, false
	}
	p, present := mounts.Program(scalar.Shard())
	if !present || p == nil || scalar.Term() == 0 {
		return Operand{}, false
	}
	body, _, _, positioned := p.Source().Index().Position(scalar.Term())
	integer, literal := sourceIntegerLiteral(p, scalar.Body(), scalar.Term())
	if !positioned || body != scalar.Body() || !literal || !p.Flow().Executable().Contains(scalar.Term()) ||
		!p.Flow().Executable().Contains(body) {
		return Operand{}, false
	}
	root, rooted := algebra.RootFor(scalar.Shard(), scalar.Body())
	key, keyed := algebra.KeyFor(root)
	atom, atomized := algebra.AtomFor(scalar)
	if !rooted || !keyed || !atomized {
		return Operand{}, false
	}
	zero, zeroOK := algebra.Zero()
	value, admitted := algebra.IntegerTranslation(key, atom, zero, integer)
	if !zeroOK || !admitted || algebra.Equal(value, algebra.Default()) {
		return Operand{}, false
	}
	return Operand{source: source, algebra: algebra, scalar: scalar, key: key, atom: atom, integer: integer, content: literalContent(source.ContentID(), uint32(shardIndex+1), scalar)}, true
}

// sourceIntegerLiteral validates the exact typed Source row and its lexical
// owner before exposing the literal value to Numeric.
func sourceIntegerLiteral(p *program.Program, body, term keyspace.Term) (int64, bool) {
	if p == nil || body == 0 || term == 0 {
		return 0, false
	}
	if keyspace.TermFamily(term) != keyspace.FamilyInteger || keyspace.TermOrdinal(term) == 0 {
		return 0, false
	}
	integers := p.Source().Literals().Integers()
	returned, owner, value, ok := integers.At(int(keyspace.TermOrdinal(term) - 1))
	return value, ok && returned == term && owner == body
}

// Scalar returns the existing Numeric literal occurrence. It does not expose an
// ordinal or create a second scalar identity.
func (operand Operand) Scalar() (numeric.Scalar, bool) {
	if !operand.valid() {
		return numeric.Scalar{}, false
	}
	return operand.scalar, true
}

func (operand Operand) valid() bool {
	if operand.source == nil || operand.algebra == nil || !operand.key.Valid() || !operand.atom.Valid() || operand.content == [32]byte{} ||
		operand.algebra.Link() != operand.source || operand.source.ContentID() != operand.algebra.LinkID() {
		return false
	}
	actual, ok := NewOperand(operand.source, operand.algebra, operand.scalar)
	return ok && actual.key == operand.key && actual.atom == operand.atom && actual.integer == operand.integer && actual.content == operand.content
}

func literalContent(linkID keyspace.ContentID, shardOrdinal uint32, scalar numeric.Scalar) [32]byte {
	var payload [64]byte
	copy(payload[:32], linkID[:])
	copy(payload[32:40], []byte("num-lit!"))
	binary.BigEndian.PutUint64(payload[40:48], uint64(shardOrdinal))
	binary.BigEndian.PutUint64(payload[48:56], uint64(scalar.Body()))
	binary.BigEndian.PutUint64(payload[56:64], uint64(scalar.Term()))
	return sha256.Sum256(payload[:])
}

// Rule owns the one zero-input Numeric literal declaration. Its raw engine
// Rule stays private so every later instance derives the operand and exact
// Numeric result together.
type Rule struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[numeric.Value, Operand]
	owner    *numericowner.Owner
	write    engine.Write[numeric.Value]
}

func Declare(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *numericowner.Owner) (*Rule, bool) {
	if composition == nil || owner == nil || owner.Algebra() == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() ||
		semantic == operandFamily || semantic == evidence || operandFamily == evidence {
		return nil, false
	}
	declaration := &Rule{semantic: semantic, owner: owner}
	declared, ok := engine.DeclareRule(composition, engine.RuleSpec[numeric.Value, Operand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: operandContent, Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[numeric.Value, Operand]) bool {
		write, written := engine.WriteTo(rule, owner.ExactWrite())
		if written {
			declaration.rule, declaration.write = rule, write
		}
		return written
	})
	if !ok || declared == nil || declaration.rule != declared {
		return nil, false
	}
	return declaration, true
}

// Instance forms one cold binding only after the surrounding composition has
// sealed. It cannot accept a detached scalar, Numeric key, or fact.
func (rule *Rule) Instance(operand Operand) (*engine.RuleInstance[numeric.Value, Operand], bool) {
	if rule == nil || rule.rule == nil || !validOperand(rule.owner, operand) {
		return nil, false
	}
	ref, ok := rule.owner.Locate(operand.key)
	if !ok {
		return nil, false
	}
	return engine.NewRuleInstance(rule.rule, operand, func(binding *engine.RuleBinding[numeric.Value, Operand]) bool {
		return engine.InstanceWrite(binding, rule.write, ref)
	})
}

func operandContent(operand Operand) (Operand, [32]byte, bool) {
	return operand, operand.content, operand.valid()
}

func validOperand(owner *numericowner.Owner, operand Operand) bool {
	if owner == nil || owner.Algebra() == nil || operand.source == nil || operand.content == [32]byte{} ||
		operand.algebra != owner.Algebra() || owner.Algebra().Link() != operand.source ||
		operand.source.ContentID() != owner.Algebra().LinkID() {
		return false
	}
	// Reconstruct with the Owner's exact Algebra; this is the owner fence and
	// also proves all source/key/atom/mask correlations in one place.
	expected, ok := NewOperand(operand.source, owner.Algebra(), operand.scalar)
	return ok && expected.algebra == operand.algebra && expected.key == operand.key && expected.atom == operand.atom && expected.integer == operand.integer && expected.content == operand.content
}

func literalResult(owner *numericowner.Owner, operand Operand) (numeric.Value, bool) {
	if !validOperand(owner, operand) {
		return numeric.Value{}, false
	}
	zero, ok := owner.Algebra().Zero()
	if !ok {
		return numeric.Value{}, false
	}
	return owner.Algebra().IntegerTranslation(operand.key, operand.atom, zero, operand.integer)
}

func (rule *Rule) transfer(access engine.Access[numeric.Value, Operand]) bool {
	operand, ok := engine.Operand(access)
	if !ok {
		return false
	}
	value, ok := literalResult(rule.owner, operand)
	if !ok {
		return false
	}
	rows := 0
	completed := engine.Product(access, func(row engine.Row) bool {
		rows++
		return rows == 1 && engine.StageValue(access, row, value)
	})
	return completed && rows == 1
}

func (rule *Rule) check(derivation engine.RuleDerivation[numeric.Value, Operand]) (engine.RuleEvidence, bool) {
	if rule == nil || rule.owner == nil || derivation.Rule() != rule.semantic || derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	if !ok || !validOperand(rule.owner, operand) || !derivation.OperandContentMatches(operand.content) {
		return engine.RuleEvidence{}, false
	}
	disposition, ok := derivation.DispositionAt(0)
	if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.TargetCount() != 1 || disposition.Guard().Empty() {
		return engine.RuleEvidence{}, false
	}
	target, ok := disposition.TargetAt(0)
	ref, refOK := rule.owner.Locate(operand.key)
	value, valueOK := literalResult(rule.owner, operand)
	actual, actualOK := disposition.Value()
	if !ok || !refOK || !valueOK || !actualOK || !engine.TargetMatchesRef(target, ref) || !rule.owner.Algebra().Equal(actual, value) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
