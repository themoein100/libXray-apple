//go:build ios

package memory

import (
	"runtime"
	"runtime/debug"
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
		go func() {
			for {
				time.Sleep(scavengeInterval)

				limit := memoryLimit.Load()
				if limit <= 0 {
					continue
				}

				// ReadMemStats briefly stops the world, so it is only worth
				// paying for at this cadence, never per second.
				var ms runtime.MemStats
				runtime.ReadMemStats(&ms)

				if ms.HeapInuse > uint64(limit/100*scavengeHighWatermarkPercent) {
					debug.FreeOSMemory()
				}
			}
		}()
	})
}
