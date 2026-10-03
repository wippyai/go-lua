package lua

import "testing"

func TestRootProtectedCallDeliversErrorAfterSuspension(t *testing.T) {
	for _, call := range []string{
		`pcall(function() %s error("boom") end)`,
		`xpcall(function() %s error("boom") end, function(e) return "H:" .. tostring(e) end)`,
	} {
		for _, mode := range []string{"yield", "preempt"} {
			for _, into := range []bool{false, true} {
				body, budget := "coroutine.yield(1)", int64(-1)
				if mode == "preempt" {
					body, budget = slowBody, 3
				}
				src := "return " + sprintfBody(call, body)
				L := NewState()
				fn, err := L.LoadString(src)
				if err != nil {
					t.Fatal(err)
				}
				co, cancel := L.NewThread()
				var ret []LValue
				for i := 0; ; i++ {
					if i > 1000 {
						t.Fatal("no progress")
					}
					L.SetTickBudget(budget)
					var st ResumeState
					var err error
					if into {
						st, ret, err = L.ResumeInto(co, fn, make([]LValue, 0, 4))
					} else {
						st, ret, err = L.Resume(co, fn)
					}
					if err != nil {
						t.Fatalf("%s %s into=%v: resume error %v", call, mode, into, err)
					}
					if st == ResumeOK {
						break
					}
				}
				if len(ret) != 2 || ret[0] != LFalse || ret[1] == LNil {
					t.Fatalf("%s %s into=%v: got %v", call, mode, into, ret)
				}
				if !co.Dead || co.Parent != nil || L.G.CurrentThread != L {
					t.Fatalf("%s %s: inconsistent state dead=%v parent=%v", call, mode, co.Dead, co.Parent)
				}
				cancel()
				L.Close()
			}
		}
	}
}
