package main

/*
================================================================================
CONCEPT 2: DEFER
================================================================================
`defer` schedules a function call to run when the SURROUNDING FUNCTION returns
(normally, via early return, or via panic). It's Go's cleanup mechanism:
closing files, unlocking mutexes, closing DB rows, recovering from panics.

Three rules to burn into memory:
  RULE 1: Deferred calls run LIFO (last-in, first-out — like a stack).
  RULE 2: Arguments are evaluated IMMEDIATELY at the defer line, not at exit.
  RULE 3: Deferred funcs can read AND MODIFY named return values.

Run:  go run 02_defer.go
================================================================================
*/

import (
	"fmt"
	"os"
)

// ----------------------------------------------------------------------------
// EXAMPLE 1: Basic ordering — LIFO (stack order)
// ----------------------------------------------------------------------------
func lifoOrder() {
	fmt.Println("[Example 1] function start")
	defer fmt.Println("[Example 1] deferred FIRST  (prints LAST)")
	defer fmt.Println("[Example 1] deferred SECOND (prints middle)")
	defer fmt.Println("[Example 1] deferred THIRD  (prints FIRST)")
	fmt.Println("[Example 1] function end")
	// Output order: start, end, THIRD, SECOND, FIRST
	// Why LIFO? Cleanup should unwind in reverse of setup:
	//   open A, open B  →  close B, close A
}

// ----------------------------------------------------------------------------
// EXAMPLE 2 (RULE 2): Arguments evaluated AT the defer statement
// ----------------------------------------------------------------------------
func argEvaluation() {
	x := 10
	defer fmt.Println("[Example 2] deferred sees x =", x) // x=10 captured NOW
	x = 99
	fmt.Println("[Example 2] before return, x =", x) // 99
	// The deferred line still prints 10, because arguments were snapshotted
	// at the moment `defer` executed.
}

// ----------------------------------------------------------------------------
// EXAMPLE 3: ...but CLOSURES capture variables by REFERENCE
// ----------------------------------------------------------------------------
func closureCapture() {
	x := 10
	// No argument passed — the closure references x itself, so it sees the
	// FINAL value at execution time.
	defer func() {
		fmt.Println("[Example 3] closure sees x =", x) // prints 99
	}()
	x = 99
}

// ----------------------------------------------------------------------------
// EXAMPLE 4 (RULE 3): defer can MODIFY named return values
// ----------------------------------------------------------------------------
// This is how you wrap/annotate errors, or convert panics into errors.
func doubleIt(n int) (result int) { // <-- NAMED return value
	defer func() {
		result *= 2 // runs AFTER `return n`, mutates the named result
	}()
	return n // sets result = n, THEN defer doubles it
}

// Classic real-world use: wrapping errors with context on the way out.
func readConfig(path string) (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("readConfig(%s): %w", path, err)
		}
	}()
	_, err = os.Open(path) // will fail for a fake path
	return err
}

func namedReturnExample() {
	fmt.Println("[Example 4] doubleIt(5) =", doubleIt(5)) // 10, not 5
	fmt.Println("[Example 4] wrapped error:", readConfig("/no/such/file.yaml"))
}

// EDGE CASE: with UNNAMED returns, defer cannot change what's returned.
func cannotModify() int {
	result := 5
	defer func() { result *= 2 }() // modifies the local var AFTER it was copied out
	return result                  // returns 5 — the return value was already set
}

// ----------------------------------------------------------------------------
// EXAMPLE 5: The classic use — resource cleanup
// ----------------------------------------------------------------------------
func fileCleanup() {
	f, err := os.CreateTemp("", "defer-demo-*.txt")
	if err != nil {
		fmt.Println("[Example 5] temp file error:", err)
		return
	}
	// Idiom: acquire, check error, IMMEDIATELY defer the release.
	defer os.Remove(f.Name()) // 2nd deferred → runs last (LIFO)
	defer f.Close()           // 1st deferred → runs first: close, THEN remove

	f.WriteString("hello defer")
	fmt.Println("[Example 5] wrote to", f.Name(), "— close & remove happen automatically")
}

// ----------------------------------------------------------------------------
// EXAMPLE 6: defer + panic + recover — Go's "try/catch"
// ----------------------------------------------------------------------------
// Deferred functions STILL RUN during a panic. recover() only works inside
// a deferred function, and stops the panic from crashing the program.
func safeDivide(a, b int) (result int, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered from panic: %v", r)
		}
	}()
	result = a / b // b == 0 → runtime panic: integer divide by zero
	return result, nil
}

func panicRecoverExample() {
	r1, err1 := safeDivide(10, 2)
	fmt.Println("[Example 6] 10/2 =", r1, "err:", err1)
	r2, err2 := safeDivide(10, 0)
	fmt.Println("[Example 6] 10/0 =", r2, "err:", err2) // no crash!
}

// ----------------------------------------------------------------------------
// EDGE CASE 7: defer inside a LOOP — deferred calls pile up
// ----------------------------------------------------------------------------
// Defers run at FUNCTION exit, not loop-iteration exit. Deferring inside a
// big loop (e.g. opening 10,000 files) holds ALL resources until the end.
func deferInLoopBad() {
	for i := 0; i < 3; i++ {
		defer fmt.Println("[Edge 7-bad] deferred i =", i) // all 3 pile up
	}
	fmt.Println("[Edge 7-bad] loop done — NOW the 3 defers fire in reverse (2,1,0)")
}

// FIX: wrap the body in a function so defer runs per-iteration.
func deferInLoopGood() {
	for i := 0; i < 3; i++ {
		func(i int) {
			defer fmt.Println("[Edge 7-good] cleaned up i =", i, "immediately")
			// ... use resource for iteration i ...
		}(i)
	}
}

// ----------------------------------------------------------------------------
// EDGE CASE 8: deferred method calls — receiver evaluated at defer time
// ----------------------------------------------------------------------------
type Logger struct{ name string }

func (l Logger) Log() { fmt.Println("[Edge 8] logger name:", l.name) }

func receiverSnapshot() {
	l := Logger{name: "before"}
	defer l.Log() // l (value receiver) copied NOW → prints "before"
	l.name = "after"
}

// ----------------------------------------------------------------------------
// EDGE CASE 9: os.Exit SKIPS all deferred calls
// ----------------------------------------------------------------------------
// defer fmt.Println("never printed")
// os.Exit(1)  // process dies instantly, no defers, no panic unwinding
// Same story: log.Fatal (it calls os.Exit(1) internally). Be careful mixing
// log.Fatal with defers that flush buffers or remove temp files.

// ----------------------------------------------------------------------------
// EDGE CASE 10: deferring a nil function — panics AT EXECUTION time
// ----------------------------------------------------------------------------
func nilDefer() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("[Edge 10] recovered:", r) // "invalid memory address..."
		}
	}()
	var f func()
	defer f() // legal to write; panics when the defer actually RUNS at exit
	fmt.Println("[Edge 10] body finished normally — the panic happens after this")
}

// ----------------------------------------------------------------------------
// EDGE CASE 11: evaluating the error of a deferred Close — the loop-hole
// ----------------------------------------------------------------------------
// `defer f.Close()` DISCARDS Close's error. Usually fine for reads; for
// WRITES the flush can fail on Close and you'd silently lose data.
func writeFile(path string, data []byte) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := f.Close()
		if err == nil { // don't overwrite a more important earlier error
			err = closeErr
		}
	}()
	_, err = f.Write(data)
	return err
}

func closeErrorExample() {
	tmp := os.TempDir() + "/defer-close-demo.txt"
	err := writeFile(tmp, []byte("data"))
	os.Remove(tmp)
	fmt.Println("[Edge 11] writeFile err:", err, "(Close error captured via named return)")
}

func main2() {
	fmt.Println("=========== DEFER DEEP DIVE ===========")
	lifoOrder()
	argEvaluation()
	closureCapture()
	namedReturnExample()
	fmt.Println("[Example 4] cannotModify() =", cannotModify(), "(unnamed return → defer can't change it)")
	fileCleanup()
	panicRecoverExample()
	deferInLoopBad()
	deferInLoopGood()
	receiverSnapshot()
	nilDefer()
	closeErrorExample()

	fmt.Println(`
KEY TAKEAWAYS:
1. LIFO order — cleanup unwinds in reverse of setup.
2. defer f(x): x is snapshotted NOW. defer func(){ use x }(): x read at exit.
3. Named returns + defer = error wrapping & panic→error conversion.
4. Defers run at FUNCTION exit — wrap loop bodies in funcs for per-iteration cleanup.
5. Defers survive panics (that's how recover works) but NOT os.Exit/log.Fatal.
6. defer f.Close() swallows the error — capture it for writes.
7. Acquire → check err → defer release, all in 3 consecutive lines. Every time.`)
}
