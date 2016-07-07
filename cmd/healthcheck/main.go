package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

var network = flag.String(
	"network",
	"tcp",
	"network type to dial with (e.g. unix, tcp)",
)

var uri = flag.String(
	"uri",
	"",
	"uri to healthcheck",
)

var port = flag.String(
	"port",
	"8080",
	"port to healthcheck",
)

var timeout = flag.Duration(
	"timeout",
	1*time.Second,
	"dial timeout",
)

func main() {
	flag.Parse()
	// flag.Parse will fail with exit code 2 if failure to parse
	// failHealthCheck(1, "failure to parse flags")

	interfaces, err := net.Interfaces()
	if err != nil {
		failHealthCheck(1, "failure to get interfaces")
	} else {
		for _, intf := range interfaces {
			addrs, err := intf.Addrs()
			if err != nil {
				continue
			}
			for _, a := range addrs {
				if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
					if ipnet.IP.To4() != nil {
						if len(*uri) > 0 {
							httpHealthCheck(ipnet.IP.String())
						} else {
							portHealthCheck(ipnet.IP.String())
						}
					}
				}
			}
		}
		failHealthCheck(3, "failure to find suitable interface")
	}
}

func portHealthCheck(ip string) {
	addr := ip + ":" + *port
	conn, err := net.DialTimeout(*network, addr, *timeout)
	if err == nil {
		conn.Close()
		fmt.Println("healthcheck passed")
		os.Exit(0)
	} else {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			failHealthCheck(64, "timeout when making tcp connection")
		} else {
			failHealthCheck(4, "failure to make TCP request")
		}
	}
}

func httpHealthCheck(ip string) {
	addr := fmt.Sprintf("http://%s:%s%s", ip, *port, *uri)
	client := http.Client{
		Timeout: *timeout,
	}
	resp, err := client.Get(addr)
	if err == nil {
		if resp.StatusCode == http.StatusOK {
			fmt.Println("healthcheck passed")
			os.Exit(0)
		} else {
			failHealthCheck(6, "failure to get valid HTTP status code")
		}
	} else {
		if err, ok := err.(net.Error); ok && err.Timeout() {
			failHealthCheck(65, "timeout when making HTTP request")
		} else {
			failHealthCheck(5, "failure to make HTTP request")
		}
	}
}

func failHealthCheck(code int, reason string) {
	fmt.Println("healthcheck failed: " + reason)
	os.Exit(code)
}
