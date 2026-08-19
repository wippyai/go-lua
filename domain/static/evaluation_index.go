package static

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	"github.com/wippyai/go-lua/analysis/program/target/contract"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/type/typ"
)

func (a *Authority) sealHotProjections(contract *contract.Contract) error {
	if a == nil || !a.linkID.Available() || contract == nil {
		return errors.New("static: unavailable hot projection source")
	}
	if len(a.mounts) == 0 {
		return errors.New("static: mounted artifacts required")
	}
	for _, mount := range a.mounts {
		if !mount.NamespaceID.Available() {
			return errors.New("static: unavailable mounted namespace")
		}
		a.namespaceIDs[mount.NamespaceID] = struct{}{}
	}
	return nil
}

func (a *Authority) sealContainedOperands() error {
	if a == nil || !a.linkID.Available() || a.types == nil {
		return errors.New("static: unavailable contained-operand source")
	}
	if len(a.mounts) == 0 {
		return errors.New("static: mounted artifacts required")
	}
	return a.sealMountedContainedOperands()
}

func (a *Authority) sealMountedContainedOperands() error {
	for _, mount := range a.mounts {
		inputCount, countOK := mount.Program.StaticInputCount()
		if !mount.Program.Available() || !countOK {
			return errors.New("static: unavailable mounted input family")
		}
		for index := 0; index < inputCount; index++ {
			row, ok := mount.Program.StaticInputAt(index)
			if !ok || !row.Available() || row.Owner() != mount.Program.ProgramID {
				return errors.New("static: malformed mounted input row")
			}
			operand := ContainedOperand{owner: row.Owner(), source: row.OperandID(), namespace: mount.NamespaceID, law: a.lawID, dependency: row.Owner(), site: row.SourceID(), frontierBody: row.FrontierID(), frontierCursor: row.Cursor()}
			switch staticquery.StaticOperandKind(row.OperandKind()) {
			case staticquery.StaticOperandKnown:
				literal := row.OperandLiteral()
				var value typ.Type
				switch literal.Kind {
				case keyspace.LiteralBool:
					value = typ.LiteralBool(literal.Bool)
				case keyspace.LiteralInteger:
					value = typ.LiteralInt(literal.Integer)
				case keyspace.LiteralFloat:
					value = typ.LiteralNumber(math.Float64frombits(literal.FloatBits))
				case keyspace.LiteralString:
					value = typ.LiteralString(literal.String)
				case 0: // zero LiteralValue is the exact Program-issued nil.
					value = typ.Nil
				default:
					return errors.New("static: malformed mounted literal operand")
				}
				closed, err := a.addClosed(value)
				if err != nil {
					return err
				}
				operand.kind, operand.known = OperandKnown, closed
			case staticquery.StaticOperandTypeValue:
				ref, refOK := a.types.FindByReferenceID(row.OperandReferenceID())
				value, valueOK := a.types.Resolve(ref)
				if !refOK || !valueOK {
					return errors.New("static: mounted TypeValue operand reference unavailable")
				}
				closed, err := a.addClosed(value)
				if err != nil {
					return err
				}
				operand.kind, operand.known = OperandKnown, closed
			case staticquery.StaticOperandRuntimeSubject:
				subject := RuntimeSubject{linkID: a.linkID, id: row.OperandSubjectID(), body: row.OperandBodyPathID()}
				if !subject.Valid() {
					return errors.New("static: mounted RuntimeSubject receipt unavailable")
				}
				operand.kind, operand.subject = OperandRuntimeSubject, subject
			default:
				return errors.New("static: malformed mounted operand disposition")
			}
			a.operands[row.ID()] = operand
		}
	}
	return nil
}

func (a *Authority) sealTypeOfOutputs() error {
	if a == nil || !a.linkID.Available() || a.typeOfOutputs != nil {
		return errors.New("static: unavailable typeof output projection")
	}
	a.typeOfOutputs = make(map[identity.ContentID]Coordinate)
	if len(a.mounts) == 0 {
		return errors.New("static: mounted artifacts required")
	}
	return a.sealMountedTypeOfOutputs()
}

func (a *Authority) sealMountedTypeOfOutputs() error {
	for _, mount := range a.mounts {
		inputCount, countOK := mount.Program.StaticInputCount()
		expressionCount, expressionsOK := mount.Program.StaticExpressionCount()
		if !mount.Program.Available() || !countOK || !expressionsOK {
			return errors.New("static: unavailable mounted static family")
		}
		for index := 0; index < inputCount; index++ {
			row, ok := mount.Program.StaticInputAt(index)
			if !ok || !row.Available() || row.Owner() != mount.Program.ProgramID || row.Kind() != programschema.StaticInputTypeOf {
				continue
			}
			var expression programschema.StaticExpression
			expressionOK := false
			for expressionIndex := 0; expressionIndex < expressionCount; expressionIndex++ {
				candidate, candidateOK := mount.Program.StaticExpressionAt(expressionIndex)
				if candidateOK && candidate.ID() == row.ExpressionID() {
					expression, expressionOK = candidate, true
					break
				}
			}
			if !expressionOK || !expression.Available() {
				return errors.New("static: mounted typeof expression unavailable")
			}
			ref, refOK := a.types.FindByReferenceID(expression.ReferenceID())
			if !refOK {
				return errors.New("static: mounted typeof reference unavailable")
			}
			coordinate, coordinateOK := a.coordinateFor(coordinateKey{reference: ref, namespace: mount.NamespaceID})
			if !coordinateOK {
				return errors.New("static: mounted typeof coordinate unavailable")
			}
			if _, duplicate := a.typeOfOutputs[row.ID()]; duplicate {
				return errors.New("static: duplicate mounted typeof output")
			}
			a.typeOfOutputs[row.ID()] = coordinate
		}
	}
	return nil
}
