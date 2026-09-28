//go:build ios

package memory

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"runtime/metrics"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// How often the scavenger goroutine is allowed to look at the heap.
	//
	// This used to be 1s, and every tick that found HeapInuse over half the
	// limit called debug.FreeOSMemory(). FreeOSMemory is a full stop-the-world
	// GC plus a complete heap scavenge — under real traffic HeapInuse is always
	// over that watermark, so the tunnel paid an STW pause and returned every
	// free page to the kernel once per second, forever, only to fault them all
	// back in during the next second. Combined with GOMAXPROCS(1) that pause
	// landed directly on the thread carrying packets, which is what made the
	// tunnel feel like it stuttered about once a second.
	//
	// The scavenger is kept — an iOS Network Extension really can be jetsammed
	// for holding memory — but it now runs rarely and only when the heap is
	// genuinely near the ceiling. debug.SetMemoryLimit below is the real
	// backstop; this is just belt-and-braces for returning pages to the OS.
	scavengeInterval = 30 * time.Second

	// Fraction of the limit above which a scavenge is worth its cost.
	scavengeHighWatermarkPercent = 90

	// Upper bound on GOMAXPROCS.
	//
	// This used to be pinned to 1 on the theory that a packet tunnel is purely
	// I/O-bound. It is not: TLS record encryption/decryption, WebSocket
	// framing, the fragmenter and the whole gVisor network stack are CPU work
	// in the data path, and with a single P they serialise against each other
	// *and* against the garbage collector. Two Ps let GC overlap with packet
	// forwarding while still keeping per-P mcache/stack overhead small enough
	// for the extension's memory budget.
	maxProcs = 2
)

// Go heap budget for an iOS packet-tunnel process. This is only the Go runtime
// budget; Swift, gVisor, socket queues and Xray native allocations need the
// remaining process headroom.
//
// The previous default was 10 MB. Xray's real live heap with TLS buffers, the
// gVisor netstack and a parsed geosite.dat sits around that figure on its own,
// so the soft limit was permanently in reach and Go answered by running the
// collector back-to-back (the "GC death spiral") — burning up to half the
// available CPU on collection instead of moving traffic.
var memoryLimit atomic.Int64

func init() {
	memoryLimit.Store(36 * 1024 * 1024)
}

// SetMemoryLimitMB overrides the Go heap ceiling. Safe to call at any time.
func SetMemoryLimitMB(mb int64) {
	limit := mb * 1024 * 1024
	memoryLimit.Store(limit)
	debug.SetMemoryLimit(limit)
}

var initOnce sync.Once

func InitForceFree() {
	// Let the heap double before collecting. The memory limit below is the
	// actual ceiling; a low GC percentage on top of it only adds collections
	// that the limit would have triggered anyway. The old value of 20 meant a
	// collection for every 20% of heap growth on a heap that was already being
	// held against its limit.
	debug.SetGCPercent(100)
	debug.SetMemoryLimit(memoryLimit.Load())

	if runtime.NumCPU() > maxProcs {
		runtime.GOMAXPROCS(maxProcs)
	}

	// Goroutine must only be started once — each RunXray call would otherwise add another.
	initOnce.Do(func() {
		// Bytes in in-use heap spans: the metrics equivalent of MemStats.HeapInuse.
		heapSample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
		go func() {
			for {
				time.Sleep(scavengeInterval)

				limit := memoryLimit.Load()
				if limit <= 0 {
					continue
				}

				// runtime/metrics reads the heap without stopping the world,
				// unlike ReadMemStats, so the check itself no longer pauses the
				// goroutines carrying packets. FreeOSMemory still does, which is
				// why it stays behind the high watermark.
				metrics.Read(heapSample)
				if heapSample[0].Value.Kind() != metrics.KindUint64 {
					continue
				}

				if heapSample[0].Value.Uint64() > uint64(limit/100*scavengeHighWatermarkPercent) {
					debug.FreeOSMemory()
				}
			}
		}()
	})
}

// FreeOSMemory forces a collection and returns every free page to the OS.
//
// It stops the world, so it is not for the steady state (the scavenger above
// deliberately avoids it). It exists for the moments an iOS Network Extension
// is about to be jetsammed: after the Xray core has been stopped, the old
// core's heap is garbage the runtime would otherwise keep mapped — a device
// run held 33–38 MB after a full stop — and while the footprint sits near the
// ceiling, returning idle spans is cheaper than losing the process.
func FreeOSMemory() {
	debug.FreeOSMemory()
}

var reportSamples = []metrics.Sample{
	{Name: "/memory/classes/total:bytes"},
	{Name: "/memory/classes/heap/objects:bytes"},
	{Name: "/memory/classes/heap/unused:bytes"},
	{Name: "/memory/classes/heap/free:bytes"},
	{Name: "/memory/classes/heap/released:bytes"},
	{Name: "/memory/classes/heap/stacks:bytes"},
	{Name: "/memory/classes/os-stacks:bytes"},
	{Name: "/sched/goroutines:goroutines"},
	{Name: "/gc/cycles/total:gc-cycles"},
}

var reportMu sync.Mutex

// Report summarises the Go runtime's memory without stopping the world, so the
// process footprint can be split into what Go holds and what lives outside it.
func Report() string {
	reportMu.Lock()
	defer reportMu.Unlock()
	metrics.Read(reportSamples)
	mb := func(i int) float64 {
		if reportSamples[i].Value.Kind() != metrics.KindUint64 {
			return -1
		}
		return float64(reportSamples[i].Value.Uint64()) / (1 << 20)
	}
	count := func(i int) uint64 {
		if reportSamples[i].Value.Kind() != metrics.KindUint64 {
			return 0
		}
		return reportSamples[i].Value.Uint64()
	}
	return fmt.Sprintf(
		"goTotal=%.1fMB live=%.1fMB fragmented=%.1fMB freeMapped=%.1fMB released=%.1fMB stacks=%.1fMB osStacks=%.1fMB goroutines=%d gcCycles=%d limit=%dMB",
		mb(0), mb(1), mb(2), mb(3), mb(4), mb(5), mb(6), count(7), count(8), memoryLimit.Load()>>20,
	)
}
