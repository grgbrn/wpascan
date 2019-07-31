package main

/*

assorted functions to check for internet connectivity
and deal with captive portals

*/

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

// CheckConnectivity does simple checks to see if we seem to have
// a working internet connection
func CheckConnectivity(interfaceName string) bool {
	// XXX specifying which interface to use is somewhat complicated
	//     https://stackoverflow.com/a/44571400
	//     create a net.Dialer and http.RoundTripper

	// XXX flush dns cache? it could get hosed if we're constantly
	//     connecting and disconnecting...

	// XXX i should be explicit about timeouts here!

	// android captive portal detection:
	// http://androidxref.com/6.0.0_r1/xref/frameworks/base/packages/CaptivePortalLogin/src/com/android/captiveportallogin/CaptivePortalLoginActivity.java

	//const testURL = "http://clients3.google.com/generate_204" // KitKat era url
	const testURL = "http://connectivitycheck.gstatic.com/generate_204"

	resp, err := http.Get(testURL)
	if err != nil {
		fmt.Printf("conn check failed:%s\n", err)
		return false
	}

	// do we always need to read the body?
	b, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		fmt.Printf("conn check failed:%s\n", err)
		return false
	}

	fmt.Printf("status=%d   body_bytes=%d\n", resp.StatusCode, len(b))

	if resp.StatusCode == 204 {
		return true
	}

	fmt.Printf("conn check failed, status code=%d\n", resp.StatusCode)

	// lots of weird captive portals out there, maybe best to assume
	// we can get a Host header from just about anywhere
	if host := resp.Header.Get("Host"); host != "" {
		fmt.Printf("found host header: %s\n", host)
		// XXX what to do with this?
	}

	// don't try to project too much structure at this point
	// just log the response code and http body
	// XXX figure out where to log this
	if len(b) > 0 {
		fmt.Println("==================")
		fmt.Println(string(b))
		fmt.Println("==================")
	}

	return false
}
