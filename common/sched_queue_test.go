package common

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
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
	schedQueue.Flush()
	for i := 0; i < testSecs; i++ {
		clk.Add(time.Second)
		schedQueue.Flush()
	}

	t.Logf("Now: %s - calls: %d", clk.Now(), schedQueue.Count())
	require.Equal(t, testSecs, (int)(schedQueue.Count()), "Number of calls")
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
	schedQueue.Flush()
	for i := 0; i < testSecs; i++ {
		clk.Add(time.Second)
		schedQueue.Flush()
	}

	t.Logf("Now: %s - calls: %d", clk.Now(), schedQueue.Count())
	require.Equal(t, testSecs-100+1, (int)(schedQueue.Count()), "Number of calls")
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
	schedQueue.Flush()
	for i := 0; i < testSecs; i++ {
		clk.Add(time.Second)
		schedQueue.Flush()
	}

	t.Logf("Now: %s - calls: %d", clk.Now(), schedQueue.Count())
	require.Equal(t, testSecs/2, (int)(schedQueue.Count()), "Number of calls")
}
