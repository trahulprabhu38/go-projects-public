package main

/*
================================================================================
CONCEPT 4: UNBUFFERED CHANNELS
================================================================================
ch := make(chan T)        // capacity 0 = unbuffered

An unbuffered channel is a RENDEZVOUS point:
  - A SEND (ch <- v) blocks until another goroutine is ready to RECEIVE.
  - A RECEIVE (<-ch) blocks until another goroutine is ready to SEND.
  - The handoff is simultaneous — sender and receiver "meet".

This makes unbuffered channels a SYNCHRONIZATION tool, not just a data pipe:
when the send completes, you KNOW the receiver has the value.

Run:  go run 04_channels_unbuffered.go
================================================================================
*/

import (
	"fmt"
	"time"
)

// ----------------------------------------------------------------------------
// EXAMPLE 1: basic send/receive — the rendezvous
// ----------------------------------------------------------------------------
func basicRendezvous() {
	ch := make(chan string) // unbuffered

	go func() {
		fmt.Println("[Example 1] goroutine: about to send (will block until main receives)")
		ch <- "hello" // blocks here until main executes <-ch
		fmt.Println("[Example 1] goroutine: send completed — receiver definitely has it")
	}()

	time.Sleep(50 * time.Millisecond) // prove the sender is waiting on us
	msg := <-ch                       // the rendezvous happens on this line
	fmt.Println("[Example 1] main received:", msg)
	time.Sleep(10 * time.Millisecond)
}

// ----------------------------------------------------------------------------
// EDGE CASE 2: DEADLOCK — send with no receiver (same goroutine)
// ----------------------------------------------------------------------------
func deadlockSend() {
	fmt.Println("[Edge 2] the classic beginner deadlock (shown commented):")
	// ch := make(chan int)
	// ch <- 1        // fatal error: all goroutines are asleep - deadlock!
	// <-ch           // never reached
	//
	// Why: the send blocks main forever waiting for a receiver, but the ONLY
	// receiver is the next line of main — which can never run.
	// Fixes: (a) receive in another goroutine, (b) use a buffered channel.
	fmt.Println("        ch <- 1 in main with no other goroutine = instant deadlock")
}

// ----------------------------------------------------------------------------
// EXAMPLE 3: using an unbuffered channel purely as a SIGNAL ("done" pattern)
// ----------------------------------------------------------------------------
func doneSignal() {
	done := make(chan struct{}) // struct{} = zero bytes; "pure signal, no data"

	go func() {
		fmt.Println("[Example 3] worker: doing job...")
		time.Sleep(30 * time.Millisecond)
		close(done) // closing broadcasts to ALL receivers, and it's idiomatic
	}()

	<-done // blocks until the channel is closed (receive on closed ch returns immediately)
	fmt.Println("[Example 3] main: worker signalled completion")
}

// ----------------------------------------------------------------------------
// EXAMPLE 4: sequencing — unbuffered channels enforce lock-step ping-pong
// ----------------------------------------------------------------------------
func pingPong() {
	ping := make(chan int)
	pong := make(chan int)

	go func() { // player B
		for i := 0; i < 3; i++ {
			n := <-ping
			fmt.Println("[Example 4] B got", n, "→ hitting back")
			pong <- n + 1
		}
	}()

	for i := 0; i < 3; i++ { // player A (main)
		ping <- i * 10
		back := <-pong
		fmt.Println("[Example 4] A got", back, "back")
	}
	// Neither side can "run ahead" — every exchange is synchronized.
}

// ----------------------------------------------------------------------------
// EXAMPLE 5: close() semantics + the comma-ok idiom
// ----------------------------------------------------------------------------
func closeSemantics() {
	ch := make(chan int)

	go func() {
		for i := 1; i <= 3; i++ {
			ch <- i
		}
		close(ch) // ONLY the sender should close. Never the receiver.
	}()

	// Receiving from a CLOSED channel never blocks: returns zero value + ok=false
	for {
		v, ok := <-ch
		if !ok {
			fmt.Println("[Example 5] channel closed — ok=false, v is zero value:", v)
			break
		}
		fmt.Println("[Example 5] received:", v)
	}

	// EDGE CASES around close:
	// close(ch) twice          → panic: close of closed channel
	// ch <- 1 after close      → panic: send on closed channel
	// var c chan int; close(c) → panic: close of nil channel
	// This is why: sender owns the channel, sender closes it, exactly once.
}

// ----------------------------------------------------------------------------
// EXAMPLE 6: `for range` over a channel — the clean receive loop
// ----------------------------------------------------------------------------
func rangeOverChannel() {
	ch := make(chan string)
	go func() {
		defer close(ch) // range EXITS only when channel is closed — don't forget!
		for _, w := range []string{"go", "is", "fun"} {
			ch <- w
		}
	}()

	for word := range ch { // receives until close; no comma-ok needed
		fmt.Println("[Example 6] ranged:", word)
	}
	// EDGE CASE: if the sender forgets close(ch), this range blocks forever
	// → deadlock (if nothing else runs) or a goroutine leak.
}

// ----------------------------------------------------------------------------
// EDGE CASE 7: nil channels block FOREVER (send and receive)
// ----------------------------------------------------------------------------
func nilChannel() {
	var ch chan int // nil — declared but never make()'d
	fmt.Println("[Edge 7] a nil channel's send AND receive block forever (shown commented):")
	_ = ch
	// <-ch   // blocks forever
	// ch <- 1 // blocks forever
	// Surprisingly USEFUL: in a select{}, setting a channel variable to nil
	// permanently disables that case (see the select file, 07).
}

// ----------------------------------------------------------------------------
// EXAMPLE 8: directional channel types — compiler-enforced roles
// ----------------------------------------------------------------------------
// chan<- int : send-only    <-chan int : receive-only
// A bidirectional channel converts to either automatically. This documents
// intent AND the compiler stops you closing/receiving where you shouldn't.
func producer(out chan<- int, n int) { // can ONLY send on out
	for i := 1; i <= n; i++ {
		out <- i * i
	}
	close(out) // sender closes — legal on send-only channel
	// x := <-out // compile error: cannot receive from send-only channel
}

func consumer(in <-chan int) { // can ONLY receive from in
	for v := range in {
		fmt.Println("[Example 8] consumed square:", v)
	}
	// close(in) // compile error: cannot close receive-only channel — nice!
}

func directionalExample() {
	ch := make(chan int)
	go producer(ch, 3)
	consumer(ch)
}

// ----------------------------------------------------------------------------
// EXAMPLE 9: handoff guarantee — why unbuffered ≠ buffered-with-size-1
// ----------------------------------------------------------------------------
func handoffGuarantee() {
	unbuf := make(chan int)
	go func() {
		unbuf <- 42
		// This line CANNOT print before main has the value:
		fmt.Println("[Example 9] unbuffered: send returned ⇒ receiver HAS the value")
	}()
	v := <-unbuf
	fmt.Println("[Example 9] main got", v)
	time.Sleep(10 * time.Millisecond)

	buf := make(chan int, 1)
	buf <- 99 // returns IMMEDIATELY — value parked in buffer, maybe never read
	fmt.Println("[Example 9] buffered(1): send returned but NO ONE has received yet")
	<-buf
	// Use unbuffered when you need the "message delivered" guarantee;
	// buffered when you want decoupling (next file, 05).
}

func main4() {
	fmt.Println("=========== UNBUFFERED CHANNELS DEEP DIVE ===========")
	basicRendezvous()
	deadlockSend()
	doneSignal()
	pingPong()
	closeSemantics()
	rangeOverChannel()
	nilChannel()
	directionalExample()
	handoffGuarantee()

	fmt.Println(`
KEY TAKEAWAYS:
1. Unbuffered = capacity 0 = rendezvous. Send and receive block until both arrive.
2. Send with no receiver in the same goroutine = classic deadlock.
3. close(done) is the idiomatic "broadcast completion" signal (chan struct{}).
4. Receive from closed channel: instant zero value; use ` + "`v, ok := <-ch`" + `.
5. Sender closes, exactly once. Double-close / send-after-close = panic.
6. for range ch loops until close — forgetting close leaks/deadlocks the receiver.
7. nil channels block forever (a feature inside select, a bug everywhere else).
8. Directional types (chan<-, <-chan) let the compiler enforce protocol roles.`)
}
