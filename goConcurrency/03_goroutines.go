package main

/*
================================================================================
CONCEPT 3: GOROUTINES
================================================================================
A goroutine is a lightweight thread managed by the Go runtime (NOT the OS).
  - Starts with ~2KB stack (OS threads: ~1MB) that grows/shrinks as needed
  - Millions can run in one process
  - Multiplexed onto OS threads by Go's scheduler (G-M-P model)
  - Created with a single keyword: `go someFunc()`

Run:            go run 03_goroutines.go
With race det.: go run -race 03_goroutines.go
================================================================================
*/

import (
	"fmt"
	"runtime"
	"sync"
	"time"
)

// ----------------------------------------------------------------------------
// EXAMPLE 1: The basics — `go` runs a function concurrently
// ----------------------------------------------------------------------------
func sayHello(name string) {
	fmt.Println("[Example 1] hello from", name)
}

func basicGoroutine() {
	go sayHello("goroutine") // scheduled to run concurrently — returns instantly
	sayHello("main")         // runs right now, synchronously

	// crude sync for demo only — real code uses WaitGroups/channels (see later files)
	time.Sleep(50 * time.Millisecond)
}

// ----------------------------------------------------------------------------
// EDGE CASE 2: main() exits → ALL goroutines are killed instantly
// ----------------------------------------------------------------------------
// Goroutines are not "background jobs" that outlive the program. When main
// returns, the process dies — running goroutines get NO chance to finish,
// no cleanup, no deferred calls.
func mainExitsFirst() {
	go func() {
		time.Sleep(1 * time.Hour) // pretend long job
		fmt.Println("[Edge 2] you will NEVER see this line")
	}()
	fmt.Println("[Edge 2] main doesn't wait — without sleep/WaitGroup, that goroutine dies silently")
	// (we deliberately don't sleep an hour here!)
}

// ----------------------------------------------------------------------------
// EDGE CASE 3: THE loop-variable capture bug (and Go 1.22's fix)
// ----------------------------------------------------------------------------
func loopVariableCapture() {
	var wg sync.WaitGroup

	// Go 1.22+ : each iteration gets a FRESH `i`, so this now prints 0..4 (in
	// some order) — correct. In Go <= 1.21 all goroutines shared ONE `i` and
	// typically all printed 5. This was the single most common Go bug.
	fmt.Println("[Edge 3] closure capture (correct on Go 1.22+):")
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Println("   captured i =", i)
		}()
	}
	wg.Wait()

	// The version that is correct on ALL Go versions — pass i as a parameter,
	// snapshotting its value at spawn time. Still the safest habit:
	fmt.Println("[Edge 3] parameter passing (correct everywhere):")
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fmt.Println("   param n =", n)
		}(i)
	}
	wg.Wait()
}

// ----------------------------------------------------------------------------
// EXAMPLE 4: goroutines interleave — order is NOT guaranteed
// ----------------------------------------------------------------------------
func interleaving() {
	var wg sync.WaitGroup
	worker := func(id string) {
		defer wg.Done()
		for i := 1; i <= 3; i++ {
			fmt.Printf("[Example 4] worker %s step %d\n", id, i)
			time.Sleep(time.Millisecond) // yield so we visibly interleave
		}
	}
	wg.Add(2)
	go worker("A")
	go worker("B")
	wg.Wait()
	// Run it multiple times — A and B interleave differently each run.
	// NEVER write code that depends on goroutine scheduling order.
}

// ----------------------------------------------------------------------------
// EDGE CASE 5: shared state between goroutines = data race
// ----------------------------------------------------------------------------
// (Full treatment in the mutex file.) Rule of thumb — Go proverb:
// "Don't communicate by sharing memory; share memory by communicating."
func sharedStateRace() {
	total := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 1; i <= 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			mu.Lock()
			total += n // safe only because of the mutex
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	fmt.Println("[Edge 5] sum 1..100 computed concurrently =", total, "(5050)")
}

// ----------------------------------------------------------------------------
// EDGE CASE 6: GOROUTINE LEAKS — goroutines blocked forever
// ----------------------------------------------------------------------------
// A goroutine that blocks on a channel nobody will ever write to (or read
// from) never exits and never gets garbage collected. In a server this
// leaks memory request-by-request until OOM.
func goroutineLeak() {
	before := runtime.NumGoroutine()

	leakyCh := make(chan int) // nobody ever sends on this
	go func() {
		<-leakyCh // blocks FOREVER → leaked goroutine
		fmt.Println("never printed")
	}()

	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()
	fmt.Printf("[Edge 6] goroutines before=%d after=%d — the +1 is a LEAK\n", before, after)

	// FIXES in real code:
	//   a) use context.Context and select { case <-ctx.Done(): return ... }
	//   b) close channels so blocked receivers unblock
	//   c) use buffered channels when the receiver might bail early
}

// ----------------------------------------------------------------------------
// EDGE CASE 7: a panic inside ANY goroutine crashes the WHOLE program
// ----------------------------------------------------------------------------
// recover() in main does NOT catch panics from other goroutines. Each
// goroutine must recover for itself (this is what HTTP servers do per-request).
func panicIsolation() {
	done := make(chan bool)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Println("[Edge 7] goroutine recovered ITSELF from:", r)
			}
			done <- true
		}()
		panic("boom inside goroutine")
		// Without the deferred recover above, this panic would kill the
		// entire process — main's recover cannot save you.
	}()
	<-done
}

// ----------------------------------------------------------------------------
// EXAMPLE 8: how many goroutines can we spawn? (they're CHEAP)
// ----------------------------------------------------------------------------
func cheapness() {
	var wg sync.WaitGroup
	n := 100_000
	start := time.Now()
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// tiny bit of "work"
			_ = n * 2
		}(i)
	}
	wg.Wait()
	fmt.Printf("[Example 8] spawned & finished %d goroutines in %v\n", n, time.Since(start))
	// Try doing that with OS threads. (Don't.)
}

// ----------------------------------------------------------------------------
// EXAMPLE 9: peeking at the scheduler — GOMAXPROCS & NumGoroutine
// ----------------------------------------------------------------------------
func schedulerInfo() {
	fmt.Println("[Example 9] CPU cores available :", runtime.NumCPU())
	fmt.Println("[Example 9] GOMAXPROCS (P's)    :", runtime.GOMAXPROCS(0)) // 0 = query only
	fmt.Println("[Example 9] live goroutines now :", runtime.NumGoroutine())
	// G-M-P model in one line: G (goroutines) are queued onto P (logical
	// processors, = GOMAXPROCS) which are executed by M (OS threads).
	// Blocking syscalls move the M aside; the P picks up another G. That's
	// why 100k goroutines doing network I/O only need a handful of threads.
}

// ----------------------------------------------------------------------------
// EXAMPLE 10: fire-and-forget vs supervised goroutines (production pattern)
// ----------------------------------------------------------------------------
func supervised() {
	// Anti-pattern: `go doThing()` with no way to know it failed.
	// Pattern: return errors over a channel.
	errCh := make(chan error, 1)

	go func() {
		// pretend work that fails
		errCh <- fmt.Errorf("upload failed: disk full")
	}()

	select {
	case err := <-errCh:
		fmt.Println("[Example 10] supervised goroutine reported:", err)
	case <-time.After(time.Second):
		fmt.Println("[Example 10] timed out waiting for worker")
	}
}

func main3() {
	fmt.Println("=========== GOROUTINES DEEP DIVE ===========")
	basicGoroutine()
	mainExitsFirst()
	loopVariableCapture()
	interleaving()
	sharedStateRace()
	goroutineLeak()
	panicIsolation()
	cheapness()
	schedulerInfo()
	supervised()

	fmt.Println(`
KEY TAKEAWAYS:
1. ` + "`go f()`" + ` returns immediately; f runs concurrently, order not guaranteed.
2. main() exiting kills ALL goroutines instantly — always synchronize (WaitGroup).
3. Loop-variable capture: fixed in Go 1.22, but passing params is bulletproof.
4. Shared state needs a mutex or a channel — run -race in CI, always.
5. Blocked-forever goroutines LEAK — use context cancellation to guarantee exit.
6. A panic in any goroutine kills the process; each goroutine recovers for itself.
7. Goroutines are ~2KB — spawning 100k is fine; leaking 100k is not.`)
}
