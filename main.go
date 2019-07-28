package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	wifi "github.com/mark2b/wpa-connect"
)

func scanExample(interfaceName string) error {

	// kind of a weird way to pass params to the scanManager
	scanner := wifi.ScanManager
	scanner.NetInterface = interfaceName

	bssList, err := scanner.Scan()
	if err != nil {
		return err
	}
	for _, bss := range bssList {
		//fmt.Println(bss.SSID, bss.Signal, bss.KeyMgmt)
		fmt.Printf("%+v\n", bss)
	}
	return nil
}

// XXX haven't really tested this yet
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

	scanner := wifi.ScanManager
	scanner.NetInterface = interfaceName

	// XXX doesn't really belong on scanner...
	// XXX clean this up in general
	// XXX this explodes if you're not connected to a network
	return scanner.CurrentStatus()
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
	WPS       string
	Frequency uint16

	// updated on each successful scan
	SignalHistory []int16
	AgeHistory    []uint32
}

func (net *SeenNetwork) Describe() string {
	return fmt.Sprintf("%s [%s] %d", net.SSID, net.BSSID, net.Frequency)
}

func wanderLoop(interfaceName string) {
	scanner := wifi.ScanManager
	scanner.NetInterface = interfaceName

	networks := make(map[string]*SeenNetwork)

	for { // ever
		time.Sleep(time.Second * 10)
		now := time.Now()
		fmt.Printf(">>> starting scan - %v\n", now)

		bssList, err := scanner.Scan()
		if err != nil {
			fmt.Printf("Error scanning network:%v\n", err)
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

		for _, bss := range bssList {
			bssid := bss.BSSID
			currentSeen[bssid] = true

			net, ok := networks[bssid]
			if !ok { // new network found
				net = &SeenNetwork{
					First:         now,
					BSSID:         bssid,
					SSID:          bss.SSID,
					WPS:           bss.WPS,
					Frequency:     bss.Frequency,
					SignalHistory: []int16{bss.Signal},
					AgeHistory:    []uint32{bss.Age},
				}
				networks[bssid] = net
				fmt.Printf("    new network:%s\n", net.Describe())
				newCount++
			} else { // known network, just update signal
				fmt.Printf("    updating network:%s\n", net.Describe())
				net.SignalHistory = append(net.SignalHistory, bss.Signal)
				net.AgeHistory = append(net.AgeHistory, bss.Age)
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
				fmt.Printf("    removing network:%s\n", net.Describe())
				net.Last = now

				// XXX log this into our master network history
				b, err := json.MarshalIndent(net, "", "  ")
				if err != nil {
					fmt.Println("can't marshal data:", err)
				}
				fmt.Println(string(b))

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

		fmt.Printf(">>> currently %d networks in range\n", len(networks))
		fmt.Printf(">>> %d updated / %d new / %d removed\n", updateCount, newCount, delCount)
		fmt.Printf(">>> completed scan - %v\n", time.Now())
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
	*/

	validCommands := []string{"status", "connect", "disconnect", "scan", "wander"}

	connectCmd := flag.NewFlagSet("connect", flag.ExitOnError)
	connectNet := connectCmd.String("network", "", "network ssid")
	connectPass := connectCmd.String("pass", "", "network password")

	disconnectCmd := flag.NewFlagSet("connect", flag.ExitOnError)
	disconnectNet := disconnectCmd.String("network", "", "network ssid")

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
		fmt.Printf(">>> connect to net:'%s' on interface:%s\n", connectNet, interfaceName)
		err = connectExample(interfaceName, *connectNet, *connectPass)
	case "disconnect":
		disconnectCmd.Parse(os.Args[2:])
		fmt.Printf(">>> disconnecting from net:'%s' on interface:%s\n", connectNet, interfaceName)
		err = disconnectExample(interfaceName, *disconnectNet)
	case "scan":
		fmt.Printf(">>> single scan of network:%s\n", interfaceName)
		err = scanExample(interfaceName)
	case "wander":
		fmt.Printf(">>> start monitoring network:%s\n", interfaceName)
		wanderLoop(interfaceName)
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
