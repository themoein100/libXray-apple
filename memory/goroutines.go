package memory

import (
	"bytes"
	"fmt"
	"runtime/pprof"
	"sort"
	"strings"
)

// GoroutineSummary groups live goroutines by the first frame that belongs to
// Xray, gVisor or x/net (falling back to the first non-runtime frame) and
// returns the `top` largest groups, e.g. "412×gvisor.dev/.../tcp.(*endpoint).Read".
//
// A device run kept ~830 goroutines alive after the Xray core had been fully
// stopped, holding ~14 MB of live heap. This names where they are parked.
// Takes a goroutine profile (a brief stop-the-world), so only call it in an
// emergency path.
func GoroutineSummary(top int) string {
	var buf bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&buf, 1); err != nil {
		return "goroutine profile failed: " + err.Error()
	}
	counts := map[string]int{}
	total := 0
	var pending int
	var key string
	flush := func() {
		if pending > 0 {
			if key == "" {
				key = "?"
			}
			counts[key] += pending
			total += pending
		}
		pending, key = 0, ""
	}
	isNoise := func(fn string) bool {
		for _, p := range []string{"runtime.", "runtime/", "internal/", "sync.", "sync/", "time.", "io.", "bufio.", "net.", "os.", "crypto/", "syscall."} {
			if strings.HasPrefix(fn, p) {
				return true
			}
		}
		return false
	}
	preferred := func(fn string) bool {
		return strings.Contains(fn, "xray-core") || strings.Contains(fn, "gvisor.dev") || strings.Contains(fn, "golang.org/x/net") || strings.Contains(fn, "libxray")
	}
	var fallback string
	for _, line := range strings.Split(buf.String(), "\n") {
		if n, ok := strings.CutPrefix(line, ""); ok && len(n) > 0 && n[0] >= '0' && n[0] <= '9' && strings.Contains(n, " @ ") {
			if key == "" {
				key = fallback
			}
			flush()
			fallback = ""
			fmt.Sscanf(n, "%d", &pending)
			continue
		}
		if !strings.HasPrefix(line, "#\t") || pending == 0 {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		fn := fields[2]
		if i := strings.LastIndex(fn, "+0x"); i > 0 {
			fn = fn[:i]
		}
		if key == "" && preferred(fn) {
			key = fn
		}
		if fallback == "" && !isNoise(fn) {
			fallback = fn
		}
	}
	if key == "" {
		key = fallback
	}
	flush()

	type group struct {
		fn string
		n  int
	}
	groups := make([]group, 0, len(counts))
	for fn, n := range counts {
		groups = append(groups, group{fn, n})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].n > groups[j].n })
	if len(groups) > top {
		groups = groups[:top]
	}
	parts := make([]string, 0, len(groups))
	for _, g := range groups {
		fn := g.fn
		if i := strings.LastIndex(fn, "/"); i >= 0 {
			fn = fn[i+1:]
		}
		parts = append(parts, fmt.Sprintf("%d×%s", g.n, fn))
	}
	return fmt.Sprintf("total=%d top: %s", total, strings.Join(parts, ", "))
}
