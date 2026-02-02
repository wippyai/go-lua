package lua

import "sync"

// SpawnRequest is yielded by coroutine.spawn to signal the scheduler
type SpawnRequest struct {
	Fn *LFunction
}

func (s *SpawnRequest) String() string   { return "spawn-request" }
func (s *SpawnRequest) Type() LValueType { return LTUserData }

var spawnRequestPool = sync.Pool{
	New: func() any {
		return &SpawnRequest{}
	},
}

// ReleaseSpawnRequest returns a SpawnRequest to the pool.
func ReleaseSpawnRequest(sr *SpawnRequest) {
	sr.Fn = nil
	spawnRequestPool.Put(sr)
}

func OpenCoroutine(L *LState) int {
	mod := L.RegisterGoModule(CoroutineLibName, coFuncs).(*LTable)
	L.Push(mod)
	return 1
}

var coFuncs = map[string]LGoFunc{
	"create":  coCreate,
	"yield":   coYield,
	"resume":  coResume,
	"running": coRunning,
	"status":  coStatus,
	"wrap":    coWrap,
	"spawn":   coSpawn,
}

func coSpawn(L *LState) int {
	fn := L.CheckFunction(1)
	req := spawnRequestPool.Get().(*SpawnRequest)
	req.Fn = fn
	L.Push(req)
	return -1 // yield with the SpawnRequest
}

func coCreate(L *LState) int {
	fn := L.CheckFunction(1)
	newthread := L.NewThreadWithContext(L.ctx)
	newthread.stack.Push(callFrame{
		Fn:         fn,
		Pc:         0,
		Base:       0,
		LocalBase:  1,
		ReturnBase: 0,
		NArgs:      0,
		NRet:       MultRet,
		TailCall:   0,
	})
	L.Push(newthread)
	return 1
}

func coYield(_ *LState) int {
	return -1
}

func coResume(L *LState) int {
	th := L.CheckThread(1)
	if L.G.CurrentThread == th {
		msg := "can not resume a running thread"
		if th.wrapped {
			L.RaiseError(msg)
			return 0
		}
		L.Push(LFalse)
		L.Push(LString(msg))
		return 2
	}
	if th.Dead {
		msg := "can not resume a dead thread"
		if th.wrapped {
			L.RaiseError(msg)
			return 0
		}
		L.Push(LFalse)
		L.Push(LString(msg))
		return 2
	}
	th.Parent = L
	L.G.CurrentThread = th
	if !th.isStarted() {
		cf := th.stack.Last()
		th.currentFrame = cf
		th.SetTop(0)
		nargs := L.GetTop() - 1
		L.XMoveTo(th, nargs)
		cf.NArgs = int16(nargs)
		th.initCallFrame(cf)
		th.Panic = panicWithoutTraceback
	} else {
		nargs := L.GetTop() - 1
		L.XMoveTo(th, nargs)
	}
	top := L.GetTop()
	threadRun(th)
	return L.GetTop() - top
}

func coRunning(L *LState) int {
	if L.G.MainThread == L {
		L.Push(LNil)
		return 1
	}
	L.Push(L.G.CurrentThread)
	return 1
}

func coStatus(L *LState) int {
	L.Push(LString(L.Status(L.CheckThread(1))))
	return 1
}

func wrapaux(L *LState) int {
	L.Insert(L.ToThread(UpvalueIndex(1)), 1)
	return coResume(L)
}

func coWrap(L *LState) int {
	coCreate(L)
	L.CheckThread(L.GetTop()).wrapped = true
	v := L.Get(L.GetTop())
	L.Pop(1)
	L.Push(L.NewClosure(wrapaux, v))
	return 1
}
