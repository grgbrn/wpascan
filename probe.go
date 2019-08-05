package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"time"

	wifi "github.com/mark2b/wpa-connect"
)

func disconnect(log *bufio.Writer, status *wifi.ConnectionStatus) error {
	if status == nil {
		fmt.Fprintf(log, "not connected, ignoring disconnect() call")
	}
	conMan := wifi.ConnectManager // XXX

	fmt.Fprintf(log, "disconnecting from %s\n", status.SSID)
	t := time.Now()
	_, err := conMan.Disconnect(status.SSID, time.Second*10)
	fmt.Fprintf(log, "disconnected in %s\n", time.Since(t))
	return err
}

func ProbeNetwork(log *bufio.Writer, interfaceName string, net *SeenNetwork) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = interfaceName

	connected, status, err := conMan.GetCurrentStatus()
	if err != nil {
		fmt.Fprintf(log, "couldn't get network status")
		return err
	}
	if connected {
		fmt.Fprintf(log, "currently connected to:%s, must disconnect first\n", status.SSID)
		disconnect(log, status)
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
		fmt.Fprintf(log, "couldn't connect: %s\n", err)
		return err
	}

	fmt.Fprintln(log, "Connected", conn.NetInterface, conn.SSID, conn.IP4.String(), conn.IP6.String())
	fmt.Fprintf(log, "Connected in %v\n", time.Since(start))

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
	fmt.Fprintf(log, "Checking for network connectivity....")
	ret := CheckConnectivity(interfaceName)

	fmt.Fprintf(log, "connectivity=%v\n", ret)

	//
	// avahi scan
	//
	avahiCmd := exec.Command("avahi-browse", "-a", "-t")
	avahiOut, err := avahiCmd.Output()
	if err != nil {
		fmt.Fprintf(log, "error executing avahi-browse: %s\n", err)
	} else {
		fmt.Fprintf(log, "*** avahi-browse [%d bytes] ****\n", len(avahiOut))
		fmt.Fprintf(log, string(avahiOut))
	}

	//
	// arp neighbors
	//
	ipCmd := exec.Command("ip", "neighbor", "show")
	ipOut, err := ipCmd.Output()
	if err != nil {
		fmt.Fprintf(log, "error executing ip: %s\n", err)
	} else {
		fmt.Fprintf(log, "*** ip neighbors [%d bytes] ****\n", len(ipOut))
		fmt.Fprintf(log, string(ipOut))
	}

	//
	// finished probe, so disconnect
	//
	fmt.Fprintf(log, "probed network:%s in %s; disconnecting\n", net.SSID, time.Since(start))
	disconnect(log, status)

	return nil
}
