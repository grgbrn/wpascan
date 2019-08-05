package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"time"

	wifi "github.com/mark2b/wpa-connect"
)

type NetworkProbe struct {
	InterfaceName string
	IO            *bufio.Writer
}

func (probe *NetworkProbe) Log(format string, args ...interface{}) {

	needsNewline := len(args) == 0 && format[len(format)-1] != '\n'

	fmt.Printf(format, args...)
	if needsNewline {
		fmt.Printf("\n")
	}
	fmt.Fprintf(probe.IO, format, args...)
	if needsNewline {
		fmt.Fprintf(probe.IO, "\n")
	}
	probe.IO.Flush()
}

func (probe *NetworkProbe) Disconnect(status *wifi.ConnectionStatus) error {
	if status == nil {
		probe.Log("not connected, ignoring disconnect() call")
	}
	conMan := wifi.ConnectManager // XXX

	probe.Log("disconnecting from %s\n", status.SSID)
	t := time.Now()
	_, err := conMan.Disconnect(status.SSID, time.Second*10)
	probe.Log("disconnected in %s\n", time.Since(t))
	return err
}

func (probe *NetworkProbe) ProbeNetwork(net *SeenNetwork) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = probe.InterfaceName

	connected, status, err := conMan.GetCurrentStatus()
	if err != nil {
		probe.Log("couldn't get network status")
		return err
	}
	if connected {
		probe.Log("currently connected to:%s, must disconnect first\n", status.SSID)
		probe.Disconnect(status)
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
		probe.Log("couldn't connect: %s\n", err)
		return err
	}

	probe.Log("Connected", conn.NetInterface, conn.SSID, conn.IP4.String(), conn.IP6.String())
	probe.Log("Connected in %v\n", time.Since(start))

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
	probe.Log("Checking for network connectivity....")
	ret := CheckConnectivity(probe.InterfaceName)

	probe.Log("connectivity=%v\n", ret)

	//
	// avahi scan
	//
	avahiCmd := exec.Command("avahi-browse", "-a", "-t")
	avahiOut, err := avahiCmd.Output()
	if err != nil {
		probe.Log("error executing avahi-browse: %s\n", err)
	} else {
		probe.Log("*** avahi-browse [%d bytes] ****\n", len(avahiOut))
		probe.Log(string(avahiOut))
	}

	//
	// arp neighbors
	//
	ipCmd := exec.Command("ip", "neighbor", "show")
	ipOut, err := ipCmd.Output()
	if err != nil {
		probe.Log("error executing ip: %s\n", err)
	} else {
		probe.Log("*** ip neighbors [%d bytes] ****\n", len(ipOut))
		probe.Log(string(ipOut))
	}

	//
	// finished probe, so disconnect
	//
	probe.Log("probed network:%s in %s; disconnecting\n", net.SSID, time.Since(start))
	probe.Disconnect(status)

	return nil
}
