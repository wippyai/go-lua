package typestate

import "testing"

func TestAcquireTransitionCloseClearsObligationForAnyProtocol(t *testing.T) {
	resource := Resource{ID: "query-result#42", Protocol: "cursor"}
	got := Empty().
		Acquire(resource, "open", Obligation{Final: "closed"}).
		Transition(resource, "open", "closed")

	if obligations := got.OpenObligations(); len(obligations) != 0 {
		t.Fatalf("open obligations = %#v, want none", obligations)
	}
}

func TestAcquireTransitionClosesOnAnyObligationFinalState(t *testing.T) {
	tx := Resource{ID: "db.tx", Protocol: "transaction"}
	obligation := Obligation{Finals: NewFinalStates("committed", "rolled_back")}

	for _, final := range []State{"committed", "rolled_back"} {
		got := Empty().
			Acquire(tx, "active", obligation).
			Transition(tx, "active", final)
		if obligations := got.OpenObligations(); len(obligations) != 0 {
			t.Fatalf("final %s open obligations = %#v, want none", final, obligations)
		}
	}

	got := Empty().
		Acquire(tx, "active", obligation).
		Transition(tx, "active", "prepared")
	obligations := got.OpenObligations()
	if len(obligations) != 1 {
		t.Fatalf("prepared obligations = %#v, want still open", obligations)
	}
}

func TestFinalStatesAreCanonicalComparableSets(t *testing.T) {
	left := NewFinalStates("rolled_back", "committed", "committed")
	right := NewFinalStates("committed", "rolled_back")
	if left != right {
		t.Fatalf("final states = %q/%q, want canonical equality", left, right)
	}
	got := left.States()
	if len(got) != 2 || got[0] != "committed" || got[1] != "rolled_back" {
		t.Fatalf("states = %#v, want sorted unique finals", got)
	}
	if !left.Contains("committed") || !left.Contains("rolled_back") || left.Contains("active") {
		t.Fatalf("contains checks failed for %q", left)
	}
}

func TestResourceIDIsOpaqueToTypestateStore(t *testing.T) {
	resource := Resource{ID: "not a path key and not parsed", Protocol: "handle"}
	got := Empty().Acquire(resource, "open", Obligation{Final: "closed"})

	obligations := got.OpenObligations()
	if len(obligations) != 1 {
		t.Fatalf("open obligations = %#v, want one opaque resource obligation", obligations)
	}
	if obligations[0].Resource.ID.String() != "not a path key and not parsed" {
		t.Fatalf("resource id = %q, want original opaque spelling", obligations[0].Resource.ID)
	}
}

func TestMapResourcesJoinsCollidingAliases(t *testing.T) {
	left := Resource{ID: "left", Protocol: "transaction"}
	right := Resource{ID: "right", Protocol: "transaction"}
	canonical := Resource{ID: "canonical", Protocol: "transaction"}
	got := Empty().
		Acquire(left, "active", Obligation{Final: "finished"}).
		Acquire(right, "active", Obligation{Final: "finished"}).
		MapResources(func(Resource) Resource {
			return canonical
		})

	obligations := got.OpenObligations()
	if len(obligations) != 1 {
		t.Fatalf("open obligations = %#v, want one joined obligation", obligations)
	}
	if obligations[0].Resource != canonical ||
		obligations[0].Current != "active" ||
		!obligations[0].Obligation.SatisfiedBy("finished") {
		t.Fatalf("joined obligation = %#v, want canonical active transaction", obligations[0])
	}
}

func TestJoinKeepsObligationWhenAnyBranchMayReturnOpen(t *testing.T) {
	tx := Resource{ID: "root.tx", Protocol: "transaction"}
	open := Empty().Acquire(tx, "active", Obligation{Final: "finished"})
	closed := open.Transition(tx, "active", "finished")

	got := Join(open, closed)
	obligations := got.OpenObligations()
	if len(obligations) != 1 {
		t.Fatalf("open obligations = %#v, want one maybe-open transaction", obligations)
	}
	if obligations[0].Resource != tx || obligations[0].Locality != LocalityOpen {
		t.Fatalf("obligation = %#v, want open transaction obligation", obligations[0])
	}
}

func TestEscapedResourceDoesNotProduceLocalObligation(t *testing.T) {
	socket := Resource{ID: "client.socket", Protocol: "socket"}
	got := Empty().
		Acquire(socket, "connected", Obligation{Final: "closed"}).
		Escape(socket)

	if obligations := got.OpenObligations(); len(obligations) != 0 {
		t.Fatalf("open obligations = %#v, want none after escape", obligations)
	}
}

func TestEscapeRevokesAClosedResourcesLocalStateAuthority(t *testing.T) {
	resource := Resource{ID: "client.socket", Protocol: "socket"}
	escaped := Empty().
		Acquire(resource, "connected", Obligation{Final: "closed"}).
		Transition(resource, "connected", "closed").
		Escape(resource)
	slot, ok := escaped.Lookup(resource)
	if !ok || slot.Locality != LocalityEscaped || slot.Current != "closed" {
		t.Fatalf("escaped closed slot = %#v/%v", slot, ok)
	}
}

func TestJoinOfClosedAndEscapedRemainsLocallySatisfied(t *testing.T) {
	lock := Resource{ID: "guard.lock", Protocol: "lock"}
	open := Empty().Acquire(lock, "held", Obligation{Final: "released"})
	closed := open.Transition(lock, "held", "released")
	escaped := open.Escape(lock)

	got := Join(closed, escaped)
	if obligations := got.OpenObligations(); len(obligations) != 0 {
		t.Fatalf("open obligations = %#v, want none for closed-or-escaped lock", obligations)
	}
}

func TestLoopBodyAcquireAndCloseConvergesWithoutLeaking(t *testing.T) {
	cursor := Resource{ID: "loop.cursor", Protocol: "cursor"}
	state := Empty()
	for i := 0; i < 4; i++ {
		body := state.
			Acquire(cursor, "open", Obligation{Final: "closed"}).
			Transition(cursor, "open", "closed")
		next := Join(state, body)
		if Equal(next, state) {
			break
		}
		state = next
	}

	if obligations := state.OpenObligations(); len(obligations) != 0 {
		t.Fatalf("open obligations = %#v, want no loop-carried leak", obligations)
	}
}

func TestLoopCarriedOpenObligationReachesFixpoint(t *testing.T) {
	handle := Resource{ID: "loop.handle", Protocol: "handle"}
	state := Empty()
	for i := 0; i < 4; i++ {
		body := state.Acquire(handle, "open", Obligation{Final: "closed"})
		next := Widen(state, body)
		if Equal(next, state) {
			break
		}
		state = next
	}

	obligations := state.OpenObligations()
	if len(obligations) != 1 {
		t.Fatalf("open obligations = %#v, want one loop-carried obligation", obligations)
	}
	if obligations[0].Resource != handle || obligations[0].Current != "open" {
		t.Fatalf("obligation = %#v, want open handle obligation", obligations[0])
	}
}

func TestNoProtocolNameGuessing(t *testing.T) {
	dbish := Resource{ID: "looks.like.db", Protocol: "not-a-database"}
	closer := Resource{ID: "db.close", Protocol: "made-up-protocol"}
	got := Empty().
		Acquire(dbish, "alpha", Obligation{Final: "omega"}).
		Acquire(closer, "start", Obligation{Final: "end"})

	obligations := got.OpenObligations()
	if len(obligations) != 2 {
		t.Fatalf("open obligations = %#v, want both arbitrary protocols tracked", obligations)
	}
	if obligations[0].Resource.Protocol != "made-up-protocol" || obligations[1].Resource.Protocol != "not-a-database" {
		t.Fatalf("obligations ordered/protocols = %#v, want canonical protocol ordering without name heuristics", obligations)
	}
}

func TestLatticeLawsForRepresentativeStores(t *testing.T) {
	db := Resource{ID: "db.tx", Protocol: "tx"}
	cursor := Resource{ID: "rows", Protocol: "cursor"}
	empty := Domain.Bottom()
	top := Domain.Top()
	openDB := empty.Acquire(db, "active", Obligation{Final: "done"})
	closedDB := openDB.Transition(db, "active", "done")
	escapedDB := openDB.Escape(db)
	openCursor := empty.Acquire(cursor, "open", Obligation{Final: "closed"})
	samples := []Store{empty, openDB, closedDB, escapedDB, openCursor, Join(openDB, openCursor), top}

	for i, a := range samples {
		if !Domain.Equal(Domain.Join(a, a), a) {
			t.Fatalf("join idempotence failed for sample %d", i)
		}
		if !Domain.LessOrEq(Domain.Bottom(), a) {
			t.Fatalf("bottom <= sample %d failed", i)
		}
		if !Domain.LessOrEq(a, Domain.Top()) {
			t.Fatalf("sample %d <= top failed", i)
		}
		for j, b := range samples {
			if !Domain.Equal(Domain.Join(a, b), Domain.Join(b, a)) {
				t.Fatalf("join commutativity failed for samples %d/%d", i, j)
			}
			if !Domain.Equal(Domain.Meet(a, b), Domain.Meet(b, a)) {
				t.Fatalf("meet commutativity failed for samples %d/%d", i, j)
			}
			join := Domain.Join(a, b)
			if !Domain.LessOrEq(a, join) || !Domain.LessOrEq(b, join) {
				t.Fatalf("join upper-bound failed for samples %d/%d", i, j)
			}
			meet := Domain.Meet(a, b)
			if !Domain.LessOrEq(meet, a) || !Domain.LessOrEq(meet, b) {
				t.Fatalf("meet lower-bound failed for samples %d/%d: %#v", i, j, meet)
			}
			if !Domain.Equal(Domain.Meet(a, join), a) {
				t.Fatalf("meet/join absorption failed for samples %d/%d", i, j)
			}
			if !Domain.Equal(Domain.Join(a, meet), a) {
				t.Fatalf("join/meet absorption failed for samples %d/%d", i, j)
			}
			for k, c := range samples {
				left := Domain.Join(Domain.Join(a, b), c)
				right := Domain.Join(a, Domain.Join(b, c))
				if !Domain.Equal(left, right) {
					t.Fatalf("join associativity failed for samples %d/%d/%d", i, j, k)
				}
				left = Domain.Meet(Domain.Meet(a, b), c)
				right = Domain.Meet(a, Domain.Meet(b, c))
				if !Domain.Equal(left, right) {
					t.Fatalf("meet associativity failed for samples %d/%d/%d", i, j, k)
				}
			}
		}
	}
}

func TestMeetKeepsOnlySharedCompatibleTypestateFacts(t *testing.T) {
	tx := Resource{ID: "db.tx", Protocol: "tx"}
	cursor := Resource{ID: "rows", Protocol: "cursor"}
	openTX := Empty().Acquire(tx, "active", Obligation{Final: "done"})
	openCursor := Empty().Acquire(cursor, "open", Obligation{Final: "closed"})
	generalTX := Join(openTX, openTX.Transition(tx, "active", "done"))

	got := Meet(Join(openTX, openCursor), generalTX)

	obligations := got.OpenObligations()
	if len(obligations) != 1 {
		t.Fatalf("open obligations = %#v, want only shared tx obligation", obligations)
	}
	if obligations[0].Resource != tx || obligations[0].Current != "active" || obligations[0].Obligation.Final != "done" {
		t.Fatalf("obligation = %#v, want precise tx obligation", obligations[0])
	}
}

func TestMeetDropsIncompatibleProtocolStates(t *testing.T) {
	tx := Resource{ID: "db.tx", Protocol: "tx"}
	active := Empty().Acquire(tx, "active", Obligation{Final: "done"})
	rolledBack := Empty().Acquire(tx, "rolled_back", Obligation{Final: "done"})

	got := Meet(active, rolledBack)
	if !Equal(got, Empty()) {
		t.Fatalf("meet = %#v, want bottom for incompatible states", got)
	}
}

func TestTransitionRetainsProvenInvalidStateWithSite(t *testing.T) {
	resource := Resource{ID: "db.conn", Protocol: "connection"}
	got := Empty().
		Acquire(resource, "open", Obligation{Final: "closed"}).
		TransitionAt(resource, "open", "closed", 12).
		TransitionAt(resource, "open", "closed", 19)

	invalid := got.InvalidTransitions()
	if len(invalid) != 1 {
		t.Fatalf("invalid transitions = %#v, want one", invalid)
	}
	if invalid[0].Resource != resource || invalid[0].Expected != "open" || invalid[0].Found != "closed" || invalid[0].Site != 19 {
		t.Fatalf("invalid transition = %#v, want closed double-transition at site 19", invalid[0])
	}
}

func TestTransitionSilencesUnknownAndEscapedResources(t *testing.T) {
	resource := Resource{ID: "db.conn", Protocol: "connection"}
	unknown := Empty().TransitionAt(resource, "open", "closed", 1)
	if invalid := unknown.InvalidTransitions(); len(invalid) != 0 {
		t.Fatalf("unknown invalid transitions = %#v, want none", invalid)
	}
	escaped := Empty().Acquire(resource, "open", Obligation{Final: "closed"}).Escape(resource).TransitionAt(resource, "open", "closed", 2)
	if invalid := escaped.InvalidTransitions(); len(invalid) != 0 {
		t.Fatalf("escaped invalid transitions = %#v, want none", invalid)
	}
}

func TestInvalidTransitionsJoinAndAliasCanonicalization(t *testing.T) {
	left := Resource{ID: "left", Protocol: "connection"}
	right := Resource{ID: "right", Protocol: "connection"}
	canonical := Resource{ID: "canonical", Protocol: "connection"}
	withInvalid := Empty().Acquire(left, "open", Obligation{Final: "closed"}).TransitionAt(left, "closed", "open", 7)
	joined := Join(Empty(), withInvalid).MapResources(func(Resource) Resource { return canonical })
	invalid := joined.InvalidTransitions()
	if len(invalid) != 1 || invalid[0].Resource != canonical || invalid[0].Expected != "closed" || invalid[0].Found != "open" {
		t.Fatalf("joined invalid transitions = %#v, want canonical failure", invalid)
	}
	if invalid := Join(withInvalid, Empty().Acquire(right, "open", Obligation{Final: "closed"})).InvalidTransitions(); len(invalid) != 1 {
		t.Fatalf("join invalid transitions = %#v, want failure preserved", invalid)
	}
}
