package factflow

// Return describes the ordered value sources returned at a CFG point.
type Return struct {
	sources []ValueSource
}

// NewReturn creates a return fact from ordered return-slot sources.
func NewReturn(sources []ValueSource) Return {
	return Return{sources: copyValueSources(sources)}
}

// Sources returns the ordered return-slot sources.
func (r Return) Sources() []ValueSource { return copyValueSources(r.sources) }

func (r Return) copy() Return {
	r.sources = copyValueSources(r.sources)
	return r
}
