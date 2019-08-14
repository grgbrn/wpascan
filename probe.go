package main

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	wifi "github.com/mark2b/wpa-connect"
	wpaconnect "github.com/mark2b/wpa-connect"
)

type NetworkProbe struct {
	InterfaceName   string
	IO              *bufio.Writer
	TimeoutDuration time.Duration
}

func (probe *NetworkProbe) Log(format string, args ...interface{}) {

	needsNewline := len(args) == 0 && format[len(format)-1] != '\n'

	// xxx need to just have multiple writers here
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
	ok, err := conMan.Disconnect(status.SSID, probe.TimeoutDuration)
	if err != nil {
		probe.Log("ERROR disconnecting %v\n", err)
	}
	probe.Log("disconnect success=%t in %s", ok, time.Since(t))

	return err
}

func (probe *NetworkProbe) checkConnectionStatus() (bool, *wpaconnect.ConnectionStatus, error) {
	// check multiple sources about whether we're connected to
	// a network. if there's a mismatch, panic
	// XXX when do we care about error?

	conMan := wifi.ConnectManager
	conMan.NetInterface = probe.InterfaceName

	// source 1
	connected, status, err := conMan.GetCurrentStatus()
	if err != nil {
		probe.Log("couldn't get network status")
		return false, status, err
	}
	// source 2
	connected2, err := probe.ExtStatusCheck()
	if err != nil {
		probe.Log("couldn't get network status [2]")
		return false, status, err
	}

	// four cases here to validate:
	// 1) a:disconnected	b:disconnected	=> ok
	// 2) a:connected		b:disconnected	=> panic
	// 3) a:disconnected 	b:connected		=> panic
	// 4) a:connected		b:connected		=> ok

	// case 1
	if !connected && connected2 == "" {
		return false, status, nil
	}
	// case 2
	if connected && connected2 == "" {
		probe.Log("status disagree! s1=%s s2=disconnected", status.SSID)
		panic("status disagree [1]")
	}

	// case 3
	if !connected && connected2 != "" {
		probe.Log("status disagree! s1=disconnected s2=%s", connected2)
		panic("status disagree [2]")
	}

	// case 4
	if connected && connected2 != "" {
		// further check that network names match
		if status.SSID != connected2 {
			probe.Log("status disagree! s1=%s s2=%s", status.SSID, connected2)
			panic("status disagree [3]")
		}
		return true, status, nil
	}

	panic("logic error in checkConnectionStatus")
}

func (probe *NetworkProbe) ProbeNetwork(net *SeenNetwork) error {

	conMan := wifi.ConnectManager
	conMan.NetInterface = probe.InterfaceName

	connected, status, err := probe.checkConnectionStatus()
	if err != nil {
		probe.Log("couldn't get network status")
		return err
	}
	if connected {
		probe.Log("currently connected to:%s, must disconnect first\n", status.SSID)
		err = probe.Disconnect(status)
		if err != nil {
			probe.Log("couldn't disconnect [0] %v\n", err)
			panic("couldn't disconnect [0] failing probe")
		}
	}

	// XXX this may connect to the wrong network if we have multiple
	//     with the same SSID - can we connect by BSSID instead?

	// XXX review timeout here - how long does this normally take
	//     connect timeout of 60s and disconnect timeout of 10s
	//     could probably use tweaking

	pass := "" // only unprotected nets for now

	start := time.Now()

	conn, err := conMan.Connect(net.SSID, pass, probe.TimeoutDuration)
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
		if len(avahiOut) > 0 {
			probe.Log(string(avahiOut))
		}
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
		if len(ipOut) > 0 {
			probe.Log(string(ipOut))
		}
	}

	//
	// finished probe, so disconnect
	//
	probe.Log("probed network:%s in %s; disconnecting\n", net.SSID, time.Since(start))
	err = probe.Disconnect(status)
	if err != nil {
		probe.Log("couldn't disconnect [1] %v\n", err)
		panic("couldn't disconnect [1] failing probe")
	}

	return nil
}

// ExtStatusCheck calls wpa_cli for external verification of connection status
// connectedTo contains the SSID of the connected network, "" == disconnected
func (probe *NetworkProbe) ExtStatusCheck() (connectedTo string, err error) {

	// probe.Log("vvvvvvvvv ExtStatusCheck")
	wpaCliCmd := exec.Command("wpa_cli", "-i", probe.InterfaceName, "status")
	wpaCliOut, err := wpaCliCmd.Output()
	if err != nil {
		probe.Log("error executing wpa_cli: %s\n", err)
		return
	}

	strOut := string(wpaCliOut)

	// line we're looking for is:
	// ssid=NETGEAR28
	re := regexp.MustCompile(`(?m)^ssid=(.+)$`)
	m := re.FindStringSubmatch(strOut)
	fmt.Println(m)
	if len(m) == 2 {
		connectedTo = m[1]
	}

	// probe.Log("*** wpa_cli [%d bytes] ****\n", len(wpaCliOut))
	// probe.Log(strOut)
	// probe.Log("^^^^^^^^^ ExtStatusCheck")
	return
}
