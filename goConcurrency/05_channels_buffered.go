package main

/*
================================================================================
CONCEPT 5: BUFFERED CHANNELS
================================================================================
ch := make(chan T, N)     // capacity N > 0 = buffered

A buffered channel is a fixed-size FIFO queue between goroutines:
  - SEND blocks ONLY when the buffer is FULL.
  - RECEIVE blocks ONLY when the buffer is EMPTY.
  - Sender and receiver are DECOUPLED — no rendezvous while there's room.

Use it for: smoothing bursts, job queues, semaphores (limiting concurrency),
and letting a producer finish without waiting on a slow consumer.

Run:  go run 05_channels_buffered.go
================================================================================
*/

import (
	"fmt"
	"sync"
	"time"
)

// ----------------------------------------------------------------------------
// EXAMPLE 1: sends don't block until the buffer is full
// ----------------------------------------------------------------------------
func basicBuffered() {
	ch := make(chan int, 3) // room for 3 values

	ch <- 1 // instant — buffer [1]
	ch <- 2 // instant — buffer [1 2]
	ch <- 3 // instant — buffer [1 2 3]  (FULL)
	fmt.Println("[Example 1] sent 3 values with NO receiver running")
	fmt.Println("[Example 1] len(ch) =", len(ch), "cap(ch) =", cap(ch)) // 3, 3

	// ch <- 4 // would BLOCK now (buffer full, no receiver) → deadlock here

	fmt.Println("[Example 1] drained:", <-ch, <-ch, <-ch) // FIFO order: 1 2 3
}

// ----------------------------------------------------------------------------
// EDGE CASE 2: full-buffer deadlock — buffered ≠ unlimited
// ----------------------------------------------------------------------------
func fullBufferDeadlock() {
	fmt.Println("[Edge 2] buffered channels still deadlock when FULL (commented):")
	// ch := make(chan int, 2)
	// ch <- 1
	// ch <- 2
	// ch <- 3 // fatal error: all goroutines are asleep - deadlock!
	//
	// The buffer only DELAYS blocking; it doesn't remove it. Sizing a buffer
	// "big enough so it never fills" is a design smell — under load it WILL
	// fill and your bug appears only in production.
	fmt.Println("        capacity 2, third send with no receiver = deadlock")
}

// ----------------------------------------------------------------------------
// EXAMPLE 3: decoupling — fast producer, slow consumer, burst absorption
// ----------------------------------------------------------------------------
func burstAbsorption() {
	jobs := make(chan int, 5) // buffer absorbs the burst
	var wg sync.WaitGroup
	wg.Add(1)

	// slow consumer
	go func() {
		defer wg.Done()
		for j := range jobs {
			time.Sleep(20 * time.Millisecond) // pretend slow processing
			fmt.Println("[Example 3] processed job", j)
		}
	}()

	start := time.Now()
	for i := 1; i <= 5; i++ {
		jobs <- i // all 5 fit in the buffer — producer doesn't wait for consumer
	}
	fmt.Printf("[Example 3] producer enqueued 5 jobs in %v (didn't wait!)\n", time.Since(start))
	close(jobs)
	wg.Wait()
}

// ----------------------------------------------------------------------------
// EXAMPLE 4: SEMAPHORE pattern — limit max concurrency
// ----------------------------------------------------------------------------
// A buffered channel of capacity N is a counting semaphore: at most N
// goroutines can "hold a token" simultaneously. THE production pattern for
// rate-limiting outbound calls, DB connections, file handles, etc.
func semaphore() {
	const maxConcurrent = 2
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup

	task := func(id int) {
		defer wg.Done()
		sem <- struct{}{}        // ACQUIRE token (blocks if 2 already running)
		defer func() { <-sem }() // RELEASE token

		fmt.Printf("[Example 4] task %d running (in-flight: %d)\n", id, len(sem))
		time.Sleep(50 * time.Millisecond)
	}

	for i := 1; i <= 6; i++ {
		wg.Add(1)
		go task(i)
	}
	wg.Wait()
	fmt.Println("[Example 4] all 6 done, never more than 2 at once")
}

// ----------------------------------------------------------------------------
// EXAMPLE 5: len() and cap() — inspection (and why not to trust them)
// ----------------------------------------------------------------------------
func lenCap() {
	ch := make(chan string, 4)
	ch <- "a"
	ch <- "b"
	fmt.Println("[Example 5] len =", len(ch), "(queued now), cap =", cap(ch), "(max)")
	// WARNING: in concurrent code len(ch) is stale the instant you read it —
	// another goroutine may send/receive in between. Use it for metrics and
	// debugging, NEVER for control flow like `if len(ch) < cap(ch) { ch <- x }`
	// (that check-then-send is a race). For non-blocking sends use select
	// with default (file 07).
	<-ch
	<-ch
}

// ----------------------------------------------------------------------------
// EXAMPLE 6: draining after close — buffered values are NOT lost
// ----------------------------------------------------------------------------
func drainAfterClose() {
	ch := make(chan int, 3)
	ch <- 10
	ch <- 20
	ch <- 30
	close(ch) // close with 3 values still inside

	// Receivers still get every buffered value, THEN the closed signal:
	for v := range ch {
		fmt.Println("[Example 6] drained after close:", v)
	}
	v, ok := <-ch
	fmt.Println("[Example 6] after drain: v =", v, "ok =", ok) // 0 false
	// Rule: close() means "no more SENDS", not "discard the queue".
}

// ----------------------------------------------------------------------------
// EXAMPLE 7: the classic WORKER POOL (jobs in, results out)
// ----------------------------------------------------------------------------
func workerPool() {
	const numWorkers = 3
	jobs := make(chan int, 10)
	results := make(chan string, 10)
	var wg sync.WaitGroup

	// spin up workers
	for w := 1; w <= numWorkers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := range jobs { // each worker pulls until jobs is closed & empty
				time.Sleep(10 * time.Millisecond)
				results <- fmt.Sprintf("worker %d finished job %d", id, j)
			}
		}(w)
	}

	// enqueue work
	for j := 1; j <= 6; j++ {
		jobs <- j
	}
	close(jobs) // tells workers "no more work" → their range loops exit

	// close results ONLY after all workers are done (else we might close early)
	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		fmt.Println("[Example 7]", r)
	}
}

// ----------------------------------------------------------------------------
// EDGE CASE 8: buffer size 1 — "mailbox" / latest-value patterns
// ----------------------------------------------------------------------------
func sizeOne() {
	// (a) Error mailbox: goroutine can report and EXIT even if nobody reads.
	errCh := make(chan error, 1)
	go func() {
		errCh <- fmt.Errorf("something broke")
		// with capacity 1 this send never blocks → no goroutine leak even if
		// the caller timed out and stopped listening. With an UNBUFFERED
		// channel + a caller that gave up, this goroutine would leak forever.
	}()
	fmt.Println("[Edge 8] mailbox got:", <-errCh)

	// (b) time.After / signal.Notify use buffered size-1 channels internally
	// for exactly this reason: the runtime can drop off a value and move on.
}

// ----------------------------------------------------------------------------
// EDGE CASE 9: choosing a buffer size — the honest answer
// ----------------------------------------------------------------------------
func choosingSize() {
	fmt.Println(`[Edge 9] Buffer sizing guide:
    0 (unbuffered) → default. You get synchronization guarantees for free.
    1              → mailbox / "don't leak the sender" / latest-value handoff.
    N = known limit→ semaphores (max concurrency), batch sizes you can PROVE.
    N = "big"      → usually a smell: hides backpressure until prod melts.
  If you can't explain WHY the number, use 0 or 1.`)
}

func main5() {
	fmt.Println("=========== BUFFERED CHANNELS DEEP DIVE ===========")
	basicBuffered()
	fullBufferDeadlock()
	burstAbsorption()
	semaphore()
	lenCap()
	drainAfterClose()
	workerPool()
	sizeOne()
	choosingSize()

	fmt.Println(`
KEY TAKEAWAYS:
1. Send blocks only when FULL; receive blocks only when EMPTY. FIFO order.
2. A buffer delays blocking — it never eliminates it. Full buffer = same deadlocks.
3. cap-N channel = counting semaphore: the standard "max N concurrent" pattern.
4. len(ch) is for observability only — using it for control flow is a race.
5. close() with queued values: receivers drain everything first, then see closed.
6. Worker pool: close(jobs) ends workers; wg.Wait() then close(results).
7. Size-1 buffers prevent sender leaks when the receiver might give up.
8. Can't justify the buffer size? Use 0 or 1.`)
}
