package static

// resolveViewComponent is the only bridge from a Finalizer-bound view to the
// authored component. Ordinary sealed views carry component directly; a
// construction view carries only draftState and resolves while the owner is
// claimed. Consequently copied views retain no Component after Commit/Abort.
func resolveViewComponent(component *Component, state *draftState) *Component {
	if state == nil {
		return component
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.phase != draftClaimed {
		return nil
	}
	return state.component
}

func (view View) componentOf() *Component { return resolveViewComponent(view.component, view.state) }

// Available reports whether this view currently resolves to committed authored
// Static content. A construction view is available only while its Finalizer
// remains claimed; an ordinary Component view must also carry the nonzero
// content identity that fences every typed projection.
func (view View) Available() bool {
	component := view.componentOf()
	return component != nil && component.contentID.Available()
}

func (view Types) componentOf() *Component { return resolveViewComponent(view.component, view.state) }
func (view Primitives) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Literals) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Optionals) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Unions) componentOf() *Component { return resolveViewComponent(view.component, view.state) }
func (view Intersections) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Generics) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Arrays) componentOf() *Component  { return resolveViewComponent(view.component, view.state) }
func (view Maps) componentOf() *Component    { return resolveViewComponent(view.component, view.state) }
func (view Records) componentOf() *Component { return resolveViewComponent(view.component, view.state) }
func (view Fields) componentOf() *Component  { return resolveViewComponent(view.component, view.state) }
func (view References) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Declarations) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Aliases) componentOf() *Component { return resolveViewComponent(view.component, view.state) }
func (view TypeParams) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Interfaces) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view DeclaredTypes) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Signatures) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view TypeFunctions) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Assertions) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Contracts) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view EffectRows) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Functions) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Calls) componentOf() *Component { return resolveViewComponent(view.component, view.state) }
func (view Publications) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Operators) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view TypeOfs) componentOf() *Component { return resolveViewComponent(view.component, view.state) }
func (view KeyOfs) componentOf() *Component  { return resolveViewComponent(view.component, view.state) }
func (view IndexAccesses) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Conditionals) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Operands) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view ClaimTargets) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view TypeValueTargets) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
func (view Annotations) componentOf() *Component {
	return resolveViewComponent(view.component, view.state)
}
