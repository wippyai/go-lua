// Package schemaadapter is the sole bridge from the exact Program schema
// receipt to programartifact's compiler-only grammar capability.
package schemaadapter

import (
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/internal/grammar"
	"github.com/wippyai/go-lua/analysis/internal/programschema"
	"github.com/wippyai/go-lua/program"
)

func Compile(input program.TransformerInput, receipt programschema.CompilationReceipt) (*programartifact.Artifact, bool) {
	artifact, failure := CompileDetailed(input, receipt)
	return artifact, artifact != nil && !failure.Available()
}

// CompileDetailed preserves exact receipt validation and returns only the
// closed immutable failure projection from the authorized compiler.
func CompileDetailed(input program.TransformerInput, receipt programschema.CompilationReceipt) (*programartifact.Artifact, programartifact.CompileFailure) {
	capability, _ := capability(receipt)
	return programartifact.CompileDetailedAuthorized(input, capability)
}

func NewCompileKey(input program.TransformerInput, receipt programschema.CompilationReceipt) (programartifact.CompileKey, bool) {
	capability, ok := capability(receipt)
	if !ok {
		return programartifact.CompileKey{}, false
	}
	return programartifact.NewCompileKeyAuthorized(input, capability)
}

func capability(receipt programschema.CompilationReceipt) (grammar.Capability, bool) {
	if !receipt.Available() {
		return grammar.Capability{}, false
	}
	return grammar.Issue(receipt.Digest(), receipt.Version())
}
