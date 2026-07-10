package parse

// Keep the parser generator pinned to a release that supports the module's
// Go 1.23 toolchain. CI regenerates parser.go and rejects a dirty result.
//
//go:generate sh generate.sh
