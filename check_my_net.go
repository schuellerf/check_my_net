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
    "runtime"

	"github.com/docopt/docopt-go"
	"github.com/schuellerf/go-ping"
	"github.com/schuellerf/traceroute"
)

var usage = `Check My Net
Simple program to check the availability of the necessary nodes to get to the internet
and see if the important nodes are up.

Usage:
  check_my_net ping [--count=<int>] [--interval=<time.Duration>] [--json=<Filename>]
  check_my_net server [--interval=<time.Duration>] [--maxHops=<Int>] [--json=<Filename>]

Options:
  -h --help                      Show this screen.
  --version                      Show version.
  -c --count=<int>               Number of pings to complete [default: 1].
  -i --interval=<time.Duration>  Interval between pings [default: 5s].
  -j --json=<Filename>           JSON file with "ping_targets [default: default.json]"
  -h --maxHops=<int>             Number of hops to check to the internet [default: 2].
`

var hint_format = "%v (%v)"
var max_width int
var color_warning = "\033[33m"
var color_err = "\033[31m"
var color_default = "\033[39m"

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

		ret.addr.lastUpdate = time.Now().Local()
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
		ret.addr.lastUpdate = time.Now().Local()
		ret.addr.responseTime = stats.MaxRtt

		if (pinger.Count != -1 && stats.PacketsRecv != pinger.Count) ||
			(pinger.Count == -1 && stats.PacketsRecv == 0) {
			ret.err = fmt.Sprintf("%sTimeout after %0.1fs (%v/%v)%s", color_err, timeout.Seconds(), stats.PacketsRecv, pinger.Count, color_default)
			ch <- ret
		}
	}
}
func pingWorker(ch chan<- result, addr *ping_target, timeout time.Duration, interval time.Duration, count int) {
	var ret result
	ret.addr = addr

	for {
		pinger, err := ping.NewPinger(addr.Target)
		if runtime.GOOS == "windows" {
			pinger.SetPrivileged(true)
		}
		if err != nil {
			ret.err = err.Error()
			ret.addr.lastUpdate = time.Now().Local()
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

func traceRouteWorker(hop_ch chan<- traceroute.TracerouteHop, traceTarget string, interval time.Duration, maxHops int) {

	for {
        opts := new(traceroute.TracerouteOptions)
        opts.SetMaxHops(maxHops)
        out, err := traceroute.Traceroute(traceTarget, opts)
        if err == nil {
            if len(out.Hops) == 0 {
                fmt.Printf("TestTraceroute failed. Expected at least one hop\n")
                return
            }
        } else {
            fmt.Printf("Traceroute error: %v", err)
            return
        }

        for _, hop := range out.Hops {
            hop_ch <- hop
        }
        time.Sleep(interval)
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
func prettyPrintPing(a *result, interval *time.Duration ) {
	var host string
    var color_start string

	host = a.addr.print_line
	if len(a.err) > 0 {
		// print the error message
		format := fmt.Sprintf("%s%%-%ds%s\t ---.---ms %%v (%sERR: %%s%s; last online: %%v)\n", color_err, max_width, color_default,color_err, color_default)
		if a.addr.lastOnlineTime.IsZero() {
			fmt.Printf(format, host, a.addr.lastUpdate.Format(time.StampMilli), a.err, "never")
		} else {
			fmt.Printf(format, host, a.addr.lastUpdate.Format(time.StampMilli), a.err, a.addr.lastOnlineTime.Format(time.StampMilli))
		}
	} else {

        if time.Now().Local().Unix() - a.addr.lastUpdate.Unix() > int64(interval.Seconds()) {
            color_start = color_warning
        } else {
            color_start = color_default
        }
		format := fmt.Sprintf("%%-%ds\t%%8.3fms %s%%v%s (%%v - %%vs ago)\n", max_width, color_start, color_default)
		fmt.Printf(format,
			host,
			a.addr.responseTime.Seconds()*1000,
			a.addr.lastUpdate.Format(time.StampMilli),
			a.addr.status,
            time.Now().Local().Unix() - a.addr.lastUpdate.Unix())
	}
}
func printHop(hop traceroute.TracerouteHop) {
	fmt.Printf("%-3d %-15v %v\n", hop.TTL, hop.AddressString(), hop.ElapsedTime)
}

func main() {
    var addrs []ping_target
	rand.Seed(time.Now().Local().UnixNano())

	args, err := docopt.ParseArgs(usage, os.Args[1:], "0.0.1")
	if err != nil {
		panic(err)
    }
    maxHops, err := args.Int("--maxHops")
    if err != nil {
		panic(err)
    }

	count, err := args.Int("--count")
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
            if err.(*os.PathError).Err == syscall.ENOENT {
                fmt.Printf("Ignoring that default.json does not exist\n")
            } else {
                panic(err)
            }
        } else {
            defer jsonFile.Close()
            byteValue, _ := ioutil.ReadAll(jsonFile)

            json.Unmarshal(byteValue, &addrs)
            use_default = false
        }
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
	hop_ch := make(chan traceroute.TracerouteHop)

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
                prettyPrintPing(&r, &interval)
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
		hops := make(map[int]traceroute.TracerouteHop)
		start_time := time.Now().Local()
		for i := range addrs {
			addrs[i].lastUpdate = start_time
			go pingWorker(ch, &addrs[i], interval, interval, -1)
			results[addrs[i].Target] = result{addr: &addrs[i], err: "No reply yet"}
        }

		if runtime.GOOS != "windows" {
			go traceRouteWorker(hop_ch, "1.1.1.1", interval, maxHops)
		}

		max_width = max_len(&addrs, hint_format)


		for {
			select {
			case r := <-ch:
				results[r.addr.Target] = r
            case h := <-hop_ch:
                hops[h.TTL] = h
			case <-c:
				close(ch)
				return
            }
            fmt.Printf("\033[2J\033[H--- \n\n")

            fmt.Printf("Ping: %v\n", time.Now().Local().Format(time.Stamp))

            keys := make([]string, 0, len(results))
            for k := range results {
                keys = append(keys, k)
            }
            sort.Strings(keys)

            for _, k := range keys {
                a := results[k]
                prettyPrintPing(&a, &interval)
            }

            fmt.Printf("\nFirst %v Hops:\n", maxHops)
            h_keys := make([]int, 0, len(hops))
            for k := range hops {
                h_keys = append(h_keys, k)
            }
            sort.Ints(h_keys)
            for _,k := range h_keys {
                h := hops[k]
                printHop(h)
            }
		}
	}
}
