package engine

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

const entryValue = "front/closed-entry/v1"

const branchPredicatePrefix = "front/branch-predicate/v1/"

// branchPredicateWire mirrors the front's closed predicate wire vocabulary.
// It intentionally contains only resolved WIR data, never an AST expression or
// an evaluator callback.
type branchPredicateWire struct {
	Kind           string `json:"kind"`
	Path           string `json:"path,omitempty"`
	OtherPath      string `json:"other_path,omitempty"`
	TypeName       string `json:"type_name,omitempty"`
	Literal        string `json:"literal,omitempty"`
	LenFloor       int64  `json:"len_floor,omitempty"`
	NumFloor       int64  `json:"num_floor,omitempty"`
	NumCeil        int64  `json:"num_ceil,omitempty"`
	HasNumCeil     bool   `json:"has_num_ceil,omitempty"`
	NumCeilNegated bool   `json:"num_ceil_negated,omitempty"`
	Negated        bool   `json:"negated,omitempty"`
	ProducerPoint  uint32 `json:"producer_point,omitempty"`
	HasProducer    bool   `json:"has_producer,omitempty"`
}

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
		Outcomes: publishedOutcomes(evaluation.Closure.Outcomes), Diagnostics: evaluation.Closure.Diagnostics,
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
	allocationTemplate, err := binding("allocation-template", equation.KernelFunc(allocationTemplateKernel))
	if err != nil {
		return nil, err
	}
	objectMaterialization, err := binding("object-materialization", equation.KernelFunc(objectMaterializationKernel))
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
	apply, err := binding("apply", equation.KernelFunc(applyKernel))
	if err != nil {
		return nil, err
	}
	results, err := binding("call-results", equation.KernelFunc(callResultsKernel))
	if err != nil {
		return nil, err
	}
	external, err := binding("external-call", equation.KernelFunc(externalCallKernel))
	if err != nil {
		return nil, err
	}
	publication, err := binding("publication", equation.KernelFunc(publicationKernel))
	if err != nil {
		return nil, err
	}
	registry, err := equation.NewKernelRegistry([]equation.KernelBinding{entry, allocationTemplate, objectMaterialization, write, branch, apply, external, results, publication})
	if err != nil {
		return nil, fmt.Errorf("engine: build kernel registry: %w", err)
	}
	return registry, nil
}

func entryKernel(equation.BoundEquation, equation.Partition) (equation.TransactionResult, error) {
	return equation.TransactionResult{Complete: true}, nil
}

// allocationTemplateKernel admits only a sealed, complete allocation graph.
// Its identity transport is structural at this walking stage; it deliberately
// does not turn an absent/nil member into a value fact.
func allocationTemplateKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if _, err := requiredOperandsByRole(operation.Operands, "site", "result", "kind"); err != nil {
		return equation.TransactionResult{}, err
	}
	return equation.TransactionResult{Complete: true}, nil
}

// objectMaterializationKernel runs only after its template dependency.  The
// current engine retains no heap state, but validates the sealed identity and
// object-kind surface so unsupported materialization cannot pass as a hidden
// fallback transaction.
func objectMaterializationKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	if _, err := requiredOperandsByRole(operation.Operands, "site", "result", "kind"); err != nil {
		return equation.TransactionResult{}, err
	}
	return equation.TransactionResult{Complete: true}, nil
}

func writeKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := operandsByRole(operation.Operands, "target", "display", "value", "read-before", "absence")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	target := string(operands["target"])
	if !strings.HasPrefix(target, "path/") {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed assignment target")
	}
	value, err := resolveValue(operands["value"], operands["read-before"], operands["absence"], partition)
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
	for _, operand := range operation.Operands {
		if operand.Role != "condition" {
			continue
		}
		value, err := resolveCurrentValue(operand.Value, partition)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		if string(value) == "scalar/top" {
			return equation.TransactionResult{Complete: true}, nil
		}
	}
	truth, err := branchTruth(operation.Operands, partition)
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

// applyKernel validates the sealed direct or method-call shape without
// inventing a callee outcome.
func applyKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	hasCallee, hasReceiver, hasMethod := false, false, false
	for _, operand := range operation.Operands {
		switch operand.Role {
		case "callee":
			hasCallee = true
		case "receiver":
			hasReceiver = true
		case "method":
			hasMethod = true
		}
	}
	if hasCallee == (hasReceiver && hasMethod) || hasReceiver != hasMethod {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed call application shape")
	}
	return equation.TransactionResult{Complete: true}, nil
}

// externalCallKernel is a sealed provider-boundary factor.  It intentionally
// owns no result term: call-results remains the sole result-slot owner for
// every call, whether the callee is local or external.
func externalCallKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	operands, err := requiredOperandsByRole(operation.Operands, "application", "provider", "argument-spread", "result-arity", "result-spread", "context")
	if err != nil {
		return equation.TransactionResult{}, err
	}
	if !strings.HasPrefix(string(operands["application"]), "call/") ||
		(!strings.HasPrefix(string(operands["provider"]), "provider/global/") && !strings.HasPrefix(string(operands["provider"]), "provider/module/")) ||
		(string(operands["argument-spread"]) != "scalar/bool/true" && string(operands["argument-spread"]) != "scalar/bool/false") ||
		(string(operands["result-spread"]) != "scalar/bool/true" && string(operands["result-spread"]) != "scalar/bool/false") ||
		!strings.HasPrefix(string(operands["context"]), "call-context/") {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed external call boundary")
	}
	if _, err := strconv.Atoi(string(operands["result-arity"])); err != nil {
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed external result arity")
	}
	for _, operand := range operation.Operands {
		if strings.HasPrefix(operand.Role, "argument-") || operand.Role == "receiver" || operand.Role == "method" || operand.Role == "application" || operand.Role == "provider" || operand.Role == "argument-spread" || operand.Role == "result-arity" || operand.Role == "result-spread" || operand.Role == "context" {
			continue
		}
		return equation.TransactionResult{}, fmt.Errorf("engine: malformed external call role %q", operand.Role)
	}
	return equation.TransactionResult{Complete: true}, nil
}

// callResultsKernel publishes explicit Top facts for unresolved owned result
// slots, never a missing slot or an invented concrete value.
func callResultsKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	resultTerms := map[string][]byte{}
	targetTerms := map[string][]byte{}
	hasApplication := false
	for _, operand := range operation.Operands {
		switch {
		case operand.Role == "application":
			if hasApplication || !strings.HasPrefix(string(operand.Value), "call/") {
				return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result application")
			}
			hasApplication = true
		case strings.HasPrefix(operand.Role, "result-"):
			resultTerms[strings.TrimPrefix(operand.Role, "result-")] = operand.Value
		case strings.HasPrefix(operand.Role, "target-"):
			targetTerms[strings.TrimPrefix(operand.Role, "target-")] = operand.Value
		default:
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result role %q", operand.Role)
		}
	}
	if !hasApplication || len(resultTerms) != len(targetTerms) {
		return equation.TransactionResult{}, fmt.Errorf("engine: incomplete call result transaction")
	}
	values := make([]equation.Fact, 0, len(resultTerms))
	for key, result := range resultTerms {
		if len(result) == 0 || len(targetTerms[key]) == 0 || !strings.HasPrefix(string(result), "temp/") {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed call result %q", key)
		}
		values = append(values, equation.Fact{Key: "value/" + string(result) + "/" + operation.Target.Name, Value: []byte("scalar/top")})
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Values: values}}, nil
}

// publicationKernel resolves every selected return slot before publishing any
// output.  A false or unknown guard contributes no tuple; a selected guard
// contributes the complete indexed tuple, including nil-valued slots.
func publicationKernel(operation equation.BoundEquation, partition equation.Partition) (equation.TransactionResult, error) {
	if !guardsHold(operation.Guards, partition) {
		return equation.TransactionResult{Complete: true}, nil
	}
	values := make([][]byte, len(operation.Operands))
	for _, operand := range operation.Operands {
		const prefix = "return-value-"
		if !strings.HasPrefix(operand.Role, prefix) {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		indexText := strings.TrimPrefix(operand.Role, prefix)
		if len(indexText) != 8 {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		index, err := strconv.Atoi(indexText)
		if err != nil || index < 0 || index >= len(values) || values[index] != nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: malformed return operand role %q", operand.Role)
		}
		value, err := resolveCurrentValue(operand.Value, partition)
		if err != nil {
			return equation.TransactionResult{}, err
		}
		values[index] = value
	}
	for index, value := range values {
		if value == nil {
			return equation.TransactionResult{}, fmt.Errorf("engine: missing return value %d", index)
		}
	}
	outcomes := make([]equation.Fact, 0, len(values)+1)
	outcomes = append(outcomes, equation.Fact{Key: "return/arity", Value: []byte(strconv.Itoa(len(values)))})
	for index, value := range values {
		outcomes = append(outcomes, equation.Fact{Key: "return/" + strconv.Itoa(index), Value: value})
	}
	return equation.TransactionResult{Complete: true, Closure: equation.OutputClosure{Outcomes: outcomes}}, nil
}

// branchTruth evaluates exactly one selector.  An unavailable selector is an
// error, not a false edge: absence, bottom, and a complete falsy value stay
// distinct throughout a branch transaction.
func branchTruth(operands []equation.BoundOperand, partition equation.Partition) (bool, error) {
	var condition, predicate []byte
	for _, operand := range operands {
		switch operand.Role {
		case "condition":
			if condition != nil {
				return false, fmt.Errorf("engine: duplicate branch condition")
			}
			condition = operand.Value
		case "predicate":
			if predicate != nil {
				return false, fmt.Errorf("engine: duplicate branch predicate")
			}
			predicate = operand.Value
		default:
			// Evidence, arm boundaries, and difference constraints are closed
			// branch metadata. They are intentionally not alternate selectors.
			if !strings.HasPrefix(operand.Role, "implied-") && !strings.HasPrefix(operand.Role, "sufficient-") && !strings.HasPrefix(operand.Role, "difference-") {
				return false, fmt.Errorf("engine: malformed branch operand role %q", operand.Role)
			}
		}
	}
	if condition != nil {
		value, err := resolveCurrentValue(condition, partition)
		if err != nil {
			return false, err
		}
		return luaTruthy(value)
	}
	if predicate == nil {
		return false, fmt.Errorf("engine: branch has no selector")
	}
	return evaluateBranchPredicate(predicate, partition)
}

func evaluateBranchPredicate(encoded []byte, partition equation.Partition) (bool, error) {
	if !strings.HasPrefix(string(encoded), branchPredicatePrefix) {
		return false, fmt.Errorf("engine: malformed branch predicate")
	}
	var predicate branchPredicateWire
	if err := json.Unmarshal(encoded[len(branchPredicatePrefix):], &predicate); err != nil || predicate.Kind == "" {
		if err != nil {
			return false, fmt.Errorf("engine: decode branch predicate: %w", err)
		}
		return false, fmt.Errorf("engine: branch predicate has no kind")
	}
	value, err := branchPathValue(predicate.Path, partition)
	if err != nil {
		return false, err
	}
	var result bool
	switch predicate.Kind {
	case "truthy":
		result, err = luaTruthy(value)
	case "falsy":
		result, err = luaTruthy(value)
		result = !result
	case "nil":
		result = string(value) == "scalar/nil"
	case "not-nil":
		result = string(value) != "scalar/nil"
	case "literal-equal", "literal-not":
		if predicate.Literal == "" {
			return false, fmt.Errorf("engine: literal predicate has no literal")
		}
		result = string(value) == predicate.Literal
		if predicate.Kind == "literal-not" {
			result = !result
		}
	case "path-equal", "path-not":
		other, valueErr := branchPathValue(predicate.OtherPath, partition)
		if valueErr != nil {
			return false, valueErr
		}
		result = string(value) == string(other)
		if predicate.Kind == "path-not" {
			result = !result
		}
	case "type-equal", "type-not":
		typeName := predicate.TypeName
		if typeName == "" {
			other, valueErr := branchPathValue(predicate.OtherPath, partition)
			if valueErr != nil {
				return false, valueErr
			}
			typeName, valueErr = scalarString(other)
			if valueErr != nil {
				return false, valueErr
			}
		}
		actual, valueErr := scalarType(value)
		if valueErr != nil {
			return false, valueErr
		}
		result = actual == typeName
		if predicate.Kind == "type-not" {
			result = !result
		}
	case "len-ge":
		length, valueErr := scalarLength(value)
		if valueErr != nil {
			return false, valueErr
		}
		result = length >= predicate.LenFloor
	case "num-ge":
		number, valueErr := scalarNumber(value)
		if valueErr != nil {
			return false, valueErr
		}
		result = number >= float64(predicate.NumFloor)
	case "num-le":
		number, valueErr := scalarNumber(value)
		if valueErr != nil {
			return false, valueErr
		}
		if !predicate.HasNumCeil {
			return false, fmt.Errorf("engine: numeric ceiling predicate has no ceiling")
		}
		result = number <= float64(predicate.NumCeil)
	case "index-in-range", "frozen-table":
		return false, fmt.Errorf("engine: branch predicate %q has no scalar evaluator", predicate.Kind)
	default:
		return false, fmt.Errorf("engine: unknown branch predicate %q", predicate.Kind)
	}
	if err != nil {
		return false, err
	}
	if predicate.Negated {
		result = !result
	}
	return result, nil
}

func branchPathValue(key string, partition equation.Partition) ([]byte, error) {
	if key == "" {
		return nil, fmt.Errorf("engine: branch predicate has an absent path")
	}
	return resolveCurrentValue([]byte("path/"+key), partition)
}

func scalarType(value []byte) (string, error) {
	switch {
	case string(value) == "scalar/nil":
		return "nil", nil
	case strings.HasPrefix(string(value), "scalar/bool/"):
		return "boolean", nil
	case strings.HasPrefix(string(value), "scalar/number/"):
		return "number", nil
	case strings.HasPrefix(string(value), "scalar/string/"):
		return "string", nil
	default:
		return "", fmt.Errorf("engine: malformed scalar value %q", value)
	}
}

func scalarString(value []byte) (string, error) {
	if !strings.HasPrefix(string(value), "scalar/string/") {
		return "", fmt.Errorf("engine: type predicate path is not a string")
	}
	decoded, err := strconv.Unquote(strings.TrimPrefix(string(value), "scalar/string/"))
	if err != nil {
		return "", err
	}
	return decoded, nil
}

func scalarLength(value []byte) (int64, error) {
	decoded, err := scalarString(value)
	if err != nil {
		return 0, err
	}
	return int64(len(decoded)), nil
}

func scalarNumber(value []byte) (float64, error) {
	if !strings.HasPrefix(string(value), "scalar/number/") {
		return 0, fmt.Errorf("engine: numeric predicate path is not a number")
	}
	parsed, err := strconv.ParseFloat(strings.TrimPrefix(string(value), "scalar/number/"), 64)
	if err != nil {
		return 0, fmt.Errorf("engine: decode numeric predicate: %w", err)
	}
	return parsed, nil
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

// requiredOperandsByRole is the structural counterpart to operandsByRole: it
// checks required roles exactly once while allowing the allocation family to
// carry its sealed member/capture inventory as additional closed operands.
func requiredOperandsByRole(operands []equation.BoundOperand, roles ...string) (map[string][]byte, error) {
	wanted := make(map[string]bool, len(roles))
	for _, role := range roles {
		wanted[role] = true
	}
	result := make(map[string][]byte, len(roles))
	for _, operand := range operands {
		if !wanted[operand.Role] {
			continue
		}
		if result[operand.Role] != nil {
			return nil, fmt.Errorf("engine: duplicate operand role %q", operand.Role)
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

func resolveValue(term, readBefore, absence []byte, partition equation.Partition) ([]byte, error) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), nil
	}
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
		return nil, fmt.Errorf("engine: unsupported scalar term %q", term)
	}
	const readBeforePrefix = "front/read-before/"
	if !strings.HasPrefix(string(readBefore), readBeforePrefix) {
		return nil, fmt.Errorf("engine: malformed assignment read boundary %q", readBefore)
	}
	cutoff := strings.TrimPrefix(string(readBefore), readBeforePrefix)
	if cutoff == "" {
		return nil, fmt.Errorf("engine: empty assignment read boundary")
	}
	prefix := "value/" + string(term) + "/"
	var value []byte
	latestKey := ""
	for _, fact := range partition.Values() {
		name := strings.TrimPrefix(fact.Key, prefix)
		if strings.HasPrefix(fact.Key, prefix) && name <= cutoff && (value == nil || fact.Key > latestKey) {
			value = fact.Value
			latestKey = fact.Key
		}
	}
	if value == nil {
		switch string(absence) {
		case "front/absence/nil":
			return []byte("scalar/nil"), nil
		case "front/absence/error":
			return nil, fmt.Errorf("engine: value path %q has no completed write before %q", term, cutoff)
		default:
			return nil, fmt.Errorf("engine: malformed assignment absence policy %q", absence)
		}
	}
	return append([]byte(nil), value...), nil
}

// resolveCurrentValue is for non-assignment families whose own contract owns
// their read timing. Environment-write deliberately does not use it: every
// assignment read carries an explicit pre-write boundary instead.
func resolveCurrentValue(term []byte, partition equation.Partition) ([]byte, error) {
	if strings.HasPrefix(string(term), "scalar/") {
		return append([]byte(nil), term...), nil
	}
	if !strings.HasPrefix(string(term), "path/") && !strings.HasPrefix(string(term), "temp/") {
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

func publishedOutcomes(stored []equation.Fact) []equation.Fact {
	outcomes := make([]equation.Fact, 0, len(stored))
	for _, storedOutcome := range stored {
		outcome := equation.Fact{Key: storedOutcome.Key, Value: append([]byte(nil), storedOutcome.Value...)}
		if strings.HasPrefix(outcome.Key, "return/") && outcome.Key != "return/arity" {
			decoded, err := displayValue(outcome.Value)
			if err == nil {
				outcome.Value = decoded
			}
		}
		outcomes = append(outcomes, outcome)
	}
	sort.Slice(outcomes, func(i, j int) bool { return outcomes[i].Key < outcomes[j].Key })
	return outcomes
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
		return []byte(strconv.Quote(stringValue)), nil
	case encoded == "scalar/top":
		return []byte("unknown"), nil
	default:
		return nil, fmt.Errorf("engine: invalid stored scalar")
	}
}
