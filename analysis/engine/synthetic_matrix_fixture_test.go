package engine

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
)

type staticMatrixPermutation struct {
	factors  []int
	rules    []int
	queries  []int
	topology []int
}

type staticMatrixObservation struct {
	composition CompositionID
	topology    *equation.Batch
	values      []uint64
}

type staticMatrixFixture struct {
	composition *Composition
	topology    *equation.Batch
	solver      *Solver
	queries     []*Query[uint64]
	receipts    []QueryReceipt[uint64]
}

func staticMatrixPermutations(count int) []staticMatrixPermutation {
	forward := staticMatrixForward(count)
	return []staticMatrixPermutation{
		{factors: forward, rules: staticMatrixRuleOrder(count, false), queries: forward, topology: forward},
		{factors: staticMatrixReverse(count), rules: staticMatrixRuleOrder(count, true), queries: staticMatrixEvenOdd(count), topology: staticMatrixRotate(count)},
		{factors: staticMatrixEvenOdd(count), rules: staticMatrixRuleOrder(count, false), queries: staticMatrixRotate(count), topology: staticMatrixReverse(count)},
		{factors: staticMatrixRotate(count), rules: staticMatrixRuleOrder(count, true), queries: staticMatrixReverse(count), topology: staticMatrixEvenOdd(count)},
	}
}

func staticMatrixForward(count int) []int {
	order := make([]int, count)
	for index := range order {
		order[index] = index
	}
	return order
}

func staticMatrixReverse(count int) []int {
	order := staticMatrixForward(count)
	for index := range order {
		order[index] = count - 1 - index
	}
	return order
}

func staticMatrixEvenOdd(count int) []int {
	order := make([]int, 0, count)
	for index := 0; index < count; index += 2 {
		order = append(order, index)
	}
	for index := 1; index < count; index += 2 {
		order = append(order, index)
	}
	return order
}

func staticMatrixRotate(count int) []int {
	order := staticMatrixForward(count)
	shift := count / 2
	for index := range order {
		order[index] = (index + shift) % count
	}
	return order
}

func staticMatrixRuleOrder(count int, reverse bool) []int {
	order := make([]int, 0, count*2)
	for _, factor := range staticMatrixEvenOdd(count) {
		order = append(order, factor)
	}
	for _, factor := range staticMatrixRotate(count) {
		order = append(order, count+factor)
	}
	if reverse {
		for left, right := 0, len(order)-1; left < right; left, right = left+1, right-1 {
			order[left], order[right] = order[right], order[left]
		}
	}
	return order
}

func staticMatrixSemantic(offset uint64) SemanticKey { return testSemanticKey(38_000 + offset) }

func staticMatrixArity(index int) int {
	return [...]int{1, 2, 7}[index%3]
}

func staticMatrixPredecessor(factor, input, count int) int {
	return (factor + input + 1) % count
}

func staticMatrixValue(factor, count int) uint64 {
	value := uint64(1_000 + factor)
	for input := 0; input < staticMatrixArity(factor); input++ {
		value += uint64(staticMatrixPredecessor(factor, input, count) + 1)
	}
	return value
}

func staticMatrixProjection(token *QueryRead[OrderedCells[uint64]]) func(Observation) uint64 {
	return func(observation Observation) uint64 {
		if token == nil {
			return 0
		}
		var value uint64
		rows := 0
		if !ProjectRows(observation, func(row QueryRow) bool {
			cells, ok := QueryValue(row, *token)
			if !ok || cells.Count() != 1 {
				return false
			}
			entry, present, valid := cells.At(0)
			if !valid || !present {
				return false
			}
			value, rows = entry, rows+1
			return true
		}) || rows != 1 {
			return 0
		}
		return value
	}
}

func staticMatrixFrozen(index int) FrozenResult[uint64] {
	return FrozenResult[uint64]{
		Semantic:    staticMatrixSemantic(uint64(3_000 + index)),
		Freeze:      func(value uint64) uint64 { return value },
		Clone:       func(value uint64) uint64 { return value },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
	}
}

func runStaticMatrixFixture(t *testing.T, count int, permutation staticMatrixPermutation) staticMatrixObservation {
	t.Helper()
	fixture := buildStaticMatrixFixture(t, count, permutation)
	state, status := fixture.solver.Solve(context.Background())
	if status != SolveComplete || state == nil {
		t.Fatalf("static matrix Solve = %v", status)
	}
	values := make([]uint64, len(fixture.queries))
	for index := range fixture.queries {
		value, readable := QueryResult(fixture.receipts[index], state)
		if !readable || value != staticMatrixValue(index, count) {
			t.Fatalf("QueryResult[%d] = %d/%t, want %d/true", index, value, readable, staticMatrixValue(index, count))
		}
		values[index] = value
	}
	return staticMatrixObservation{composition: fixture.composition.ID(), topology: fixture.topology, values: values}
}

func buildStaticMatrixFixture(t *testing.T, count int, permutation staticMatrixPermutation) staticMatrixFixture {
	t.Helper()
	if count < 7 || !staticMatrixOrder(permutation.factors, count) || !staticMatrixOrder(permutation.rules, count*2) || !staticMatrixOrder(permutation.queries, count) || !staticMatrixOrder(permutation.topology, count) {
		t.Fatal("invalid static matrix permutation")
	}
	cold := NewComposition()
	factors := make([]*Factor[uint64, uint64], count)
	reads := make([]ReadForm[uint64, OrderedCells[uint64]], count)
	writes := make([]WriteForm[uint64], count)
	carries := make([]CarryForm, count)
	for _, index := range permutation.factors {
		factor, declared := DeclareFactor(cold, coldFactorSpec(staticMatrixSemantic(uint64(100+index))), func(*Factor[uint64, uint64]) bool { return true })
		if !declared || factor == nil {
			t.Fatal("static matrix Factor declaration")
		}
		read, readOK := ExactReadForm(factor)
		write, writeOK := ExactWriteForm(factor)
		carry, carryOK := Carry(factor)
		if !readOK || !writeOK || !carryOK {
			t.Fatal("static matrix Factor forms")
		}
		factors[index], reads[index], writes[index], carries[index] = factor, read, write, carry
	}

	rules := make([]*Rule[uint64, ruleUnit], count*2)
	ruleReads := make([][]Read[OrderedCells[uint64]], count*2)
	ruleWrites := make([]Write[uint64], count*2)
	for _, logical := range permutation.rules {
		factor := logical % count
		if logical < count {
			rule, declared := DeclareRule(cold, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
				Semantic: staticMatrixSemantic(uint64(1_000 + factor)), Output: factors[factor].Output(), Inputs: 0, Admission: testTrustedTheorem[uint64](uint64(40_000 + factor)),
				Transfer: func(access Access[uint64, ruleUnit]) bool {
					return Product(access, func(row Row) bool { return StageValue(access, row, uint64(factor+1)) })
				},
			}, func(rule *Rule[uint64, ruleUnit]) bool {
				var ok bool
				ruleWrites[logical], ok = WriteTo(rule, writes[factor])
				return ok
			})
			if !declared || rule == nil {
				t.Fatal("static matrix ingress Rule")
			}
			rules[logical] = rule
			continue
		}

		arity := staticMatrixArity(factor)
		boundReads := make([]Read[OrderedCells[uint64]], arity)
		rule, declared := DeclareRule(cold, RuleSpec[uint64, ruleUnit]{OperandFamily: unitOperandFamily, OperandContent: ruleUnitContent,
			Semantic: staticMatrixSemantic(uint64(2_000 + factor)), Output: factors[factor].Output(), Inputs: arity, Admission: testTrustedTheorem[uint64](uint64(41_000 + factor)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				return Product(access, func(row Row) bool {
					value := uint64(1_000 + factor)
					for _, read := range boundReads {
						cells, ok := ReadValue(access, row, read)
						if !ok || cells.Count() != 1 {
							return false
						}
						entry, present, valid := cells.At(0)
						if !valid || !present {
							return false
						}
						value += entry
					}
					return StageValue(access, row, value)
				})
			},
		}, func(rule *Rule[uint64, ruleUnit]) bool {
			for input := 0; input < arity; input++ {
				port, portOK := rule.InputAt(input)
				read, readOK := ReadFrom(rule, port, reads[staticMatrixPredecessor(factor, input, count)])
				if !portOK || !readOK {
					return false
				}
				boundReads[input] = read
			}
			carryPort, carryOK := rule.InputAt(0)
			write, writeOK := WriteTo(rule, writes[factor])
			ruleWrites[logical] = write
			return carryOK && CarryFrom(rule, carryPort, carries[factor]) && writeOK
		})
		if !declared || rule == nil {
			t.Fatal("static matrix transform Rule")
		}
		rules[logical], ruleReads[logical] = rule, boundReads
	}

	queries := make([]*Query[uint64], count)
	queryReads := make([]QueryRead[OrderedCells[uint64]], count)
	for _, index := range permutation.queries {
		var token QueryRead[OrderedCells[uint64]]
		query, declared := DeclareQuery(cold, QuerySpec[uint64]{Semantic: staticMatrixSemantic(uint64(3_500 + index)), Project: staticMatrixProjection(&token), Result: staticMatrixFrozen(index)}, func(query *Query[uint64]) bool {
			var ok bool
			token, ok = QueryReadFrom(query, reads[index])
			return ok
		})
		if !declared || query == nil {
			t.Fatal("static matrix Query declaration")
		}
		queries[index], queryReads[index] = query, token
	}
	if !cold.Seal() {
		t.Fatal("static matrix Composition seal")
	}

	instances := make([]*RuleInstance[uint64, ruleUnit], count*2)
	for _, factor := range permutation.topology {
		for role := 0; role < 2; role++ {
			factor, role := factor, role
			logical := factor + role*count
			write, writeOK := factors[factor].Ref(0)
			instance, instanceOK := NewRuleInstance(rules[logical], ruleUnitForSemantic(staticMatrixSemantic(uint64(6_000+factor*2+role))), func(binding *RuleBinding[uint64, ruleUnit]) bool {
				if role == 1 {
					for input := 0; input < staticMatrixArity(factor); input++ {
						read, readOK := factors[staticMatrixPredecessor(factor, input, count)].Ref(0)
						if !readOK || !InstanceRead(binding, ruleReads[logical][input], read) {
							return false
						}
					}
				}
				return writeOK && InstanceWrite(binding, ruleWrites[logical], write)
			})
			if !writeOK || !instanceOK {
				t.Fatalf("static matrix rule instance %d/%d", factor, role)
			}
			instances[logical] = instance
		}
	}

	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	sites := make([][2]equation.Site, count)
	occurrences := make([][2]equation.Occurrence, count)
	operands := make([][2]equation.Operand, count)
	for _, factor := range permutation.topology {
		for role := 0; role < 2; role++ {
			init, disposition := equation.FalseExpr(), equation.InitAbsent
			if role == 0 {
				init, disposition = equation.TrueExpr(), equation.InitPresent
			}
			site, siteOK := batch.AdmitSite(staticMatrixSemantic(uint64(5_000+factor*2+role)).compositionKey(), scope, init, disposition)
			occurrence, occurrenceOK := batch.At(site)
			logical := factor + role*count
			operand, operandOK := admitInstanceOperand(batch, occurrence, instances[logical])
			if !siteOK || !occurrenceOK || !operandOK {
				t.Fatalf("static matrix source row %d/%d", factor, role)
			}
			sites[factor][role], occurrences[factor][role], operands[factor][role] = site, occurrence, operand
		}
	}
	if !scope.Available() || !batch.Seal() {
		t.Fatal("static matrix source batch")
	}
	queryInstances := make([]*QueryInstance[uint64], count)
	solver, compiled := assemble(cold, batch, func(assembly *Assembly) bool {
		points := make([][2]*assemblyPoint, count)
		groups := make([][2]*assemblyGroup, count)
		for _, factor := range permutation.topology {
			for role := 0; role < 2; role++ {
				logical := factor + role*count
				point := admitPoint(assembly, sites[factor][role])
				instance := instances[logical]
				member := admitInstance(assembly, point, occurrences[factor][role], operands[factor][role], instance)
				if point == nil || instance == nil || member == nil {
					t.Fatalf("static matrix rule assembly %d/%d", factor, role)
				}
				group := admitGroup(assembly, point, member)
				if group == nil {
					t.Fatalf("static matrix write assembly %d/%d", factor, role)
				}
				points[factor][role] = point
				groups[factor][role] = group
			}
		}
		for _, factor := range permutation.topology {
			for input := 0; input < staticMatrixArity(factor); input++ {
				predecessor := staticMatrixPredecessor(factor, input, count)
				boundary := equation.BoundaryInput(sites[predecessor][0], sites[factor][1], staticMatrixSemantic(uint64(7_000+factor*10+input)).compositionKey(), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
				if !boundary.Available() || !admitBoundary(assembly, groups[factor][1], boundary) {
					t.Fatalf("static matrix boundary assembly %d/%d", factor, input)
				}
			}
		}
		for _, factor := range permutation.queries {
			read, readOK := factors[factor].Ref(0)
			queryInstance, queryInstanceOK := NewQueryInstance(queries[factor], func(binding *QueryBinding[uint64]) bool {
				return readOK && InstanceQueryRead(binding, queryReads[factor], read)
			})
			queryInstances[factor] = queryInstance
			observation := admitQueryAt(assembly, points[factor][1], queryInstance)
			if observation == nil || !queryInstanceOK || !readOK {
				t.Fatalf("static matrix query assembly %d", factor)
			}
		}
		return true
	})
	if !compiled || solver == nil {
		t.Fatal("static matrix Solver compilation")
	}
	receipts := make([]QueryReceipt[uint64], count)
	for index, instance := range queryInstances {
		var receiptOK bool
		receipts[index], receiptOK = instance.Receipt()
		if !receiptOK {
			t.Fatalf("static matrix query receipt %d", index)
		}
	}
	return staticMatrixFixture{composition: cold, topology: batch, solver: solver, queries: queries, receipts: receipts}
}

func staticMatrixOrder(order []int, size int) bool {
	if len(order) != size {
		return false
	}
	seen := make([]bool, size)
	for _, value := range order {
		if value < 0 || value >= size || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
