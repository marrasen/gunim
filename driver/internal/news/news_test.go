package news

import (
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestOneThatStartsToWatchHearsTheLastThenEach(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := Latest[int]{First: func(v int) int { return -v }}
		l.Set(1)
		l.Set(2)
		var heard []int
		stop := l.Watch(func(v int) { heard = append(heard, v) })
		synctest.Wait()
		l.Set(3)
		synctest.Wait()
		stop()
		l.Set(4)
		synctest.Wait()
		if !slices.Equal(heard, []int{-2, 3}) {
			t.Fatalf("heard %v", heard)
		}
	})
}

func TestASlowOneHearsTheMergedLast(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := Latest[int]{Merge: func(old, next int) int { return old | next }}
		var heard []int
		l.Watch(func(v int) { time.Sleep(time.Second); heard = append(heard, v) })
		l.Set(1)
		synctest.Wait()
		l.Set(2)
		l.Set(4)
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if !slices.Equal(heard, []int{1, 6}) {
			t.Fatalf("heard %v", heard)
		}
	})
}

func TestStopFromWithinAndTwice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var l Latest[int]
		var calls atomic.Int32
		var stop func()
		stop = l.Watch(func(int) { calls.Add(1); stop(); stop() })
		l.Set(1)
		synctest.Wait()
		l.Set(2)
		synctest.Wait()
		if calls.Load() != 1 {
			t.Fatalf("%d calls", calls.Load())
		}
	})
}

func TestNothingIsHeardAfterForgetUntilTheNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var l Latest[int]
		l.Set(1)
		l.Forget()
		var heard []int
		l.Watch(func(v int) { heard = append(heard, v) })
		synctest.Wait()
		l.Set(2)
		synctest.Wait()
		if !slices.Equal(heard, []int{2}) {
			t.Fatalf("heard %v", heard)
		}
	})
}

func TestAQueueKeepsTheLastUntilOneHears(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := Queue[int]{Max: 3}
		for i := range 5 {
			q.Add(i)
		}
		var heard []int
		q.Hear(func(v int) { heard = append(heard, v) })
		synctest.Wait()
		q.Add(5)
		synctest.Wait()
		q.Hear(nil)
		q.Add(6)
		synctest.Wait()
		if !slices.Equal(heard, []int{2, 3, 4, 5}) {
			t.Fatalf("heard %v", heard)
		}
		q.Hear(func(v int) { heard = append(heard, -v) })
		synctest.Wait()
		if heard[len(heard)-1] != -6 {
			t.Fatalf("heard %v", heard)
		}
	})
}
