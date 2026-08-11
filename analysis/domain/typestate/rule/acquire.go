// Package rule declares Typestate's Link-backed provider lifecycle Rules.
// It owns no protocol carrier, holder-count relation, or composition root.
package rule

import (
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	typestateowner "github.com/wippyai/go-lua/analysis/domain/typestate/owner"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

// AcquireOperand is one exact Typestate acquisition declaration. The opaque
// declaration was derived at the Typestate boundary from a ResourceOrigin and
// the corresponding Contract acquisition row.
type AcquireOperand struct {
	acquisition typestate.Acquisition
}

// NewAcquireOperand binds one already-derived Typestate acquisition
// declaration. It cannot reconstruct or substitute a Link protocol row.
func NewAcquireOperand(schema typestate.Schema, acquisition typestate.Acquisition) (AcquireOperand, bool) {
	if !schema.Valid() || !schema.ValidAcquisition(acquisition) {
		return AcquireOperand{}, false
	}
	return AcquireOperand{acquisition: acquisition}, true
}

// OriginID returns the stable structural resource-origin identity.
func (operand AcquireOperand) OriginID() keyspace.ContentID { return operand.acquisition.ContentID() }

// AcquireDeclaration is Typestate's typed cold acquisition Rule capability.
type AcquireDeclaration struct {
	semantic engine.SemanticKey
	rule     *engine.Rule[typestate.Relation, AcquireOperand]
	owner    *typestateowner.Owner
	write    engine.Write[typestate.Relation]
}

// DeclareAcquire records the source-independent provider acquisition
// judgment. Its only semantic source is the exact Link acquisition operand;
// Value/Call reachability and outcome transport are separate owner Rules and
// are deliberately not reconstructed here.
func DeclareAcquire(composition *engine.Composition, semantic, operandFamily, evidence engine.SemanticKey, owner *typestateowner.Owner) (*AcquireDeclaration, bool) {
	if composition == nil || owner == nil || !semantic.Available() || !operandFamily.Available() || !evidence.Available() {
		return nil, false
	}
	declaration := &AcquireDeclaration{semantic: semantic, owner: owner}
	rule, ok := engine.DeclareRule(composition, engine.RuleSpec[typestate.Relation, AcquireOperand]{
		Semantic: semantic, OperandFamily: operandFamily, OperandContent: acquireOperandContent, Output: owner.Output(), Inputs: 0,
		Admission: engine.AdmitRuleByDerivation(evidence, declaration.check), Transfer: declaration.transfer,
	}, func(rule *engine.Rule[typestate.Relation, AcquireOperand]) bool {
		write, writeOK := engine.WriteTo(rule, owner.Write())
		if !writeOK {
			return false
		}
		declaration.rule, declaration.write = rule, write
		return true
	})
	if !ok || rule == nil || declaration.rule != rule {
		return nil, false
	}
	return declaration, true
}

// NewInstance binds one exact acquisition key after its cold composition
// seals. It does not construct a Program or an engine execution root.
func (declaration *AcquireDeclaration) NewInstance(operand AcquireOperand) (*engine.RuleInstance[typestate.Relation, AcquireOperand], bool) {
	if declaration == nil || declaration.rule == nil || declaration.owner == nil || !validAcquireOperand(operand, declaration.owner.Schema()) {
		return nil, false
	}
	return engine.NewRuleInstance(declaration.rule, operand, func(binding *engine.RuleBinding[typestate.Relation, AcquireOperand]) bool {
		ref, ok := declaration.owner.Locate(operand.acquisition.Key())
		return ok && engine.InstanceWrite(binding, declaration.write, ref)
	})
}

func acquireOperandContent(operand AcquireOperand) (AcquireOperand, [32]byte, bool) {
	content := operand.acquisition.ContentID()
	if !content.Available() || !operand.acquisition.Key().Resource.ContentID().Available() {
		return AcquireOperand{}, [32]byte{}, false
	}
	return operand, [32]byte(content), true
}

func validAcquireOperand(operand AcquireOperand, schema typestate.Schema) bool {
	return schema.ValidAcquisition(operand.acquisition)
}

func (declaration *AcquireDeclaration) transfer(access engine.Access[typestate.Relation, AcquireOperand]) bool {
	operand, operandOK := engine.Operand(access)
	if !operandOK || declaration == nil || declaration.owner == nil || !validAcquireOperand(operand, declaration.owner.Schema()) {
		return false
	}
	return engine.Product(access, func(row engine.Row) bool {
		value, ok := declaration.owner.Algebra().Acquire(operand.acquisition)
		if !ok || value.Key != operand.acquisition.Key() {
			return engine.NoCandidate(access, row)
		}
		return engine.StageValue(access, row, value.Value)
	})
}

func (declaration *AcquireDeclaration) check(derivation engine.RuleDerivation[typestate.Relation, AcquireOperand]) (engine.RuleEvidence, bool) {
	// A zero-input source has exactly one total disposition.  In particular,
	// accepting an empty disposition list would admit a forged no-op proof;
	// accepting an arbitrary admitted relation would let a same-shaped source
	// fabricate a protocol state.  The Link row determines both target and
	// exact initial relation.
	if declaration == nil || declaration.owner == nil || derivation.Rule() != declaration.semantic ||
		derivation.InputCount() != 0 || derivation.ReadCount() != 0 || derivation.DispositionCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	operand, ok := derivation.Operand()
	if !ok || !validAcquireOperand(operand, declaration.owner.Schema()) || !derivation.OperandContentMatches([32]byte(operand.acquisition.ContentID())) {
		return engine.RuleEvidence{}, false
	}
	ref, refOK := declaration.owner.Locate(operand.acquisition.Key())
	if !refOK {
		return engine.RuleEvidence{}, false
	}
	disposition, ok := derivation.DispositionAt(0)
	if !ok || disposition.Kind() != engine.RuleDispositionStaged || disposition.Guard().Empty() || disposition.TargetCount() != 1 {
		return engine.RuleEvidence{}, false
	}
	value, staged := disposition.Value()
	target, targetOK := disposition.TargetAt(0)
	expected, expectedOK := declaration.owner.Algebra().Acquire(operand.acquisition)
	if !staged || !targetOK || !expectedOK || expected.Key != operand.acquisition.Key() ||
		!declaration.owner.Algebra().Equal(value, expected.Value) || !engine.TargetMatchesRef(target, ref) {
		return engine.RuleEvidence{}, false
	}
	return derivation.Accept()
}
