package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/pkg/mflag"
	"github.com/mgutz/ansi"
)

const (
	schedulerHost = "positive-cocoa-90213.appspot.com"
	JSON          = "application/json"
)

var (
	start = ansi.ColorCode("black+ub")
	fail  = ansi.ColorCode("red+b")
	succ  = ansi.ColorCode("green+b")
	reset = ansi.ColorCode("reset")

	useScheduler = false
	runParallel  = false
	verbose      = false

	consoleLock = sync.Mutex{}
)

type test struct {
	name  string
	hosts int
}

type schedule struct {
	Tests []string `json:"tests"`
}

type result struct {
	test
	errored bool
	hosts   []string
}

type tests []test

func (ts tests) Len() int      { return len(ts) }
func (ts tests) Swap(i, j int) { ts[i], ts[j] = ts[j], ts[i] }
func (ts tests) Less(i, j int) bool {
	if ts[i].hosts != ts[j].hosts {
		return ts[i].hosts < ts[j].hosts
	}
	return ts[i].name < ts[j].name
}

func (ts *tests) pick(availible int) (test, bool) {
	// pick the first test that fits in the availible hosts
	for i, test := range *ts {
		if test.hosts <= availible {
			*ts = append((*ts)[:i], (*ts)[i+1:]...)
			return test, true
		}
	}

	return test{}, false
}

func (t test) run(hosts []string) bool {
	consoleLock.Lock()
	fmt.Printf("%s>>> Running %s on %s%s\n", start, t.name, hosts, reset)
	consoleLock.Unlock()

	var out bytes.Buffer

	cmd := exec.Command(t.name)
	cmd.Env = os.Environ()
	cmd.Stdout = &out
	cmd.Stderr = &out

	// replace HOSTS in env
	for i, env := range cmd.Env {
		if strings.HasPrefix(env, "HOSTS") {
			cmd.Env[i] = fmt.Sprintf("HOSTS=%s", strings.Join(hosts, " "))
			break
		}
	}

	start := time.Now()
	err := cmd.Run()
	duration := float64(time.Now().Sub(start)) / float64(time.Second)

	consoleLock.Lock()
	if err != nil {
		fmt.Printf("%s>>> Test %s finished after %0.1f secs with error: %v%s\n", fail, t.name, duration, err, reset)
	} else {
		fmt.Printf("%s>>> Test %s finished with success after %0.1f secs%s\n", succ, t.name, duration, reset)
	}
	if err != nil || verbose {
		fmt.Print(out.String())
		fmt.Println()
	}
	consoleLock.Unlock()

	if err != nil && useScheduler {
		updateScheduler(t.name, duration)
	}

	return err != nil
}

func updateScheduler(test string, duration float64) {
	req := &http.Request{
		Method: "POST",
		Host:   schedulerHost,
		URL: &url.URL{
			Opaque: fmt.Sprintf("/record/%s/%0.2f", url.QueryEscape(test), duration),
			Scheme: "http",
			Host:   schedulerHost,
		},
		Close: true,
	}
	if resp, err := http.DefaultClient.Do(req); err != nil {
		fmt.Printf("Error updating scheduler: %v\n", err)
	} else {
		resp.Body.Close()
	}
}

func getSchedule(tests []string) ([]string, error) {
	var (
		testRun     = "integration-" + os.Getenv("CIRCLE_BUILD_NUM")
		shardCount  = os.Getenv("CIRCLE_NODE_TOTAL")
		shardID     = os.Getenv("CIRCLE_NODE_INDEX")
		requestBody = &bytes.Buffer{}
	)
	if err := json.NewEncoder(requestBody).Encode(schedule{tests}); err != nil {
		return []string{}, err
	}
	url := fmt.Sprintf("http://%s/schedule/%s/%s/%s", schedulerHost, testRun, shardCount, shardID)
	resp, err := http.Post(url, JSON, requestBody)
	if err != nil {
		return []string{}, err
	}
	var sched schedule
	if err := json.NewDecoder(resp.Body).Decode(&sched); err != nil {
		return []string{}, err
	}
	return sched.Tests, nil
}

func getTests(testNames []string) (tests, error) {
	var err error
	if useScheduler {
		testNames, err = getSchedule(testNames)
		if err != nil {
			return tests{}, err
		}
	}
	tests := tests{}
	for _, name := range testNames {
		parts := strings.Split(strings.TrimSuffix(name, "_test.sh"), "_")
		numHosts, err := strconv.Atoi(parts[len(parts)-1])
		if err != nil {
			numHosts = 1
		}
		tests = append(tests, test{name, numHosts})
		fmt.Printf("Test %s needs %d hosts\n", name, numHosts)
	}
	return tests, nil
}

func summary(tests, failed tests) {
	if len(failed) > 0 {
		fmt.Printf("%s>>> Ran %d tests, %d failed%s\n", fail, len(tests), len(failed), reset)
		for _, test := range failed {
			fmt.Printf("%s>>> Fail %s%s\n", fail, test.name, reset)
		}
	} else {
		fmt.Printf("%s>>> Ran %d tests, all succeeded%s\n", succ, len(tests), reset)
	}
}

func parallel(ts tests, hosts []string) bool {
	testsCopy := ts
	sort.Sort(sort.Reverse(ts))
	resultsChan := make(chan result)
	outstanding := 0
	failed := tests{}
	for len(ts) > 0 || outstanding > 0 {
		// While we have some free hosts, try and schedule
		// a test on them
		for len(hosts) > 0 {
			test, ok := ts.pick(len(hosts))
			if !ok {
				break
			}
			testHosts := hosts[:test.hosts]
			hosts = hosts[test.hosts:]

			go func() {
				errored := test.run(testHosts)
				resultsChan <- result{test, errored, testHosts}
			}()
			outstanding++
		}

		// Otherwise, wait for the test to finish and return
		// the hosts to the pool
		result := <-resultsChan
		hosts = append(hosts, result.hosts...)
		outstanding--
		if result.errored {
			failed = append(failed, result.test)
		}
	}
	summary(testsCopy, failed)
	return len(failed) > 0
}

func sequential(ts tests, hosts []string) bool {
	failed := tests{}
	for _, test := range ts {
		if test.run(hosts) {
			failed = append(failed, test)
		}
	}
	summary(ts, failed)
	return len(failed) > 0
}

func main() {
	mflag.BoolVar(&useScheduler, []string{"scheduler"}, false, "Use scheduler to distribute tests across shards")
	mflag.BoolVar(&runParallel, []string{"parallel"}, false, "Run tests in parallel on hosts where possible")
	mflag.BoolVar(&verbose, []string{"v"}, false, "Print output from all tests (Also enabled via DEBUG=1)")
	mflag.Parse()

	if len(os.Getenv("DEBUG")) > 0 {
		verbose = true
	}

	tests, err := getTests(mflag.Args())
	if err != nil {
		fmt.Printf("Error parsing tests: %v\n", err)
		os.Exit(1)
	}

	hosts := strings.Fields(os.Getenv("HOSTS"))
	maxHosts := len(hosts)
	if maxHosts == 0 {
		fmt.Print("No HOSTS specified.\n")
		os.Exit(1)
	}

	var errored bool
	if runParallel {
		errored = parallel(tests, hosts)
	} else {
		errored = sequential(tests, hosts)
	}

	if errored {
		os.Exit(1)
	}
}
