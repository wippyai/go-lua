// Package parsersource reads the sources the parser is generated from. Two
// source authorities live here: compiler/parse/parser.go.y, which states the
// grammar alternatives, their yacc semantic actions and the parser-only helper
// functions those actions may call; and the compiler/ast declaration graph,
// which states the constructor and field schema those actions build.
//
// Everything derived here is source authority. The package never runs a
// fixture, parses Lua, binds source, lowers a Program, or observes parser
// output, so its rows describe what the parser can construct rather than what
// any particular program did construct.
package parsersource
