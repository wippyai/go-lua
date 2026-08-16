package grammarproof

// grammarCorpus is deliberately small and hand-authored. It supplements the
// canonical source cases and fixture corpus with grammar shapes whose purpose
// is parser reachability, not an alternate semantic test suite.
var grammarCorpus = []source{
	{id: "grammar:empty", text: "", required: true},
	{id: "grammar:semicolon", text: ";;", required: true},
	{id: "grammar:final-semicolon", text: "local value = 1;", required: true},
	{id: "grammar:final-return-semicolon", text: "return 1;", required: true},
	{id: "grammar:interior-semicolon", text: "local left = 1; local right = 2", required: true},
	{id: "grammar:labels", text: "::again::\ngoto again", required: true},
	{id: "grammar:control", text: "if true then elseif false then elseif false then else end\nwhile false do break end\nrepeat until true\nfor i = 1, 2, 1 do end\nfor k, v in pairs({}) do end", required: true},
	{id: "grammar:typed-local-without-initializer", text: "local value: string", required: true},
	// These four sources discharge valid whole-AST states which the reduction
	// hook cannot observe reliably. Their exact state is checked by the joined
	// grammar matrix; keeping text here makes public ingress the only witness
	// corpus authority.
	{id: "grammar:residue-scalar-vararg", text: "local function f(...) return (...) end", required: true},
	{id: "grammar:residue-generic-function-type", text: "interface Subject\n function map<T: string>(value: T): T\nend", required: true},
	{id: "grammar:residue-empty-interface", text: "interface Empty end", required: true},
	{id: "grammar:residue-optional-interface-field", text: "interface Subject\n field?: string\nend", required: true},
	{id: "grammar:values", text: "local a, b = nil, true\na = -#~a\na = a and b or a\na = (a < b) == (a ~= b)\na = {a, [b] = a, k = b}\nreturn a, b, ...", required: true},
	{id: "grammar:bitwise", text: "local value = 7\nvalue = value & 3\nvalue = value | 3\nvalue = value ~ 3\nvalue = value << 1\nvalue = value >> 1", required: true},
	{id: "grammar:functions", text: "local f = function<T: string>(value: T): T return value end\nlocal root = {}\nfunction root:method(value) return value end\nreturn root:method(\"x\")", required: true},
	{id: "grammar:types", text: "type Subject = readonly { field: string } | string?\nlocal value: string = \"value\"", required: true},
	{id: "grammar:annotations", text: "type Subject = string @plain @empty() @arguments(1, \"two\")", required: true},
	{id: "grammar:table-contextual-fields", text: "local value = { type = 1, interface = 2, readonly = 3, as = 4, asserts = 5, is = 6, keyof = 7, extends = 8; ordinary = 9 }", required: true},
	{id: "grammar:static-contextual-fields", text: "type Subject = { type: string, interface: string, readonly: string, as: string, asserts: string, is: string, keyof: string, extends: string, typeof: string @tag(1) }", required: true},
	{id: "grammar:contextual-members", text: "local object = {}\nfunction object:type() end\nfunction object:interface() end\nfunction object:readonly() end\nfunction object:as() end\nfunction object:asserts() end\nfunction object:is() end\nobject.type = 1\nobject.interface = 2\nobject.readonly = 3\nobject.as = 4\nobject.asserts = 5\nobject.is = 6", required: true},
	{id: "grammar:generic-call", text: "local identity = function<T>(value: T): T return value end\nreturn identity::<string>(\"value\")", required: true},
	{id: "grammar:generic-method-call", text: "local object = {}\nfunction object:identity<T>(value: T): T return value end\nreturn object:identity::<string>(\"value\")", required: true},
	{id: "grammar:string-call-argument", text: "local sink = function(value) end\nsink \"value\"", required: true},
	{id: "grammar:function-types", text: "type Arrow = () -> (string, number, boolean)\ntype FunList = fun(): (string, number, boolean)\ntype FunEmpty = fun(): ()\ntype Variadic = (... string) -> string", required: true},
	{id: "grammar:function-tails", text: "local fixedVararg = function(value: string, ...) return value end\ntype FunctionTail = fun(value: string): ()\ntype FunctionVararg = (value: string, ... boolean) -> string", required: true},
	{id: "grammar:qualified-types", text: "type Generic = Namespace.Subject<string>\ntype Deep = Namespace.Inner.Subject", required: true},
	{id: "grammar:interface-members", text: "interface Subject: Parent, Namespace.Parent\n function type<T>()\n function interface()\n function readonly()\n function as()\n function asserts()\n function is()\nend\ninterface Plain\n field: string\n function map(value: string)\nend", required: true},
	{id: "grammar:interface-type-literals", text: "type NonEmpty = interface { field: string @tag(1) }\ntype Empty = interface {}\ntype Annotated = { field: string @tag(1) }\ntype Optional = { field?: string @tag(1) }\ntype OptionalTail = { field: string? @tag(1) }", required: true},
	{id: "grammar:static-forms", text: "type FunctionTail = fun(value: string): ()\ntype Compact = string @element[]\ntype ReadArray = readonly {string}\ntype ReadMap = readonly {[string]: number}\ntype EmptyRecord = {}\ntype BareAssert = asserts value\ntype NarrowAssert = asserts value is string\nlocal value = 1\ntype ValueType = typeof(value)\ntype ValueKeys = keyof(User)\ntype ValueIndex = User[\"field\"]\ntype Conditional = A extends B ? C : D", required: true},
}

type source struct {
	id       string
	text     string
	required bool
}
