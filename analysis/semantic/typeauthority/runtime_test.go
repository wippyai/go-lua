package typeauthority_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/semantic/typeauthority"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestRuntimeSealsFiniteExactStructuralClosure(t *testing.T) {
	p, source, types := runtimeFixture(t, `
type Text = string
type Choice = number | string
type Shape = {name: string, next: string?}
type Fn = fun(value: Shape): string
type Whole = integer
type Numeric = number
type Box<T: string> = {value: T}
type Boxed = Box<string>
type Pair<T, U: T> = {first: T, second: U}
type Other<T, U: T> = {first: T, second: U}
`)
	inputs := make([]typeauthority.RuntimeInput, 10)
	for index := range inputs {
		inputs[index] = runtimeInputForAlias(t, types, p, index)
	}
	runtime, inners, err := typeauthority.SealRuntime(types, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(inners), len(inputs); got != want {
		t.Fatalf("returned Runtime inners = %d, want %d", got, want)
	}
	if !runtime.ContentID().Available() || runtime.LinkID() != source.ContentID() {
		t.Fatal("Runtime lost its Link-fenced content identity")
	}

	text, choice, shape, function := inners[0], inners[1], inners[2], inners[3]
	whole, numeric, box, boxed := inners[4], inners[5], inners[6], inners[7]
	pair, other := inners[8], inners[9]
	if form, ok := runtime.Form(text); !ok || form != typeauthority.FormString || form.KindString() != "string" {
		t.Fatalf("Text form = %v/%v", form, ok)
	}
	if form, ok := runtime.Form(choice); !ok || form != typeauthority.FormUnion || runtime.VariantCount(choice) != 2 {
		t.Fatalf("Choice form/variants = %v/%v/%d", form, ok, runtime.VariantCount(choice))
	}
	seenNumber, seenString := false, false
	for index := 0; index < runtime.VariantCount(choice); index++ {
		variant, present, ok := runtime.VariantAt(choice, index)
		if !ok || !present {
			t.Fatalf("Choice variant %d = %v/%v", index, present, ok)
		}
		form, _ := runtime.Form(variant)
		seenNumber = seenNumber || form == typeauthority.FormNumber
		seenString = seenString || form == typeauthority.FormString
	}
	if !seenNumber || !seenString {
		t.Fatal("Choice reflection lost a direct member")
	}
	if runtime.FieldCount(shape) != 2 {
		t.Fatalf("Shape field count = %d, want 2", runtime.FieldCount(shape))
	}
	name, next, present, ok := runtime.FieldAt(shape, 1)
	if !ok || !present || name != "next" {
		t.Fatalf("Shape.next = %q/%v/%v", name, present, ok)
	}
	if form, ok := runtime.Form(next); !ok || form != typeauthority.FormOptional {
		t.Fatalf("Shape.next form = %v/%v", form, ok)
	}
	if got, present, found := runtime.Field(shape, "next"); !found || !present || !runtime.Equal(got, next) {
		t.Fatalf("Shape Field(next) = %v/%v/%v", got, present, found)
	}
	if runtime.ParameterCount(function) != 1 {
		t.Fatalf("Fn parameter count = %d, want 1", runtime.ParameterCount(function))
	}
	if returned, ok := runtime.Return(function); !ok {
		t.Fatal("Fn return is absent")
	} else if form, ok := runtime.Form(returned); !ok || form != typeauthority.FormString {
		t.Fatalf("Fn return form = %v/%v", form, ok)
	}
	if form, ok := runtime.Form(box); !ok || form != typeauthority.FormGeneric || runtime.TypeParameterCount(box) != 1 {
		t.Fatalf("Box form/tparams = %v/%v/%d", form, ok, runtime.TypeParameterCount(box))
	}
	if name, constraint, present, ok := runtime.TypeParameterAt(box, 0); !ok || !present || name != "T" {
		t.Fatalf("Box T parameter = %q/%v/%v", name, present, ok)
	} else if form, ok := runtime.Form(constraint); !ok || form != typeauthority.FormString {
		t.Fatalf("Box T constraint form = %v/%v", form, ok)
	}
	if form, ok := runtime.Form(boxed); !ok || form != typeauthority.FormInstantiated {
		t.Fatalf("Boxed form = %v/%v", form, ok)
	}
	if instantiated, ok := runtime.Instantiate(box, []typeauthority.RuntimeInner{text}); !ok || !runtime.Equal(instantiated, boxed) {
		t.Fatal("fixed generic instantiation did not recover Boxed")
	}
	if count := runtime.InstantiationCount(); count != 1 {
		t.Fatalf("instantiation count = %d, want 1", count)
	}
	result, base, argumentCount, ok := runtime.InstantiationAt(0)
	if !ok || !runtime.Equal(result, boxed) || !runtime.Equal(base, box) || argumentCount != 1 {
		t.Fatalf("instantiation row = %v/%v/%d/%v", result, base, argumentCount, ok)
	}
	if argument, ok := runtime.InstantiationArgumentAt(0, 0); !ok || !runtime.Equal(argument, text) {
		t.Fatalf("instantiation argument = %v/%v", argument, ok)
	}
	if answer, decided := runtime.Subtype(whole, numeric); !decided || !answer {
		t.Fatal("Runtime did not retain integer <: number")
	}
	if answer, decided := runtime.StructuralEqual(text, choice); !decided || answer {
		t.Fatal("Runtime structural equality followed neither exact canonical rows nor closure")
	}

	_, pairConstraint, pairPresent, pairOK := runtime.TypeParameterAt(pair, 1)
	_, otherConstraint, otherPresent, otherOK := runtime.TypeParameterAt(other, 1)
	if !pairOK || !pairPresent || !otherOK || !otherPresent {
		t.Fatal("scoped generic constraints were not reflected")
	}
	if runtime.Equal(pairConstraint, otherConstraint) {
		t.Fatal("distinct generic binder formals collapsed into one Runtime inner")
	}
	if answer, decided := runtime.StructuralEqual(pairConstraint, otherConstraint); decided || answer {
		t.Fatal("separate scoped formals were fabricated as structurally decidable")
	}
	firstID, ok := runtime.Identity(boxed)
	if !ok || !firstID.Available() {
		t.Fatal("Runtime inner identity is unavailable")
	}
	if secondID, ok := boxed.Identity(); !ok || secondID != firstID {
		t.Fatal("RuntimeInner Identity disagrees with its owning Runtime")
	}
}

func TestRuntimeSeparatesStructuralIdentityFromMutualSubtyping(t *testing.T) {
	p, _, types := runtimeFixture(t, `
type Anything = any
type UnknownThing = unknown
`)
	inputs := []typeauthority.RuntimeInput{
		runtimeInputForAlias(t, types, p, 0),
		runtimeInputForAlias(t, types, p, 1),
	}
	runtime, inners, err := typeauthority.SealRuntime(types, inputs)
	if err != nil {
		t.Fatal(err)
	}
	anything, unknown := inners[0], inners[1]
	if runtime.Equal(anything, unknown) {
		t.Fatal("Runtime collapsed exact any and unknown identities")
	}
	if equal, decided := runtime.StructuralEqual(anything, unknown); !decided || equal {
		t.Fatal("Runtime structural equality followed semantic mutual subtyping")
	}
	if answer, decided := runtime.Subtype(anything, unknown); !decided || !answer {
		t.Fatal("Runtime lost any <: unknown")
	}
	if answer, decided := runtime.Subtype(unknown, anything); !decided || !answer {
		t.Fatal("Runtime lost unknown <: any")
	}
}

func TestRuntimeStructuralAdmissionRejectsFreeAndUnsupportedForms(t *testing.T) {
	_, _, types := runtimeFixture(t, "type Closed = string")

	free := typ.NewTypeParam("Free", nil)
	freeEncoding, err := typ.EncodeCanonical(context.Background(), free)
	if err != nil {
		t.Fatalf("encode free type parameter: %v", err)
	}
	if _, ok := types.RuntimeInput(freeEncoding); ok {
		t.Fatal("RuntimeInput admitted a free type parameter")
	}

	unresolvedEncoding, err := typ.EncodeCanonical(context.Background(), typ.NewRef("missing", "Type"))
	if err != nil {
		t.Fatalf("encode unresolved reference: %v", err)
	}
	if unresolved, ok := types.RuntimeInput(unresolvedEncoding); ok {
		runtime, inners, sealErr := typeauthority.SealRuntime(types, []typeauthority.RuntimeInput{unresolved})
		if sealErr == nil || runtime != nil || inners != nil {
			t.Fatalf("unresolved reference sealing = %v/%d/%v, want nil/nil/error", runtime, len(inners), sealErr)
		}
	}
}

func TestRuntimeInstantiationTrieOwnsPrefixesSharedBaseAndHighArity(t *testing.T) {
	_, _, types := runtimeFixture(t, `local marker = true`)
	left := typ.NewTypeParam("L", nil)
	right := typ.NewTypeParam("R", nil)
	generic := typ.NewGeneric("Prefix", []*typ.TypeParam{left, right}, typ.NewTuple(left, right))
	extended := typ.Instantiate(generic, typ.String, typ.Number)
	sibling := typ.Instantiate(generic, typ.String, typ.Boolean)
	branch := typ.Instantiate(generic, typ.Number, typ.String)

	const highArity = 4097
	highParameters := make([]*typ.TypeParam, highArity)
	highArguments := make([]typ.Type, highArity)
	for index := range highArguments {
		highParameters[index] = typ.NewTypeParam("T"+strconv.Itoa(index), nil)
		highArguments[index] = typ.String
	}
	highGeneric := typ.NewGeneric("High", highParameters, typ.String)
	high := typ.Instantiate(highGeneric, highArguments...)
	zeroGeneric := typ.NewGeneric("Zero", nil, typ.String)
	zero := typ.Instantiate(zeroGeneric)

	values := []typ.Type{generic, extended, sibling, branch, highGeneric, high, typ.String, typ.Number, typ.Boolean, zeroGeneric, zero}
	const fanout = 257
	fanoutLiteralStart := len(values)
	for index := 0; index < fanout; index++ {
		values = append(values, typ.LiteralString("branch-"+strconv.Itoa(index)))
	}
	fanoutResultStart := len(values)
	for index := 0; index < fanout; index++ {
		values = append(values, typ.Instantiate(generic, values[fanoutLiteralStart+index], typ.Number))
	}
	duplicateExtended := len(values)
	values = append(values, extended)
	inputs := make([]typeauthority.RuntimeInput, len(values))
	for index, value := range values {
		inputs[index] = runtimeInputForType(t, types, value)
	}
	runtime, inners, err := typeauthority.SealRuntime(types, inputs)
	if err != nil {
		t.Fatal(err)
	}
	base, extendedResult, siblingResult := inners[0], inners[1], inners[2]
	branchResult, highBase, highResult := inners[3], inners[4], inners[5]
	stringInner, numberInner, booleanInner := inners[6], inners[7], inners[8]
	zeroBase, zeroResult := inners[9], inners[10]
	if !runtime.Equal(inners[duplicateExtended], extendedResult) {
		t.Fatal("duplicate instantiation input did not reuse its canonical result")
	}
	if count := runtime.InstantiationCount(); count != 5+fanout {
		t.Fatalf("fixed instantiation count = %d, want %d", count, 5+fanout)
	}

	root, ok := runtime.BeginInstantiation(base)
	if !ok {
		t.Fatal("shared fixed-instantiation base is absent")
	}
	stringPrefix, ok := runtime.MatchInstantiationArgument(root, stringInner)
	if !ok {
		t.Fatal("shared string prefix is absent")
	}
	if _, ok := runtime.FinishInstantiation(stringPrefix); ok {
		t.Fatal("nonterminal shared prefix was accepted as an exact instantiation")
	}
	extendedMatch, ok := runtime.MatchInstantiationArgument(stringPrefix, numberInner)
	if !ok {
		t.Fatal("extended string/number path is absent")
	}
	if result, ok := runtime.FinishInstantiation(extendedMatch); !ok || !runtime.Equal(result, extendedResult) {
		t.Fatal("extended prefix did not recover its exact result")
	}
	siblingMatch, ok := runtime.MatchInstantiationArgument(stringPrefix, booleanInner)
	if !ok {
		t.Fatal("shared-prefix sibling path is absent")
	}
	if result, ok := runtime.FinishInstantiation(siblingMatch); !ok || !runtime.Equal(result, siblingResult) {
		t.Fatal("shared-prefix sibling recovered the wrong result")
	}
	branchMatch, ok := runtime.MatchInstantiationArgument(root, numberInner)
	if !ok {
		t.Fatal("number branch is absent")
	}
	branchMatch, ok = runtime.MatchInstantiationArgument(branchMatch, stringInner)
	if !ok {
		t.Fatal("number/string branch suffix is absent")
	}
	if result, ok := runtime.FinishInstantiation(branchMatch); !ok || !runtime.Equal(result, branchResult) {
		t.Fatal("number branch recovered the wrong result")
	}
	zeroMatch, ok := runtime.BeginInstantiation(zeroBase)
	if !ok {
		t.Fatal("zero-arity instantiation base is absent")
	}
	if result, ok := runtime.FinishInstantiation(zeroMatch); !ok || !runtime.Equal(result, zeroResult) {
		t.Fatal("zero-arity instantiation did not finish at its root")
	}

	fanoutMatch, ok := runtime.BeginInstantiation(base)
	if !ok {
		t.Fatal("high-fanout base is absent")
	}
	fanoutMatch, ok = runtime.MatchInstantiationArgument(fanoutMatch, inners[fanoutLiteralStart+fanout-1])
	if !ok {
		t.Fatal("high-fanout binary search lost its last branch")
	}
	fanoutMatch, ok = runtime.MatchInstantiationArgument(fanoutMatch, numberInner)
	if !ok {
		t.Fatal("high-fanout branch suffix is absent")
	}
	if result, ok := runtime.FinishInstantiation(fanoutMatch); !ok || !runtime.Equal(result, inners[fanoutResultStart+fanout-1]) {
		t.Fatal("high-fanout branch recovered the wrong result")
	}

	highMatch, ok := runtime.BeginInstantiation(highBase)
	if !ok {
		t.Fatal("high-arity base is absent")
	}
	for index := 0; index < highArity; index++ {
		highMatch, ok = runtime.MatchInstantiationArgument(highMatch, stringInner)
		if !ok {
			t.Fatalf("high-arity path stopped at argument %d", index)
		}
	}
	if result, ok := runtime.FinishInstantiation(highMatch); !ok || !runtime.Equal(result, highResult) {
		t.Fatal("high-arity path recovered the wrong result")
	}
	if result, ok := runtime.Instantiate(base, []typeauthority.RuntimeInner{stringInner, booleanInner}); !ok || !runtime.Equal(result, siblingResult) {
		t.Fatal("slice convenience lookup diverged from the authoritative trie")
	}
	foreign, foreignInners, err := typeauthority.SealRuntime(types, inputs)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := runtime.BeginInstantiation(foreignInners[0]); ok {
		t.Fatal("Runtime accepted a foreign base inner")
	}
	foreignMatch, ok := foreign.BeginInstantiation(foreignInners[0])
	if !ok {
		t.Fatal("foreign Runtime lost its own base")
	}
	if _, ok := runtime.FinishInstantiation(foreignMatch); ok {
		t.Fatal("Runtime accepted a foreign instantiation cursor")
	}
	if _, ok := runtime.MatchInstantiationArgument(stringPrefix, foreignInners[6]); ok {
		t.Fatal("Runtime accepted a foreign argument inner")
	}

	if allocations := testing.AllocsPerRun(1000, func() {
		match, matched := runtime.BeginInstantiation(base)
		if matched {
			match, matched = runtime.MatchInstantiationArgument(match, stringInner)
		}
		if matched {
			match, matched = runtime.MatchInstantiationArgument(match, numberInner)
		}
		if matched {
			_, _ = runtime.FinishInstantiation(match)
		}
	}); allocations != 0 {
		t.Fatalf("Runtime instantiation trie allocated %.2f objects/run", allocations)
	}
}

func runtimeFixture(t testing.TB, sourceText string) (*program.Program, *link.Link, *typeauthority.Authority) {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "runtime_authority.lua", Text: []byte(sourceText)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "runtime_authority", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	types, ok := typeauthority.Seal(source)
	if !ok {
		t.Fatal("selector Authority did not seal")
	}
	return p, source, types
}

func runtimeInputForAlias(t testing.TB, types *typeauthority.Authority, p *program.Program, index int) typeauthority.RuntimeInput {
	t.Helper()
	term, ok := p.Static().Declarations().Aliases().At(index)
	if !ok {
		t.Fatalf("missing alias %d", index)
	}
	selector, ok := types.Find(p.ContentID(), term)
	if !ok {
		t.Fatalf("missing selector for alias %d", index)
	}
	value, ok := types.Materialize(selector)
	if !ok {
		t.Fatalf("alias %d did not materialize", index)
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		t.Fatalf("alias %d canonical encoding: %v", index, err)
	}
	input, ok := types.RuntimeInput(encoded)
	if !ok {
		t.Fatalf("alias %d canonical input rejected", index)
	}
	return input
}

func runtimeInputForReference(t testing.TB, types *typeauthority.Authority, reference typeauthority.StaticTypeRef) typeauthority.RuntimeInput {
	t.Helper()
	selector, ok := types.Lookup(reference)
	if !ok {
		t.Fatal("runtime descriptor target escaped selector authority")
	}
	value, ok := types.Materialize(selector)
	if !ok {
		t.Fatal("runtime descriptor target did not materialize")
	}
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		t.Fatalf("runtime descriptor target canonical encoding: %v", err)
	}
	input, ok := types.RuntimeInput(encoded)
	if !ok {
		t.Fatal("runtime descriptor target input rejected")
	}
	return input
}

func runtimeInputForType(t testing.TB, types *typeauthority.Authority, value typ.Type) typeauthority.RuntimeInput {
	t.Helper()
	encoded, err := typ.EncodeCanonical(context.Background(), value)
	if err != nil {
		t.Fatalf("canonical Runtime input encoding: %v", err)
	}
	input, ok := types.RuntimeInput(encoded)
	if !ok {
		t.Fatal("canonical Runtime input rejected")
	}
	return input
}
