package generate

import "github.com/wippyai/go-lua/cmd/flashrefactor/internal/cutplan"

// Provider is an explicit, pure implementation registered when a Registry is
// built.  Identity is the stable implementation identity recorded in a Lock.
// Render receives no repository or execution capability.
type Provider struct {
	Name     cutplan.Provider
	Identity string
	Render   Render
}

// Render deterministically transforms declared bytes into one output file.
type Render func(Request) ([]byte, error)

// Request contains the complete declared generator input and output metadata.
// Inputs preserve the authored Generate.Inputs order; there are no implicit
// files, environment variables, commands, or workspace handles.
type Request struct {
	Inputs      []Input
	Destination Destination
}

// Input binds exact declared bytes to one repository-relative path.
type Input struct {
	Path  string
	Bytes []byte
}

// Destination is output metadata only.  The provider cannot write it.
type Destination struct {
	Path string
}

// Result is deterministic generated output plus Lock-ready provider evidence.
type Result struct {
	Evidence cutplan.ProviderEvidence
	Bytes    []byte
}

// Registry is immutable after construction.  Its provider map and concrete
// provider values are deliberately unexported.
type Registry struct {
	providers map[cutplan.Provider]provider
}

type provider struct {
	name     cutplan.Provider
	identity string
	render   Render
}
