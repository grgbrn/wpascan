package main

import (
	"fmt"
	"os/exec"
	"time"

	wifi "github.com/mark2b/wpa-connect"
)

func disconnect(status *wifi.ConnectionStatus) error {
	if status == nil {
		fmt.Printf("not connected, ignoring disconnect() call")
	}
	conMan := wifi.ConnectManager // XXX

	fmt.Printf("disconnecting from %s\n", status.SSID)
	t := time.Now()
	_, err := conMan.Disconnect(status.SSID, time.Second*10)
	fmt.Printf("disconnected in %s\n", time.Since(t))
	return err
}

func ProbeNetwork(interfaceName string, net *SeenNetwork) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = interfaceName

	connected, status, err := conMan.GetCurrentStatus()
	if err != nil {
		fmt.Println("couldn't get network status")
		return err
	}
	if connected {
		fmt.Printf("currently connected to:%s, must disconnect first\n", status.SSID)
		disconnect(status)
	}

	// XXX this may connect to the wrong network if we have multiple
	//     with the same SSID - can we connect by BSSID instead?

	// XXX review timeout here - how long does this normally take
	//     connect timeout of 60s and disconnect timeout of 10s
	//     could probably use tweaking

	pass := "" // only unprotected nets for now

	start := time.Now()

	conn, err := conMan.Connect(net.SSID, pass, time.Second*60)
	if err != nil {
		fmt.Printf("couldn't connect: %s\n", err)
		return err
	}

	fmt.Println("Connected", conn.NetInterface, conn.SSID, conn.IP4.String(), conn.IP6.String())
	fmt.Printf("Connected in %v\n", time.Since(start))

	// confirm that we're really connected
	connected, status, err = conMan.GetCurrentStatus()
	if err != nil {
		return err
	}
	if !connected {
		return fmt.Errorf("connection status mismatch - not really connected [0]")
	}

	// connectivity check / captive portal
	//
	fmt.Println("Checking for network connectivity....")
	ret := CheckConnectivity(interfaceName)

	fmt.Printf("connectivity=%v\n", ret)

	//
	// avahi scan
	//
	avahiCmd := exec.Command("avahi-browse", "-a", "-t")
	avahiOut, err := avahiCmd.Output()
	if err != nil {
		fmt.Printf("error executing avahi-browse: %s\n", err)
	} else {
		fmt.Printf("*** avahi-browse [%d bytes] ****\n", len(avahiOut))
		fmt.Println(string(avahiOut))
	}

	//
	// arp neighbors
	//
	ipCmd := exec.Command("ip", "neighbor", "show")
	ipOut, err := ipCmd.Output()
	if err != nil {
		fmt.Printf("error executing ip: %s\n", err)
	} else {
		fmt.Printf("*** ip neighbors [%d bytes] ****\n", len(ipOut))
		fmt.Println(string(ipOut))
	}

	//
	// finished probe, so disconnect
	//
	fmt.Printf("probed network:%s in %s; disconnecting\n", net.SSID, time.Since(start))
	disconnect(status)

	return nil
}
