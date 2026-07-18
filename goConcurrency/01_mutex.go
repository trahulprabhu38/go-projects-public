package main

/*
================================================================================
CONCEPT 1: MUTEX (sync.Mutex and sync.RWMutex)
================================================================================
A Mutex (MUTual EXclusion lock) protects shared data from being accessed by
multiple goroutines at the same time. Only ONE goroutine can hold the lock
at any moment. Everyone else calling Lock() BLOCKS until Unlock() is called.

Run with race detector to see the difference:
    go run -race 01_mutex.go
================================================================================
*/

import (
	"fmt"
	"sync"
	"time"
)

// ----------------------------------------------------------------------------
// EXAMPLE 1: THE PROBLEM — a data race WITHOUT a mutex
// ----------------------------------------------------------------------------
// counter++ is NOT atomic. It is actually 3 steps:
//  1. read counter
//  2. add 1
//  3. write counter back
//
// Two goroutines can both read the same value, both add 1, and both write
// back the SAME result — one increment is lost.
func raceCondition() {
	counter := 0
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counter++ // DATA RACE! run with -race and Go will scream at you
		}()
	}
	wg.Wait()
	// Expected 1000, but you'll often get 950, 987, etc.
	fmt.Println("[Example 1] WITHOUT mutex, counter =", counter, "(expected 1000 — often wrong!)")
}

// ----------------------------------------------------------------------------
// EXAMPLE 2: THE FIX — protect the critical section with sync.Mutex
// ----------------------------------------------------------------------------
func withMutex() {
	counter := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()   // acquire the lock — others wait here
			counter++   // critical section: only one goroutine at a time
			mu.Unlock() // release the lock
		}()
	}
	wg.Wait()
	fmt.Println("[Example 2] WITH mutex, counter =", counter, "(always 1000)")
}

// ----------------------------------------------------------------------------
// EXAMPLE 3: BEST PRACTICE — defer mu.Unlock()
// ----------------------------------------------------------------------------
// If the code between Lock and Unlock panics or returns early, you'd never
// unlock → every other goroutine deadlocks forever. defer guarantees the
// unlock runs no matter how the function exits.
type SafeBank struct {
	mu      sync.Mutex
	balance map[string]int
}

func (b *SafeBank) Deposit(user string, amt int) {
	b.mu.Lock()
	defer b.mu.Unlock() // runs even if code below panics
	b.balance[user] += amt
}

func (b *SafeBank) Withdraw(user string, amt int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.balance[user] < amt {
		// early return — defer STILL unlocks. Without defer this would deadlock.
		return fmt.Errorf("insufficient funds for %s", user)
	}
	b.balance[user] -= amt
	return nil
}

func deferUnlockExample() {
	bank := &SafeBank{balance: make(map[string]int)}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bank.Deposit("rahul", 10)
		}()
	}
	wg.Wait()
	err := bank.Withdraw("rahul", 5000) // more than balance → early return path
	fmt.Println("[Example 3] balance =", bank.balance["rahul"], "| withdraw error:", err)
}

// ----------------------------------------------------------------------------
// EXAMPLE 4: sync.RWMutex — many readers OR one writer
// ----------------------------------------------------------------------------
// If your data is read a LOT and written rarely (caches, config), a plain
// Mutex forces readers to queue one-by-one. RWMutex allows:
//   - unlimited concurrent RLock() holders (readers)
//   - exactly one Lock() holder (writer), and no readers while writing
type ConfigCache struct {
	mu   sync.RWMutex
	data map[string]string
}

func (c *ConfigCache) Get(key string) string {
	c.mu.RLock() // read lock — many goroutines can hold this together
	defer c.mu.RUnlock()
	return c.data[key]
}

func (c *ConfigCache) Set(key, val string) {
	c.mu.Lock() // write lock — exclusive
	defer c.mu.Unlock()
	c.data[key] = val
}

func rwMutexExample() {
	cache := &ConfigCache{data: map[string]string{"env": "prod"}}
	var wg sync.WaitGroup

	// 5 concurrent readers — they do NOT block each other
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = cache.Get("env")
		}(i)
	}
	// 1 writer — waits for all readers to release, then blocks new readers
	wg.Add(1)
	go func() {
		defer wg.Done()
		cache.Set("env", "staging")
	}()
	wg.Wait()
	fmt.Println("[Example 4] RWMutex final value:", cache.Get("env"))
}

// ----------------------------------------------------------------------------
// EDGE CASE 5: DEADLOCK — locking a mutex you already hold
// ----------------------------------------------------------------------------
// Go mutexes are NOT reentrant (unlike Java's synchronized). If the same
// goroutine calls Lock() twice, it blocks on itself forever.
func deadlockDemo() {
	var mu sync.Mutex
	mu.Lock()
	fmt.Println("[Edge 5] first Lock acquired")

	// mu.Lock() // <-- UNCOMMENT: fatal error: all goroutines are asleep - deadlock!
	// Common real-world version of this bug: a locked method calling another
	// locked method on the same struct:
	//   func (s *S) A() { s.mu.Lock(); defer s.mu.Unlock(); s.B() }
	//   func (s *S) B() { s.mu.Lock(); ... }  // deadlock when called via A!
	// Fix: have B's logic in an unexported unlocked helper that both call.

	mu.Unlock()
	fmt.Println("[Edge 5] (double-lock line is commented out — it would deadlock)")
}

// ----------------------------------------------------------------------------
// EDGE CASE 6: NEVER COPY a mutex (or a struct containing one)
// ----------------------------------------------------------------------------
// A copied mutex is a DIFFERENT mutex. Two goroutines "locking" two copies
// protect nothing. `go vet` catches this (copylocks check).
type Counter struct {
	mu sync.Mutex
	n  int
}

// BAD: value receiver copies the struct (and the mutex inside it)
// func (c Counter) IncBroken() { c.mu.Lock(); c.n++; c.mu.Unlock() }

// GOOD: pointer receiver — everyone shares the same mutex
func (c *Counter) Inc() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func copyMutexExample() {
	c := &Counter{}
	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.Inc() }()
	}
	wg.Wait()
	fmt.Println("[Edge 6] pointer-receiver counter =", c.n, "(500, correct)")
}

// ----------------------------------------------------------------------------
// EDGE CASE 7: unlocking an unlocked mutex = FATAL runtime error
// ----------------------------------------------------------------------------
// This is nastier than a normal panic: "fatal error: sync: unlock of
// unlocked mutex" is a runtime THROW — recover() CANNOT catch it. The
// process dies, period. (Same category as deadlock detection and stack
// overflow.) That's why the demo below is commented out.
func unlockUnlocked() {
	// var mu sync.Mutex
	// mu.Unlock() // fatal error: sync: unlock of unlocked mutex — UNRECOVERABLE
	fmt.Println("[Edge 7] Unlock without Lock = fatal error that recover() can't catch (commented)")
	// Also fatal: Unlocking a mutex locked by checking twice, or unlocking
	// in a different goroutine than locked IS allowed (mutexes aren't owner-
	// tracked), but unlocking more times than locked is always fatal.
}

// ----------------------------------------------------------------------------
// EXAMPLE 8: TryLock (Go 1.18+) — attempt without blocking
// ----------------------------------------------------------------------------
// Useful for "skip if busy" patterns (e.g. a cron job that shouldn't overlap
// with itself). NOTE: the Go team says most uses of TryLock are a design
// smell — prefer channels or redesign. But it exists.
func tryLockExample() {
	var mu sync.Mutex
	mu.Lock()

	go func() {
		if mu.TryLock() {
			fmt.Println("[Example 8] goroutine got the lock (unexpected)")
			mu.Unlock()
		} else {
			fmt.Println("[Example 8] TryLock failed — lock busy, skipping work instead of blocking")
		}
	}()
	time.Sleep(50 * time.Millisecond)
	mu.Unlock()
}

// ----------------------------------------------------------------------------
// EDGE CASE 9: keep critical sections SMALL — don't hold locks during I/O
// ----------------------------------------------------------------------------
func smallCriticalSection() {
	var mu sync.Mutex
	shared := []string{}

	slowNetworkCall := func() string {
		time.Sleep(10 * time.Millisecond) // pretend HTTP call
		return "result"
	}

	// BAD: lock held during the slow call → everyone serialized behind I/O
	// mu.Lock()
	// shared = append(shared, slowNetworkCall())
	// mu.Unlock()

	// GOOD: do slow work OUTSIDE the lock, only guard the shared write
	res := slowNetworkCall()
	mu.Lock()
	shared = append(shared, res)
	mu.Unlock()

	fmt.Println("[Edge 9] shared =", shared, "(lock held only for the append)")
}

func main() {
	fmt.Println("=========== MUTEX DEEP DIVE ===========")
	raceCondition()
	withMutex()
	deferUnlockExample()
	rwMutexExample()
	deadlockDemo()
	copyMutexExample()
	unlockUnlocked()
	tryLockExample()
	smallCriticalSection()

	fmt.Println(`
KEY TAKEAWAYS:
1. counter++ is not atomic — unprotected shared writes are data races.
2. Lock() then defer Unlock() immediately — survives panics & early returns.
3. RWMutex: many readers OR one writer. Use for read-heavy data.
4. Go mutexes are NOT reentrant — double Lock in one goroutine = deadlock.
5. Never copy a mutex: use pointer receivers; 'go vet' catches copies.
6. Unlocking an unlocked mutex panics at runtime.
7. Keep critical sections tiny; never hold a lock across network/disk I/O.
8. Always test concurrent code with: go test -race / go run -race`)
}
