// Package profile authors the frozen Lua 5.3 and Wippy target-operation
// catalogue.  It deliberately contains static envelopes only: an operation's
// Rule owns defaults, cardinality, conditional result selection, and the
// element checks of an open Values tail.
package profile

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	domaincontract "github.com/wippyai/go-lua/analysis/domain/type/typecontract"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/target"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

// Contract seals a fresh authoring input.  Callers that need to inspect or
// extend the static authoring input should use Spec instead.
func Contract() (*target.Contract, error) {
	spec, err := authoredSpec()
	if err != nil {
		return nil, err
	}
	return target.Seal(&spec)
}

// Spec returns the complete closed static catalogue.  Each call owns its
// authoring storage, as Seal consumes the input it receives.
func Spec() target.Spec {
	spec, err := authoredSpec()
	if err != nil {
		// The catalogue is closed source owned by this package.  Spec preserves
		// its historical no-error API, while Contract exposes the fail-closed
		// authoring error to normal callers.
		panic(err)
	}
	return spec
}

func authoredSpec() (target.Spec, error) {
	catalogue, err := operations()
	if err != nil {
		return target.Spec{}, err
	}
	// Callable-valued results refer to the sole ordinary operation that they
	// produce.  The dynamically selected iterator/resume behavior is Rule work.
	produce := func(producer, child string, captures ...target.CaptureSpec) error {
		return catalogue.produce(producer, child, captures...)
	}
	for _, relation := range []struct {
		producer, child string
		captures        []target.CaptureSpec
	}{
		{"coroutine.wrap", "coroutine.wrap.invoke", []target.CaptureSpec{{Kind: target.CaptureCallback, Ordinal: 1}}},
		{"string.gmatch", "string.gmatch.next", []target.CaptureSpec{{Kind: target.CaptureValueFormal, Ordinal: 0}, {Kind: target.CaptureValueFormal, Ordinal: 1}}},
		{"ipairs", "ipairs.aux", nil},
		{"utf8.codes", "utf8.codes.aux", nil},
	} {
		if err := produce(relation.producer, relation.child, relation.captures...); err != nil {
			return target.Spec{}, err
		}
	}
	// pairs has two distinct successful laws.  Only the no-__pairs fallback
	// produces next; a user __pairs hook supplies all three arbitrary results.
	if err := catalogue.produceAt("pairs", 1, "next"); err != nil {
		return target.Spec{}, err
	}
	for _, alias := range []struct {
		operation      string
		result, source uint32
	}{
		{"setmetatable", 0, 0}, {"rawset", 0, 0}, {"table.freeze", 0, 0},
		{"ipairs", 1, 0}, {"utf8.codes", 1, 0},
	} {
		if err := catalogue.resultAlias(alias.operation, alias.result, alias.source); err != nil {
			return target.Spec{}, err
		}
	}
	if err := catalogue.resultAliasAt("pairs", 1, 1, 0); err != nil {
		return target.Spec{}, err
	}
	// These result roots are nominal allocation facts. Their ordinary values,
	// callback, and produced-operation relations remain separate Target rows.
	for _, fresh := range []struct {
		operation string
		kind      target.FreshKind
	}{
		{"table.pack", target.FreshTable},
		{"table.create", target.FreshTable},
		{"coroutine.create", target.FreshThread},
		{"coroutine.wrap", target.FreshFunction},
		{"string.gmatch", target.FreshFunction},
		{"errors.new", target.FreshError},
		{"errors.wrap", target.FreshError},
		{"errors.Error.details", target.FreshTable},
	} {
		if err := catalogue.freshResult(fresh.operation, fresh.kind); err != nil {
			return target.Spec{}, err
		}
	}
	if err := catalogue.selfEffects(); err != nil {
		return target.Spec{}, err
	}
	boot, err := bootLedger(catalogue)
	if err != nil {
		return target.Spec{}, err
	}
	return target.Spec{
		Semantics:         domaincontract.NewSemantics(),
		Operations:        catalogue.operations,
		InitialRoots:      boot.roots,
		InitialEntries:    boot.entries,
		InitialBindings:   boot.bindings,
		InitialMetatables: boot.metatables,
	}, nil
}

// Admitted returns every bound source/provider identity in the profile.  The
// returned bindings are copies and may be safely retained by callers.
func Admitted() []target.BindingSpec {
	catalogue, err := operations()
	if err != nil {
		panic(err)
	}
	out := make([]target.BindingSpec, 0, len(catalogue.operations))
	for _, op := range catalogue.operations {
		out = append(out, op.Bindings...)
	}
	return out
}

// Excluded is the complete intentionally unsupported boundary.  It is data,
// rather than absence by convention, so profile coverage can reject drift.
func Excluded() []target.BindingSpec {
	out := make([]target.BindingSpec, len(excluded))
	for i, binding := range excluded {
		out[i] = target.BindingSpec{Namespace: binding.Namespace, Owner: append([]string(nil), binding.Owner...), Member: append([]string(nil), binding.Member...)}
	}
	return out
}

type operationRef uint32

// authoredCatalogue owns the one closed name-to-operation identity table.
// A zero operationRef is invalid, so an absent name can never accidentally
// become target.SpecRef(1), the first valid authored operation.
type authoredCatalogue struct {
	operations []target.OperationSpec
	names      map[string]operationRef
}

func (catalogue *authoredCatalogue) add(name string, operation target.OperationSpec) {
	if catalogue.names == nil {
		catalogue.names = make(map[string]operationRef)
	}
	ref := operationRef(len(catalogue.operations) + 1)
	catalogue.names[name] = ref
	catalogue.operations = append(catalogue.operations, operation)
}

func (catalogue *authoredCatalogue) lookup(name string) (operationRef, bool) {
	ref, ok := catalogue.names[name]
	if !ok || ref == 0 || int(ref) > len(catalogue.operations) {
		return 0, false
	}
	return ref, true
}

func (catalogue *authoredCatalogue) require(name string) (operationRef, error) {
	ref, ok := catalogue.lookup(name)
	if !ok {
		return 0, fmt.Errorf("profile: unknown authored operation %q", name)
	}
	return ref, nil
}

func (catalogue *authoredCatalogue) at(ref operationRef) *target.OperationSpec {
	return &catalogue.operations[uint32(ref)-1]
}

func (catalogue *authoredCatalogue) replace(name string, operation target.OperationSpec) error {
	ref, err := catalogue.require(name)
	if err != nil {
		return err
	}
	*catalogue.at(ref) = operation
	return nil
}

func (catalogue *authoredCatalogue) produce(producer, child string, captures ...target.CaptureSpec) error {
	return catalogue.produceAt(producer, 0, child, captures...)
}

func (catalogue *authoredCatalogue) produceAt(producer string, outcome int, child string, captures ...target.CaptureSpec) error {
	producerRef, err := catalogue.require(producer)
	if err != nil {
		return err
	}
	childRef, err := catalogue.require(child)
	if err != nil {
		return err
	}
	op := catalogue.at(producerRef)
	if outcome < 0 || outcome >= len(op.Outcomes) {
		return fmt.Errorf("profile: outcome %d outside %q", outcome, producer)
	}
	op.Outcomes[outcome].Produced = []target.ProducedSpec{{
		Result: 0, Operation: target.SpecRef(childRef), Captures: captures,
	}}
	return nil
}

func (catalogue *authoredCatalogue) resultAlias(operation string, result, source uint32) error {
	return catalogue.resultAliasAt(operation, 0, result, source)
}

func (catalogue *authoredCatalogue) resultAliasAt(operation string, outcome int, result, source uint32) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	op := catalogue.at(ref)
	if outcome < 0 || outcome >= len(op.Outcomes) {
		return fmt.Errorf("profile: outcome %d outside %q", outcome, operation)
	}
	op.Outcomes[outcome].ResultAliases = []target.ResultAliasSpec{{
		Result: result, Source: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: source},
	}}
	return nil
}

func (catalogue *authoredCatalogue) freshResult(operation string, kind target.FreshKind) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	op := catalogue.at(ref)
	if len(op.Outcomes) == 0 || len(op.Outcomes[0].Values.Fixed) == 0 {
		return fmt.Errorf("profile: %q has no first fixed normal result for FreshResult", operation)
	}
	op.Outcomes[0].FreshResults = []target.FreshResultSpec{{Result: 0, Kind: kind}}
	return nil
}

func (catalogue *authoredCatalogue) inputTailType(operation string, class typ.Type) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	values := &catalogue.at(ref).Input
	if values.Tail != target.ValuesVariable {
		return fmt.Errorf("profile: %q input has no open Values tail", operation)
	}
	values.TailType = portable(class)
	return nil
}

func (catalogue *authoredCatalogue) outcomeTailType(operation string, outcome int, class typ.Type) error {
	ref, err := catalogue.require(operation)
	if err != nil {
		return err
	}
	op := catalogue.at(ref)
	if outcome < 0 || outcome >= len(op.Outcomes) || op.Outcomes[outcome].Values.Tail != target.ValuesVariable {
		return fmt.Errorf("profile: %q outcome %d has no open Values tail", operation, outcome)
	}
	op.Outcomes[outcome].Values.TailType = portable(class)
	return nil
}

func operations() (authoredCatalogue, error) {
	var catalogue authoredCatalogue
	add := catalogue.add
	// Base library.
	add("assert", openSame(builtin("assert")))
	add("error", throws(builtin("error"), []typ.Type{typ.Any}, true))
	add("getmetatable", fixed(builtin("getmetatable"), []typ.Type{typ.Any}, []typ.Type{typ.Any}))
	add("setmetatable", fixed(builtin("setmetatable"), []typ.Type{typ.Any, typ.Any}, []typ.Type{typ.Any}))
	add("next", alternatives(builtin("next"), []typ.Type{typ.Any}, true, [][]typ.Type{{typ.Nil}, {typ.Any, typ.Any}}))
	add("pcall", protected(builtin("pcall"), 1))
	add("print", printProfile())
	add("rawequal", fixed(builtin("rawequal"), []typ.Type{typ.Any, typ.Any}, []typ.Type{typ.Boolean}))
	add("rawget", fixed(builtin("rawget"), []typ.Type{typ.Any, typ.Any}, []typ.Type{typ.Any}))
	add("rawlen", fixed(builtin("rawlen"), []typ.Type{typ.Any}, []typ.Type{typ.Integer}))
	add("rawset", fixed(builtin("rawset"), []typ.Type{typ.Any, typ.Any, typ.Any}, []typ.Type{typ.Any}))
	add("select", normal(builtin("select"), []typ.Type{typ.Any}, true, nil, true))
	add("tonumber", normal(builtin("tonumber"), []typ.Type{typ.Any}, true, []typ.Type{typ.Number}, false))
	add("tostring", tostringProfile())
	add("type", fixed(builtin("type"), []typ.Type{typ.Any}, []typ.Type{typ.String}))
	// Lua's global unpack and table.unpack are the same initial callable.
	// They therefore share one Target operation, not merely equal ABIs.
	add("unpack", normal(target.OperationSpec{Bindings: []target.BindingSpec{
		{Namespace: target.BindingBuiltin, Member: []string{"unpack"}},
		{Namespace: target.BindingModule, Owner: []string{"table"}, Member: []string{"unpack"}},
	}}, []typ.Type{typ.Any}, true, nil, true))
	add("xpcall", protected(builtin("xpcall"), 2))
	add("ipairs", ipairsProfile())
	add("pairs", pairsProfile())
	// Link owns literal-module selection. Target retains the complete ordinary
	// call envelope so dynamic or nonliteral calls remain representable.
	add("require", normal(builtin("require"), []typ.Type{typ.Any}, true, []typ.Type{typ.Any}, false))

	// table
	for _, item := range []struct {
		name    string
		in, out []typ.Type
		tail    bool
	}{
		{"getn", []typ.Type{typ.Any}, []typ.Type{typ.Number}, false},
		{"concat", []typ.Type{typ.Any}, []typ.Type{typ.String}, true},
		{"insert", []typ.Type{typ.Any, typ.Any}, nil, true},
		{"move", []typ.Type{typ.Any, typ.Integer, typ.Integer, typ.Integer}, []typ.Type{typ.Any}, true},
		{"pack", nil, []typ.Type{typ.Any}, true},
		{"maxn", []typ.Type{typ.Any}, []typ.Type{typ.Number}, false},
		{"remove", []typ.Type{typ.Any}, []typ.Type{typ.Any}, true},
		{"sort", []typ.Type{typ.Any}, nil, true},
		{"create", []typ.Type{typ.Integer, typ.Integer}, []typ.Type{typ.Any}, false},
		{"freeze", []typ.Type{typ.Any}, []typ.Type{typ.Any}, false},
		{"isfrozen", []typ.Type{typ.Any}, []typ.Type{typ.Boolean}, false},
	} {
		add("table."+item.name, normal(module("table", item.name), item.in, item.tail, item.out, item.name == "unpack"))
	}
	if err := catalogue.replace("table.sort", tableSortProfile()); err != nil {
		return authoredCatalogue{}, err
	}
	for _, item := range []struct {
		name string
		op   target.OperationSpec
	}{
		{"table.concat", tableConcatProfile()},
		{"table.insert", tableInsertProfile()},
		{"table.move", tableMoveProfile()},
		{"table.remove", tableRemoveProfile()},
		{"table.sort", tableSortProfile()},
		{"unpack", tableUnpackProfile()},
	} {
		if err := catalogue.replace(item.name, item.op); err != nil {
			return authoredCatalogue{}, err
		}
	}

	// string (gfind is an equivalent binding of gmatch).
	for _, item := range []struct {
		name            string
		in, out         []typ.Type
		tailIn, tailOut bool
	}{
		{"byte", []typ.Type{typ.String}, []typ.Type{typ.Integer}, true, false}, {"char", nil, []typ.Type{typ.String}, true, false},
		{"find", []typ.Type{typ.String, typ.String}, []typ.Type{typ.Integer, typ.Integer}, true, false},
		{"format", []typ.Type{typ.String}, []typ.Type{typ.String}, true, false}, {"gsub", []typ.Type{typ.String, typ.String, typ.Any}, []typ.Type{typ.String, typ.Integer}, true, false},
		{"len", []typ.Type{typ.String}, []typ.Type{typ.Integer}, false, false}, {"lower", []typ.Type{typ.String}, []typ.Type{typ.String}, false, false},
		{"match", []typ.Type{typ.String, typ.String}, []typ.Type{typ.Any}, true, true}, {"pack", []typ.Type{typ.String}, []typ.Type{typ.String}, true, false},
		{"packsize", []typ.Type{typ.String}, []typ.Type{typ.Integer}, false, false}, {"rep", []typ.Type{typ.String, typ.Integer}, []typ.Type{typ.String}, true, false},
		{"reverse", []typ.Type{typ.String}, []typ.Type{typ.String}, false, false}, {"sub", []typ.Type{typ.String, typ.Integer}, []typ.Type{typ.String}, true, false},
		{"unpack", []typ.Type{typ.String, typ.String}, nil, true, true}, {"upper", []typ.Type{typ.String}, []typ.Type{typ.String}, false, false},
	} {
		add("string."+item.name, normal(module("string", item.name), item.in, item.tailIn, item.out, item.tailOut))
	}
	find := normal(module("string", "find"), []typ.Type{typ.String, typ.String}, true, []typ.Type{typ.Integer, typ.Integer}, true)
	find.Outcomes = append(find.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.Nil}, false, 0)})
	if err := catalogue.replace("string.find", find); err != nil {
		return authoredCatalogue{}, err
	}
	match := normal(module("string", "match"), []typ.Type{typ.String, typ.String}, true, nil, true)
	match.Outcomes = append(match.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.Nil}, false, 0)})
	if err := catalogue.replace("string.match", match); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.replace("string.gsub", callbackGsubProfile()); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.replace("string.format", formatProfile()); err != nil {
		return authoredCatalogue{}, err
	}
	add("string.gmatch", normal(aliasModule("string", "gmatch", "gfind"), []typ.Type{typ.String, typ.String}, false, []typ.Type{typ.Any}, false))
	byte := normal(module("string", "byte"), []typ.Type{typ.String}, true, nil, true)
	byte.Outcomes[0].Values.TailType = portable(typ.Integer)
	if err := catalogue.replace("string.byte", byte); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.inputTailType("string.char", typ.Integer); err != nil {
		return authoredCatalogue{}, err
	}
	unpack := normal(module("string", "unpack"), []typ.Type{typ.String, typ.String}, true, nil, true)
	unpack.Outcomes[0].Values.Suffix = portableList([]typ.Type{typ.Integer})
	if err := catalogue.replace("string.unpack", unpack); err != nil {
		return authoredCatalogue{}, err
	}

	// math
	for _, item := range []struct {
		name    string
		in, out []typ.Type
		tail    bool
	}{
		{"abs", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"acos", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"asin", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false},
		{"atan", []typ.Type{typ.Number}, []typ.Type{typ.Number}, true}, {"atan2", []typ.Type{typ.Number, typ.Number}, []typ.Type{typ.Number}, false}, {"ceil", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false},
		{"cos", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"cosh", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"deg", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"exp", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false},
		{"floor", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"fmod", []typ.Type{typ.Number, typ.Number}, []typ.Type{typ.Number}, false}, {"frexp", []typ.Type{typ.Number}, []typ.Type{typ.Number, typ.Integer}, false},
		{"ldexp", []typ.Type{typ.Number, typ.Integer}, []typ.Type{typ.Number}, false}, {"log", []typ.Type{typ.Number}, []typ.Type{typ.Number}, true}, {"log10", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false},
		{"max", []typ.Type{typ.Number}, []typ.Type{typ.Number}, true}, {"min", []typ.Type{typ.Number}, []typ.Type{typ.Number}, true}, {"mod", []typ.Type{typ.Number, typ.Number}, []typ.Type{typ.Number}, false},
		{"modf", []typ.Type{typ.Number}, []typ.Type{typ.Number, typ.Number}, false}, {"pow", []typ.Type{typ.Number, typ.Number}, []typ.Type{typ.Number}, false}, {"rad", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false},
		{"random", nil, []typ.Type{typ.Number}, true}, {"randomseed", nil, nil, true}, {"sin", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"sinh", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false},
		{"sqrt", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"tan", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false}, {"tanh", []typ.Type{typ.Number}, []typ.Type{typ.Number}, false},
		{"tointeger", []typ.Type{typ.Any}, []typ.Type{typ.Integer}, false}, {"type", []typ.Type{typ.Any}, []typ.Type{typ.String}, false}, {"ult", []typ.Type{typ.Integer, typ.Integer}, []typ.Type{typ.Boolean}, false},
	} {
		if item.name == "min" || item.name == "max" {
			// These preserve a tie-winning input identity after ordinary <
			// comparison; their arguments are Values, not Numbers.
			if item.name == "min" {
				add("math."+item.name, minMaxProfile(module("math", item.name)))
			} else {
				add("math."+item.name, minMaxProfile(module("math", item.name)))
			}
			continue
		}
		add("math."+item.name, total(module("math", item.name), item.in, item.tail, item.out, false))
	}
	random := normal(module("math", "random"), nil, true, []typ.Type{typ.Number}, false)
	random.Outcomes = append(random.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.Integer}, false, 0)})
	if err := catalogue.replace("math.random", random); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.replace("math.tointeger", alternativesTotal(module("math", "tointeger"), []typ.Type{typ.Any}, false, [][]typ.Type{{typ.Integer}, {typ.Nil}})); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.replace("math.type", alternativesTotal(module("math", "type"), []typ.Type{typ.Any}, false, [][]typ.Type{{typ.String}, {typ.Nil}})); err != nil {
		return authoredCatalogue{}, err
	}

	add("coroutine.create", callbackCreate(module("coroutine", "create")))
	add("coroutine.resume", resumeEnvelope())
	add("coroutine.running", fixed(module("coroutine", "running"), nil, []typ.Type{typ.Any, typ.Boolean}))
	add("coroutine.status", fixed(module("coroutine", "status"), []typ.Type{typ.Any}, []typ.Type{typ.String}))
	add("coroutine.wrap", callbackWrap(module("coroutine", "wrap")))
	add("coroutine.spawn", callbackSpawn())
	yield := normal(module("coroutine", "yield"), nil, true, nil, true)
	yield.Outcomes = append(yield.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeYield, Values: values(nil, true, 0)})
	yield.Suspensions = []target.SuspensionSpec{{Yield: 2, Reentry: 0, Source: target.ReentryByCall, Multiplicity: target.ReentryOnce}}
	add("coroutine.yield", yield)

	for _, item := range []struct {
		name            string
		in, out         []typ.Type
		tailIn, tailOut bool
	}{
		{"char", nil, []typ.Type{typ.String}, true, false}, {"codepoint", []typ.Type{typ.String}, []typ.Type{typ.Integer}, true, true},
		{"len", []typ.Type{typ.String}, []typ.Type{typ.Integer}, true, false}, {"offset", []typ.Type{typ.String, typ.Integer}, []typ.Type{typ.Integer}, true, false},
	} {
		add("utf8."+item.name, normal(module("utf8", item.name), item.in, item.tailIn, item.out, item.tailOut))
	}
	if err := catalogue.replace("utf8.len", alternativesTotal(module("utf8", "len"), []typ.Type{typ.String}, true, [][]typ.Type{{typ.Integer}, {typ.Nil, typ.Integer}})); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.replace("utf8.offset", alternatives(module("utf8", "offset"), []typ.Type{typ.String, typ.Integer}, true, [][]typ.Type{{typ.Integer}, {typ.Nil}})); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.inputTailType("utf8.char", typ.Integer); err != nil {
		return authoredCatalogue{}, err
	}
	if err := catalogue.outcomeTailType("utf8.codepoint", 0, typ.Integer); err != nil {
		return authoredCatalogue{}, err
	}
	add("utf8.codes", fixed(module("utf8", "codes"), []typ.Type{typ.String}, []typ.Type{typ.Any, typ.String, typ.Integer}))

	for _, item := range []struct {
		name    string
		in, out []typ.Type
		tail    bool
	}{
		{"getupvalue", []typ.Type{typ.Any, typ.Integer}, []typ.Type{typ.Any}, false},
	} {
		add("debug."+item.name, normal(module("debug", item.name), item.in, item.tail, item.out, false))
	}
	if err := catalogue.replace("debug.getupvalue", alternatives(module("debug", "getupvalue"), []typ.Type{typ.Any, typ.Integer}, false, [][]typ.Type{{typ.String, typ.Any}, {typ.Nil}})); err != nil {
		return authoredCatalogue{}, err
	}
	for _, item := range []struct {
		name    string
		in, out []typ.Type
		tail    bool
	}{
		{"new", []typ.Type{typ.Any}, []typ.Type{typ.Any}, false}, {"wrap", []typ.Type{typ.Any, typ.Any}, []typ.Type{typ.Any}, false},
		{"is", []typ.Type{typ.Any, typ.String}, []typ.Type{typ.Boolean}, false},
	} {
		add("errors."+item.name, normal(module("errors", item.name), item.in, item.tail, item.out, false))
	}
	for _, item := range []struct {
		name string
		in   []typ.Type
		out  []typ.Type
	}{
		{"__tostring", nil, []typ.Type{typ.String}},
		{"__concat", []typ.Type{typ.Any}, []typ.Type{typ.String}},
		{"kind", nil, []typ.Type{typ.String}},
		{"retryable", nil, []typ.Type{typ.MaterializeUnion([]typ.Type{typ.Boolean, typ.Nil})}},
		{"details", nil, []typ.Type{typ.Any}},
		{"message", nil, []typ.Type{typ.String}},
	} {
		add("errors.Error."+item.name, method("errors", "Error", item.name, item.in, item.out))
	}
	details := target.OperationSpec{Bindings: []target.BindingSpec{{Namespace: target.BindingModule, Owner: []string{"errors"}, Member: []string{"Error", "details"}}}}
	details = alternativesTotal(details, []typ.Type{typ.Any}, false, [][]typ.Type{{typ.BuiltinTableTopMarker()}, {typ.Nil}})
	if err := catalogue.replace("errors.Error.details", details); err != nil {
		return authoredCatalogue{}, err
	}

	// Produced-only callable operations deliberately have no source binding.
	wrapInvoke := normal(target.OperationSpec{}, nil, true, nil, true)
	wrapInvoke.Resumes = []target.ResumeSpec{resumeRelation(target.ResumeSourceProduced, 0, 0, 0, 1)}
	add("coroutine.wrap.invoke", wrapInvoke)
	gmatchNext := normal(target.OperationSpec{}, nil, false, nil, true)
	gmatchNext.Outcomes = append(gmatchNext.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values(nil, false, 0)})
	add("string.gmatch.next", gmatchNext)
	add("ipairs.aux", ipairsAuxProfile())
	add("utf8.codes.aux", alternatives(target.OperationSpec{}, []typ.Type{typ.String, typ.Integer}, false, [][]typ.Type{nil, {typ.Integer, typ.Integer}}))
	return catalogue, nil
}

func builtin(name string) target.OperationSpec {
	return target.OperationSpec{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{name}}}}
}
func module(owner, name string) target.OperationSpec {
	return target.OperationSpec{Bindings: []target.BindingSpec{{Namespace: target.BindingModule, Owner: []string{owner}, Member: []string{name}}}}
}
func aliasModule(owner string, members ...string) target.OperationSpec {
	b := make([]target.BindingSpec, len(members))
	for i, member := range members {
		b[i] = target.BindingSpec{Namespace: target.BindingModule, Owner: []string{owner}, Member: []string{member}}
	}
	return target.OperationSpec{Bindings: b}
}
func method(owner, family, name string, in, out []typ.Type) target.OperationSpec {
	return fixed(target.OperationSpec{Bindings: []target.BindingSpec{{Namespace: target.BindingModule, Owner: []string{owner}, Member: []string{family, name}}}}, append([]typ.Type{typ.Any}, in...), out)
}
func values(fixed []typ.Type, open bool, variable target.ValuesVar) target.ValuesSpec {
	tail := target.ValuesClosed
	var tailType schematype.Type
	if open {
		tail = target.ValuesVariable
		tailType = portable(typ.Any)
	}
	return target.ValuesSpec{Fixed: portableList(fixed), Tail: tail, Var: variable, TailType: tailType}
}

// portable is the only place this Lua catalogue crosses into Program's
// neutral authored ABI. All interpretation and validation stays in the Lua
// type-domain adapter; target receives only schema/typecontract declarations.
func portable(value typ.Type) schematype.Type {
	encoded, err := domaincontract.EncodeStorage(context.Background(), value, nil)
	if err != nil {
		panic(fmt.Sprintf("profile: portable type: %v", err))
	}
	return encoded
}

func portableList(values []typ.Type) []schematype.Type {
	if len(values) == 0 {
		return nil
	}
	out := make([]schematype.Type, len(values))
	for index, value := range values {
		out[index] = portable(value)
	}
	return out
}

func closed(fixed ...typ.Type) target.ValuesSpec {
	return values(fixed, false, 0)
}

func anyValue() target.ValuesSpec { return closed(typ.Any) }

func emptyValues() target.ValuesSpec { return closed() }

func rejectedYield() target.ValuesSpec {
	return closed(typ.LiteralString("attempt to yield across a C-call boundary"))
}

func terminals(normal, returned, thrown, yielded, canceled target.ValuesSpec) []target.TerminalSpec {
	return []target.TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: normal},
		{Kind: flowkind.OutcomeReturn, Values: returned},
		{Kind: flowkind.OutcomeThrow, Values: thrown},
		{Kind: flowkind.OutcomeYield, Values: yielded},
		{Kind: flowkind.OutcomeCancel, Values: canceled},
	}
}

func outcomeRoute(kind flowkind.OutcomeKind, result target.ValuesSpec, adjustment target.Adjustment, placement target.Placement, outcome uint32) target.SubedgeRouteSpec {
	return target.SubedgeRouteSpec{Kind: kind, Route: target.RouteOutcome, Adjustment: adjustment, Result: result, Placement: placement, Outcome: outcome}
}

func siblingRoute(kind flowkind.OutcomeKind, result target.ValuesSpec, adjustment target.Adjustment, sibling target.SubedgeRef) target.SubedgeRouteSpec {
	return target.SubedgeRouteSpec{Kind: kind, Route: target.RouteSubedge, Adjustment: adjustment, Result: result, Placement: target.PlacementFixed, Subedge: sibling}
}

func continueRoute(kind flowkind.OutcomeKind, result target.ValuesSpec, adjustment target.Adjustment) target.SubedgeRouteSpec {
	return target.SubedgeRouteSpec{Kind: kind, Route: target.RouteContinue, Adjustment: adjustment, Result: result}
}

func propagateRoute(values target.ValuesSpec) target.SubedgeRouteSpec {
	return target.SubedgeRouteSpec{Kind: flowkind.OutcomeYield, Route: target.RoutePropagateYield, Adjustment: target.AdjustmentPreserve, Result: values}
}

func rejectRoute(outcome uint32) target.SubedgeRouteSpec {
	return target.SubedgeRouteSpec{Kind: flowkind.OutcomeYield, Route: target.RouteRejectYield, Adjustment: target.AdjustmentExact, Result: rejectedYield(), Placement: target.PlacementFixed, Outcome: outcome}
}

func rejectSiblingRoute(sibling target.SubedgeRef) target.SubedgeRouteSpec {
	return target.SubedgeRouteSpec{Kind: flowkind.OutcomeYield, Route: target.RouteRejectYield, Adjustment: target.AdjustmentExact, Result: rejectedYield(), Placement: target.PlacementFixed, Subedge: sibling}
}

func admissionToOutcome(result target.ValuesSpec, adjustment target.Adjustment, placement target.Placement, outcome uint32) target.AdmissionFailureSpec {
	return target.AdmissionFailureSpec{Values: anyValue(), Route: target.AdmissionRouteSpec{Route: target.RouteOutcome, Adjustment: adjustment, Result: result, Placement: placement, Outcome: outcome}}
}

func admissionToSibling(result target.ValuesSpec, sibling target.SubedgeRef) target.AdmissionFailureSpec {
	return target.AdmissionFailureSpec{Values: anyValue(), Route: target.AdmissionRouteSpec{Route: target.RouteSubedge, Adjustment: target.AdjustmentExact, Result: result, Placement: target.PlacementFixed, Subedge: sibling}}
}

func tailInputOrigin(variable target.ValuesVar) []target.ArgumentOrigin {
	return []target.ArgumentOrigin{{Segment: target.ArgumentTail, Kind: target.ArgumentSourceInput, Source: target.InputSource{Kind: target.InputSourceValuesVar, Ordinal: uint32(variable)}}}
}

func ruleOrigins(count int) []target.ArgumentOrigin {
	out := make([]target.ArgumentOrigin, count)
	for index := range out {
		out[index] = target.ArgumentOrigin{Segment: target.ArgumentFixed, Index: uint32(index), Kind: target.ArgumentSourceRule}
	}
	return out
}

func ruleTailOrigin() []target.ArgumentOrigin {
	return []target.ArgumentOrigin{{Segment: target.ArgumentTail, Kind: target.ArgumentSourceRule}}
}

func fixedInputOrigin(index uint32) target.ArgumentOrigin {
	return target.ArgumentOrigin{Segment: target.ArgumentFixed, Index: index, Kind: target.ArgumentSourceInput, Source: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: index}}
}

// ruleFamilyEdge declares a non-yieldable target-machine application whose
// operands and dependent result formula belong to the one owning operation
// Rule.  The Target row owns only the typed application boundary and complete
// terminal disposition.
func ruleFamilyEdge(role uint32, family target.SubedgeFamily, arguments target.ValuesSpec, throwOutcome, cancelOutcome uint32, cancel target.ValuesSpec) target.SubedgeSpec {
	return target.SubedgeSpec{
		Role: role, Family: family, Admission: target.OrdinaryCallable, Arguments: arguments,
		ArgumentOrigins:  ruleOrigins(len(arguments.Fixed)),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), cancel),
		AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, throwOutcome),
		Routes: []target.SubedgeRouteSpec{
			continueRoute(flowkind.OutcomeNormal, anyValue(), target.AdjustmentExact),
			continueRoute(flowkind.OutcomeReturn, anyValue(), target.AdjustmentExact),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentPreserve, target.PlacementFixed, throwOutcome),
			rejectRoute(throwOutcome),
			outcomeRoute(flowkind.OutcomeCancel, cancel, target.AdjustmentPreserve, target.PlacementTail, cancelOutcome),
		},
	}
}

func ruleMetaCallEdge(role uint32, key keyspace.LiteralValue, arguments target.ValuesSpec, throwOutcome, cancelOutcome uint32, cancel target.ValuesSpec) target.SubedgeSpec {
	return target.SubedgeSpec{
		Role: role, Family: target.SubedgeFamilyCall,
		Callee: target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeMetaKey, MetaKey: key}, Admission: target.OrdinaryCallable,
		Arguments: arguments, ArgumentOrigins: ruleOrigins(len(arguments.Fixed)),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), cancel),
		AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, throwOutcome),
		Routes: []target.SubedgeRouteSpec{
			continueRoute(flowkind.OutcomeNormal, anyValue(), target.AdjustmentExact),
			continueRoute(flowkind.OutcomeReturn, anyValue(), target.AdjustmentExact),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentPreserve, target.PlacementFixed, throwOutcome),
			rejectRoute(throwOutcome),
			outcomeRoute(flowkind.OutcomeCancel, cancel, target.AdjustmentPreserve, target.PlacementTail, cancelOutcome),
		},
	}
}

func literalKey(text string) keyspace.LiteralValue {
	return keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text}
}
func normal(op target.OperationSpec, in []typ.Type, openIn bool, out []typ.Type, openOut bool) target.OperationSpec {
	vars := uint32(0)
	inVar, outVar := target.ValuesVar(0), target.ValuesVar(0)
	if openIn {
		vars++
		inVar = 0
	}
	if openOut {
		outVar = target.ValuesVar(vars)
		vars++
	}
	op.ValuesVars = vars
	op.Input = values(in, openIn, inVar)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values(out, openOut, outVar)}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}
func fixed(op target.OperationSpec, in, out []typ.Type) target.OperationSpec {
	return normal(op, in, false, out, false)
}
func total(op target.OperationSpec, in []typ.Type, openIn bool, out []typ.Type, openOut bool) target.OperationSpec {
	vars := uint32(0)
	inVar, outVar := target.ValuesVar(0), target.ValuesVar(0)
	if openIn {
		vars++
		inVar = 0
	}
	if openOut {
		outVar = target.ValuesVar(vars)
		vars++
	}
	op.ValuesVars = vars
	op.Input = values(in, openIn, inVar)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values(out, openOut, outVar)}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}
func throws(op target.OperationSpec, in []typ.Type, open bool) target.OperationSpec {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}
func openSame(op target.OperationSpec) target.OperationSpec {
	op.ValuesVars = 1
	op.Input = values(nil, true, 0)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values(nil, true, 0)}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}
func alternatives(op target.OperationSpec, in []typ.Type, open bool, outputs [][]typ.Type) target.OperationSpec {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	for _, out := range outputs {
		op.Outcomes = append(op.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values(out, false, 0)})
	}
	op.Outcomes = append(op.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)})
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}
func alternativesTotal(op target.OperationSpec, in []typ.Type, open bool, outputs [][]typ.Type) target.OperationSpec {
	vars := uint32(0)
	if open {
		vars = 1
	}
	op.ValuesVars = vars
	op.Input = values(in, open, 0)
	for _, out := range outputs {
		op.Outcomes = append(op.Outcomes, target.OutcomeSpec{Kind: flowkind.OutcomeNormal, Values: values(out, false, 0)})
	}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}
func protected(op target.OperationSpec, fixed int) target.OperationSpec {
	if fixed == 1 {
		return pcallProfile(op)
	}
	return xpcallProfile(op)
}

func pcallProfile(op target.OperationSpec) target.OperationSpec {
	op.ValuesVars = 4
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Callbacks = []target.CallbackSpec{{
		Function: target.InputSource{Kind: target.InputSourceValueFormal}, Admission: target.OrdinaryCallable,
		Arguments: callbackTail(0), Outcomes: terminals(callbackTail(1), callbackTail(1), anyValue(), callbackTail(2), callbackTail(3)),
		Lifecycle: target.CallbackSyncRequiredOnce, Effects: target.RowSpec{Tail: target.RowClosed},
	}}
	op.Outcomes = []target.OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.LiteralBool(false), typ.Any)},
		{Kind: flowkind.OutcomeYield, Values: callbackTail(2)},
		{Kind: flowkind.OutcomeCancel, Values: callbackTail(3)},
	}
	op.Subedges = []target.SubedgeSpec{{
		Role: 1, Family: target.SubedgeFamilyCall, Callee: target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeCallback, Callback: 1},
		ArgumentOrigins:  tailInputOrigin(0),
		AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1),
		Routes: []target.SubedgeRouteSpec{
			outcomeRoute(flowkind.OutcomeNormal, callbackTail(1), target.AdjustmentPreserve, target.PlacementTail, 0),
			outcomeRoute(flowkind.OutcomeReturn, callbackTail(1), target.AdjustmentPreserve, target.PlacementTail, 0),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1),
			propagateRoute(callbackTail(2)),
			outcomeRoute(flowkind.OutcomeCancel, callbackTail(3), target.AdjustmentPreserve, target.PlacementTail, 3),
		},
	}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func xpcallProfile(op target.OperationSpec) target.OperationSpec {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any, typ.Any}, true, 0)
	op.Callbacks = []target.CallbackSpec{
		{Function: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0}, Admission: target.OrdinaryCallable, Arguments: callbackTail(0), Outcomes: terminals(callbackTail(1), callbackTail(1), anyValue(), callbackTail(2), callbackTail(3)), Lifecycle: target.CallbackSyncRequiredOnce, Effects: target.RowSpec{Tail: target.RowClosed}},
		{Function: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1}, Admission: target.DirectFunction, Arguments: anyValue(), Outcomes: terminals(callbackTail(4), callbackTail(4), anyValue(), anyValue(), callbackTail(3)), Lifecycle: target.CallbackSyncOptionalMany, Effects: target.RowSpec{Tail: target.RowClosed}},
	}
	op.Outcomes = []target.OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.LiteralBool(false), typ.Any)},
		{Kind: flowkind.OutcomeYield, Values: callbackTail(2)},
		{Kind: flowkind.OutcomeCancel, Values: callbackTail(3)},
		{Kind: flowkind.OutcomeThrow, Values: anyValue()},
	}
	op.Subedges = []target.SubedgeSpec{
		{
			Role: 1, Family: target.SubedgeFamilyCall, Callee: target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: tailInputOrigin(0),
			AdmissionFailure: admissionToSibling(anyValue(), 2),
			Routes: []target.SubedgeRouteSpec{
				outcomeRoute(flowkind.OutcomeNormal, callbackTail(1), target.AdjustmentPreserve, target.PlacementTail, 0),
				outcomeRoute(flowkind.OutcomeReturn, callbackTail(1), target.AdjustmentPreserve, target.PlacementTail, 0),
				siblingRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentExact, 2),
				propagateRoute(callbackTail(2)),
				outcomeRoute(flowkind.OutcomeCancel, callbackTail(3), target.AdjustmentPreserve, target.PlacementTail, 3),
			},
		},
		{
			Role: 2, Family: target.SubedgeFamilyCall, Callee: target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeCallback, Callback: 2},
			AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 4),
			Routes: []target.SubedgeRouteSpec{
				outcomeRoute(flowkind.OutcomeNormal, anyValue(), target.AdjustmentExact, target.PlacementFixed, 1),
				outcomeRoute(flowkind.OutcomeReturn, anyValue(), target.AdjustmentExact, target.PlacementFixed, 1),
				siblingRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentExact, 2),
				rejectSiblingRoute(2),
				outcomeRoute(flowkind.OutcomeCancel, callbackTail(3), target.AdjustmentPreserve, target.PlacementTail, 3),
			},
		},
	}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func printProfile() target.OperationSpec {
	op := builtin("print")
	op.ValuesVars = 2
	op.Input = callbackTail(0)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: emptyValues()}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(1)}}
	op.Subedges = []target.SubedgeSpec{{
		Role: 1, Family: target.SubedgeFamilyCall,
		Callee:    target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeCapturedInitialRead, Read: target.CapturedInitialReadSpec{Root: globalEnvRoot, Key: literalKey("tostring")}},
		Admission: target.OrdinaryCallable, Arguments: anyValue(), ArgumentOrigins: ruleOrigins(1),
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), callbackTail(1)),
		AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1),
		Routes: []target.SubedgeRouteSpec{
			continueRoute(flowkind.OutcomeNormal, anyValue(), target.AdjustmentExact),
			continueRoute(flowkind.OutcomeReturn, anyValue(), target.AdjustmentExact),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1),
			rejectRoute(1),
			outcomeRoute(flowkind.OutcomeCancel, callbackTail(1), target.AdjustmentPreserve, target.PlacementTail, 2),
		},
	}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func tostringProfile() target.OperationSpec {
	op := builtin("tostring")
	op.ValuesVars = 1
	op.Input = anyValue()
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed(typ.String)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(0)}}
	edge := ruleMetaCallEdge(1, literalKey("__tostring"), anyValue(), 1, 2, callbackTail(0))
	edge.ArgumentOrigins = []target.ArgumentOrigin{fixedInputOrigin(0)}
	op.Subedges = []target.SubedgeSpec{edge}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func formatProfile() target.OperationSpec {
	op := module("string", "format")
	op.ValuesVars = 2
	op.Input = values([]typ.Type{typ.String}, true, 0)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed(typ.String)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(1)}}
	op.Subedges = []target.SubedgeSpec{ruleMetaCallEdge(1, literalKey("__tostring"), anyValue(), 1, 2, callbackTail(1))}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func pairsProfile() target.OperationSpec {
	op := builtin("pairs")
	op.ValuesVars = 1
	op.Input = anyValue()
	// The meta-hook and raw fallback are separate ordinary outcomes.  A hook
	// controls every returned slot; only the fallback proves next/input/nil.
	metaThree := closed(typ.Any, typ.Any, typ.Any)
	fallbackThree := closed(typ.Any, typ.Any, typ.Nil)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: metaThree}, {Kind: flowkind.OutcomeNormal, Values: fallbackThree}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(0)}}
	op.Subedges = []target.SubedgeSpec{{
		Role: 1, Family: target.SubedgeFamilyCall, Callee: target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeMetaKey, MetaKey: literalKey("__pairs")},
		Admission: target.OrdinaryCallable, Arguments: anyValue(), ArgumentOrigins: []target.ArgumentOrigin{{Segment: target.ArgumentFixed, Kind: target.ArgumentSourceInput, Source: target.InputSource{Kind: target.InputSourceValueFormal}}},
		Outcomes:         terminals(anyValue(), anyValue(), anyValue(), anyValue(), callbackTail(0)),
		AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 2),
		Routes: []target.SubedgeRouteSpec{
			outcomeRoute(flowkind.OutcomeNormal, metaThree, target.AdjustmentExact, target.PlacementFixed, 0),
			outcomeRoute(flowkind.OutcomeReturn, metaThree, target.AdjustmentExact, target.PlacementFixed, 0),
			outcomeRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 2),
			rejectRoute(2),
			outcomeRoute(flowkind.OutcomeCancel, callbackTail(0), target.AdjustmentPreserve, target.PlacementTail, 3),
		},
	}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func ipairsProfile() target.OperationSpec {
	op := fixed(builtin("ipairs"), []typ.Type{typ.Any}, []typ.Type{typ.Any, typ.Any, typ.Integer})
	return op
}

func ipairsAuxProfile() target.OperationSpec {
	op := target.OperationSpec{ValuesVars: 1, Input: closed(typ.Any, typ.Integer), Outcomes: []target.OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.Nil)},
		{Kind: flowkind.OutcomeNormal, Values: closed(typ.Integer, typ.Any)},
		{Kind: flowkind.OutcomeThrow, Values: anyValue()},
		{Kind: flowkind.OutcomeCancel, Values: callbackTail(0)},
	}, Effects: target.RowSpec{Tail: target.RowClosed}}
	edge := ruleFamilyEdge(1, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 2, 3, callbackTail(0))
	edge.ArgumentOrigins = []target.ArgumentOrigin{fixedInputOrigin(0), {Segment: target.ArgumentFixed, Index: 1, Kind: target.ArgumentSourceRule}}
	op.Subedges = []target.SubedgeSpec{edge}
	return op
}

func tableOperation(name string, input []typ.Type, open bool, output target.ValuesSpec) target.OperationSpec {
	op := module("table", name)
	op.ValuesVars = 2
	op.Input = values(input, open, 0)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: output}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(1)}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func tableConcatProfile() target.OperationSpec {
	op := tableOperation("concat", []typ.Type{typ.Any}, true, closed(typ.String))
	op.Subedges = []target.SubedgeSpec{
		ruleFamilyEdge(1, target.SubedgeFamilyLength, anyValue(), 1, 2, callbackTail(1)),
		ruleFamilyEdge(2, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, callbackTail(1)),
	}
	return op
}

func ruleDiscardFamilyEdge(role uint32, family target.SubedgeFamily, arguments target.ValuesSpec, throwOutcome, cancelOutcome uint32, cancel target.ValuesSpec) target.SubedgeSpec {
	edge := ruleFamilyEdge(role, family, arguments, throwOutcome, cancelOutcome, cancel)
	edge.Routes[0] = continueRoute(flowkind.OutcomeNormal, emptyValues(), target.AdjustmentExact)
	edge.Routes[1] = continueRoute(flowkind.OutcomeReturn, emptyValues(), target.AdjustmentExact)
	return edge
}

func tableInsertProfile() target.OperationSpec {
	op := tableOperation("insert", []typ.Type{typ.Any}, true, emptyValues())
	op.Subedges = []target.SubedgeSpec{
		ruleFamilyEdge(1, target.SubedgeFamilyLength, anyValue(), 1, 2, callbackTail(1)),
		ruleFamilyEdge(2, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, callbackTail(1)),
		ruleDiscardFamilyEdge(3, target.SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, callbackTail(1)),
		ruleDiscardFamilyEdge(4, target.SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, callbackTail(1)),
	}
	return op
}

func tableRemoveProfile() target.OperationSpec {
	op := tableOperation("remove", []typ.Type{typ.Any}, true, anyValue())
	op.Subedges = []target.SubedgeSpec{
		ruleFamilyEdge(1, target.SubedgeFamilyLength, anyValue(), 1, 2, callbackTail(1)),
		ruleFamilyEdge(2, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, callbackTail(1)),
		ruleFamilyEdge(3, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, callbackTail(1)),
		ruleDiscardFamilyEdge(4, target.SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, callbackTail(1)),
		ruleDiscardFamilyEdge(5, target.SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Nil), 1, 2, callbackTail(1)),
	}
	return op
}

func tableMoveProfile() target.OperationSpec {
	op := tableOperation("move", []typ.Type{typ.Any, typ.Integer, typ.Integer, typ.Integer}, true, anyValue())
	op.Subedges = []target.SubedgeSpec{
		ruleFamilyEdge(1, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, callbackTail(1)),
		ruleDiscardFamilyEdge(2, target.SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, callbackTail(1)),
		ruleFamilyEdge(3, target.SubedgeFamilyEqual, closed(typ.Any, typ.Any), 1, 2, callbackTail(1)),
	}
	return op
}

func tableUnpackProfile() target.OperationSpec {
	op := target.OperationSpec{Bindings: []target.BindingSpec{{Namespace: target.BindingBuiltin, Member: []string{"unpack"}}, {Namespace: target.BindingModule, Owner: []string{"table"}, Member: []string{"unpack"}}}, ValuesVars: 3,
		Input: values([]typ.Type{typ.Any}, true, 0), Outcomes: []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: callbackTail(1)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(2)}}, Effects: target.RowSpec{Tail: target.RowClosed}}
	op.Subedges = []target.SubedgeSpec{
		ruleFamilyEdge(1, target.SubedgeFamilyLength, anyValue(), 1, 2, callbackTail(2)),
		ruleFamilyEdge(2, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, callbackTail(2)),
	}
	return op
}

func tableSortProfile() target.OperationSpec {
	op := module("table", "sort")
	op.ValuesVars = 1
	op.Input = closed(typ.Any, typ.Any)
	op.Callbacks = []target.CallbackSpec{{Function: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 1}, Admission: target.DirectFunction, Arguments: closed(typ.Any, typ.Any), Outcomes: terminals(callbackTail(0), callbackTail(0), anyValue(), anyValue(), callbackTail(0)), Lifecycle: target.CallbackSyncOptionalMany, Effects: target.RowSpec{Tail: target.RowClosed}}}
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: emptyValues()}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(0)}}
	comparator := target.SubedgeSpec{Role: 3, Family: target.SubedgeFamilyCall, Callee: target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: ruleOrigins(2), AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1), Routes: []target.SubedgeRouteSpec{
		continueRoute(flowkind.OutcomeNormal, anyValue(), target.AdjustmentExact), continueRoute(flowkind.OutcomeReturn, anyValue(), target.AdjustmentExact), outcomeRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1), rejectRoute(1), outcomeRoute(flowkind.OutcomeCancel, callbackTail(0), target.AdjustmentPreserve, target.PlacementTail, 2),
	}}
	op.Subedges = []target.SubedgeSpec{
		ruleFamilyEdge(1, target.SubedgeFamilyLength, anyValue(), 1, 2, callbackTail(0)),
		ruleFamilyEdge(2, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Integer), 1, 2, callbackTail(0)),
		comparator,
		ruleFamilyEdge(4, target.SubedgeFamilyLess, closed(typ.Any, typ.Any), 1, 2, callbackTail(0)),
		ruleDiscardFamilyEdge(5, target.SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, callbackTail(0)),
		ruleDiscardFamilyEdge(6, target.SubedgeFamilyIndexSet, closed(typ.Any, typ.Integer, typ.Any), 1, 2, callbackTail(0)),
	}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func callbackGsubProfile() target.OperationSpec {
	op := module("string", "gsub")
	op.ValuesVars = 4
	op.Input = values([]typ.Type{typ.String, typ.String, typ.Any}, true, 0)
	op.Callbacks = []target.CallbackSpec{{Function: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 2}, Admission: target.DirectFunction, Arguments: callbackTail(1), Outcomes: terminals(callbackTail(2), callbackTail(2), anyValue(), anyValue(), callbackTail(3)), Lifecycle: target.CallbackSyncOptionalMany, Effects: target.RowSpec{Tail: target.RowClosed}}}
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: closed(typ.String, typ.Integer)}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(3)}}
	function := target.SubedgeSpec{Role: 1, Family: target.SubedgeFamilyCall, Callee: target.SubedgeCalleeSpec{Kind: target.SubedgeCalleeCallback, Callback: 1}, ArgumentOrigins: ruleTailOrigin(), AdmissionFailure: admissionToOutcome(anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1), Routes: []target.SubedgeRouteSpec{
		continueRoute(flowkind.OutcomeNormal, anyValue(), target.AdjustmentExact), continueRoute(flowkind.OutcomeReturn, anyValue(), target.AdjustmentExact), outcomeRoute(flowkind.OutcomeThrow, anyValue(), target.AdjustmentPreserve, target.PlacementFixed, 1), rejectRoute(1), outcomeRoute(flowkind.OutcomeCancel, callbackTail(3), target.AdjustmentPreserve, target.PlacementTail, 2),
	}}
	table := ruleFamilyEdge(2, target.SubedgeFamilyIndexGet, closed(typ.Any, typ.Any), 1, 2, callbackTail(3))
	// The table branch is distinct from the function callback: gsub indexes
	// the replacement table with the first capture or whole match, supplied by
	// its own closed Rule coordinate rather than an invented callback input.
	table.ArgumentOrigins = []target.ArgumentOrigin{{Segment: target.ArgumentFixed, Index: 0, Kind: target.ArgumentSourceInput, Source: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 2}}, {Segment: target.ArgumentFixed, Index: 1, Kind: target.ArgumentSourceRule}}
	op.Subedges = []target.SubedgeSpec{function, table}
	op.GsubTableReplacement = &target.GsubTableReplacementSpec{Replacement: 2, Access: 2, ResultOutcome: 0, Result: 0, EffectAliases: []uint32{0}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func minMaxProfile(op target.OperationSpec) target.OperationSpec {
	op.ValuesVars = 2
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: anyValue()}, {Kind: flowkind.OutcomeThrow, Values: anyValue()}, {Kind: flowkind.OutcomeCancel, Values: callbackTail(1)}}
	op.Subedges = []target.SubedgeSpec{ruleFamilyEdge(1, target.SubedgeFamilyLess, closed(typ.Any, typ.Any), 1, 2, callbackTail(1))}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func resumeEnvelope() target.OperationSpec {
	op := module("coroutine", "resume")
	op.ValuesVars = 3
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Outcomes = []target.OutcomeSpec{
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(true)}, true, 1)},
		{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.LiteralBool(false)}, true, 2)},
	}
	op.Resumes = []target.ResumeSpec{resumeRelation(target.ResumeSourceValueFormal, 0, 0, 0, 1)}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

// resumeRelation is the complete activation-boundary correspondence shared by
// coroutine.resume and a produced coroutine.wrap invocation. Successful
// return and yield are ordinary successful results; restored throw/cancel
// select the operation's failure outcome. The carrier's resumption arguments
// are exactly the operation's incoming open Values variable.
func resumeRelation(source target.ResumeSource, carrier target.ValueFormal, arguments target.ValuesVar, success, failure uint32) target.ResumeSpec {
	return target.ResumeSpec{
		Source: source, Carrier: carrier, Arguments: callbackTail(arguments),
		Outcomes: []target.ResumeOutcomeSpec{
			{Kind: flowkind.OutcomeNormal, Outcome: success},
			{Kind: flowkind.OutcomeReturn, Outcome: success},
			{Kind: flowkind.OutcomeThrow, Outcome: failure},
			{Kind: flowkind.OutcomeYield, Outcome: success},
			{Kind: flowkind.OutcomeCancel, Outcome: failure},
		},
	}
}

func callbackCreate(op target.OperationSpec) target.OperationSpec {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any}, false, 0)
	op.Callbacks = []target.CallbackSpec{{Function: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0}, Admission: target.DirectFunction, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: target.CallbackRetainedOptionalOnce, Effects: target.RowSpec{Tail: target.RowClosed}}}
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.Any}, false, 0), CallbackResults: []target.CallbackResultSpec{{Result: 0, Callback: 1}}}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func callbackWrap(op target.OperationSpec) target.OperationSpec {
	op.ValuesVars = 5
	op.Input = values([]typ.Type{typ.Any}, false, 0)
	op.Callbacks = []target.CallbackSpec{{Function: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0}, Admission: target.DirectFunction, Arguments: callbackTail(0), Outcomes: callbackOutcomes(1, 1, 2, 3, 4), Lifecycle: target.CallbackRetainedOptionalOnce, Effects: target.RowSpec{Tail: target.RowClosed}}}
	op.Outcomes = []target.OutcomeSpec{{Kind: flowkind.OutcomeNormal, Values: values([]typ.Type{typ.Any}, false, 0)}, {Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

// callbackSpawn is a detached, one-shot callback activation.  Its parent
// system-yield and later empty resume share the typed SpawnSpec; the callback
// keeps the exact function/closure authority and its complete outcome rows.
func callbackSpawn() target.OperationSpec {
	op := module("coroutine", "spawn")
	op.ValuesVars = 7 // input tail, empty child entry coordinate, five child outcomes
	op.Input = values([]typ.Type{typ.Any}, true, 0)
	op.Callbacks = []target.CallbackSpec{{
		Function:  target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0},
		Admission: target.DirectFunction, Arguments: callbackTail(1), Outcomes: callbackOutcomes(2, 3, 4, 5, 6),
		Lifecycle: target.CallbackRetainedRequiredOnce, Effects: target.RowSpec{Tail: target.RowClosed},
	}}
	op.Outcomes = []target.OutcomeSpec{
		{Kind: flowkind.OutcomeYield, Values: values(nil, false, 0)},
		{Kind: flowkind.OutcomeNormal, Values: values(nil, false, 0)},
		{Kind: flowkind.OutcomeThrow, Values: values([]typ.Type{typ.Any}, false, 0)},
	}
	op.Suspensions = []target.SuspensionSpec{{Yield: 0, Reentry: 1, Source: target.ReentryByProvider, Multiplicity: target.ReentryOnce}}
	op.Spawns = []target.SpawnSpec{{
		Function: target.InputSource{Kind: target.InputSourceValueFormal, Ordinal: 0}, Child: 1,
		Yield: 0, ParentResume: 1, ChildEntry: 1,
		Alternatives: []target.SpawnSiblingAlternative{target.SpawnChildEntryThenParentResume, target.SpawnParentResumeThenChildEntry},
	}}
	op.Effects = target.RowSpec{Tail: target.RowClosed}
	return op
}

func callbackTail(variable target.ValuesVar) target.ValuesSpec {
	return values(nil, true, variable)
}

func callbackOutcomes(normal, returned, thrown, yielded, canceled target.ValuesVar) []target.TerminalSpec {
	return []target.TerminalSpec{
		{Kind: flowkind.OutcomeNormal, Values: callbackTail(normal)},
		{Kind: flowkind.OutcomeReturn, Values: callbackTail(returned)},
		{Kind: flowkind.OutcomeThrow, Values: callbackTail(thrown)},
		{Kind: flowkind.OutcomeYield, Values: callbackTail(yielded)},
		{Kind: flowkind.OutcomeCancel, Values: callbackTail(canceled)},
	}
}

// selfEffects is the closed, self-labelled Koka inventory justified by the
// target surface.  The occurrence carries the complete existing input
// coordinate correspondence; it does not introduce an effect-kind vocabulary.
func (catalogue *authoredCatalogue) selfEffects() error {
	for _, name := range []string{
		"getmetatable", "setmetatable", "next", "print", "rawget", "rawlen", "rawset", "unpack", "require",
		"table.getn", "table.maxn", "table.concat", "unpack", "table.insert", "table.move", "table.pack", "table.remove", "table.sort", "table.freeze", "table.isfrozen",
		"string.gsub", "string.gmatch.next", "math.random", "math.randomseed",
		"coroutine.running", "coroutine.status", "debug.getupvalue", "errors.new", "errors.wrap", "ipairs.aux",
	} {
		ref, err := catalogue.require(name)
		if err != nil {
			return err
		}
		op := catalogue.at(ref)
		values := make([]target.ValueFormal, len(op.Input.Fixed))
		for i := range values {
			values[i] = target.ValueFormal(i)
		}
		vars := make([]target.ValuesVar, op.ValuesVars)
		for i := range vars {
			vars[i] = target.ValuesVar(i)
		}
		op.Effects = target.RowSpec{Occurrences: []target.EffectSpec{{Target: target.SpecRef(ref), ValueArgs: values, ValuesArgs: vars}}, Tail: target.RowClosed}
	}
	return nil
}

var excluded = []target.BindingSpec{
	{Namespace: target.BindingBuiltin, Member: []string{"load"}}, {Namespace: target.BindingBuiltin, Member: []string{"loadfile"}}, {Namespace: target.BindingBuiltin, Member: []string{"dofile"}}, {Namespace: target.BindingBuiltin, Member: []string{"collectgarbage"}},
	{Namespace: target.BindingModule, Owner: []string{"package"}, Member: []string{"loadlib"}}, {Namespace: target.BindingModule, Owner: []string{"package"}, Member: []string{"seeall"}}, {Namespace: target.BindingModule, Owner: []string{"package"}, Member: []string{"searchpath"}},
	{Namespace: target.BindingModule, Owner: []string{"string"}, Member: []string{"dump"}},
	{Namespace: target.BindingModule, Owner: []string{"debug"}, Member: []string{"getinfo"}}, {Namespace: target.BindingModule, Owner: []string{"debug"}, Member: []string{"getlocal"}}, {Namespace: target.BindingModule, Owner: []string{"debug"}, Member: []string{"traceback"}},
	{Namespace: target.BindingModule, Owner: []string{"debug"}, Member: []string{"setlocal"}}, {Namespace: target.BindingModule, Owner: []string{"debug"}, Member: []string{"setupvalue"}}, {Namespace: target.BindingModule, Owner: []string{"debug"}, Member: []string{"setmetatable"}}, {Namespace: target.BindingModule, Owner: []string{"debug"}, Member: []string{"getmetatable"}},
	{Namespace: target.BindingModule, Owner: []string{"errors"}, Member: []string{"call_stack"}}, {Namespace: target.BindingModule, Owner: []string{"errors"}, Member: []string{"Error", "stack"}},
	{Namespace: target.BindingModule, Owner: []string{"coroutine"}, Member: []string{"close"}}, {Namespace: target.BindingModule, Owner: []string{"coroutine"}, Member: []string{"isyieldable"}},
	{Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"close"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"flush"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"input"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"lines"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"open"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"output"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"popen"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"read"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"tmpfile"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"type"}}, {Namespace: target.BindingModule, Owner: []string{"io"}, Member: []string{"write"}},
	{Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"clock"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"date"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"difftime"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"execute"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"exit"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"getenv"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"remove"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"rename"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"setlocale"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"time"}}, {Namespace: target.BindingModule, Owner: []string{"os"}, Member: []string{"tmpname"}},
	{Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"arshift"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"band"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"bnot"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"bor"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"btest"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"bxor"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"extract"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"lrotate"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"lshift"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"replace"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"rrotate"}}, {Namespace: target.BindingModule, Owner: []string{"bit32"}, Member: []string{"rshift"}},
}

// Validate checks the profile's own closed inventory before sealing it.
func Validate() error {
	seen := make(map[string]struct{})
	for _, binding := range Admitted() {
		key := fmt.Sprintf("%d/%q/%q", binding.Namespace, binding.Owner, binding.Member)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("profile: duplicate admitted binding %#v", binding)
		}
		seen[key] = struct{}{}
	}
	contract, err := Contract()
	if err != nil {
		return err
	}
	for _, binding := range Admitted() {
		if _, ok := contract.Lookup(binding); !ok {
			return fmt.Errorf("profile: admitted binding missing: %#v", binding)
		}
	}
	for _, binding := range Excluded() {
		if _, ok := contract.Lookup(binding); ok {
			return fmt.Errorf("profile: excluded binding admitted: %#v", binding)
		}
	}
	return nil
}
