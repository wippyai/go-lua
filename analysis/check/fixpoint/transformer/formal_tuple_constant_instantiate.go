package transformer

import "fmt"

// instantiateConstant lowers one immutable template constant into this solve
// transaction's sole tuple carrier. Constants own product factors, never a
// persistent directory root: every run starts from its own default root and
// interns the factors into its own terminal and directory arenas.
func (a *formalTupleAlgebra) instantiateConstant(ref formalRelationTupleConstantRef) (formalRelationTuple, error) {
	if a == nil || a.program == nil || ref.template == nil || ref.target.region == nil {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple constant is unowned")
	}
	constant, ok := ref.constant(ref.target)
	if !ok || !constant.valid() || constant.forest != a.program.formalFibers ||
		int(constant.variable) > len(ref.template.rootInputs) ||
		ref.template.rootInputs[constant.variable-1].program != a.program {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple constant has foreign ownership")
	}
	if prior, present := a.constants[ref]; present {
		if err := a.validateTuple(prior); err != nil || prior.variable != constant.variable {
			if err != nil {
				return formalRelationTuple{}, err
			}
			return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple constant cache is malformed")
		}
		return prior, nil
	}
	tuple, err := a.instantiatePreparedConstant(constant)
	if err != nil {
		return formalRelationTuple{}, err
	}
	if a.constants == nil {
		a.constants = make(map[formalRelationTupleConstantRef]formalRelationTuple)
	}
	a.constants[ref] = tuple
	return tuple, nil
}

// instantiatePreparedConstant lowers one already-owned factor constant into
// this run's directory and decision arenas. Template constants and the one
// production root-entry seed share this exact full-product law.
func (a *formalTupleAlgebra) instantiatePreparedConstant(constant formalRelationTupleConstant) (formalRelationTuple, error) {
	if a == nil || a.program == nil || !constant.valid() || constant.forest != a.program.formalFibers {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple constant is unowned")
	}
	span, directory, _, ok := a.span(constant.variable)
	if !ok || span.forest != constant.forest || span.variable != constant.variable ||
		len(constant.groups) != span.groupCount {
		return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple constant has no run-local span")
	}

	tuple := formalRelationTuple{variable: constant.variable, root: directory.defaultRoot()}
	if constant.care {
		var err error
		tuple, err = a.writeCare(tuple, decisionTrue)
		if err != nil {
			return formalRelationTuple{}, err
		}
	}
	for index, value := range constant.groups {
		group := span.forest.groups[span.groupFirst+index]
		if !value.group.same(group) {
			return formalRelationTuple{}, fmt.Errorf("transformer: formal tuple constant group order drifted")
		}
		var err error
		switch group.kind {
		case formalFiberGroupValues:
			tuple, err = a.writeValuesFactor(tuple, formalValuesFiberGroup{descriptor: group}, value.values)
		case formalFiberGroupOrdinaryLane:
			tuple, err = a.writeOrdinaryFactor(tuple, formalOrdinaryLaneFiberGroup{descriptor: group}, value.factor)
		case formalFiberGroupCoordinateLane:
			tuple, err = a.writeCoordinateFactor(tuple, formalCoordinateLaneFiberGroup{descriptor: group}, value.factor)
		default:
			err = fmt.Errorf("transformer: formal tuple constant has invalid group kind")
		}
		if err != nil {
			return formalRelationTuple{}, err
		}
	}
	tuple = a.normalize(tuple)
	if tuple.bottom() {
		return formalRelationTuple{}, fmt.Errorf("transformer: live formal tuple constant normalized to Bottom")
	}
	if err := a.validateTuple(tuple); err != nil {
		return formalRelationTuple{}, err
	}
	return tuple, nil
}
