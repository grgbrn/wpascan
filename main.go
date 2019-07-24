package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	wifi "github.com/mark2b/wpa-connect"
)

func scanExample() {
	// default "wlan0" only works on a raspi
	// kubuntu nuc uses "wlp0s20f3"

	// kind of a weird way to pass params to the scanManager
	scanner := wifi.ScanManager
	scanner.NetInterface = "wlp0s20f3"

	if bssList, err := scanner.Scan(); err == nil {
		for _, bss := range bssList {
			//fmt.Println(bss.SSID, bss.Signal, bss.KeyMgmt)
			fmt.Printf("%+v\n", bss)
		}
	} else {
		fmt.Println("error scanning wifi")
		fmt.Println(err)
	}
}

// XXX haven't really tested this yet
func connectExample() {

	conMan := wifi.ConnectManager
	conMan.NetInterface = "wlp0s20f3"

	ssid := "NETGEAR28-5G"
	pass := "XXXX"

	if conn, err := conMan.Connect(ssid, pass, time.Second*60); err == nil {
		fmt.Println("Connected", conn.NetInterface, conn.SSID, conn.IP4.String(), conn.IP6.String())
	} else {
		fmt.Println(err)
	}
}

func disconnectExample() {

	ssid := "NETGEAR28-5G"

	conMan := wifi.ConnectManager
	conMan.NetInterface = "wlp0s20f3"

	ok, err := conMan.Disconnect(ssid, time.Second*10)
	if err != nil {
		fmt.Println("couldn't disconnect from network")
		fmt.Println(err)
		return
	}

	fmt.Printf("disconnect status=%v\n", ok)
}

func statusExample() {
	// default "wlan0" only works on a raspi
	// kubuntu nuc uses "wlp0s20f3"

	// kind of a weird way to pass params to the scanManager
	scanner := wifi.ScanManager
	scanner.NetInterface = "wlp0s20f3"

	// XXX doesn't really belong on scanner...
	scanner.CurrentStatus()
}

// XXXXX

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
	//wifi.SetDebugMode()

	/*
		fmt.Printf(">>> getting status at %v\n", time.Now())
		statusExample()

		time.Sleep(time.Second * 2)

		fmt.Printf(">>> disconnecting at %v\n", time.Now())
		disconnectExample()

		fmt.Printf(">>> getting status at %v\n", time.Now())
		statusExample()

		time.Sleep(time.Second * 2)

		fmt.Printf(">>> scan start at %v\n", time.Now())
		scanExample()
		fmt.Printf(">>> scan end at %v\n", time.Now())

		time.Sleep(time.Second * 5)

		fmt.Printf(">>> reconnecting at %v\n", time.Now())
		connectExample()

		fmt.Printf(">>> finished! at %v\n", time.Now())
	*/

	interfaceName := os.Getenv("SCAN_INTERFACE")
	if interfaceName == "" {
		fmt.Println("SCAN_INTERFACE not set, defaulting to wlan0")
		interfaceName = "wlan0"
	}

	fmt.Printf(">>> start monitoring network:%s\n", interfaceName)
	wanderLoop(interfaceName)
}
