package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	wifi "github.com/mark2b/wpa-connect"
)

func scanExample(interfaceName string, useFilter bool) error {

	networks := make(map[string]*SeenNetwork)

	seenNetworks, err := basicScan(interfaceName)
	if err != nil {
		return err
	}

	// put these in an unnecessary map just for use with candidates/filter
	for _, n := range seenNetworks {
		networks[n.BSSID] = n
	}

	var filter networkFilterPredicate
	if useFilter {
		filter = defaultNetworkFilters
	} else {
		filter = allNetworks
	}

	if candidates := candidates(networks, filter); len(candidates) > 0 {
		for _, net := range candidates {
			fmt.Printf("%s signal: %d\n", net, net.LastSignalStrength())
		}
	} else {
		fmt.Printf("nothing interesting found, %d networks filtered\n", len(networks))
	}
	return nil
}

func connectExample(interfaceName, ssid, pass string) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = interfaceName

	if conn, err := conMan.Connect(ssid, pass, time.Second*60); err == nil {
		fmt.Println("Connected", conn.NetInterface, conn.SSID, conn.IP4.String(), conn.IP6.String())
		return nil
	} else {
		fmt.Println(err)
		return err
	}
}

func disconnectExample(interfaceName, ssid string) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = interfaceName

	ok, err := conMan.Disconnect(ssid, time.Second*10)
	if err != nil {
		fmt.Println("couldn't disconnect from network")
		fmt.Println(err)
		return err
	}

	fmt.Printf("disconnect status=%v\n", ok)
	return nil
}

func statusExample(interfaceName string) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = interfaceName

	connected, status, err := conMan.GetCurrentStatus()
	if err != nil {
		fmt.Println("error getting network status")
		fmt.Println(err)
		return err
	}
	fmt.Printf("connected=%t\n", connected)
	if connected {
		fmt.Printf("%+v\n", status)
	}

	return nil
}

func probeExample(interfaceName, ssid string) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = interfaceName

	// need our seenNetwork object to connect
	seenNetworks, err := basicScan(interfaceName)
	if err != nil {
		return err
	}

	var targetNet *SeenNetwork
	for _, net := range seenNetworks {
		if net.SSID == ssid {
			targetNet = net
			break
		}
	}

	return ProbeNetwork(bufio.NewWriter(os.Stdout), interfaceName, targetNet)
}

// SeenNetwork remembers the history of networks we discover
// in 'wander' mode
type SeenNetwork struct {
	// first & last seen times
	First time.Time
	Last  time.Time

	// constant bss fields
	BSSID     string
	SSID      string
	WPS       string // XXX always unset?
	KeyMgmt   []string
	Frequency uint16

	// updated on each successful scan
	SignalHistory []int16
	AgeHistory    []uint32
}

func (net *SeenNetwork) String() string {
	// SSIDs often contain NULs so be careful when printing them
	return fmt.Sprintf("%+q [%s] %s", net.SSID, net.BSSID, net.KeyMgmt)
}

// IsUnprotected indictes whether a network has a password
func (net *SeenNetwork) IsUnprotected() bool {
	return len(net.KeyMgmt) == 0
}

// LastSignalStrength returns the last known signal strength for a network
func (net *SeenNetwork) LastSignalStrength() int16 {
	return net.SignalHistory[len(net.SignalHistory)-1]
}

// LastAge returns the age (in seconds?) of the signal strength
func (net *SeenNetwork) LastAge() uint32 {
	return net.AgeHistory[len(net.AgeHistory)-1]
}

// xxx these results are unsorted, but maybe it should be?
func basicScan(interfaceName string) ([]*SeenNetwork, error) {

	results := make([]*SeenNetwork, 0)

	scanner := wifi.ScanManager
	scanner.NetInterface = interfaceName

	bssList, err := scanner.Scan()
	if err != nil {
		return results, err
	}

	for _, bss := range bssList {
		net := &SeenNetwork{
			BSSID:         bss.BSSID,
			SSID:          bss.SSID,
			WPS:           bss.WPS,
			KeyMgmt:       bss.KeyMgmt,
			Frequency:     bss.Frequency,
			SignalHistory: []int16{bss.Signal},
			AgeHistory:    []uint32{bss.Age},
		}
		results = append(results, net)
	}

	return results, nil
}

// simple predicates to filter interesting networks

type networkFilterPredicate func(*SeenNetwork) bool

func unprotectedNetworks(n *SeenNetwork) bool {
	return n.IsUnprotected()
}

func uninterestingPublicNets(n *SeenNetwork) bool {
	if n.SSID == "Vodafone Homespot" ||
		n.SSID == "Vodafone Hotspot" ||
		n.SSID == "Telekom_FON" {
		return false
	}
	return true
}

func allNetworks(n *SeenNetwork) bool {
	return true
}

// a compound predicate passes (returns true) only if all sub-predicates return true
func compoundFilter(predicates ...networkFilterPredicate) networkFilterPredicate {
	return func(net *SeenNetwork) bool {
		for _, pred := range predicates {
			if !pred(net) {
				return false
			}
		}
		return true
	}
}

// XXX make these configurable through an environment var?
var defaultNetworkFilters = compoundFilter(unprotectedNetworks)

// candidates examines the list of current networks to identify
// which (if any) are interesting
// returns a list sorted by most recent signal strength
func candidates(networks map[string]*SeenNetwork, filter networkFilterPredicate) []*SeenNetwork {

	results := make([]*SeenNetwork, 0)

	for _, v := range networks {
		if filter(v) {
			results = append(results, v)
		}
	}

	// sort results by last value of SignalHistory
	sort.Slice(results, func(i, j int) bool {
		// use > because these are all negative numbers and we want smallest first
		return results[i].LastSignalStrength() > results[j].LastSignalStrength()
	})

	return results
}

func wanderLoop(interfaceName string, log *bufio.Writer) {

	networks := make(map[string]*SeenNetwork)

	for { // ever
		time.Sleep(time.Second * 10)
		now := time.Now()

		fmt.Printf(">>> starting scan - %v\n", now)
		fmt.Fprintf(log, ">>> starting scan - %v\n", now)

		bssList, err := basicScan(interfaceName)
		if err != nil {
			fmt.Fprintf(log, "Error scanning network:%v\n", err)
			continue
		}

		// three possibilities for each network:
		// - new network (add to the map)
		// - known network (update map entry)
		// - vanished network (remove from the map and record)

		// track the keys of all the networks we found this
		// scan so we can determine which ones have vanished
		// and need to be removed from the map
		currentSeen := make(map[string]bool)

		// for completeness...
		var newCount, updateCount, delCount int

		for _, network := range bssList {
			bssid := network.BSSID
			currentSeen[bssid] = true

			net, ok := networks[bssid]
			if !ok { // new network found
				network.First = now
				networks[bssid] = network
				//fmt.Fprintf(log, "    new network:%s\n", network)
				newCount++
			} else { // known network, just update signal
				//fmt.Fprintf(log, "    updating network:%s\n", net)
				// a little obtuse - new entry 'network' will only have one value
				// in it's signal & age arrays; copy those onto the instance in the map
				net.SignalHistory = append(net.SignalHistory, network.LastSignalStrength())
				net.AgeHistory = append(net.AgeHistory, network.LastAge())
				updateCount++
			}
		}

		// use currentSeen to determine which entries
		// need to be removed
		for _bss, net := range networks {
			_, ok := currentSeen[_bss]
			if !ok {
				// set the final seen time and remove the network
				// from the map
				//fmt.Fprintf(log, "    removing network:%s\n", net)
				net.Last = now

				// XXX log this into our master network history
				b, err := json.MarshalIndent(net, "", "  ")
				if err != nil {
					fmt.Fprintln(log, "can't marshal data:", err)
				}
				fmt.Fprintln(log, "> recording seen network")
				fmt.Fprintln(log, string(b))

				delete(networks, _bss)
				delCount++
			}
		}

		// xxx debug map
		// b, err := json.MarshalIndent(networks, "", "  ")
		// if err != nil {
		// 	fmt.Println("can't marshal network data:", err)
		// }
		// fmt.Print(string(b))

		candidates := candidates(networks, defaultNetworkFilters)
		if len(candidates) > 0 {
			fmt.Fprintf(log, ">   found %d interesting networks\n", len(candidates))
			for _, net := range candidates {
				fmt.Fprintf(log, "* %s signal: %d\n", net, net.LastSignalStrength())
			}
		} else {
			fmt.Fprintf(log, "nothing interesting found, %d networks filtered\n", len(networks))
		}

		// attempt to probe all interesting networks
		// XXX remember those that we've probed so we can ignore them in the future
		if len(candidates) > 0 {
			var probeOK, probeErr int
			for ix, candidate := range candidates {
				fmt.Fprintf(log, ">   probing network:%s [n=%d]\n", candidate, ix)
				e := ProbeNetwork(log, interfaceName, candidates[0])
				if e != nil {
					probeErr++
				} else {
					probeOK++
				}
				time.Sleep(time.Second * 1) // why not?
			}
			fmt.Fprintf(log, ">>> %d probes successful, %d errors\n", probeOK, probeErr)
		}

		fmt.Fprintf(log, ">>> currently %d networks in range\n", len(networks))
		fmt.Fprintf(log, ">>> %d updated / %d new / %d removed\n", updateCount, newCount, delCount)
		fmt.Fprintf(log, ">>> completed scan - %v\n", time.Now())
		// also go to stdout
		fmt.Printf(">>> completed scan - %v\n", time.Now())
		err = log.Flush()
		if err != nil {
			fmt.Println("Error flushing log")
			fmt.Println(err)
		}

	}
}

func main() {
	// default "wlan0" only works on a raspi
	// kubuntu desktop uses "wlp0s20f3"
	interfaceName := os.Getenv("SCAN_INTERFACE")
	if interfaceName == "" {
		fmt.Println("SCAN_INTERFACE not set, defaulting to wlan0")
		interfaceName = "wlan0"
	}

	// subcommand structure cribbed from here
	// https://gobyexample.com/command-line-subcommands
	/*
		wpascan status
		wpascan connect NETWORK PASSWORD
		wpascan disconnect
		wpascan scan
		wpascan wander
		wpascan check
		wpascan probe NETWORK
	*/

	validCommands := []string{
		"status", "connect", "disconnect", "scan", "wander", "check", "probe",
	}

	connectCmd := flag.NewFlagSet("connect", flag.ExitOnError)
	connectNet := connectCmd.String("network", "", "network ssid")
	connectPass := connectCmd.String("pass", "", "network password")

	disconnectCmd := flag.NewFlagSet("connect", flag.ExitOnError)
	disconnectNet := disconnectCmd.String("network", "", "network ssid")

	scanCmd := flag.NewFlagSet("scan", flag.ExitOnError)
	scanFilter := scanCmd.Bool("filter", false, "uninteresting network filter")

	probeCmd := flag.NewFlagSet("probe", flag.ExitOnError)
	probeNet := probeCmd.String("network", "", "network ssid")

	if len(os.Args) < 2 {
		fmt.Printf("Expected subcommand: %s\n", strings.Join(validCommands, ","))
		os.Exit(1)
	}

	// XXX how can i add a toplevel flag?
	// e.g. 'wpascan -d scan'
	//wifi.SetDebugMode()

	var err error

	switch os.Args[1] {
	case "status":
		fmt.Printf(">>> status of interface:%s\n", interfaceName)
		err = statusExample(interfaceName)
	case "connect":
		connectCmd.Parse(os.Args[2:])
		fmt.Printf(">>> connect to net:'%s' on interface:%s\n", *connectNet, interfaceName)
		err = connectExample(interfaceName, *connectNet, *connectPass)
	case "disconnect":
		disconnectCmd.Parse(os.Args[2:])
		fmt.Printf(">>> disconnecting from net:'%s' on interface:%s\n", *connectNet, interfaceName)
		err = disconnectExample(interfaceName, *disconnectNet)
	case "scan":
		scanCmd.Parse(os.Args[2:])
		fmt.Printf(">>> single scan of network:%s\n", interfaceName)
		err = scanExample(interfaceName, *scanFilter)
	case "wander":
		fmt.Printf(">>> start monitoring network:%s\n", interfaceName)

		// create a per-session log (just in $PWD for now)
		// XXX may be better to close & reopen this file each time?
		logName := time.Now().Format("2006-01-02_150405.log")
		fmt.Printf(">>> logging session to %s\n", logName)
		f, err := os.Create(logName)
		if err == nil {
			defer f.Close()
			wanderLoop(interfaceName, bufio.NewWriter(f))
		}
	case "check":
		fmt.Printf(">>> connectivity check for network:%s\n", interfaceName)
		connected := CheckConnectivity(interfaceName)
		fmt.Printf("connected=%v\n", connected)
	case "probe":
		probeCmd.Parse(os.Args[2:])
		fmt.Printf(">>> probing network:%s\n", *probeNet)
		probeExample(interfaceName, *probeNet)
	default:
		fmt.Printf("Expected subcommand: %s\n", strings.Join(validCommands, ","))
	}

	if err != nil {
		fmt.Println("ERROR")
		fmt.Println(err)
		os.Exit(1)
	}
	// should exit with 0?
}
