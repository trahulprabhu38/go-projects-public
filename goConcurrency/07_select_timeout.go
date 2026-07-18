package main

/*
================================================================================
CONCEPT 7: SELECT & TIMEOUTS
================================================================================
`select` is `switch` for CHANNEL operations. It waits on MULTIPLE channel
sends/receives simultaneously and runs the first case that becomes ready.

Core semantics:
  - Blocks until at least one case is ready.
  - If MULTIPLE cases are ready at once → picks one UNIFORMLY AT RANDOM
    (prevents starvation — you cannot rely on case order!).
  - `default` case → makes the whole select NON-BLOCKING.
  - select {} with no cases → blocks forever.
  - Cases on nil channels are silently DISABLED (never ready).

Timeouts are built by racing your work-channel against a timer channel
(time.After / time.NewTimer / context.WithTimeout).

Run:  go run 07_select_timeout.go
================================================================================
*/

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ----------------------------------------------------------------------------
// EXAMPLE 1: basic select — first ready case wins
// ----------------------------------------------------------------------------
func basicSelect() {
	fast := make(chan string)
	slow := make(chan string)

	go func() { time.Sleep(10 * time.Millisecond); fast <- "fast result" }()
	go func() { time.Sleep(50 * time.Millisecond); slow <- "slow result" }()

	select {
	case msg := <-fast:
		fmt.Println("[Example 1] got:", msg) // this one — it was ready first
	case msg := <-slow:
		fmt.Println("[Example 1] got:", msg)
	}
	// NOTE: the slow goroutine is now blocked on `slow <-` forever = LEAK.
	// Real code uses buffered channels or context cancellation (Example 6).
}

// ----------------------------------------------------------------------------
// EXAMPLE 2: random choice when several cases are ready — no priority!
// ----------------------------------------------------------------------------
func randomChoice() {
	counts := map[string]int{}
	for i := 0; i < 6; i++ {
		a := make(chan string, 1)
		b := make(chan string, 1)
		a <- "A"
		b <- "B" // BOTH cases ready before select runs

		select { // both ready → runtime picks uniformly at random
		case v := <-a:
			counts[v]++
		case v := <-b:
			counts[v]++
		}
	}
	fmt.Println("[Example 2] picks over 6 rounds:", counts, "— roughly random, NEVER assume order")
	// If you need priority, nest selects:
	//   select { case v := <-high: ... default:
	//       select { case v := <-high: ... case v := <-low: ... } }
}

// ----------------------------------------------------------------------------
// EXAMPLE 3: `default` — non-blocking send/receive (poll, don't wait)
// ----------------------------------------------------------------------------
func nonBlocking() {
	ch := make(chan int, 1)

	// non-blocking RECEIVE
	select {
	case v := <-ch:
		fmt.Println("[Example 3] received", v)
	default:
		fmt.Println("[Example 3] nothing available — moving on (didn't block)")
	}

	// non-blocking SEND (this is how you do "try-send" — NOT via len/cap!)
	ch <- 1 // fill the buffer
	select {
	case ch <- 2:
		fmt.Println("[Example 3] sent 2")
	default:
		fmt.Println("[Example 3] buffer full — dropped the value instead of blocking")
		// exactly how metrics libraries drop samples under load
	}
	<-ch
}

// ----------------------------------------------------------------------------
// EXAMPLE 4: TIMEOUT with time.After — race work vs the clock
// ----------------------------------------------------------------------------
func timeoutBasic() {
	work := func(d time.Duration) chan string {
		ch := make(chan string, 1) // buffered! see edge note below
		go func() {
			time.Sleep(d)
			ch <- "work finished"
		}()
		return ch
	}

	// Case A: work (20ms) beats timeout (100ms)
	select {
	case res := <-work(20 * time.Millisecond):
		fmt.Println("[Example 4A]", res)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("[Example 4A] timed out")
	}

	// Case B: timeout (30ms) beats work (200ms)
	select {
	case res := <-work(200 * time.Millisecond):
		fmt.Println("[Example 4B]", res)
	case <-time.After(30 * time.Millisecond):
		fmt.Println("[Example 4B] timed out after 30ms")
		// EDGE: the worker goroutine still completes later. Because its
		// channel is BUFFERED(1), its send succeeds and it exits — no leak.
		// With an unbuffered channel it would block forever = goroutine leak.
	}
}

// ----------------------------------------------------------------------------
// EDGE CASE 5: time.After in a LOOP leaks timers — use time.NewTimer/Ticker
// ----------------------------------------------------------------------------
func timerInLoop() {
	// BAD (before Go 1.23 this leaked memory; still wasteful):
	//   for { select { case <-ch: ...
	//                  case <-time.After(time.Minute): ... } }
	// A fresh timer is allocated EVERY iteration.

	// GOOD: one timer, reset each round:
	ch := make(chan int)
	go func() {
		for i := 1; i <= 2; i++ {
			time.Sleep(15 * time.Millisecond)
			ch <- i
		}
		close(ch)
	}()

	timer := time.NewTimer(50 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				fmt.Println("[Edge 5] channel closed — exiting loop")
				return
			}
			fmt.Println("[Edge 5] got", v, "(resetting idle timer)")
			if !timer.Stop() {
				select { // drain if it already fired
				case <-timer.C:
				default:
				}
			}
			timer.Reset(50 * time.Millisecond)
		case <-timer.C:
			fmt.Println("[Edge 5] idle timeout — no message for 50ms")
			return
		}
	}
}

// ----------------------------------------------------------------------------
// EXAMPLE 6: context.WithTimeout — the PRODUCTION way to time out
// ----------------------------------------------------------------------------
// time.After times out ONE select. Context propagates cancellation through
// your whole call tree (HTTP clients, DB drivers, gRPC all accept it), so
// the worker can actually STOP instead of burning CPU for a caller that left.
func slowAPI(ctx context.Context) (string, error) {
	select {
	case <-time.After(200 * time.Millisecond): // pretend the API takes 200ms
		return "api payload", nil
	case <-ctx.Done(): // fires on timeout OR explicit cancel
		return "", ctx.Err() // context.DeadlineExceeded / context.Canceled
	}
}

func contextTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel() // ALWAYS defer cancel — releases the timer even on success

	res, err := slowAPI(ctx)
	if errors.Is(err, context.DeadlineExceeded) {
		fmt.Println("[Example 6] request timed out via context:", err)
	} else {
		fmt.Println("[Example 6] got:", res)
	}
}

// ----------------------------------------------------------------------------
// EXAMPLE 7: nil channels DISABLE cases — dynamic select composition
// ----------------------------------------------------------------------------
// Receiving from nil blocks forever, so a nil-channel case can never fire.
// Trick: set a channel variable to nil once it's exhausted → cleanly merge
// two streams without busy-looping on the closed one.
func nilDisables() {
	odds := make(chan int)
	evens := make(chan int)
	go func() {
		for _, v := range []int{1, 3, 5} {
			odds <- v
		}
		close(odds)
	}()
	go func() {
		for _, v := range []int{2, 4} {
			evens <- v
		}
		close(evens)
	}()

	for odds != nil || evens != nil { // loop until BOTH disabled
		select {
		case v, ok := <-odds:
			if !ok {
				odds = nil // exhausted → this case can never fire again
				continue
			}
			fmt.Println("[Example 7] odd :", v)
		case v, ok := <-evens:
			if !ok {
				evens = nil
				continue
			}
			fmt.Println("[Example 7] even:", v)
		}
	}
	fmt.Println("[Example 7] both streams drained")
	// WITHOUT the nil trick, a closed channel is ALWAYS ready (returns 0,false
	// instantly), so select would spin on it in a hot loop. Big gotcha.
}

// ----------------------------------------------------------------------------
// EXAMPLE 8: heartbeat / periodic work — select + time.Ticker
// ----------------------------------------------------------------------------
func heartbeat() {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop() // ALWAYS stop tickers — they leak otherwise
	done := make(chan struct{})

	go func() { time.Sleep(70 * time.Millisecond); close(done) }()

	beats := 0
	for {
		select {
		case <-ticker.C:
			beats++
			fmt.Println("[Example 8] 💓 heartbeat", beats)
		case <-done:
			fmt.Println("[Example 8] shutdown signal — sent", beats, "beats")
			return
		}
	}
}

// ----------------------------------------------------------------------------
// EDGE CASE 9: select{} and single-case selects
// ----------------------------------------------------------------------------
func degenerateSelects() {
	fmt.Println("[Edge 9] select{} with zero cases blocks FOREVER (used to park main in daemons)")
	// select {} // uncomment → fatal deadlock here (nothing else running)

	// A select with ONE case and no default behaves exactly like a plain
	// channel op — pointless. Reach for select only for: multiple channels,
	// timeouts, or default (non-blocking).
}

func main7() {
	fmt.Println("=========== SELECT & TIMEOUT DEEP DIVE ===========")
	basicSelect()
	randomChoice()
	nonBlocking()
	timeoutBasic()
	timerInLoop()
	contextTimeout()
	nilDisables()
	heartbeat()
	degenerateSelects()

	fmt.Println(`
KEY TAKEAWAYS:
1. select waits on many channel ops; first ready wins; ties broken RANDOMLY.
2. default = non-blocking (try-send/try-receive; drop-on-full patterns).
3. Timeout = race work vs time.After. Give the worker a BUFFERED channel or
   it leaks when the timeout wins.
4. time.After inside loops wastes timers — use time.NewTimer + Reset (drain first!).
5. Production timeouts = context.WithTimeout + defer cancel(); check ctx.Done()
   inside workers so they truly stop.
6. Closed channels are ALWAYS ready in select → set them to nil to disable
   the case, or you'll spin at 100% CPU.
7. Stop your Tickers/Timers. select{} blocks forever. Single-case select is noise.`)
}
