package lua

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// Line coverage built on the debug hook (hook.go). Enabled by setting
// WIPPY_COVERAGE in the environment before the process starts. When on, every
// LState is armed with a line hook (covArm) so the hook's line-event path in
// callHook records each executing source line — coverage is a hook consumer,
// not a separate instrumentation path. The denominator (coverable lines,
// including never-executed functions) is gathered from every compiled prototype
// at CompileWithOptions time. WriteCoverageLCOV emits a standard LCOV tracefile.
var (
	covOn        = os.Getenv("WIPPY_COVERAGE") != ""
	covMu        sync.Mutex
	covHits      = map[string]map[int]bool{}
	covCoverable = map[string]map[int]bool{}
)

// CoverageEnabled reports whether coverage collection is active.
func CoverageEnabled() bool { return covOn }

// covArm turns on the line hook for a freshly created/pooled LState so the hook
// records coverage. No-op unless WIPPY_COVERAGE is set, so non-coverage runs and
// explicit debug.sethook users are unaffected.
func covArm(ls *LState) {
	if covOn {
		ls.hookMask |= HookMaskLine
		ls.hookLastLine = 0
	}
}

// covRecordHit is called from the hook's line-event path in callHook.
func covRecordHit(src string, line int) {
	if line <= 0 || src == "" {
		return
	}
	covMu.Lock()
	m := covHits[src]
	if m == nil {
		m = map[int]bool{}
		covHits[src] = m
	}
	m[line] = true
	covMu.Unlock()
}

func covRegisterProto(p *FunctionProto) {
	if !covOn || p == nil {
		return
	}
	covMu.Lock()
	covRegisterProtoLocked(p)
	covMu.Unlock()
}

func covRegisterProtoLocked(p *FunctionProto) {
	if p == nil {
		return
	}
	if p.SourceName != "" {
		m := covCoverable[p.SourceName]
		if m == nil {
			m = map[int]bool{}
			covCoverable[p.SourceName] = m
		}
		for _, ln := range p.DbgSourcePositions {
			if ln > 0 {
				m[ln] = true
			}
		}
	}
	for _, child := range p.FunctionPrototypes {
		covRegisterProtoLocked(child)
	}
}

// WriteCoverageLCOV writes an LCOV tracefile for every registered source whose
// name satisfies filter (all sources if filter is nil). The denominator is the
// set of coverable lines gathered from every compiled prototype; the numerator
// is the set of lines the line hook observed executing.
func WriteCoverageLCOV(path string, filter func(src string) bool) error {
	covMu.Lock()
	defer covMu.Unlock()

	srcs := make([]string, 0, len(covCoverable))
	for s := range covCoverable {
		if filter == nil || filter(s) {
			srcs = append(srcs, s)
		}
	}
	sort.Strings(srcs)

	var b strings.Builder
	for _, s := range srcs {
		cov := covCoverable[s]
		hit := covHits[s]
		nums := make([]int, 0, len(cov))
		for ln := range cov {
			nums = append(nums, ln)
		}
		sort.Ints(nums)
		b.WriteString("SF:" + s + "\n")
		lh := 0
		for _, ln := range nums {
			c := 0
			if hit != nil && hit[ln] {
				c = 1
				lh++
			}
			b.WriteString("DA:" + strconv.Itoa(ln) + "," + strconv.Itoa(c) + "\n")
		}
		b.WriteString("LF:" + strconv.Itoa(len(nums)) + "\n")
		b.WriteString("LH:" + strconv.Itoa(lh) + "\n")
		b.WriteString("end_of_record\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// CoverageSummary returns aggregate (linesFound, linesHit) over filtered sources.
func CoverageSummary(filter func(src string) bool) (lf int, lh int) {
	covMu.Lock()
	defer covMu.Unlock()
	for s, cov := range covCoverable {
		if filter != nil && !filter(s) {
			continue
		}
		hit := covHits[s]
		for ln := range cov {
			lf++
			if hit != nil && hit[ln] {
				lh++
			}
		}
	}
	return lf, lh
}
