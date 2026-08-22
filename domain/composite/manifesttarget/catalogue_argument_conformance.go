package manifesttarget

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/domain/type/subst"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typecall"
	domaincontract "github.com/wippyai/go-lua/domain/type/typecontract"
	"github.com/wippyai/go-lua/manifest"
	moduleio "github.com/wippyai/go-lua/manifest/wire"
)

// callbackConformance is the seal-time consumer of every declared callback
// argument vector.
//
// A callback declaration states two independent facts about one coordinate:
// the input formal that carries the callable, and the Values relation the
// operation applies to it. Nothing downstream joins them - class registration
// walks the vector for type identity and the content-ID encoders hash it - so
// without this gate a manifest may declare a formal typed () -> any and apply
// three operands to it, and the contradiction seals clean.
//
// The gate resolves the declared type of the named formal and, when that type
// carries a callable arm with a parameter list, requires the applied vector to
// conform to it: the operand count the operation may apply must lie inside the
// arm's declared arity, and every segment's element type must be assignable to
// each parameter that segment can land on. A formal whose declared type states
// no parameter list - gradual any, or a callable with no function witness -
// states no arity and carries no obligation; a formal whose declared type does
// not admit the callback's own declared admission is refused, because no call
// site can satisfy that pair.
func callbackConformance(functions []manifest.Function) error {
	var refusals []error
	for _, declaration := range functions {
		law, ok := declaration.Operation()
		if !ok || len(law.Callbacks) == 0 {
			continue
		}
		if !law.Replace {
			refusals = append(refusals, fmt.Errorf(
				"target catalogue: %s declares %d callback(s) on an amending operation law; the callback relation reaches the sealed operation through a replacing law only, so the declaration is refused rather than dropped",
				declaration.CanonicalPath(), len(law.Callbacks),
			))
			continue
		}
		formals := inputFormals(declaration, law)
		for index, callback := range law.Callbacks {
			if err := conformCallback(formals, index, callback); err != nil {
				refusals = append(refusals, fmt.Errorf("target catalogue: %s: %w", declaration.CanonicalPath(), err))
			}
		}
	}
	return errors.Join(refusals...)
}

// inputFormals collects every declared type stated for the operation's input
// coordinates. A callable's public type contract and its operational envelope
// address one coordinate by the same ordinal - the relation this package
// already uses to resolve a signature ParamRef against operation input
// geometry - so both declarations bind the same formal and both are checked.
type inputFormalTypes struct {
	signature []typ.Type
	variadic  typ.Type
	law       []typ.Type
	lawTail   typ.Type
}

func inputFormals(declaration manifest.Function, law moduleio.Operation) inputFormalTypes {
	out := inputFormalTypes{law: law.Input.Fixed, lawTail: law.Input.TailType}
	function := declaration.Signature().Type
	if function == nil {
		return out
	}
	arguments := make([]typ.Type, len(function.TypeParams))
	for index, parameter := range function.TypeParams {
		arguments[index] = typ.Any
		if parameter != nil && parameter.Constraint != nil {
			arguments[index] = parameter.Constraint
		}
	}
	for _, parameter := range function.Params {
		out.signature = append(out.signature, subst.Params(parameter.Type, function.TypeParams, arguments))
	}
	if function.Variadic != nil {
		out.variadic = subst.Params(function.Variadic, function.TypeParams, arguments)
	}
	return out
}

// at returns every declared type stated for one fixed input ordinal.
func (formals inputFormalTypes) at(ordinal uint32) []typ.Type {
	var out []typ.Type
	if int(ordinal) < len(formals.signature) {
		out = append(out, formals.signature[ordinal])
	}
	if int(ordinal) < len(formals.law) {
		out = append(out, formals.law[ordinal])
	}
	return out
}

// tail returns every declared element type stated for the input tail.
func (formals inputFormalTypes) tail() []typ.Type {
	var out []typ.Type
	if formals.variadic != nil {
		out = append(out, formals.variadic)
	}
	if formals.lawTail != nil {
		out = append(out, formals.lawTail)
	}
	return out
}

func conformCallback(formals inputFormalTypes, index int, callback moduleio.Callback) error {
	declared, coordinate, err := callbackFormalTypes(formals, callback.Function)
	if err != nil {
		return fmt.Errorf("callback %d: %w", index, err)
	}
	admission, err := callbackAdmission(callback.Admission)
	if err != nil {
		return fmt.Errorf("callback %d: %w", index, err)
	}
	for _, formal := range declared {
		if !domaincontract.Admits(formal, admission) {
			return fmt.Errorf(
				"callback %d names %s, declared %s, which does not admit the callback's declared callable admission",
				index, coordinate, typeText(formal),
			)
		}
		function, ok := typecall.ContextualCallable(formal)
		if !ok {
			continue
		}
		if err := conformArguments(materializeFunction(function), callback.Arguments); err != nil {
			return fmt.Errorf(
				"callback %d argument vector does not conform to the declared function type of %s (%s): %w",
				index, coordinate, typeText(formal), err,
			)
		}
	}
	return nil
}

// callbackFormalTypes resolves the input coordinate a callback names into the
// declared types stated for it. A coordinate the declaration never declares is
// refused: the callback would then apply its vector to a formal no declaration
// describes.
func callbackFormalTypes(formals inputFormalTypes, source moduleio.InputSource) ([]typ.Type, string, error) {
	switch source.Kind {
	case moduleio.InputSourceValue:
		coordinate := fmt.Sprintf("input formal %d", source.Ordinal)
		declared := formals.at(source.Ordinal)
		if len(declared) == 0 {
			return nil, coordinate, fmt.Errorf("names %s, which the declaration does not declare", coordinate)
		}
		return declared, coordinate, nil
	case moduleio.InputSourceValues:
		coordinate := fmt.Sprintf("input tail %d", source.Ordinal)
		declared := formals.tail()
		if len(declared) == 0 {
			return nil, coordinate, fmt.Errorf("names %s, which the declaration gives no element type", coordinate)
		}
		return declared, coordinate, nil
	default:
		return nil, "", fmt.Errorf("names no input coordinate")
	}
}

func callbackAdmission(admission moduleio.CallableAdmission) (domaincontract.Admission, error) {
	switch admission {
	case moduleio.CallableAdmissionDirectFunction:
		return domaincontract.DirectFunction, nil
	case moduleio.CallableAdmissionOrdinary:
		return domaincontract.OrdinaryCallable, nil
	default:
		return domaincontract.AdmissionInvalid, fmt.Errorf("declares no callable admission")
	}
}

// materializeFunction replaces a declared callable arm's own type parameters
// with their constraints, so a generic arm is checked against the widest shape
// it admits instead of being skipped for carrying a formal.
func materializeFunction(function *typ.Function) *typ.Function {
	if function == nil || len(function.TypeParams) == 0 {
		return function
	}
	arguments := make([]typ.Type, len(function.TypeParams))
	for index, parameter := range function.TypeParams {
		arguments[index] = typ.Any
		if parameter != nil && parameter.Constraint != nil {
			arguments[index] = parameter.Constraint
		}
	}
	out := &typ.Function{Params: make([]typ.Param, len(function.Params)), Returns: function.Returns}
	for index, parameter := range function.Params {
		out.Params[index] = typ.Param{
			Name: parameter.Name, Optional: parameter.Optional, Receiver: parameter.Receiver,
			Type: subst.Params(parameter.Type, function.TypeParams, arguments),
		}
	}
	if function.Variadic != nil {
		out.Variadic = subst.Params(function.Variadic, function.TypeParams, arguments)
	}
	return out
}

// conformArguments is the argument-vector conformance law. A vector is a fixed
// prefix, an optional open tail of one element type, and an end-anchored
// suffix. The declared arm is a fixed parameter list whose trailing entries may
// be optional, plus an optional variadic element type.
func conformArguments(function *typ.Function, vector moduleio.Values) error {
	if function == nil {
		return nil
	}
	required := requiredParameters(function)
	total := len(function.Params)
	variadic := function.Variadic != nil
	fixed, suffix := len(vector.Fixed), len(vector.Suffix)
	open := vector.Tail == moduleio.ValuesVariable || vector.Tail == moduleio.ValuesUnknown
	minimum := fixed + suffix

	if minimum < required {
		return fmt.Errorf("applies at least %d operand(s) to a parameter list requiring %d", minimum, required)
	}
	if !variadic && open {
		return fmt.Errorf("applies an open argument tail to a parameter list of at most %d fixed parameter(s)", total)
	}
	if !variadic && minimum > total {
		return fmt.Errorf("applies %d operand(s) to a parameter list of at most %d", minimum, total)
	}

	parameterAt := func(index int) typ.Type {
		if index < total {
			return function.Params[index].Type
		}
		return function.Variadic
	}
	for index, operand := range vector.Fixed {
		if err := assignableOperand(operand, parameterAt(index), fmt.Sprintf("fixed operand %d", index)); err != nil {
			return err
		}
	}
	// An open tail and, behind it, an end-anchored suffix may land on any
	// parameter from their earliest position onward, so each must satisfy every
	// parameter it can reach and the variadic element behind them.
	if open {
		element := vector.TailType
		if element == nil {
			element = typ.Any
		}
		if err := reachableConform(element, function, fixed, "tail element"); err != nil {
			return err
		}
		for index, operand := range vector.Suffix {
			if err := reachableConform(operand, function, fixed+index, fmt.Sprintf("suffix operand %d", index)); err != nil {
				return err
			}
		}
		return nil
	}
	for index, operand := range vector.Suffix {
		if err := assignableOperand(operand, parameterAt(fixed+index), fmt.Sprintf("suffix operand %d", index)); err != nil {
			return err
		}
	}
	return nil
}

// reachableConform checks one operand that has no fixed landing position
// against every parameter at or after its earliest position, and against the
// variadic element it reaches past the fixed list.
func reachableConform(operand typ.Type, function *typ.Function, earliest int, role string) error {
	for index := earliest; index < len(function.Params); index++ {
		if err := assignableOperand(operand, function.Params[index].Type, fmt.Sprintf("%s at parameter %d", role, index)); err != nil {
			return err
		}
	}
	return assignableOperand(operand, function.Variadic, role+" at the variadic parameter")
}

func assignableOperand(operand, parameter typ.Type, role string) error {
	if operand == nil || parameter == nil {
		return nil
	}
	if domaincontract.Assignable(operand, parameter) {
		return nil
	}
	return fmt.Errorf("%s is declared %s, which is not assignable to the declared parameter type %s", role, typeText(operand), typeText(parameter))
}

// requiredParameters is the count of leading parameters a call must supply.
// An optional parameter followed by a required one is still required, so the
// bound is the position of the last non-optional entry.
func requiredParameters(function *typ.Function) int {
	required := 0
	for index, parameter := range function.Params {
		if !parameter.Optional {
			required = index + 1
		}
	}
	return required
}

func typeText(value typ.Type) string {
	if value == nil {
		return "<none>"
	}
	return value.String()
}
