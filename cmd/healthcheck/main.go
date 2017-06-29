package main

import (
	"errors"
	"flag"
	"net"
	"os"
	"time"

	"code.cloudfoundry.org/healthcheck"
)

var (
	network           string
	uri               string
	port              string
	timeout           time.Duration
	readinessInterval time.Duration
	livenessInterval  time.Duration
)

func init() {
	flag.StringVar(
		&network,
		"network",
		"tcp",
		"network type to dial with (e.g. unix, tcp)",
	)
	flag.StringVar(
		&uri,
		"uri",
		"",
		"uri to healthcheck",
	)
	flag.StringVar(
		&port,
		"port",
		"8080",
		"port to healthcheck",
	)
	flag.DurationVar(
		&timeout,
		"timeout",
		1*time.Second,
		"dial timeout",
	)
	flag.DurationVar(
		&readinessInterval,
		"readiness-interval",
		-1,
		"if set, starts the healthcheck in readiness mode, i.e. do not exit until the healthcheck passes. runs checks every readiness-interval",
	)
	flag.DurationVar(
		&livenessInterval,
		"liveness-interval",
		-1,
		"if set, starts the healthcheck in liveness mode, i.e. do not exit until the healthcheck fail. runs checks every liveness-interval",
	)
}

func realMain() error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return errors.New("failure to get interfaces: " + err.Error())
	}

	h := newHealthCheck(network, uri, port, timeout)

	if readinessInterval > 0 {
		for {
			if err := h.CheckInterfaces(interfaces); err == nil {
				return nil
			}
			time.Sleep(readinessInterval)
		}
	}

	if livenessInterval > 0 {
		for {
			if err := h.CheckInterfaces(interfaces); err != nil {
				return err
			}
			time.Sleep(livenessInterval)
		}
	}

	return h.CheckInterfaces(interfaces)
}

func main() {
	flag.Parse()

	if err := realMain(); err != nil {
		if e, ok := err.(*healthcheck.HealthCheckError); ok {
			os.Stdout.WriteString("healthcheck failed: " + e.Message)
			os.Exit(e.Code)
		}
		os.Stdout.WriteString("healthcheck failed(unknown error)" + err.Error())
		os.Exit(127)
	}
	os.Stdout.WriteString("healthcheck passed")
}
