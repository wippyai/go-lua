package pipeline

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/returns"
	"github.com/wippyai/go-lua/compiler/check/store"
)

// fixpointProfile is only constructed when WIPPY_FIXPOINT_PROFILE=1. It is
// local to one Check call, so graph IDs from different modules cannot collide.
type fixpointProfile struct {
	mu                  sync.Mutex
	functions           map[uint64]*functionProfile
	rounds              int
	previousRoundOutput map[api.GraphKey]api.Facts
	sccs                []sccProfile
}

type sccProfile struct {
	ParentGraphID uint64         `json:"parent_graph_id"`
	OuterRound    uint64         `json:"outer_round"`
	Symbols       []cfg.SymbolID `json:"symbols"`
	Iterations    int            `json:"iterations"`
}

func (p *fixpointProfile) scc(parentGraphID, outerRound uint64, symbols []cfg.SymbolID, iterations int) {
	if p == nil {
		return
	}
	p.sccs = append(p.sccs, sccProfile{
		ParentGraphID: parentGraphID,
		OuterRound:    outerRound,
		Symbols:       append([]cfg.SymbolID(nil), symbols...),
		Iterations:    iterations,
	})
}

// FixpointProfile is an opaque profiling handle shared by the driver and
// function runner of a single checker invocation.
type FixpointProfile = fixpointProfile

func NewFixpointProfile() *FixpointProfile {
	if os.Getenv("WIPPY_FIXPOINT_PROFILE") != "1" {
		return nil
	}
	return &fixpointProfile{}
}

type functionProfile struct {
	GraphID             uint64      `json:"graph_id"`
	Line                int         `json:"line"`
	Analyses            int         `json:"analyses"`
	SameOwnFacts        int         `json:"same_own_facts"`
	SameInterprocOutput int         `json:"same_interproc_output"`
	ReturnNS            int64       `json:"returns_ns"`
	SynthNS             int64       `json:"synth_ns"`
	FlowNS              int64       `json:"flow_ns"`
	NarrowNS            int64       `json:"narrow_ns"`
	PostflowNS          int64       `json:"postflow_ns"`
	Rounds              map[int]int `json:"rounds"`
	previousParent      uint64
	previousOwnFacts    api.Facts
	previousOwnSeen     bool
}

func (p *fixpointProfile) function(id uint64, line int) *functionProfile {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.functions == nil {
		p.functions = make(map[uint64]*functionProfile)
	}
	f := p.functions[id]
	if f == nil {
		f = &functionProfile{GraphID: id, Line: line, Rounds: make(map[int]int)}
		p.functions[id] = f
	}
	return f
}

func (p *fixpointProfile) begin(id uint64, line, round int, parentHash uint64, st api.IterationStore) {
	if p == nil {
		return
	}
	f := p.function(id, line)
	f.Analyses++
	f.Rounds[round]++
	if concrete, ok := st.(*store.SessionStore); ok {
		current, seen := concrete.InterprocPrev.Facts[api.GraphKey{GraphID: id, ParentHash: parentHash}]
		if f.Analyses > 1 && f.previousParent == parentHash && f.previousOwnSeen == seen &&
			returns.FactsEqual(f.previousOwnFacts, current) {
			f.SameOwnFacts++
		}
		f.previousParent = parentHash
		f.previousOwnFacts = current
		f.previousOwnSeen = seen
	}
}

func (p *fixpointProfile) roundOutput(st api.IterationStore) {
	if p == nil {
		return
	}
	concrete, ok := st.(*store.SessionStore)
	if !ok || concrete.InterprocNext == nil {
		return
	}
	current := concrete.InterprocNext.Facts
	for key, facts := range current {
		if old, seen := p.previousRoundOutput[key]; seen && returns.FactsEqual(old, facts) {
			p.function(key.GraphID, 0).SameInterprocOutput++
		}
	}
	p.previousRoundOutput = current
}

func (p *fixpointProfile) phase(id uint64, name string, elapsed time.Duration) {
	if p == nil {
		return
	}
	f := p.function(id, 0)
	switch name {
	case "returns":
		f.ReturnNS += int64(elapsed)
	case "synth":
		f.SynthNS += int64(elapsed)
	case "flow":
		f.FlowNS += int64(elapsed)
	case "narrow":
		f.NarrowNS += int64(elapsed)
	case "postflow":
		f.PostflowNS += int64(elapsed)
	}
}

func (p *fixpointProfile) start() time.Time {
	if p == nil {
		return time.Time{}
	}
	return time.Now()
}

func (p *fixpointProfile) mark(id uint64, name string, start time.Time) {
	if p != nil {
		p.phase(id, name, time.Since(start))
	}
}

func (p *fixpointProfile) report(source string) {
	if p == nil {
		return
	}
	ids := make([]uint64, 0, len(p.functions))
	for id := range p.functions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	functions := make([]*functionProfile, 0, len(ids))
	for _, id := range ids {
		functions = append(functions, p.functions[id])
	}
	_ = json.NewEncoder(os.Stderr).Encode(struct {
		Source    string             `json:"source"`
		Rounds    int                `json:"rounds"`
		Functions []*functionProfile `json:"functions"`
		SCCs      []sccProfile       `json:"sccs"`
	}{source, p.rounds, functions, p.sccs})
}
