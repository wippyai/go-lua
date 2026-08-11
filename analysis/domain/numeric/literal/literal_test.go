package literal

import (
	"encoding/binary"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/numeric"
	numericowner "github.com/wippyai/go-lua/analysis/domain/numeric/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestLiteralRuleDeclaresOneZeroReadNumericWriter(t *testing.T) {
	algebra, _ := literalFixture(t, "literal_declare", "return 1, 2.5")
	composition, owner, rule := literalComposition(t, algebra, 0)
	if rule == nil || owner.Algebra() != algebra {
		t.Fatal("literal declaration lost its Numeric authority")
	}
	if rule.semantic != literalKey(3) {
		t.Fatal("literal checker lost the exact Rule semantic identity")
	}
	inventory, ok := composition.RuleAdmissionInventory()
	if !ok || len(inventory.Rules) != 1 || inventory.Rules[0] != (engine.RuleAdmissionRecord{
		Rule: literalKey(3), Basis: engine.RuleAdmissionBasisDerivation, Identity: literalKey(5),
	}) {
		t.Fatal("literal rule did not publish its derivation admission")
	}
	report, ok := composition.SemanticReport()
	if !ok || len(report.Incidences) != 0 || len(report.Components) != 1 || len(report.Components[0].Factors) != 1 || report.Components[0].Factors[0] != literalKey(1) {
		t.Fatal("zero-input literal rule introduced a semantic predecessor")
	}
}

func TestLiteralOperandAndResultAreExactAndOwnerFenced(t *testing.T) {
	algebra, source := literalFixture(t, "literal_exact", "return 1, 2.5, -3")
	scalars := literalScalars(t, source, algebra)
	if len(scalars) != 3 {
		t.Fatalf("literal denominator = %d, want 3", len(scalars))
	}
	accepted := 0
	for index, scalar := range scalars {
		operand, ok := NewOperand(source, algebra, scalar)
		integer, isInteger, literal := scalarInteger(source, scalar)
		if !literal {
			t.Fatalf("literal payload %d", index)
		}
		if !isInteger {
			if ok {
				t.Fatalf("float literal %d entered the integer source rule", index)
			}
			continue
		}
		if !ok || !operand.valid() {
			t.Fatalf("integer literal operand %d", index)
		}
		accepted++
		returned, ok := operand.Scalar()
		if !ok || returned != scalar {
			t.Fatalf("literal scalar round-trip %d", index)
		}
		zero, zeroOK := algebra.Zero()
		value, ok := algebra.IntegerTranslation(operand.key, operand.atom, zero, integer)
		if !ok || !algebra.Admits(operand.key, value) || algebra.Equal(value, algebra.Default()) {
			t.Fatalf("literal result %d", index)
		}
		actual, ok := value.Eligibility(operand.atom)
		if !zeroOK || !ok || actual != numeric.MayInteger {
			t.Fatalf("literal eligibility %d = %v, want integer", index, actual)
		}
		pair, paired := algebra.Pair(operand.atom, zero)
		bound, infinite, bounded := value.Bound(pair)
		if !paired || !bounded || infinite || bound != integer {
			t.Fatalf("literal bound %d = %d/%t/%t, want %d", index, bound, infinite, bounded, integer)
		}
	}
	if accepted != 2 {
		t.Fatalf("integer literal operands = %d, want 2", accepted)
	}
	foreign, foreignSource := literalFixture(t, "literal_foreign", "return 1")
	foreignScalars := literalScalars(t, foreignSource, foreign)
	if len(foreignScalars) != 1 {
		t.Fatal("foreign literal")
	}
	foreignScalar := foreignScalars[0]
	if _, ok := NewOperand(foreignSource, algebra, foreignScalar); ok {
		t.Fatal("foreign source crossed the Numeric owner fence")
	}
	if len(scalars) == 0 {
		t.Fatal("local literal")
	}
	if _, ok := NewOperand(source, foreign, scalars[0]); ok {
		t.Fatal("foreign Numeric owner crossed the Algebra fence")
	}
}

func TestLiteralInstanceDerivesOneExactNumericTarget(t *testing.T) {
	algebra, source := literalFixture(t, "literal_instance", "return 1, 2.5")
	_, _, rule := literalComposition(t, algebra, 20)
	instances := 0
	for index, scalar := range literalScalars(t, source, algebra) {
		operand, ok := NewOperand(source, algebra, scalar)
		if !ok {
			continue
		}
		instances++
		if instance, ok := rule.Instance(operand); !ok || instance == nil {
			t.Fatalf("literal %d did not produce an exact instance", index)
		}
		frozen, digest, frozenOK := operandContent(operand)
		replayed, replayDigest, replayOK := operandContent(frozen)
		if !frozenOK || !replayOK || digest == [32]byte{} || digest != replayDigest || !replayed.valid() {
			t.Fatalf("literal %d operand content was not pure and idempotent", index)
		}
	}
	if instances != 1 {
		t.Fatalf("integer instances = %d, want 1", instances)
	}
}

func TestLiteralRuleRejectsInvalidCapabilitiesAndOperands(t *testing.T) {
	algebra, source := literalFixture(t, "literal_reject", "return 1")
	composition := engine.NewComposition()
	if rule, ok := Declare(composition, literalKey(2), literalKey(3), literalKey(4), nil); ok || rule != nil {
		t.Fatal("nil owner declared a literal rule")
	}
	foreignComposition := engine.NewComposition()
	foreignOwner, ok := numericowner.Declare(foreignComposition, literalKey(1), algebra)
	if !ok {
		t.Fatal("foreign owner")
	}
	if rule, ok := Declare(composition, literalKey(2), literalKey(3), literalKey(4), foreignOwner); ok || rule != nil {
		t.Fatal("foreign-composition owner declared a literal rule")
	}
	_, _, rule := literalComposition(t, algebra, 40)
	if instance, ok := rule.Instance(Operand{}); ok || instance != nil {
		t.Fatal("empty literal operand produced an instance")
	}
	foreignAlgebra, foreignSource := literalFixture(t, "literal_reject_foreign", "return 1")
	foreignScalars := literalScalars(t, foreignSource, foreignAlgebra)
	if len(foreignScalars) != 1 {
		t.Fatal("foreign scalar")
	}
	foreignOperand, ok := NewOperand(foreignSource, foreignAlgebra, foreignScalars[0])
	if !ok {
		t.Fatal("foreign operand")
	}
	if instance, ok := rule.Instance(foreignOperand); ok || instance != nil {
		t.Fatal("foreign operand produced an instance")
	}
	if len(literalScalars(t, source, algebra)) != 1 {
		t.Fatal("fixture denominator changed")
	}
}

func TestLiteralOperandRejectsSameContentForeignLiveOwners(t *testing.T) {
	algebra, source := literalFixture(t, "literal_same_content", "return 1")
	scalars := literalScalars(t, source, algebra)
	if len(scalars) != 1 {
		t.Fatal("local literal")
	}
	foreignSource := sameContentLiteralLink(t, source)
	foreignAlgebra, ok := numeric.New(foreignSource)
	if !ok || foreignSource == source || foreignSource.ContentID() != source.ContentID() || foreignAlgebra.Link() != foreignSource {
		t.Fatal("same-content independent Numeric fixture")
	}
	foreignScalar, ok := foreignAlgebra.ScalarFor(scalars[0].Shard(), scalars[0].Body(), scalars[0].Term())
	if !ok {
		t.Fatal("foreign literal scalar")
	}
	if _, ok := NewOperand(foreignSource, algebra, scalars[0]); ok {
		t.Fatal("same-content foreign Link crossed Numeric literal owner fence")
	}
	if _, ok := NewOperand(source, foreignAlgebra, foreignScalar); ok {
		t.Fatal("same-content foreign Algebra crossed Numeric literal owner fence")
	}
}

func sameContentLiteralLink(t testing.TB, original *link.Link) *link.Link {
	t.Helper()
	contract, ok := original.Boundary().Target()
	if !ok || contract == nil {
		t.Fatal("original Link target")
	}
	mounts := original.Project().Mounts()
	shard, ok := mounts.At(0)
	if !ok {
		t.Fatal("original Link shard")
	}
	name, nameOK := mounts.Name(shard)
	program, programOK := mounts.Program(shard)
	if !nameOK || !programOK || program == nil {
		t.Fatal("original Link module")
	}
	module := linkproject.Module{Name: name, Program: program}
	clone, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{module}})
	if err != nil {
		t.Fatal(err)
	}
	return clone
}

func literalScalars(t testing.TB, source *link.Link, algebra *numeric.Algebra) []numeric.Scalar {
	t.Helper()
	if source == nil || algebra == nil {
		t.Fatal("literal source")
	}
	var result []numeric.Scalar
	mounts := source.Project().Mounts()
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, ok := mounts.At(shardIndex)
		if !ok {
			t.Fatal("literal shard")
		}
		p, ok := source.Project().Mounts().Program(shard)
		if !ok || p == nil {
			t.Fatal("literal program")
		}
		literals := p.Source().Literals()
		for index := 0; index < literals.Integers().Count(); index++ {
			term, owner, _, ok := literals.Integers().At(index)
			if !ok || owner == 0 || !p.Flow().Executable().Contains(term) {
				t.Fatal("integer literal term")
			}
			body, _, _, positioned := p.Source().Index().Position(term)
			if !positioned || body != owner {
				t.Fatal("integer literal position")
			}
			scalar, ok := algebra.ScalarFor(shard, body, term)
			if !ok {
				t.Fatal("integer Numeric scalar")
			}
			result = append(result, scalar)
		}
		for index := 0; index < literals.Floats().Count(); index++ {
			term, owner, _, ok := literals.Floats().At(index)
			if !ok || owner == 0 || !p.Flow().Executable().Contains(term) {
				t.Fatal("float literal term")
			}
			body, _, _, positioned := p.Source().Index().Position(term)
			if !positioned || body != owner {
				t.Fatal("float literal position")
			}
			scalar, ok := algebra.ScalarFor(shard, body, term)
			if !ok {
				t.Fatal("float Numeric scalar")
			}
			result = append(result, scalar)
		}
	}
	return result
}

func scalarInteger(source *link.Link, scalar numeric.Scalar) (int64, bool, bool) {
	if source == nil {
		return 0, false, false
	}
	p, ok := source.Project().Mounts().Program(scalar.Shard())
	if !ok || p == nil {
		return 0, false, false
	}
	term := scalar.Term()
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return 0, false, false
	}
	literals := p.Source().Literals()
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyInteger:
		returned, owner, integer, ok := literals.Integers().At(int(ordinal - 1))
		if ok && returned == term && owner == scalar.Body() {
			return integer, true, true
		}
	case keyspace.FamilyFloat:
		returned, owner, _, ok := literals.Floats().At(int(ordinal - 1))
		if ok && returned == term && owner == scalar.Body() {
			return 0, false, true
		}
	}
	return 0, false, false
}

func literalFixture(t testing.TB, name, text string) (*numeric.Algebra, *link.Link) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name + ".lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	algebra, ok := numeric.New(source)
	if !ok {
		t.Fatal("Numeric algebra")
	}
	return algebra, source
}

func literalComposition(t testing.TB, algebra *numeric.Algebra, offset uint64) (*engine.Composition, *numericowner.Owner, *Rule) {
	t.Helper()
	composition := engine.NewComposition()
	owner, ok := numericowner.Declare(composition, literalKey(offset+1), algebra)
	if !ok {
		t.Fatal("Numeric owner")
	}
	rule, ok := Declare(composition, literalKey(offset+3), literalKey(offset+4), literalKey(offset+5), owner)
	if !ok || rule == nil || !literalTestQuery(composition, owner, offset+6) || !composition.Seal() {
		t.Fatal("literal composition")
	}
	return composition, owner, rule
}

func literalTestQuery(composition *engine.Composition, owner *numericowner.Owner, offset uint64) bool {
	query, ok := engine.DeclareQuery(composition, engine.QuerySpec[bool]{
		Semantic: literalKey(offset), Project: func(engine.Observation) bool { return true },
		Result: engine.FrozenResult[bool]{
			Semantic: literalKey(offset + 1), Freeze: func(value bool) bool { return value }, Clone: func(value bool) bool { return value }, Equal: func(left, right bool) bool { return left == right },
			Fingerprint: func(value bool) uint64 {
				if value {
					return 1
				}
				return 0
			},
		},
	}, func(query *engine.Query[bool]) bool {
		_, declared := engine.QueryReadFrom(query, owner.ExactRead())
		return declared
	})
	return ok && query != nil
}

func literalKey(value uint64) engine.SemanticKey {
	var digest [32]byte
	binary.BigEndian.PutUint64(digest[24:], value)
	key, ok := engine.NewSemanticKey(digest, 1)
	if !ok {
		panic("literal test key")
	}
	return key
}
