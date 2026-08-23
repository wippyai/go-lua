package activation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

func routeLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("activation-route-law/"+label, nil)
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

type routeLawDirectory struct {
	directory executioncontext.Directory
	modules   map[string]identity.ContentID
}

// routeLawLink seals one Link whose contexts are named by module and actor.
// Only the import edge main -> lib is authored, which is the complete relation
// a composition of these modules declares.
func routeLawLink(t *testing.T) routeLawDirectory {
	t.Helper()
	linkID := routeLawID(t, "link")
	modules := map[string]identity.ContentID{
		"main":   routeLawID(t, "module/main"),
		"lib":    routeLawID(t, "module/lib"),
		"worker": routeLawID(t, "module/worker"),
	}
	actors := map[string]identity.ContentID{
		"host":   routeLawID(t, "actor/host"),
		"worker": routeLawID(t, "actor/worker"),
	}
	rows := []struct{ module, actor string }{
		{"main", "host"}, {"lib", "host"}, {"worker", "worker"},
	}
	contexts := make([]executioncontext.Context, 0, len(rows))
	roots := make([]executioncontext.RootContext, 0, len(rows))
	for index, row := range rows {
		context, contextOK := executioncontext.NewContext(linkID, modules[row.module], actors[row.actor], routeLawID(t, "instance/"+row.module))
		root, rootOK := executioncontext.NewRootContext(linkID, routeLawID(t, "root/"+row.module), context.ID())
		if !contextOK || !rootOK {
			t.Fatalf("construct context %d", index)
		}
		contexts = append(contexts, context)
		roots = append(roots, root)
	}
	authored, authoredOK := executioncontext.NewTransition(linkID, contexts[0].ID(), contexts[1].ID())
	if !authoredOK {
		t.Fatal("construct the authored main -> lib import edge")
	}
	directory, sealed := executioncontext.Seal(linkID, contexts, roots, []executioncontext.Transition{authored})
	if !sealed {
		t.Fatal("seal the route law directory")
	}
	return routeLawDirectory{directory: directory, modules: modules}
}

// Call's body table is global: one activation occurrence carries a route to
// every admitted body, including bodies in modules its own module never
// imports. The route relation is therefore the directory's activation
// relation. A route reaches a body along an import, a callback body in the
// module that imported the trigger's own, and a body in the trigger's module.
// A body another actor holds is resident and reached by no edge: the two
// actors never hold one value.
func TestActivationRoutesAdmitEveryBodyOfOneActor(t *testing.T) {
	link := routeLawLink(t)
	cases := []struct {
		name          string
		trigger, body string
		edges         int
	}{
		{"a body in an imported module", "main", "lib", 1},
		{"a callback body in the importing module", "lib", "main", 1},
		{"a body in the trigger's own module", "lib", "lib", 1},
		{"a body in another actor", "main", "worker", 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			edges, refusal := activationRoutes(link.directory, link.modules[testCase.trigger], link.modules[testCase.body])
			if refusal.Available() {
				t.Fatalf("a module the directory holds was refused as %s", refusal)
			}
			if len(edges) != testCase.edges {
				t.Fatalf("route produced %d edges, want %d", len(edges), testCase.edges)
			}
			for _, edge := range edges {
				from, fromOK := link.directory.Context(edge.FromContextID())
				to, toOK := link.directory.Context(edge.ToContextID())
				if !fromOK || !toOK || from.ModuleKey() != link.modules[testCase.trigger] || to.ModuleKey() != link.modules[testCase.body] {
					t.Fatal("the route edge does not run from the trigger's module into the body's module")
				}
			}
		})
	}
}

// Residence is the one condition the producer refuses on. A module the
// directory holds no Context for is a mount the Link never made, and no edge
// may be invented for it. Nothing is silent: the refusal names which of the
// two modules is not resident and carries both module identities, so the
// envelope reports the operands rather than only the rule.
func TestActivationRoutesNameTheModuleOutsideTheDirectory(t *testing.T) {
	link := routeLawLink(t)
	unmounted := routeLawID(t, "module/unmounted")
	cases := []struct {
		name          string
		trigger, body identity.ContentID
		reason        RefusalReason
	}{
		{"an absent body module", link.modules["main"], unmounted, RefusalBodyNotResident},
		{"an absent trigger module", unmounted, link.modules["main"], RefusalTriggerNotResident},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			edges, refusal := activationRoutes(link.directory, testCase.trigger, testCase.body)
			if len(edges) != 0 {
				t.Fatalf("a refused route produced %d edges", len(edges))
			}
			if !refusal.Available() || refusal.Reason != testCase.reason {
				t.Fatalf("refusal is %s, want reason %s", refusal, testCase.reason)
			}
			if refusal.Trigger != testCase.trigger || refusal.Body != testCase.body {
				t.Fatalf("refusal names %s, want both module identities of the refused route", refusal)
			}
		})
	}
}

// A body another actor holds is resident, reached by no activation edge, and
// refuses nothing: the occurrence keeps the routes that remain. Residence and
// reachability are separate verdicts and only the first refuses.
func TestActivationRoutesLeaveAnotherActorsBodySilentlyUnreached(t *testing.T) {
	link := routeLawLink(t)
	edges, refusal := activationRoutes(link.directory, link.modules["main"], link.modules["worker"])
	if refusal.Available() {
		t.Fatalf("a resident body in another actor was refused as %s", refusal)
	}
	if len(edges) != 0 {
		t.Fatalf("a body in another actor contributed %d candidate edges", len(edges))
	}
}
