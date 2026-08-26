package testfixture

import (
	"sync"

	"github.com/wippyai/go-lua/analysis/program/target/contract"
	"github.com/wippyai/go-lua/domain/composite/manifesttarget"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/dispatch"
	"github.com/wippyai/go-lua/domain/effect/lifecycle"
	"github.com/wippyai/go-lua/domain/effect/ownership"
	"github.com/wippyai/go-lua/domain/effect/postcondition"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/ambient"
	"github.com/wippyai/go-lua/domain/type/channelselect"
	typetable "github.com/wippyai/go-lua/domain/type/table"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
	"github.com/wippyai/go-lua/domain/typestate"
	"github.com/wippyai/go-lua/internal/testfixture/wippyv1"
	"github.com/wippyai/go-lua/manifest"
	manifestwire "github.com/wippyai/go-lua/manifest/wire"
	"github.com/wippyai/go-lua/stdlib"
	"github.com/wippyai/go-lua/types/signature"
)

// StandardLibraryTarget seals the native provider manifests through the one
// domain-composition entry point. It is test scaffolding, not a runtime
// registry.
//
// The seal is one value per process. Its whole input is the compiled-in
// provider set, so every caller asks the same question, and a sealed Contract
// is immutable: sealing it again re-derives byte-identical rows, identities and
// canonical bytes for each caller that would otherwise have shared the first.
//
// The provider set has two halves. The wippyv1 package carries the reference
// half: the host modules the v1 runtime declares in production, transcribed
// from the manifests it ships. The declarations below carry the preview half:
// surfaces the runtime has no manifest for, which exist to state a contract the
// corpus measures. A module path belongs to exactly one half. Where the
// reference declares a path, the preview half declares nothing for it, because
// two declarations of one boundary are two authorities for one answer.
func StandardLibraryTarget() (*contract.Contract, error) {
	return standardLibraryTarget()
}

var standardLibraryTarget = sync.OnceValues(sealStandardLibraryTarget)

func sealStandardLibraryTarget() (*contract.Contract, error) {
	providers := stdlib.Providers()
	providers = append(providers, wippyv1.Providers()...)
	providers = append(providers, manifest.Provider{
		Identity:    "testfixture.wippy.host",
		Mount:       manifest.MountGlobals,
		Declaration: wippyHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.channel",
		Mount:       manifest.MountModule,
		Declaration: channelHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.uuid",
		Mount:       manifest.MountModule,
		Declaration: uuidHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.stream",
		Mount:       manifest.MountModule,
		Declaration: streamHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.time",
		Mount:       manifest.MountModule,
		Declaration: timeHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.assert2",
		Mount:       manifest.MountModule,
		Declaration: assert2HostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.resource",
		Mount:       manifest.MountModule,
		Declaration: resourceHostManifest,
	}, manifest.Provider{
		Identity:    "testfixture.wippy.ownership",
		Mount:       manifest.MountModule,
		Declaration: ownershipHostManifest,
	})
	catalogue, err := manifest.Seal(providers...)
	if err != nil {
		return nil, err
	}
	return manifesttarget.SealCatalogue(catalogue)
}

func wippyHostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New("wippy.host")
	functionType := typ.Func().Param("module", typ.String).Returns(typ.Any).Build()
	declaration.DefineFunctionSignature("require", signature.Function{
		Type:   functionType,
		Effect: effect.Empty.With(dispatch.ModuleLoad{}),
	})
	declaration.DefineGlobalType("require", functionType)
	return declaration
}

func channelHostManifest() *manifestwire.Manifest {
	selectType := channelselect.SelectFunction()
	channelType := typ.Instantiate(ambient.ChannelGeneric(), typ.Any)
	newType := typ.Func().OptParam("buffer", typ.Integer).Returns(channelType).Build()
	declaration := manifestwire.New(channelselect.ModuleName)
	declaration.DefineFunctionSignature("select", signature.Function{Type: selectType})
	declaration.DefineFunctionSignature("new", signature.Function{Type: newType})
	declaration.SetExport(typetable.NewRecord().
		Field("select", selectType).
		Field("new", newType).
		Build())
	return declaration
}

// uuidHostManifest declares the identifier generator surface the corpus
// fixtures require. Only v7 is declared, because that is the only member the
// fixture corpus calls; a generated identifier is a plain string value and the
// call carries no ownership, dispatch, or transfer effect.
func uuidHostManifest() *manifestwire.Manifest {
	v7Type := typ.Func().Returns(typ.String).Build()
	declaration := manifestwire.New("uuid")
	declaration.DefineFunctionSignature("v7", signature.Function{Type: v7Type})
	declaration.SetExport(typetable.NewRecord().
		Field("v7", v7Type).
		Build())
	return declaration
}

// streamHostManifest declares the historical host-style stream package. The
// package installs a stream global while exposing stream.Stream for qualified
// annotations; open is also registered as a callable declaration so the
// manifesttarget compiler can publish its normal operation and result type.
func streamHostManifest() *manifestwire.Manifest {
	streamType := typetable.NewRecord().
		Field("id", typ.String).
		Build()
	openType := typ.Func().
		Param("name", typ.String).
		Returns(streamType).
		Build()
	moduleType := typetable.NewRecord().
		Field("open", openType).
		Build()

	declaration := manifestwire.New("stream")
	declaration.DefineType("Stream", streamType)
	declaration.DefineFunctionSignature("open", signature.Function{Type: openType})
	declaration.SetExport(moduleType)
	return declaration
}

// timeHostManifest declares the clock surface the corpus fixtures require.
// It transcribes the Wippy v1 production time module
// (runtime/lua/modules/time/types.go): the same declared object types, the same
// members, and the same duration-like argument admission. Two facts are stated
// more exactly than v1's pre-cut declaration could state them. A duration
// argument admits the three runtime forms the v1 coercion accepts rather than
// any value, and a fallible member's nil answer is its own normal outcome arm,
// so a caller reads the nil the module actually answers instead of a
// non-nilness proof it never gave.
func timeHostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New("time")
	declaration.ErrorType = wippyv1.ErrorType()
	optionalError := typeexpr.Optional(wippyv1.ErrorType())

	durationType, durationMethods := wippyv1.DeclaredObject("time.Duration", func(self typ.Type) []typ.Method {
		unit := func(name string) typ.Method {
			return typ.Method{Name: name, Type: typ.Func().Param("self", self).Returns(typ.Number).Build()}
		}
		return []typ.Method{
			unit("nanoseconds"), unit("microseconds"), unit("milliseconds"),
			unit("seconds"), unit("minutes"), unit("hours"),
		}
	})
	locationType, locationMethods := wippyv1.DeclaredObject("time.Location", func(self typ.Type) []typ.Method {
		return []typ.Method{{Name: "string", Type: typ.Func().Param("self", self).Returns(typ.String).Build()}}
	})
	// A duration argument is a Duration handle, a Go duration string, or a
	// nanosecond count: exactly the three forms parseDurationValue admits.
	durationArgument := typ.MaterializeUnion([]typ.Type{durationType, typ.String, typ.Number})

	timeType, timeMethods := wippyv1.DeclaredObject("time.Time", func(self typ.Type) []typ.Method {
		reads := func(name string, result typ.Type) typ.Method {
			return typ.Method{Name: name, Type: typ.Func().Param("self", self).Returns(result).Build()}
		}
		compares := func(name string) typ.Method {
			return typ.Method{Name: name, Type: typ.Func().Param("self", self).Param("t", self).Returns(typ.Boolean).Build()}
		}
		rounds := func(name string) typ.Method {
			return typ.Method{Name: name, Type: typ.Func().Param("self", self).Param("d", durationType).Returns(self).Build()}
		}
		return []typ.Method{
			{Name: "add", Type: typ.Func().Param("self", self).Param("d", durationArgument).Returns(self).Build()},
			{Name: "sub", Type: typ.Func().Param("self", self).Param("t", self).Returns(durationType).Build()},
			{Name: "add_date", Type: typ.Func().Param("self", self).
				Param("year", typ.Number).Param("month", typ.Number).Param("day", typ.Number).Returns(self).Build()},
			compares("after"), compares("before"), compares("equal"),
			{Name: "format", Type: typ.Func().Param("self", self).Param("format", typ.String).Returns(typ.String).Build()},
			reads("format_rfc3339", typ.String),
			reads("unix", typ.Integer),
			reads("unix_nano", typ.Integer),
			{Name: "date", Type: typ.Func().Param("self", self).Returns(typ.Integer, typ.Integer, typ.Integer).Build()},
			{Name: "clock", Type: typ.Func().Param("self", self).Returns(typ.Integer, typ.Integer, typ.Integer).Build()},
			reads("year", typ.Integer), reads("month", typ.Integer), reads("day", typ.Integer),
			reads("hour", typ.Integer), reads("minute", typ.Integer), reads("second", typ.Integer),
			reads("nanosecond", typ.Integer), reads("weekday", typ.Integer), reads("year_day", typ.Integer),
			reads("is_zero", typ.Boolean),
			{Name: "in_location", Type: typ.Func().Param("self", self).Param("loc", locationType).Returns(self).Build()},
			reads("location", locationType),
			reads("utc", self), reads("in_local", self),
			rounds("round"), rounds("truncate"),
		}
	})
	// A tick is delivered as the instant it fired, so the delivery channel
	// carries Time rather than an unnamed payload.
	channelType := typ.Instantiate(ambient.ChannelGeneric(), timeType)

	tickerType, tickerMethods := wippyv1.DeclaredObject("time.Ticker", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "stop", Type: typ.Func().Param("self", self).Returns(typ.Boolean).Build()},
			{Name: "response", Type: typ.Func().Param("self", self).Returns(channelType).Build()},
			{Name: "channel", Type: typ.Func().Param("self", self).Returns(channelType).Build()},
		}
	})
	timerType, timerMethods := wippyv1.DeclaredObject("time.Timer", func(self typ.Type) []typ.Method {
		return []typ.Method{
			{Name: "stop", Type: typ.Func().Param("self", self).Returns(typ.Boolean).Build()},
			{Name: "reset", Type: typ.Func().Param("self", self).Param("d", durationArgument).Returns(typ.Boolean).Build()},
			{Name: "response", Type: typ.Func().Param("self", self).Returns(channelType).Build()},
			{Name: "channel", Type: typ.Func().Param("self", self).Returns(channelType).Build()},
		}
	})

	declaration.DefineType("Time", timeType)
	declaration.DefineType("Duration", durationType)
	declaration.DefineType("Location", locationType)
	declaration.DefineType("Ticker", tickerType)
	declaration.DefineType("Timer", timerType)
	wippyv1.DefineMethods(declaration, "Time", timeMethods)
	wippyv1.DefineMethods(declaration, "Duration", durationMethods)
	wippyv1.DefineMethods(declaration, "Location", locationMethods)
	wippyv1.DefineMethods(declaration, "Ticker", tickerMethods)
	wippyv1.DefineMethods(declaration, "Timer", timerMethods)

	sleepType := typ.Func().Param("d", durationArgument).Build()
	timerFuncType := typ.Func().Param("d", durationArgument).Returns(typeexpr.Optional(timerType), optionalError).Build()
	afterType := typ.Func().Param("d", durationArgument).Returns(channelType).Build()
	tickerFuncType := typ.Func().Param("d", durationArgument).Returns(typeexpr.Optional(tickerType), optionalError).Build()
	nowType := typ.Func().Returns(timeType).Build()
	dateType := typ.Func().
		Param("year", typ.Number).Param("month", typ.Number).Param("day", typ.Number).
		Param("hour", typ.Number).Param("min", typ.Number).Param("sec", typ.Number).Param("nsec", typ.Number).
		OptParam("loc", locationType).Returns(timeType).Build()
	unixType := typ.Func().Param("sec", typ.Number).Param("nsec", typ.Number).Returns(timeType).Build()
	parseType := typ.Func().Param("layout", typ.String).Param("value", typ.String).OptParam("loc", locationType).
		Returns(typeexpr.Optional(timeType), optionalError).Build()
	parseDurationType := typ.Func().Param("s", durationArgument).
		Returns(typeexpr.Optional(durationType), optionalError).Build()
	loadLocationType := typ.Func().Param("name", typ.String).
		Returns(typeexpr.Optional(locationType), optionalError).Build()
	fixedZoneType := typ.Func().Param("name", typ.String).Param("offset", typ.Number).Returns(locationType).Build()

	for name, member := range map[string]*typ.Function{
		"sleep": sleepType, "timer": timerFuncType, "after": afterType, "ticker": tickerFuncType,
		"now": nowType, "date": dateType, "unix": unixType, "parse": parseType,
		"parse_duration": parseDurationType, "load_location": loadLocationType, "fixed_zone": fixedZoneType,
	} {
		declaration.DefineFunctionSignature(name, signature.Function{Type: member})
	}
	for member, value := range map[string]typ.Type{
		"timer": timerType, "ticker": tickerType, "parse": timeType,
		"parse_duration": durationType, "load_location": locationType,
	} {
		declaration.DefineFunctionOperation(member, hostValueOrErrorOutcomes(value, wippyv1.ErrorType()))
	}

	export := typetable.NewRecord()
	for _, name := range []string{
		"NANOSECOND", "MICROSECOND", "MILLISECOND", "SECOND", "MINUTE", "HOUR",
		"JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY",
		"AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER",
		"SUNDAY", "MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY",
	} {
		export = export.Field(name, typ.Number)
	}
	for _, name := range []string{
		"RFC3339", "RFC3339NANO", "RFC822", "RFC822Z", "RFC850", "RFC1123", "RFC1123Z",
		"KITCHEN", "STAMP", "STAMP_MILLI", "STAMP_MICRO", "STAMP_NANO",
		"DATE_TIME", "DATE_ONLY", "TIME_ONLY",
	} {
		export = export.Field(name, typ.String)
	}
	declaration.SetExport(export.
		Field("utc", locationType).
		Field("localtz", locationType).
		Field("sleep", sleepType).
		Field("timer", timerFuncType).
		Field("after", afterType).
		Field("ticker", tickerFuncType).
		Field("now", nowType).
		Field("date", dateType).
		Field("unix", unixType).
		Field("parse", parseType).
		Field("parse_duration", parseDurationType).
		Field("load_location", loadLocationType).
		Field("fixed_zone", fixedZoneType).
		Build())
	return declaration
}

// hostValueOrErrorOutcomes states the two normal arms of a fallible host
// member: the success arm answers the value with no error, and the failure arm
// answers nil with the module's error. The signature-derived throw arm is left
// untouched, so a member that also raises keeps its raise.
func hostValueOrErrorOutcomes(value, failure typ.Type) manifestwire.Operation {
	return manifestwire.Operation{
		ReplaceNormalSet: true,
		ReplaceNormal: []manifestwire.Values{
			{Fixed: []typ.Type{value, typ.Nil}, Tail: manifestwire.ValuesClosed},
			{Fixed: []typ.Type{typ.Nil, failure}, Tail: manifestwire.ValuesClosed},
		},
	}
}

// assert2HostManifest declares the assertion surface the corpus fixtures
// require under require("assert2"). Wippy v1 ships it as a Lua library
// (tests/app/src/lib/assert.lua) that a process binds under the assert2 alias
// rather than as a native module, so the fixture Target states the same members
// as a host declaration. Every member raises on a failed assertion, and that
// raise is the signature-derived throw arm; the members that answer their
// subject answer it refined, because ruling the other case out is exactly what
// the assertion did.
func assert2HostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New("assert2")

	// A refuted subject is returned unchanged and is present on every path
	// that returns at all, so the refinement is the assertion's whole result.
	refuting := func() (*typ.Function, signature.Function) {
		subject := typ.NewTypeParam("T", nil)
		fnType := typ.Func().TypeParamRef(subject).
			Param("value", subject).OptParam("message", typ.String).Returns(subject).Build()
		return fnType, signature.Function{
			Type: fnType,
			Effect: effect.Empty.With(
				postcondition.NormalReturnRefinement{Target: effect.ParamRef{Index: 0}, Refinement: postcondition.Present{}},
				returns.Return{ReturnIndex: 0, Transform: returns.SameAs{Source: effect.ParamRef{Index: 0}}},
			),
		}
	}
	classifying := func(result typ.Type) *typ.Function {
		return typ.Func().Param("value", typ.Any).OptParam("message", typ.String).Returns(result).Build()
	}
	checking := func() *typ.Function {
		return typ.Func().Param("value", typ.Any).OptParam("message", typ.String).Build()
	}
	comparing := func() *typ.Function {
		return typ.Func().Param("actual", typ.Any).Param("expected", typ.Any).OptParam("message", typ.String).Build()
	}

	okType, okSignature := refuting()
	notNilType, notNilSignature := refuting()
	eqType := comparing()
	neqType := comparing()
	failType := typ.Func().OptParam("message", typ.String).Returns(typ.Never).Build()
	isNilType := checking()
	isStringType := classifying(typ.String)
	isNumberType := classifying(typ.Number)
	isTableType := classifying(typ.BuiltinTableTopMarker())
	// A callable carries no top type in the canonical vocabulary, so the
	// asserted function is answered as the value it was given.
	isFunctionType := classifying(typ.Any)
	isBooleanType := classifying(typ.Boolean)
	containsType := typ.Func().Param("value", typ.String).Param("substring", typ.String).
		OptParam("message", typ.String).Returns(typ.String).Build()
	hasErrorType := typ.Func().Param("value", typ.Any).Param("err", typ.Any).OptParam("message", typ.String).Build()
	noErrorType := typ.Func().Param("value", typ.Any).Param("err", typ.Any).OptParam("message", typ.String).Build()
	// The asserted body is invoked through pcall, whose own declaration takes
	// its callable as an unconstrained value.
	throwsType := typ.Func().Param("body", typ.Any).OptParam("message", typ.String).Returns(typ.Any).Build()
	notThrowsType := typ.Func().Param("body", typ.Any).OptParam("message", typ.String).Build()
	errorKindType := typ.Func().Param("err", typ.Any).Param("kind", typ.Any).OptParam("message", typ.String).Build()
	errorMessageType := typ.Func().Param("err", typ.Any).Param("message", typ.String).OptParam("label", typ.String).Build()
	errorContainsType := typ.Func().Param("err", typ.Any).Param("substring", typ.String).OptParam("message", typ.String).Build()

	declaration.DefineFunctionSignature("ok", okSignature)
	declaration.DefineFunctionSignature("not_nil", notNilSignature)
	for name, member := range map[string]*typ.Function{
		"eq": eqType, "neq": neqType, "fail": failType, "is_nil": isNilType,
		"is_string": isStringType, "is_number": isNumberType, "is_table": isTableType,
		"is_function": isFunctionType, "is_boolean": isBooleanType, "contains": containsType,
		"has_error": hasErrorType, "no_error": noErrorType, "throws": throwsType,
		"not_throws": notThrowsType, "error_kind": errorKindType,
		"error_message": errorMessageType, "error_contains": errorContainsType,
	} {
		declaration.DefineFunctionSignature(name, signature.Function{Type: member})
	}

	declaration.SetExport(typetable.NewRecord().
		Field("eq", eqType).
		Field("neq", neqType).
		Field("ok", okType).
		Field("fail", failType).
		Field("is_nil", isNilType).
		Field("not_nil", notNilType).
		Field("is_string", isStringType).
		Field("is_number", isNumberType).
		Field("is_table", isTableType).
		Field("is_function", isFunctionType).
		Field("is_boolean", isBooleanType).
		Field("contains", containsType).
		Field("has_error", hasErrorType).
		Field("no_error", noErrorType).
		Field("throws", throwsType).
		Field("not_throws", notThrowsType).
		Field("error_kind", errorKindType).
		Field("error_message", errorMessageType).
		Field("error_contains", errorContainsType).
		Build())
	return declaration
}

// resourceHostManifest declares the connection and transaction surface the
// declared-resource-lifecycle fixture requires. Wippy v1 ships no such module:
// this is a preview surface whose whole purpose is to state a lifecycle
// contract, so it declares exactly the five members that fixture calls.
//
// The two state machines are stated in full, each with the member that creates
// its resource, the member that moves it to its final state, and the members
// that read one without moving it: query and begin both demand an open
// connection and leave it open. detach states the third disposition a member
// has toward a governed resource: it hands the connection to a subsystem this
// analysis does not follow, so every proof about that connection is
// discharged and no successor state is stated for it.
func resourceHostManifest() *manifestwire.Manifest {
	declaration := manifestwire.New("resource")
	connectionType := typ.NewInterface("resource.Connection", nil)
	transactionType := typ.NewInterface("resource.Transaction", nil)
	declaration.DefineType("Connection", connectionType)
	declaration.DefineType("Transaction", transactionType)

	for _, definition := range []typestate.Definition{{
		Protocol:    "connection",
		States:      []typestate.State{"open", "closed"},
		FinalStates: []typestate.State{"closed"},
		Transitions: []typestate.TransitionDecl{{From: "open", To: "closed"}},
	}, {
		Protocol:    "transaction",
		States:      []typestate.State{"active", "committed"},
		FinalStates: []typestate.State{"committed"},
		Transitions: []typestate.TransitionDecl{{From: "active", To: "committed"}},
	}} {
		if err := declaration.DefineTypestateProtocol(definition); err != nil {
			panic("testfixture: resource host manifest declares an invalid " + string(definition.Protocol) + " protocol: " + err.Error())
		}
	}

	connectType := typ.Func().Returns(connectionType).Build()
	closeType := typ.Func().Param("connection", connectionType).Build()
	queryType := typ.Func().Param("connection", connectionType).Build()
	beginType := typ.Func().Param("connection", connectionType).Returns(transactionType).Build()
	commitType := typ.Func().Param("transaction", transactionType).Build()
	detachType := typ.Func().Param("connection", connectionType).Build()

	declaration.DefineFunctionSignature("connect", signature.Function{Type: connectType})
	// An acquired resource is a newly allocated host value, so the acquiring
	// result carries the fresh declaration that gives it an allocation
	// identity. Without it the acquisition names a result slot with no heap
	// root, and no per-resource state can be keyed on it.
	declaration.DefineFunctionOperation("connect", manifestwire.Operation{
		Acquisitions: []manifestwire.Acquisition{{Protocol: "connection", State: "open"}},
		OutcomeAmendments: []manifestwire.OutcomeAmendment{{
			Outcome: 0, FreshResults: []manifestwire.FreshResult{{Result: 0, Class: manifestwire.FreshUserdata}},
		}},
	})
	declaration.DefineFunctionSignature("close", signature.Function{
		Type: closeType,
		Effect: effect.Empty.With(lifecycle.Transition{
			Target: effect.ParamRef{Index: 0}, Protocol: "connection", From: "open", To: "closed",
		}),
	})
	declaration.DefineFunctionSignature("query", signature.Function{Type: queryType})
	// query reads the connection and hands it back in the same state, so the
	// constraint is a requirement rather than a transition with equal
	// endpoints: it holds on every arm and discharges no obligation.
	declaration.DefineFunctionOperation("query", manifestwire.Operation{
		Requirements: []manifestwire.Requirement{{
			Input:    manifestwire.InputSource{Kind: manifestwire.InputSourceValue},
			Protocol: "connection",
			State:    "open",
		}},
	})
	declaration.DefineFunctionSignature("begin", signature.Function{Type: beginType})
	// begin acquires a transaction from a connection it only reads: the
	// connection stays open, so the member carries both an acquisition of one
	// protocol and a requirement on the other.
	declaration.DefineFunctionOperation("begin", manifestwire.Operation{
		Acquisitions: []manifestwire.Acquisition{{Protocol: "transaction", State: "active"}},
		Requirements: []manifestwire.Requirement{{
			Input:    manifestwire.InputSource{Kind: manifestwire.InputSourceValue},
			Protocol: "connection",
			State:    "open",
		}},
		OutcomeAmendments: []manifestwire.OutcomeAmendment{{
			Outcome: 0, FreshResults: []manifestwire.FreshResult{{Result: 0, Class: manifestwire.FreshUserdata}},
		}},
	})
	declaration.DefineFunctionSignature("commit", signature.Function{
		Type: commitType,
		Effect: effect.Empty.With(lifecycle.Transition{
			Target: effect.ParamRef{Index: 0}, Protocol: "transaction", From: "active", To: "committed",
		}),
	})
	// detach hands the connection out of the analysis. An escape states no
	// arrival state because there is none to state: the resource leaves, and
	// what happens to it afterwards is not this program's to prove.
	declaration.DefineFunctionSignature("detach", signature.Function{
		Type: detachType,
		Effect: effect.Empty.With(lifecycle.Escape{
			Target: effect.ParamRef{Index: 0}, Protocol: "connection",
		}),
	})

	declaration.SetExport(typetable.NewRecord().
		Field("connect", connectType).
		Field("close", closeType).
		Field("query", queryType).
		Field("begin", beginType).
		Field("commit", commitType).
		Field("detach", detachType).
		Build())
	return declaration
}

// ownershipHostManifest declares the ownership-transfer surface the placement
// and send-safety fixtures require. Wippy v1 ships no such module: it is a
// checker-only affordance whose whole purpose is to state one escape, so it
// declares exactly the member that corpus calls.
//
// store names the value it hands to a longer-lived owner and the owner it
// hands it to. That relation is the ownership.Store label, which is the same
// escape table.insert declares for the element it appends, so the placement
// store rule reads one vocabulary rather than a second spelling of it. The
// member answers nothing: a fixture that bound a result would be claiming a
// return the module does not declare.
func ownershipHostManifest() *manifestwire.Manifest {
	storeType := typ.Func().
		Param("value", typ.Any).
		Param("into", typ.Any).
		Build()
	declaration := manifestwire.New("ownership")
	declaration.DefineFunctionSignature("store", signature.Function{
		Type: storeType,
		Effect: effect.Empty.With(
			ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 1}},
		),
	})
	declaration.SetExport(typetable.NewRecord().
		Field("store", storeType).
		Build())
	return declaration
}
