package main_test

import (
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/ghttp"
)

var _ = Describe("HealthCheck", func() {
	var (
		server     *ghttp.Server
		serverAddr string
	)

	itExitsWithCode := func(healthCheck func() *gexec.Session, code int, reason string) {
		It("exits with code "+strconv.Itoa(code)+" and logs reason", func() {
			session := healthCheck()
			Eventually(session).Should(gexec.Exit(code))
			Expect(session.Out).To(gbytes.Say(reason))
		})
	}

	BeforeEach(func() {
		ip := getNonLoopbackIP()
		server = ghttp.NewUnstartedServer()
		listener, err := net.Listen("tcp", ip+":0")
		Expect(err).NotTo(HaveOccurred())

		server.HTTPTestServer.Listener = listener
		serverAddr = listener.Addr().String()
		server.Start()
	})

	Describe("fails when parsing flags", func() {
		It("exits with code 2", func() {
			session, _ := gexec.Start(exec.Command(healthCheck, "-invalid_flag"), GinkgoWriter, GinkgoWriter)
			Eventually(session).Should(gexec.Exit(2))
		})
	})

	Describe("port healthcheck", func() {
		portHealthCheck := func() *gexec.Session {
			_, port, err := net.SplitHostPort(serverAddr)
			Expect(err).NotTo(HaveOccurred())

			session, err := gexec.Start(exec.Command(healthCheck, "-port", port, "-timeout", "100ms"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			return session
		}

		Context("when the address is listening", func() {
			itExitsWithCode(portHealthCheck, 0, "healthcheck passed")
		})

		Context("when the address is not listening", func() {
			BeforeEach(func() {
				server.Close()
				Eventually(func() error {
					_, err := net.Dial("tcp", serverAddr)
					return err
				}).Should(HaveOccurred())
			})

			itExitsWithCode(portHealthCheck, 4, "failure to make TCP request")
		})
	})

	Describe("http healthcheck", func() {
		httpHealthCheck := func() *gexec.Session {
			_, port, err := net.SplitHostPort(serverAddr)
			Expect(err).NotTo(HaveOccurred())
			session, err := gexec.Start(exec.Command(healthCheck, "-uri", "/api/_ping", "-port", port, "-timeout", "100ms"), GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
			return session
		}

		Context("when the healthcheck is properly invoked", func() {
			BeforeEach(func() {
				server.RouteToHandler("GET", "/api/_ping", ghttp.VerifyRequest("GET", "/api/_ping"))
			})

			Context("when the address is listening", func() {
				itExitsWithCode(httpHealthCheck, 0, "healthcheck passed")
			})

			Context("when the address returns error http code", func() {
				BeforeEach(func() {
					server.RouteToHandler("GET", "/api/_ping", ghttp.RespondWith(500, ""))
				})

				itExitsWithCode(httpHealthCheck, 6, "failure to get valid HTTP status code")
			})

			Context("when the address is not listening", func() {
				BeforeEach(func() {
					server.Close()
				})

				itExitsWithCode(httpHealthCheck, 5, "failure to make HTTP request")
			})

			Context("when the server is too slow to respond", func() {
				BeforeEach(func() {
					server.RouteToHandler("GET", "/api/_ping", func(w http.ResponseWriter, req *http.Request) {
						time.Sleep(2 * time.Second)
						w.WriteHeader(http.StatusOK)
					})
				})

				itExitsWithCode(httpHealthCheck, 65, "timeout when making HTTP request")
			})
		})
	})
})

func getNonLoopbackIP() string {
	interfaces, err := net.Interfaces()
	Expect(err).NotTo(HaveOccurred())
	for _, intf := range interfaces {
		addrs, err := intf.Addrs()
		if err != nil {
			continue
		}

		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
				if ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
	}
	Fail("no non-loopback address found")
	panic("non-reachable")
}
