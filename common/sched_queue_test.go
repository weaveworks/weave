package common

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	wt "github.com/weaveworks/weave/testing"
	"log"
)

// Ensure we can add new calls while forwarding the clock
func TestSchedCallsBasic(t *testing.T) {
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestSchedCallsBasic starting")

	const testSecs = 1000
	clk := clock.NewMock()
	schedQueue := NewSchedQueue(clk)
	schedQueue.Start()
	defer schedQueue.Stop()

	c := func() time.Time {
		return clk.Now().Add(time.Second)
	}

	schedQueue.Add(c, clk.Now().Add(time.Second))
	for i := 0; i < testSecs; i++ {
		clk.Add(time.Second)
	}

	t.Logf("Now: %s - calls: %d", clk.Now(), schedQueue.Count())
	wt.AssertEqualInt(t, (int)(schedQueue.Count()), testSecs, "Number of calls")
}

// Ensure we can create a 100 seconds gap in the middle of the time travel
func TestSchedCallsGap(t *testing.T) {
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestSchedCallsGap starting")

	const testSecs = 1000
	clk := clock.NewMock()
	schedQueue := NewSchedQueue(clk)
	schedQueue.Start()
	defer schedQueue.Stop()

	c2 := func() time.Time {
		if schedQueue.Count() == testSecs/2 {
			return clk.Now().Add(time.Duration(100) * time.Second)
		}
		return clk.Now().Add(time.Second)
	}

	schedQueue.Add(c2, clk.Now().Add(time.Second))
	for i := 0; i < testSecs; i++ {
		clk.Add(time.Second)
	}

	t.Logf("Now: %s - calls: %d", clk.Now(), schedQueue.Count())
	wt.AssertEqualInt(t, (int)(schedQueue.Count()), testSecs-100+1, "Number of calls")
}

func TestSchedCallsStop(t *testing.T) {
	InitDefaultLogging(testing.Verbose())
	Info.Println("TestSchedCallsStop starting")

	const testSecs = 1000
	clk := clock.NewMock()
	schedQueue := NewSchedQueue(clk)
	schedQueue.Start()
	defer schedQueue.Stop()

	c2 := func() time.Time {
		if schedQueue.Count() == testSecs/2 {
			return time.Time{}
		}
		return clk.Now().Add(time.Second)
	}

	schedQueue.Add(c2, clk.Now().Add(time.Second))
	for i := 0; i < testSecs; i++ {
		clk.Add(time.Second)
	}

	t.Logf("Now: %s - calls: %d", clk.Now(), schedQueue.Count())
	wt.AssertEqualInt(t, (int)(schedQueue.Count()), testSecs/2, "Number of calls")
}
