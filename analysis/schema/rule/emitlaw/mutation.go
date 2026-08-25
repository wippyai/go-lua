package emitlaw

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule/program"
)

// mutation is one structural edit of a declaration, stated twice: as the Go
// statement the emitted law performs, and as the function this emitter applies
// while deriving. The two are authored on one row so the outcome baked into
// the emitted law is the outcome of the statement the law actually runs.
type mutation struct {
	// name is the sentence the emitted law reports the mutation by.
	name string
	// statement is the Go source of the edit, over a local named declaration.
	statement string
	// apply performs the same edit here, so Check's verdict is observed rather
	// than predicted.
	apply func(*program.Program)
	// mandatory marks an edit that breaks the declaration by construction: it
	// clears a term Program.Available or JoinDecl.Available is stated over. An
	// admitted mandatory mutation is a hole in Check and refuses generation
	// rather than being emitted as a documented allowance.
	mandatory bool
}

// verdict is one mutation and the answer Check gave it while deriving.
type verdict struct {
	mutation
	problem program.Problem
	refused bool
}

// mutations enumerates the structural edits of one declaration. It is total
// over the terms the cold ABI carries: every join clause, every fold clause,
// every output clause, the carry when one is declared, and every transport
// row. An authored suite samples three to five of these; the emitted suite
// states all of them, which is why the generated law is stronger than the
// authored law it replaces rather than merely equal to it.
func mutations(declaration program.Program) []mutation {
	rows := []mutation{
		{
			name:      "the operand role is cleared",
			statement: `declaration.OperandRole = ""`,
			apply:     func(target *program.Program) { target.OperandRole = "" },
			mandatory: true,
		},
		{
			name:      "the candidate provider is cleared",
			statement: `declaration.Candidate = ` + memberPackage + `.CandidateRef{}`,
			apply:     func(target *program.Program) { target.Candidate = zeroCandidate() },
			mandatory: true,
		},
	}
	if declaration.Candidate.AxisRelation.Declared() {
		rows = append(rows, mutation{
			name:      "the candidate relation loses its member",
			statement: `declaration.Candidate.AxisRelation.Member = ""`,
			apply:     func(target *program.Program) { target.Candidate.AxisRelation.Member = "" },
		})
	}
	if declaration.Candidate.AxisRelation.Axis != declaration.Fold.Reducer.Axis {
		rows = append(rows, mutation{
			name:      "the candidate is provided by the axis the rule writes instead of its own owner",
			statement: "declaration.Candidate.AxisRelation.Axis = declaration.Fold.Reducer.Axis",
			apply: func(target *program.Program) {
				target.Candidate.AxisRelation.Axis = target.Fold.Reducer.Axis
			},
		})
	}
	for index := range declaration.Joins {
		rows = append(rows, joinMutations(declaration, index)...)
	}
	rows = append(rows, foldMutations(declaration)...)
	for index := range declaration.Fold.Outputs {
		rows = append(rows, outputMutations(declaration, index)...)
	}
	rows = append(rows, carryMutations(declaration)...)
	for index := range declaration.Transport {
		position := index
		rows = append(rows, mutation{
			name:      fmt.Sprintf("transport row %d loses its axis", position),
			statement: fmt.Sprintf("declaration.Transport[%d].Axis = %s.AxisRef{}", position, programPackage),
			apply:     func(target *program.Program) { target.Transport[position].Axis = program.AxisRef{} },
			mandatory: true,
		})
	}
	return rows
}

func joinMutations(declaration program.Program, index int) []mutation {
	join := declaration.Joins[index]
	rows := []mutation{
		{
			name:      fmt.Sprintf("join %d loses its relation", index),
			statement: fmt.Sprintf(`declaration.Joins[%d].Relation.Member = ""`, index),
			apply:     func(target *program.Program) { target.Joins[index].Relation.Member = "" },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its key projection", index),
			statement: fmt.Sprintf(`declaration.Joins[%d].Key.Member = ""`, index),
			apply:     func(target *program.Program) { target.Joins[index].Key.Member = "" },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its read axis", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Axis = %s.AxisRef{}", index, programPackage),
			apply:     func(target *program.Program) { target.Joins[index].Read.Axis = program.AxisRef{} },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its read form", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Form = %s.ReadFormInvalid", index, programPackage),
			apply:     func(target *program.Program) { target.Joins[index].Read.Form = program.ReadFormInvalid },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its point-bound disposition", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.PointBound = %s.PointBoundInvalid", index, programPackage),
			apply:     func(target *program.Program) { target.Joins[index].Read.PointBound = program.PointBoundInvalid },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its declared cell order", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Contract.Order = %s.OrderInvalid", index, programPackage),
			apply:     func(target *program.Program) { target.Joins[index].Read.Contract.Order = program.OrderInvalid },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its sparsity", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Contract.Sparse = %s.SparseInvalid", index, programPackage),
			apply:     func(target *program.Program) { target.Joins[index].Read.Contract.Sparse = program.SparseInvalid },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its opaque-evidence disposition", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Contract.OnOpaque = %s.OnOpaqueInvalid", index, programPackage),
			apply:     func(target *program.Program) { target.Joins[index].Read.Contract.OnOpaque = program.OnOpaqueInvalid },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d loses its multiplicity", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Contract.Multiplicity = %s.MultiplicityInvalid", index, programPackage),
			apply: func(target *program.Program) {
				target.Joins[index].Read.Contract.Multiplicity = program.MultiplicityInvalid
			},
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d repeats one of its sources", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Sources = append(declaration.Joins[%d].Sources, declaration.Joins[%d].Sources[0])", index, index, index),
			apply: func(target *program.Program) {
				target.Joins[index].Sources = append(target.Joins[index].Sources, target.Joins[index].Sources[0])
			},
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d sources a result that is not yet produced", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Sources[0] = %s.PriorSource(%d)", index, programPackage, index),
			apply: func(target *program.Program) {
				target.Joins[index].Sources[0] = program.PriorSource(uint64(index))
			},
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("join %d is unbounded", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Contract.Multiplicity = %s.MultiplicityMany", index, programPackage),
			apply: func(target *program.Program) {
				target.Joins[index].Read.Contract.Multiplicity = program.MultiplicityMany
			},
		},
	}
	if program.RequiresFactorDenominator(join.Read.Form, join.Read.Contract.Sparse, join.Parent.Declared() || join.KeyVector.Declared()) {
		rows = append(rows, mutation{
			name:      fmt.Sprintf("join %d loses the denominator its read form requires", index),
			statement: fmt.Sprintf("declaration.Joins[%d].Read.Contract.DenominatorRef = %s.DenominatorRef{}", index, programPackage),
			apply: func(target *program.Program) {
				target.Joins[index].Read.Contract.DenominatorRef = program.DenominatorRef{}
			},
			mandatory: true,
		})
	}
	if join.Predicate.Declared() {
		rows = append(rows, mutation{
			name:      fmt.Sprintf("join %d declares a predicate that resolves to nothing", index),
			statement: fmt.Sprintf(`declaration.Joins[%d].Predicate.Member = ""`, index),
			apply:     func(target *program.Program) { target.Joins[index].Predicate.Member = "" },
			mandatory: true,
		})
	}
	if join.Parent.Declared() {
		// The parent restatement is what admits an untagged summary. A join
		// that loses it is correlated by nothing, and one that keeps a name
		// resolving to no relation restates a fact the catalog does not hold.
		rows = append(rows, mutation{
			name:      fmt.Sprintf("join %d restates a parent that resolves to nothing", index),
			statement: fmt.Sprintf(`declaration.Joins[%d].Parent.Member = ""`, index),
			apply:     func(target *program.Program) { target.Joins[index].Parent.Member = "" },
			mandatory: true,
		})
	}
	return rows
}

func foldMutations(declaration program.Program) []mutation {
	rows := []mutation{
		{
			name:      "the fold names no reducer",
			statement: `declaration.Fold.Reducer.Member = ""`,
			apply:     func(target *program.Program) { target.Fold.Reducer.Member = "" },
			mandatory: true,
		},
	}
	// A mutation states a law about a term the declaration actually carries. A
	// fold that consumes nothing has no input vector to empty and no input
	// position to misdirect, so these two rows would ask the checker to refuse
	// a declaration identical to the one it just admitted. They are gated on
	// the term, exactly as the per-join rows are gated on the join restating a
	// predicate or a parent, and stay mandatory for every fold that has one.
	if len(declaration.Fold.Inputs) != 0 {
		rows = append(rows, mutation{
			name:      "the fold consumes nothing",
			statement: "declaration.Fold.Inputs = nil",
			apply:     func(target *program.Program) { target.Fold.Inputs = nil },
			mandatory: true,
		})
	}
	rows = append(rows, mutation{
		name:      "the fold publishes nothing",
		statement: "declaration.Fold.Outputs = nil",
		apply:     func(target *program.Program) { target.Fold.Outputs = nil },
		mandatory: true,
	})
	if len(declaration.Fold.Inputs) != 0 {
		rows = append(rows, mutation{
			name:      "the fold consumes a join the declaration does not carry",
			statement: fmt.Sprintf("declaration.Fold.Inputs[0] = %s.JoinRef(%d)", programPackage, len(declaration.Joins)),
			apply: func(target *program.Program) {
				target.Fold.Inputs[0] = program.JoinRef(len(declaration.Joins))
			},
			mandatory: true,
		})
	}
	if len(declaration.Fold.Inputs) > 1 {
		for position := range declaration.Fold.Inputs {
			dropped := position
			rows = append(rows, mutation{
				name: fmt.Sprintf("the fold stops consuming its argument at position %d", dropped),
				statement: fmt.Sprintf("declaration.Fold.Inputs = append(declaration.Fold.Inputs[:%d:%d], declaration.Fold.Inputs[%d:]...)",
					dropped, dropped, dropped+1),
				apply: func(target *program.Program) {
					target.Fold.Inputs = append(target.Fold.Inputs[:dropped:dropped], target.Fold.Inputs[dropped+1:]...)
				},
			})
		}
	}
	return rows
}

func outputMutations(declaration program.Program, index int) []mutation {
	output := declaration.Fold.Outputs[index]
	rows := []mutation{
		{
			name:      fmt.Sprintf("output %d loses its destination projection", index),
			statement: fmt.Sprintf(`declaration.Fold.Outputs[%d].Destination.Member = ""`, index),
			apply:     func(target *program.Program) { target.Fold.Outputs[index].Destination.Member = "" },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("output %d loses its declared column", index),
			statement: fmt.Sprintf(`declaration.Fold.Outputs[%d].Column.Key = ""`, index),
			apply:     func(target *program.Program) { target.Fold.Outputs[index].Column.Key = "" },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("output %d loses its publication mode", index),
			statement: fmt.Sprintf("declaration.Fold.Outputs[%d].Mode = %s.ModeInvalid", index, programPackage),
			apply:     func(target *program.Program) { target.Fold.Outputs[index].Mode = program.ModeInvalid },
			mandatory: true,
		},
		{
			name:      fmt.Sprintf("output %d publishes into a value slot the fold does not have", index),
			statement: fmt.Sprintf("declaration.Fold.Outputs[%d].ValueSlot = %d", index, len(declaration.Fold.Outputs)),
			apply: func(target *program.Program) {
				target.Fold.Outputs[index].ValueSlot = uint16(len(declaration.Fold.Outputs))
			},
		},
	}
	if output.Mode != program.ModeRoute {
		return append(rows, mutation{
			name:      fmt.Sprintf("output %d claims a route it does not publish through", index),
			statement: fmt.Sprintf("declaration.Fold.Outputs[%d].RouteJoinPresent = true", index),
			apply:     func(target *program.Program) { target.Fold.Outputs[index].RouteJoinPresent = true },
		})
	}
	rows = append(rows,
		mutation{
			name:      fmt.Sprintf("routed output %d names no route join", index),
			statement: fmt.Sprintf("declaration.Fold.Outputs[%d].RouteJoinPresent = false", index),
			apply:     func(target *program.Program) { target.Fold.Outputs[index].RouteJoinPresent = false },
			mandatory: true,
		},
		mutation{
			name:      fmt.Sprintf("routed output %d routes through a join the declaration does not carry", index),
			statement: fmt.Sprintf("declaration.Fold.Outputs[%d].RouteJoin = %s.JoinRef(%d)", index, programPackage, len(declaration.Joins)),
			apply: func(target *program.Program) {
				target.Fold.Outputs[index].RouteJoin = program.JoinRef(len(declaration.Joins))
			},
			mandatory: true,
		},
	)
	for position, join := range declaration.Joins {
		if join.Read.Form != program.Exact {
			continue
		}
		exact := position
		rows = append(rows, mutation{
			name:      fmt.Sprintf("routed output %d routes through the exact join at %d", index, exact),
			statement: fmt.Sprintf("declaration.Fold.Outputs[%d].RouteJoin = %s.JoinRef(%d)", index, programPackage, exact),
			apply: func(target *program.Program) {
				target.Fold.Outputs[index].RouteJoin = program.JoinRef(exact)
			},
			mandatory: true,
		})
		break
	}
	return rows
}

func carryMutations(declaration program.Program) []mutation {
	if declaration.Carry == nil {
		return nil
	}
	rows := []mutation{
		{
			name:      "the carry loses its disposition",
			statement: fmt.Sprintf("declaration.Carry.Mode = %s.CarryModeInvalid", programPackage),
			apply:     func(target *program.Program) { target.Carry.Mode = program.CarryModeInvalid },
			mandatory: true,
		},
		{
			name:      "the carry names an input port the declaration does not open",
			statement: fmt.Sprintf("declaration.Carry.Input = %s.InputRef(%d)", programPackage, inputPortCount(declaration)+1),
			apply: func(target *program.Program) {
				target.Carry.Input = program.InputRef(inputPortCount(declaration) + 1)
			},
		},
	}
	if declaration.Carry.Mode == program.CarryTransform {
		return append(rows, mutation{
			name:      "the transforming carry loses its transform",
			statement: `declaration.Carry.Transform.Member = ""`,
			apply:     func(target *program.Program) { target.Carry.Transform.Member = "" },
			mandatory: true,
		})
	}
	return append(rows, mutation{
		name: "the identity carry acquires a transform",
		statement: fmt.Sprintf(`declaration.Carry.Transform = %s.CarryTransformRef{Axis: declaration.Fold.Reducer.Axis, Member: %q}`,
			memberPackage, mutationTransformKey),
		apply: func(target *program.Program) {
			target.Carry.Transform.Axis = target.Fold.Reducer.Axis
			target.Carry.Transform.Member = mutationTransformKey
		},
		mandatory: true,
	})
}

// mutationTransformKey is the member key the identity-carry mutation names. It
// resolves to nothing: the mutation states that an identity carry with any
// transform at all is malformed, which is a property of the mode and not of
// the member the transform happens to name.
const mutationTransformKey schema.Key = "law/mutation/carry-transform"

// inputPortCount is the width of the contiguous input prefix the declaration
// opens, which is what a carry naming a port beyond it violates.
func inputPortCount(declaration program.Program) uint64 {
	highest := uint64(0)
	for _, join := range declaration.Joins {
		if port := join.Read.Input.Uint64(); port+1 > highest {
			highest = port + 1
		}
	}
	if declaration.Carry != nil {
		if port := declaration.Carry.Input.Uint64(); port+1 > highest {
			highest = port + 1
		}
	}
	return highest
}

// observe applies every mutation to a fresh clone and records Check's answer,
// so the emitted law asserts a verdict this emitter watched rather than one it
// predicted.
func observe(ruleKey schema.Key, declaration program.Program) ([]verdict, error) {
	return observeRows(ruleKey, declaration, mutations(declaration))
}

// observeRows is observe over an explicit catalog, so the emitter's own laws
// can state the gate over a row they control.
func observeRows(ruleKey schema.Key, declaration program.Program, rows []mutation) ([]verdict, error) {
	verdicts := make([]verdict, 0, len(rows))
	for _, row := range rows {
		mutated := declaration.Clone()
		row.apply(&mutated)
		problem, valid := mutated.Check()
		if valid && row.mandatory {
			return nil, unexpressible(ruleKey, "a declaration term whose removal it does not refuse",
				fmt.Sprintf("%s, and Check admits the result: the structural law suite will not certify a term the checker does not hold", row.name))
		}
		verdicts = append(verdicts, verdict{mutation: row, problem: problem, refused: !valid})
	}
	return verdicts, nil
}
