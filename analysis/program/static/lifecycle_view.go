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
