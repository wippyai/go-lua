package value_test

import (
	"testing"

	heapdomain "github.com/wippyai/go-lua/domain/heap"
)

// This file states where a fresh result lives in Value's published geometry.
//
// A fresh result is produced by a mounted call, and the call is the only thing
// that knows how many of them there are: a call site whose callee is not
// resolved to a single Target operation is admitted against every operation it
// may reach, each operation's outcomes have their own arms, and Heap issues its
// own ordinal where one arm creates more than one root. Nine roots under one
// call site is an ordinary answer, not a malformed one.
//
// So the directory is a nested member set addressed by (parent, ordinal), the
// same shape the mounted actuals use, rather than a flat global list a consumer
// re-correlates to calls. What that buys is the reason it exists: a rule whose
// candidate is a mounted call reaches this call's fresh results through the row
// it already has, at a moment the mounted lane declares - which a Link-lane
// directory addressed by Heap key alone can never offer.

// TestFreshResultsAreMembersOfTheCallThatProducesThem states the membership.
// Every published fresh result belongs to exactly one parent, sits at the
// ordinal its owner-issued tag names, and is reachable from that parent; the
// parents' censuses together account for the whole directory and for nothing
// else.
func TestFreshResultsAreMembersOfTheCallThatProducesThem(t *testing.T) {
	const source = "local t = {}\nlocal co = coroutine.create(function() end)\nreturn t, co\n"
	_, schema := sealValueSource(t, "fresh_result_member.lua", source)

	total := schema.FreshResultCallCount()
	if total == 0 {
		t.Fatal("fixture seals no fresh result: the law would be vacuous")
	}
	reached := make(map[heapdomain.Key]int, total)
	members := 0
	for index := 0; index < schema.MountedCallActualsCount(); index++ {
		parent, parentOK := schema.MountedCallActualsAt(index)
		if !parentOK {
			t.Fatalf("MountedCallActualsAt(%d)", index)
		}
		parentModule, parentModuleOK := parent.Module()
		parentCall, parentCallOK := parent.CallID()
		if !parentModuleOK || !parentCallOK {
			t.Fatalf("parent %d names no mounted call", index)
		}
		for ordinal := 0; ordinal < parent.FreshResultCount(); ordinal++ {
			member, memberOK := parent.FreshResultAt(ordinal)
			if !memberOK {
				t.Fatalf("parent %d member %d is absent from its own span", index, ordinal)
			}
			module, moduleOK := member.Module()
			call, callOK := member.CallID()
			tag, tagOK := member.ResultTag()
			if !moduleOK || !callOK || !tagOK {
				t.Fatalf("member %d of parent %d publishes no address", ordinal, index)
			}
			if module != parentModule || call != parentCall {
				t.Fatalf("parent %d answered a fresh result of another call at ordinal %d", index, ordinal)
			}
			if tag != uint64(ordinal)+1 {
				t.Fatalf("member %d of parent %d carries tag %d", ordinal, index, tag)
			}
			key, keyOK := member.Key()
			if !keyOK {
				t.Fatal("member publishes no Heap key")
			}
			reached[key]++
			members++
		}
	}
	if members != total {
		t.Fatalf("parents reach %d fresh results, the directory publishes %d", members, total)
	}
	for index := 0; index < total; index++ {
		row, rowOK := schema.FreshResultCallAt(index)
		key, keyOK := row.Key()
		if !rowOK || !keyOK {
			t.Fatalf("FreshResultCallAt(%d)", index)
		}
		if reached[key] != 1 {
			t.Fatalf("fresh result %d is reached %d times through its parents", index, reached[key])
		}
	}
}

// TestOneCallSiteAdmitsOneMemberPerOperationArm states that the member address
// is the whole arm and not a shorter prefix of it. The fixture's host call
// reaches several Target operations, so several members share a result ordinal
// and an outcome; if the address were the result ordinal alone the set could
// not hold them, and the seal would have refused the program rather than
// publish an ambiguous member.
func TestOneCallSiteAdmitsOneMemberPerOperationArm(t *testing.T) {
	const source = "local co = coroutine.create(function() end)\nreturn co\n"
	_, schema := sealValueSource(t, "fresh_result_arms.lua", source)

	widest := 0
	for index := 0; index < schema.MountedCallActualsCount(); index++ {
		parent, parentOK := schema.MountedCallActualsAt(index)
		if !parentOK {
			t.Fatalf("MountedCallActualsAt(%d)", index)
		}
		if parent.FreshResultCount() > widest {
			widest = parent.FreshResultCount()
		}
		seen := make(map[freshArmForLaw]struct{}, parent.FreshResultCount())
		for ordinal := 0; ordinal < parent.FreshResultCount(); ordinal++ {
			member, memberOK := parent.FreshResultAt(ordinal)
			operation, operationOK := member.Operation()
			outcome, outcomeOK := member.Outcome()
			result, resultOK := member.ResultIndex()
			fresh, freshOK := member.FreshOrdinal()
			if !memberOK || !operationOK || !outcomeOK || !resultOK || !freshOK {
				t.Fatalf("member %d of parent %d publishes no arm", ordinal, index)
			}
			arm := freshArmForLaw{operation: uint64(operation), outcome: outcome, result: result, fresh: fresh}
			if _, duplicate := seen[arm]; duplicate {
				t.Fatalf("parent %d publishes two members at one arm %+v", index, arm)
			}
			seen[arm] = struct{}{}
		}
	}
	if widest < 2 {
		t.Fatalf("no call site admits more than one fresh result: the law is vacuous (widest=%d)", widest)
	}
}

type freshArmForLaw struct {
	operation              uint64
	outcome, result, fresh uint32
}
