package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

const entryValue = "front/closed-entry/v1"

// Result is the complete result published by Check. Values use source display
// names for this small entrypoint; outcomes and diagnostics retain their
// equation-kernel candidate keys.
type Result struct {
	Artifact     equation.Artifact
	Values       []equation.Fact
	Outcomes     []equation.Fact
	Diagnostics  []equation.Fact
	Transactions int
}

// Check compiles source through the new front, binds its sole formal entry,
// evaluates it with equation.AcyclicVM, and returns every output channel.
func Check(source string) (Result, error) {
	artifact, err := front.CompileBody(source)
	if err != nil {
		return Result{}, err
	}
	if len(artifact.Equations) == 0 {
		return Result{}, fmt.Errorf("engine: front returned an empty artifact")
	}
	entry := artifact.Equations[0].Entry
	bound, err := equation.BindEntry(artifact, equation.EntryBinding{Parameter: entry, Value: []byte(entryValue)})
	if err != nil {
		return Result{}, fmt.Errorf("engine: bind entry: %w", err)
	}
	registry, err := registry()
	if err != nil {
		return Result{}, err
	}
	vm, err := equation.NewAcyclicVM(registry)
	if err != nil {
		return Result{}, err
	}
	evaluation, err := vm.Evaluate(bound)
	if err != nil {
		return Result{}, fmt.Errorf("engine: evaluate: %w", err)
	}
	return Result{
		Artifact: artifact, Values: publishedValues(artifact, evaluation.Closure.Values),
		Outcomes: evaluation.Closure.Outcomes, Diagnostics: evaluation.Closure.Diagnostics,
		Transactions: evaluation.Transactions,
	}, nil
}

func registry() (*equation.KernelRegistry, error) {
	binding := func(kind string, kernel equation.Kernel) (equation.KernelBinding, error) {
		kernelID, known := front.KernelID(kind)
		contract, contracted := front.ContractID(kind)
		if !known || !contracted {
			return equation.KernelBinding{}, fmt.Errorf("engine: missing front kernel contract for %q", kind)
		}
		return equation.KernelBinding{KernelID: kernelID, ContractID: contract, Kernel: kernel}, nil
	}
	entry, err := binding("entry", equation.KernelFunc(entryKernel))
	if err != nil {
		return nil, err
	}
	write, err := binding("environment-write", equation.KernelFunc(writeKernel))
	if err != nil {
		return nil, err
	}
	branch, err := binding("branch-relations", equation.KernelFunc(branchKernel))
	if err != nil {
		return nil, err
	}
	registry, err := equation.NewKernelRegistry([]equation.KernelBinding{entry, write, branch})
	if err != nil {
		return nil, fmt.Errorf("engine: build kernel registry: %w", err)
	}
	return registry, nil
}

func entryKernel(equation.BoundEquation, equation.Partition) (equation.TransactionResult, error) {
	return equation.TransactionResult{Complete: true}, nil
}

func writeKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := operandsByRole(operation.Operands, "target", "display", "value")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	target := string(operands["target"])
	if !strings.HasPrefix(target, "path/") {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed assignment target")
	}
	value, err := resolveValue(operands["value"], partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: []equation.Fact{{
		Key: "value/" + target + "/" + operation.Target.Name, Value: value,
	}}}}, nil
}

func branchKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := operandsByRole(operation.Operands, "condition")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	value, err := resolveValue(operands["condition"], partition)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	truth, err := luaTruthy(value)
	if err != nil {
		return equation.TransactionResult{}, err
	}
	edge := strconv.FormatBool(truth)
	narrowing := "falsy"
	if truth {
		narrowing = "truthy"
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Outcomes: []equation.Fact{
		{Key: "branch/" + operation.Target.Name, Value: []byte("scalar/bool/" + edge)},
		{Key: "narrowing/" + operation.Target.Name, Value: []byte(narrowing)},
	}}}, nil
}

func operandsByRole(operands []equation.BoundOperand, roles ...string) (map[string][]byte, error) {
	wanted := make(map[string]bool, len(roles))
	for _, role := range roles {
		wanted[role] = true
	}
	result := make(map[string][]byte, len(roles))
	for _, operand := range operands {
		if !wanted[operand.Role] || result[operand.Role] != nil {
			return nil, fmt.Errorf("engine: malformed operand role %q", operand.Role)
		}
		result[operand.Role] = operand.Value
	}
	for _, role := range roles {
		if len(result[role]) == 0 {
			return nil, fmt.Errorf("engine: missing operand %q", role)
		}
	}
	return result, nil
}

func guardsHold(guards []equation.Guard, partition equation.Partition) bool {
	outcomes := make(map[string][]byte, len(partition.Outcomes()))
	for _, outcome := range partition.Outcomes() {
		outcomes[outcome.Key] = outcome.Value
	}
	for _, guard := range guards {
		parts := strings.Split(string(guard.Encoding), "/")
		if len(parts) != 4 || parts[0] != "front" || parts[1] != "branch" || (parts[3] != "true" && parts[3] != "false") {
			return false
		}
		if string(outcomes["branch/"+parts[2]]) != "scalar/bool/"+parts[3] {
			return false
		}
	}
	return true
}

func resolveValue(term []byte, partition equation.Partition) ([]byte, error) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), nil
	}
	if !strings.HasPrefix(string(term), "path/") {
		return nil, fmt.Errorf("engine: unsupported scalar term %q", term)
	}
	prefix := "value/" + string(term) + "/"
	var value []byte
	latestKey := ""
	for _, fact := range partition.Values() {
		if strings.HasPrefix(fact.Key, prefix) && (value == nil || fact.Key > latestKey) {
			value = fact.Value
			latestKey = fact.Key
		}
	}
	if value == nil {
		return nil, fmt.Errorf("engine: value path %q has no completed write", term)
	}
	return append([]byte(nil), value...), nil
}

func luaTruthy(value []byte) (bool, error) {
	switch string(value) {
	case "scalar/nil", "scalar/bool/false":
		return false, nil
	case "scalar/bool/true":
		return true, nil
	default:
		if strings.HasPrefix(string(value), "scalar/number/") || strings.HasPrefix(string(value), "scalar/string/") {
			return true, nil
		}
		return false, fmt.Errorf("engine: malformed scalar value %q", value)
	}
}

func publishedValues(artifact equation.Artifact, stored []equation.Fact) []equation.Fact {
	storedByKey := make(map[string][]byte, len(stored))
	for _, fact := range stored {
		storedByKey[fact.Key] = fact.Value
	}
	byDisplay := make(map[string]equation.Fact)
	for _, operation := range artifact.Equations {
		if operation.Occurrence.Kind != "environment-write" {
			continue
		}
		operands, err := artifactOperandsByRole(operation.Operands, "target", "display")
		if err != nil {
			continue
		}
		prefix := "value/" + string(operands["target"]) + "/"
		latestKey := ""
		for key, value := range storedByKey {
			if strings.HasPrefix(key, prefix) && key > latestKey {
				latestKey = key
				decoded, err := displayValue(value)
				if err == nil {
					byDisplay[string(operands["display"])] = equation.Fact{Key: string(operands["display"]), Value: decoded}
				}
			}
		}
	}
	values := make([]equation.Fact, 0, len(byDisplay))
	for _, value := range byDisplay {
		values = append(values, value)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Key < values[j].Key })
	return values
}

func artifactOperandsByRole(operands []equation.Operand, roles ...string) (map[string][]byte, error) {
	wanted := make(map[string]bool, len(roles))
	for _, role := range roles {
		wanted[role] = true
	}
	result := make(map[string][]byte, len(roles))
	for _, operand := range operands {
		if operand.Term.Entry {
			return nil, fmt.Errorf("engine: unexpected entry operand")
		}
		if wanted[operand.Role] {
			if result[operand.Role] != nil {
				return nil, fmt.Errorf("engine: duplicate artifact operand %q", operand.Role)
			}
			result[operand.Role] = operand.Term.Encoding
		}
	}
	for _, role := range roles {
		if len(result[role]) == 0 {
			return nil, fmt.Errorf("engine: missing artifact operand %q", role)
		}
	}
	return result, nil
}

func displayValue(value []byte) ([]byte, error) {
	encoded := string(value)
	switch {
	case encoded == "scalar/nil":
		return []byte("nil"), nil
	case strings.HasPrefix(encoded, "scalar/bool/"):
		return []byte(strings.TrimPrefix(encoded, "scalar/bool/")), nil
	case strings.HasPrefix(encoded, "scalar/number/"):
		return []byte(strings.TrimPrefix(encoded, "scalar/number/")), nil
	case strings.HasPrefix(encoded, "scalar/string/"):
		stringValue, err := strconv.Unquote(strings.TrimPrefix(encoded, "scalar/string/"))
		if err != nil {
			return nil, err
		}
		return []byte(stringValue), nil
	default:
		return nil, fmt.Errorf("engine: invalid stored scalar")
	}
}
