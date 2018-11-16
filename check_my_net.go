package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"sort"
	"syscall"
    "time"
    "encoding/json"
    "io/ioutil"

	"github.com/docopt/docopt-go"
	"github.com/schuellerf/go-ping"
)

var usage = `Check My Net
Simple program to check the availability of the necessary nodes to get to the internet
and see if the important nodes are up.

Usage:
  check_my_net ping [--count=<int>] [--interval=<time.Duration>] [--json=<Filename>]
  check_my_net server [--interval=<time.Duration>] [--json=<Filename>]

Options:
  -h --help                      Show this screen.
  --version                      Show version.
  -c --count=<int>               Number of pings to complete [default: 1].
  -i --interval=<time.Duration>  Interval between pings [default: 5s].
  -j --json=<Filename>           JSON file with "ping_targets"
`

var hint_format = "%v (%v)"
var max_width int

type result struct {
	addr *ping_target
	err  string
}

type ping_target struct {
	Target         string
	Hint           string
	status         string
	lastOnlineTime time.Time
	print_line     string
	responseTime   time.Duration
	lastUpdate     time.Time
}

// generates an on-recv function while passing the data-channel
func getOnRecvFunc(ch chan<- result, ret result, pinger *ping.Pinger, timeout time.Duration) func(*ping.Packet) {
	return func(pkt *ping.Packet) {

		ret.addr.lastUpdate = time.Now()
		stats := pinger.Statistics()
		ret.addr.responseTime = stats.MaxRtt
		ipaddr := pinger.IPAddr()
		ret.addr.lastOnlineTime = ret.addr.lastUpdate

		if len(ret.addr.status) == 0 {
			ret.addr.status = ipaddr.IP.String()
		}

		ch <- ret

		//resolve again

		err := pinger.SetAddr(ret.addr.Target)
		if err != nil {
			ret.addr.status = fmt.Sprintf("Cached IP: %v - ERR: %v", ipaddr, err)
		} else {
			if !pinger.IPAddr().IP.Equal(ipaddr.IP) {
				ret.addr.status = fmt.Sprintf("Changed IP; %v -> %v", ipaddr, pinger.IPAddr())
			}
		}
	}
}

func getOnFinishFunc(ch chan<- result, ret result, pinger *ping.Pinger, timeout time.Duration) func(*ping.Statistics) {
	return func(stats *ping.Statistics) {
		ret.addr.lastUpdate = time.Now()
		ret.addr.responseTime = stats.MaxRtt

		if (pinger.Count != -1 && stats.PacketsRecv != pinger.Count) ||
			(pinger.Count == -1 && stats.PacketsRecv == 0) {
			ret.err = fmt.Sprintf("Timeout after %0.1fs (%v/%v)", timeout.Seconds(), stats.PacketsRecv, pinger.Count)
			ch <- ret
		}
	}
}
func pingWorker(ch chan<- result, addr *ping_target, timeout time.Duration, interval time.Duration, count int) {
	var ret result
	ret.addr = addr

	for {
		pinger, err := ping.NewPinger(addr.Target)
		if err != nil {
			ret.err = err.Error()
			ret.addr.lastUpdate = time.Now()
			//fmt.Printf("async error: %s %s\n", addr.Target, ret.err)
			ch <- ret
			// for the "count down" usecase
			// for -1 keep trying forever

			if count > 0 {
				count--
			}
			time.Sleep(interval)
		} else {
			ret.err = ""
			pinger.Count = count
			if timeout > 0 {
				pinger.Timeout = timeout
			}

			pinger.Interval = interval
			pinger.OnRecv = getOnRecvFunc(ch, ret, pinger, timeout)
			pinger.OnFinish = getOnFinishFunc(ch, ret, pinger, timeout)
			pinger.Run() // blocks until finished
			if count > 0 {
				count -= pinger.PacketsRecv
				if count < 0 {
					count = 0
				}
			}
		}

		if count == 0 {
			return
		}
	}
}

func max_len(list *[]ping_target, hint_format string) int {
	r := 0
	for i := range *list {
		a := &(*list)[i]
		if len(a.Hint) > 0 {
			a.print_line = fmt.Sprintf(hint_format, a.Hint, a.Target)
		} else {
			a.print_line = a.Target
		}
		if len(a.print_line) > r {
			r = len(a.print_line)
		}
	}
	return r
}
func prettyPrint(a *result) {
	var host string

	host = a.addr.print_line
	if len(a.err) > 0 {
		// print the error message
		format := fmt.Sprintf("%%-%ds\t ---.---ms %%v (ERR: %%s; last online: %%v)\n", max_width)
		if a.addr.lastOnlineTime.IsZero() {
			fmt.Printf(format, host, a.addr.lastUpdate.Format(time.StampMilli), a.err, "never")
		} else {
			fmt.Printf(format, host, a.addr.lastUpdate.Format(time.StampMilli), a.err, a.addr.lastOnlineTime.Format(time.StampMilli))
		}
	} else {

		format := fmt.Sprintf("%%-%ds\t%%8.3fms %%v (%%v)\n", max_width)
		fmt.Printf(format,
			host,
			a.addr.responseTime.Seconds()*1000,
			a.addr.lastUpdate.Format(time.StampMilli),
			a.addr.status)
	}
}
func main() {
    var addrs []ping_target
	rand.Seed(time.Now().UTC().UnixNano())

	args, err := docopt.ParseArgs(usage, os.Args[1:], "0.0.1")
	if err != nil {
		panic(err)
	}
	count, err := args.Int("--count")
	fmt.Printf("%v\n", args)
	interval_arg, err := args.String("--interval")
	if err != nil {
		panic(err)
	}
	interval, err := time.ParseDuration(interval_arg)

	if err != nil {
		panic(err)
    }

    var json_filename string
    if args["--json"] != nil {
        json_filename, err = args.String("--json")
        if err != nil {
            panic(err)
        }
    }

    use_default := true
    if len(json_filename) > 0 {
        jsonFile, err := os.Open(json_filename)
        if err != nil {
            panic(err)
        }
        defer jsonFile.Close()
        byteValue, _ := ioutil.ReadAll(jsonFile)

        fmt.Printf("bytes: %s\n", byteValue)
        json.Unmarshal(byteValue, &addrs)
        fmt.Printf("json: %v\n", addrs)
        use_default = false
    }
    if use_default {

        addrs = []ping_target{
            {Target: "192.168.0.1", Hint: "dummy Router"},
            {Target: "1.1.1.1", Hint: "dummy DNS"}}
    }

	cmd_ping, _ := args.Bool("ping")
	cmd_server, _ := args.Bool("server")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	signal.Notify(c, syscall.SIGTERM)
	ch := make(chan result)

	if cmd_ping {

		timeout := interval

		fmt.Printf("\n-- Pinging %d hosts (%d times every %v; timeout: %v) --\n\n", len(addrs), count, interval, timeout)

		for i := range addrs {
			go pingWorker(ch, &addrs[i], timeout, interval, count)
		}

		max_width = max_len(&addrs, hint_format)
		n := len(addrs)
		// we expect "count" replies from every target
		for i := 0; i < n*count; i++ {

			select {
			case r := <-ch:
				prettyPrint(&r)
			case <-c:
				close(ch)
				return
			}
		}
	}
	if cmd_server {

        fmt.Printf("Starting server...\n")
        if len(addrs) == 0 {
            panic("No IPs found in json file...")
        }
		results := make(map[string]result)
		start_time := time.Now()
		for i := range addrs {
			addrs[i].lastUpdate = start_time
			go pingWorker(ch, &addrs[i], interval, interval, -1)
			results[addrs[i].Target] = result{addr: &addrs[i], err: "No reply yet"}
		}

		max_width = max_len(&addrs, hint_format)


		for {
			select {
			case r := <-ch:
				results[r.addr.Target] = r

				fmt.Printf("\033[2J\033[H--- \n\n")

				keys := make([]string, 0, len(results))
				for k := range results {
					keys = append(keys, k)
				}
				sort.Strings(keys)

				for _, k := range keys {
					a := results[k]
					prettyPrint(&a)
				}
			case <-c:
				close(ch)
				return
			}
		}
	}
}
