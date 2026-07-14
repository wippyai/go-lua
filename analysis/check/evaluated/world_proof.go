package evaluated

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func validateWorldProof(reg *axis.Registry, proof WorldProof) error {
	for index, expression := range proof.Expressions {
		want := ExpressionID(index + 1)
		if expression.ID != want || expression.Op <= ExpressionInvalid || expression.Op > ExpressionLuaTypeName {
			return fmt.Errorf("evaluated: invalid dense expression node %d", expression.ID)
		}
		for _, argument := range expression.Args {
			if argument == 0 || argument >= expression.ID {
				return fmt.Errorf("evaluated: expression %d has non-prior argument", expression.ID)
			}
		}
		neutralConstant := product.BelongsToRegistry(reg, expression.Constant) && product.Equal(reg, expression.Constant, product.Top())
		switch expression.Op {
		case ExpressionRoot:
			if expression.RootKind <= RootInvalid || expression.RootKind > RootHeapTemplate || len(expression.Args) != 0 ||
				!expression.Scalar.IsZero() || !neutralConstant {
				return fmt.Errorf("evaluated: malformed root expression %d", expression.ID)
			}
		case ExpressionConstant:
			scalar := expression.Scalar.Valid()
			productConstant := expression.Scalar.IsZero() && artifactSafeValue(reg, expression.Constant)
			if len(expression.Args) != 0 || scalar == productConstant || scalar && !product.Equal(reg, expression.Constant, product.Top()) {
				return fmt.Errorf("evaluated: unsafe constant expression %d", expression.ID)
			}
		case ExpressionRefinement, ExpressionRuntimeValidation, ExpressionLuaTypeName:
			validConstant := neutralConstant
			if expression.Op != ExpressionLuaTypeName {
				validConstant = artifactSafeValue(reg, expression.Constant)
			}
			if !expression.Scalar.IsZero() || len(expression.Args) != 1 || !validConstant {
				return fmt.Errorf("evaluated: malformed unary expression %d", expression.ID)
			}
		default:
			if !expression.Scalar.IsZero() || !neutralConstant || len(expression.Args) < 2 {
				return fmt.Errorf("evaluated: malformed composite expression %d", expression.ID)
			}
		}
	}
	for index, predicate := range proof.Predicates {
		if predicate.ID != uint32(index) || predicate.Value == 0 || int(predicate.Value) > len(proof.Expressions) {
			return fmt.Errorf("evaluated: invalid predicate %d", predicate.ID)
		}
	}
	for index, decision := range proof.Decisions {
		want := DecisionID(index + 2)
		if decision.ID != want || int(decision.Predicate) >= len(proof.Predicates) || decision.Low >= decision.ID || decision.High >= decision.ID || decision.Low == decision.High {
			return fmt.Errorf("evaluated: invalid reduced decision %d", decision.ID)
		}
		for _, child := range []DecisionID{decision.Low, decision.High} {
			if child < 2 {
				continue
			}
			childNode := proof.Decisions[int(child)-2]
			if childNode.Predicate <= decision.Predicate {
				return fmt.Errorf("evaluated: decision %d violates predicate order", decision.ID)
			}
		}
	}
	return nil
}

func validWorldSet(proof WorldProof, set WorldSet) bool {
	return set.Root <= DecisionID(len(proof.Decisions)+1)
}

func cloneWorldProof(in WorldProof) WorldProof {
	out := WorldProof{
		Expressions: make([]Expression, len(in.Expressions)),
		Predicates:  append([]Predicate(nil), in.Predicates...),
		Decisions:   append([]Decision(nil), in.Decisions...),
	}
	for index, expression := range in.Expressions {
		out.Expressions[index] = expression
		out.Expressions[index].Args = append([]ExpressionID(nil), expression.Args...)
	}
	return out
}
