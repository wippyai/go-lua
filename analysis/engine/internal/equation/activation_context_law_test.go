package equation

import "testing"

// Context-qualified activation is an engine identity, not a module-name
// hint.  These laws deliberately use the equation boundary only: execution
// context.Directory authenticates the IDs before a row reaches this package.
func TestActivationContextMustBeEmptyOrComplete(t *testing.T) {
	if !(ActivationContext{}).WellFormed() {
		t.Fatal("empty activation context is not a valid unqualified shape")
	}
	if (ActivationContext{TransitionID: boundaryContext(1)}).WellFormed() {
		t.Fatal("partially populated activation context was accepted")
	}
	if (ActivationContext{FromContextID: boundaryContext(1), ToContextID: boundaryContext(2)}).WellFormed() {
		t.Fatal("context without authenticated transition was accepted")
	}
	complete := ActivationContext{TransitionID: boundaryContext(3), FromContextID: boundaryContext(1), ToContextID: boundaryContext(2)}
	if !complete.Available() || !complete.WellFormed() {
		t.Fatal("complete activation context was rejected")
	}
}

func TestContextQualifiedActivationRowsDoNotCollapse(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	application, target, endpoint := fixture.application, triggerBindingKey(11), triggerBindingKey(12)
	from := boundaryContext(31)
	firstContext := ActivationContext{TransitionID: boundaryContext(32), FromContextID: from, ToContextID: boundaryContext(33)}
	secondContext := ActivationContext{TransitionID: boundaryContext(34), FromContextID: from, ToContextID: boundaryContext(35)}
	first := fixture.row(application, target, endpoint)
	first.Context = firstContext
	second := fixture.row(application, target, endpoint)
	second.Context = secondContext
	topology, sealed := fixture.sealWithRows(
		[]ActivationTriggerBinding{{TriggerOrdinal: 0, Family: fixture.family, Application: application}},
		[]ActivationRowSpec{first, second},
	)
	if !sealed || topology == nil {
		t.Fatal("context-qualified activation rows refused to seal")
	}
	trigger := topology.rows.members[0]
	baseLocator := PairLocator{Application: application, Target: target, Endpoint: endpoint}
	if _, selected := topology.SelectActivationMember(trigger, baseLocator); selected {
		t.Fatal("context-qualified rows were selectable without context")
	}
	firstMember, firstOK := topology.SelectActivationMember(trigger, PairLocator{Application: application, Target: target, Endpoint: endpoint, Context: firstContext})
	secondMember, secondOK := topology.SelectActivationMember(trigger, PairLocator{Application: application, Target: target, Endpoint: endpoint, Context: secondContext})
	if !firstOK || !secondOK || firstMember.Same(secondMember) {
		t.Fatal("distinct context rows collapsed to one activation member")
	}
	if comparison, comparable := firstMember.Compare(secondMember); !comparable || comparison == 0 {
		t.Fatal("distinct context rows did not reach member identity ordering")
	}
	if locator, locatorOK := firstMember.Locator(); !locatorOK || !locator.Context.Same(firstContext) {
		t.Fatal("selected member lost its exact context transition")
	}

	selected, selectedOK := topology.SelectActivationMemberForContext(trigger, baseLocator, from)
	if selectedOK || selected.Available() {
		t.Fatal("ambiguous same-source context rows were fanned out")
	}
	if selected, selectedOK := topology.SelectActivationMemberForContext(trigger, baseLocator, boundaryContext(99)); selectedOK || selected.Available() {
		t.Fatal("foreign source context selected an activation row")
	}
}

func TestActivationSelectionUsesTheUniqueOwnerIssuedContextForSource(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	application, target, endpoint := fixture.application, triggerBindingKey(51), triggerBindingKey(52)
	from := boundaryContext(53)
	context := ActivationContext{TransitionID: boundaryContext(54), FromContextID: from, ToContextID: boundaryContext(55)}
	row := fixture.row(application, target, endpoint)
	row.Context = context
	topology, sealed := fixture.sealWithRows(
		[]ActivationTriggerBinding{{TriggerOrdinal: 0, Family: fixture.family, Application: application}},
		[]ActivationRowSpec{row},
	)
	if !sealed || topology == nil {
		t.Fatal("unique context-qualified activation row refused to seal")
	}
	trigger := topology.rows.members[0]
	member, selected := topology.SelectActivationMemberForContext(trigger, PairLocator{
		Application: application,
		Target:      target,
		Endpoint:    endpoint,
	}, from)
	locator, locatorOK := member.Locator()
	if !selected || !locatorOK || !locator.Context.Same(context) {
		t.Fatal("unique owner-issued context was not retained by source-qualified selection")
	}
}

func TestActivationRowRejectsPartialContextIdentity(t *testing.T) {
	fixture := newTriggerBindingFixture(t)
	row := fixture.row(fixture.application, triggerBindingKey(11), triggerBindingKey(12))
	row.Context = ActivationContext{FromContextID: boundaryContext(41)}
	if topology, sealed := fixture.sealWithRows(
		[]ActivationTriggerBinding{{TriggerOrdinal: 0, Family: fixture.family, Application: fixture.application}},
		[]ActivationRowSpec{row},
	); sealed || topology != nil {
		t.Fatal("partial context identity crossed the activation row boundary")
	}
}
