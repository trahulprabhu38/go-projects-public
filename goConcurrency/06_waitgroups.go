package main

/*
================================================================================
CONCEPT 6: sync.WaitGroup
================================================================================
A WaitGroup is a concurrency counter used to wait for a COLLECTION of
goroutines to finish. Three methods:

  wg.Add(n)  → increment the counter by n (announce n pending tasks)
  wg.Done()  → decrement by 1 (a task finished) — it's just Add(-1)
  wg.Wait()  → block until the counter hits 0

Golden rules:
  RULE 1: Call Add BEFORE starting the goroutine (in the parent).
  RULE 2: Call Done exactly once per Add — via defer, first line.
  RULE 3: Pass WaitGroups by POINTER. Never copy one.
  RULE 4: Counter going negative = panic. Reuse only after Wait returns.

Run:  go run 06_waitgroups.go   (and with -race)
================================================================================
*/

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ----------------------------------------------------------------------------
// EXAMPLE 1: the canonical pattern
// ----------------------------------------------------------------------------
func canonical() {
	var wg sync.WaitGroup

	for i := 1; i <= 3; i++ {
		wg.Add(1) // BEFORE the go statement, in the parent goroutine
		go func(id int) {
			defer wg.Done() // FIRST line inside — runs even on panic/early return
			time.Sleep(time.Duration(id*10) * time.Millisecond)
			fmt.Println("[Example 1] worker", id, "done")
		}(i)
	}

	wg.Wait() // parks main until counter == 0
	fmt.Println("[Example 1] all workers finished — main continues")
}

// ----------------------------------------------------------------------------
// EDGE CASE 2: THE BUG — calling Add INSIDE the goroutine (race with Wait)
// ----------------------------------------------------------------------------
func addInsideGoroutine() {
	fmt.Println("[Edge 2] Add inside the goroutine is a race (shown conceptually):")
	// var wg sync.WaitGroup
	// for i := 0; i < 3; i++ {
	//     go func() {
	//         wg.Add(1)      // ← may run AFTER wg.Wait() below!
	//         defer wg.Done()
	//         work()
	//     }()
	// }
	// wg.Wait() // counter may still be 0 → Wait returns INSTANTLY,
	//           // main exits, goroutines killed mid-flight. Flaky as hell.
	fmt.Println("        Wait() can see counter==0 before any goroutine ran Add → returns early")
	fmt.Println("        FIX: Add(1) in the parent, before `go`")
}

// ----------------------------------------------------------------------------
// EDGE CASE 3: copying a WaitGroup — pass by POINTER
// ----------------------------------------------------------------------------
// WaitGroup contains internal state; copying it forks that state. Done() on
// the copy never decrements the original → Wait() blocks forever.
// `go vet` flags this (copylocks).

// BAD: value parameter receives a COPY
// func workerBroken(wg sync.WaitGroup, id int) { defer wg.Done(); ... }

// GOOD: pointer
func workerOK(wg *sync.WaitGroup, id int) {
	defer wg.Done()
	fmt.Println("[Edge 3] worker", id, "ran (WaitGroup passed by pointer)")
}

func pointerPassing() {
	var wg sync.WaitGroup
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go workerOK(&wg, i) // &wg — share the ONE WaitGroup
	}
	wg.Wait()
	// Note: closures (Example 1) capture wg by reference automatically,
	// which is why the closure style doesn't hit this bug.
}

// ----------------------------------------------------------------------------
// EDGE CASE 4: negative counter = immediate panic
// ----------------------------------------------------------------------------
func negativeCounter() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[Edge 4] recovered:", r) // "sync: negative WaitGroup counter"
		}
	}()
	var wg sync.WaitGroup
	wg.Add(1)
	wg.Done()
	wg.Done() // one Done too many → panic
}

// ----------------------------------------------------------------------------
// EDGE CASE 5: forgetting Done (or a panic before Done) = Wait hangs forever
// ----------------------------------------------------------------------------
func forgottenDone() {
	fmt.Println("[Edge 5] a worker that never calls Done → Wait() hangs forever:")
	// wg.Add(1)
	// go func() {
	//     work()          // if this panics, or you just forgot Done...
	//     wg.Done()       // ...this never runs
	// }()
	// wg.Wait()           // deadlocks: "all goroutines are asleep"
	fmt.Println("        this is WHY `defer wg.Done()` must be the FIRST line, always")
}

// ----------------------------------------------------------------------------
// EXAMPLE 6: Add(n) once vs Add(1) per iteration — both valid
// ----------------------------------------------------------------------------
func addN() {
	var wg sync.WaitGroup
	n := 4
	wg.Add(n) // one bulk Add when you KNOW the count up front
	for i := 1; i <= n; i++ {
		go func(id int) {
			defer wg.Done()
			_ = id * id
		}(i)
	}
	wg.Wait()
	fmt.Println("[Example 6] bulk Add(4) worked — prefer Add(1) per spawn when the count is dynamic")
}

// ----------------------------------------------------------------------------
// EXAMPLE 7: collecting RESULTS — WaitGroup + channel together
// ----------------------------------------------------------------------------
// WaitGroup answers "are they done?"; channels carry the data. The trick:
// a closer goroutine calls Wait then close, so main can just range.
func fanInResults() {
	urls := []string{"https://example.com/a", "https://example.com/b", "https://example.com/c"}
	results := make(chan string, len(urls)) // buffered: workers never block on send
	var wg sync.WaitGroup

	for _, u := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			time.Sleep(15 * time.Millisecond) // pretend fetch
			results <- "fetched " + url
		}(u)
	}

	// closer: converts "counter hit 0" into "channel closed"
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results { // exits cleanly when closer closes the channel
		fmt.Println("[Example 7]", r)
	}
}

// ----------------------------------------------------------------------------
// EXAMPLE 8: reusing a WaitGroup — legal, but only in strict phases
// ----------------------------------------------------------------------------
func reuse() {
	var wg sync.WaitGroup

	for phase := 1; phase <= 2; phase++ {
		for i := 1; i <= 2; i++ {
			wg.Add(1)
			go func(p, id int) {
				defer wg.Done()
				fmt.Printf("[Example 8] phase %d worker %d\n", p, id)
			}(phase, i)
		}
		wg.Wait() // counter back to 0 → safe to reuse for the next phase
	}
	// EDGE CASE: calling Add while another goroutine is inside Wait (counter
	// possibly at 0) is a race per the docs — new Adds must "happen after"
	// the previous Wait returns. Phased reuse like above is the safe shape.
}

// ----------------------------------------------------------------------------
// EXAMPLE 9: real-world shape — bounded parallel health checks
// ----------------------------------------------------------------------------
// Combines everything: WaitGroup (completion), buffered results (data),
// semaphore (max concurrency). This is the skeleton of half the SRE tools
// you'll ever write.
func healthCheck() {
	endpoints := []string{"svc-a", "svc-b", "svc-c", "svc-d", "svc-e"}
	sem := make(chan struct{}, 2) // at most 2 concurrent "probes"
	type status struct {
		name string
		ok   bool
	}
	results := make(chan status, len(endpoints))
	var wg sync.WaitGroup

	for _, ep := range endpoints {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			time.Sleep(20 * time.Millisecond)                 // pretend HTTP probe
			results <- status{name, len(name)%2 == 1 || true} // pretend all healthy
		}(ep)
	}

	go func() { wg.Wait(); close(results) }()

	healthy := 0
	for s := range results {
		if s.ok {
			healthy++
		}
	}
	fmt.Printf("[Example 9] %d/%d endpoints healthy (max 2 probed at a time)\n",
		healthy, len(endpoints))
	_ = http.StatusOK // (imported to hint where the real probe would go)
}

func main6() {
	fmt.Println("=========== WAITGROUPS DEEP DIVE ===========")
	canonical()
	addInsideGoroutine()
	pointerPassing()
	negativeCounter()
	forgottenDone()
	addN()
	fanInResults()
	reuse()
	healthCheck()

	fmt.Println(`
KEY TAKEAWAYS:
1. Add in the PARENT before ` + "`go`" + `; ` + "`defer wg.Done()`" + ` as the worker's first line.
2. Add inside the goroutine races with Wait → Wait can return before work starts.
3. Never copy a WaitGroup — pass *sync.WaitGroup (closures capture by ref, fine).
4. Extra Done → panic (negative counter). Missing Done → Wait hangs forever.
5. WaitGroup = "done yet?"; channel = the data. Closer goroutine bridges them:
   go func(){ wg.Wait(); close(results) }()
6. Reuse only between clean phases (after Wait fully returns).
7. WaitGroup + buffered results + semaphore = the standard bounded-fanout recipe.`)
}
