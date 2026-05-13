//go:build ios

package memory

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const (
	gcInterval = 1 * time.Second
)

var memoryLimit int64 = 30 * 1024 * 1024 // default 30MB

// SetMemoryLimitMB overrides the Go heap ceiling. Safe to call at any time.
func SetMemoryLimitMB(mb int64) {
	memoryLimit = mb * 1024 * 1024
	debug.SetMemoryLimit(memoryLimit)
}

var initOnce sync.Once

func InitForceFree() {
	// Trigger GC after 20% heap growth — tighter than Go default (100%) but not CPU-burning.
	debug.SetGCPercent(20)
	// Hard-ish ceiling: Go runtime will GC aggressively to stay under this.
	debug.SetMemoryLimit(memoryLimit)
	// Limit to 1 OS thread — NE workload is I/O-bound, reduces scheduler + thread-stack overhead.
	runtime.GOMAXPROCS(1)
	// Goroutine must only be started once — each RunXray call would otherwise add another.
	initOnce.Do(func() {
		go func() {
			for {
				time.Sleep(gcInterval)
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)
				// Only pay the full scavenge cost when heap is over 1/2 of the limit.
				if ms.HeapInuse > uint64(memoryLimit/2) {
					debug.FreeOSMemory()
				}
			}
		}()
	})
}
