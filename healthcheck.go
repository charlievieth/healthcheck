package healthcheck

import (
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HealthCheckError struct {
	Code    int
	Message string
}

func (e *HealthCheckError) Error() string {
	return e.Message
}

type HealthCheck struct {
	network string
	uri     string
	port    string
	timeout time.Duration
}

func NewHealthCheck(network, uri, port string, timeout time.Duration) HealthCheck {
	return HealthCheck{
		network: network,
		uri:     uri,
		port:    port,
		timeout: timeout,
	}
}

func (h *HealthCheck) CheckInterfaces(interfaces []net.Interface) error {
	healthcheck := h.HTTPHealthCheck
	if len(h.uri) == 0 {
		healthcheck = h.PortHealthCheck
	}

	for _, intf := range interfaces {
		addrs, err := intf.Addrs()
		if err != nil {
			continue
		}

		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				err := healthcheck(ipnet.IP)
				return err
			}
		}
	}
	return &HealthCheckError{Code: 3, Message: "failure to find suitable interface"}
}

func (h *HealthCheck) PortHealthCheck(ip net.IP) error {
	conn, err := net.DialTimeout(h.network, IPString(ip)+":"+h.port, h.timeout)
	if err == nil {
		conn.Close()
		return nil
	}

	if err, ok := err.(net.Error); ok && err.Timeout() {
		return &HealthCheckError{Code: 64, Message: "timeout when making TCP connection: " + err.Error()}
	}

	return &HealthCheckError{Code: 4, Message: "failure to make TCP connection: " + err.Error()}
}

var noopScratch [4096]byte

func noopReadAll(r io.Reader) {
	for {
		_, err := r.Read(noopScratch[0:])
		if err != nil {
			break
		}
	}
}

func (h *HealthCheck) HTTPHealthCheck(ip net.IP) error {

	u, err := url.Parse("http://" + IPString(ip) + ":" + h.port + h.uri)
	if err != nil {
		// WARN (CEV): Fix code
		return &HealthCheckError{Code: -1, Message: "failed to parse URL: " + err.Error()}
	}
	if strings.LastIndex(u.Host, ":") > strings.LastIndex(u.Host, "]") {
		u.Host = strings.TrimSuffix(u.Host, ":")
	}
	req := http.Request{
		Method:     "GET",
		URL:        u,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header), // NB (CEV): memory here...
		Body:       nil,
		Host:       u.Host,
	}

	client := http.Client{
		Timeout: h.timeout,
	}
	resp, err := client.Do(&req)

	if err == nil {

		// We need to read the request body to prevent extraneous errors in the server.
		// We could make a HEAD request but there are concerns about servers that may
		// not implement the RFC correctly.
		//
		noopReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			return nil
		}

		return &HealthCheckError{Code: 6,
			Message: "failure to get valid HTTP status code: " + strconv.Itoa(resp.StatusCode)}
	}

	if err, ok := err.(net.Error); ok && err.Timeout() {
		return &HealthCheckError{Code: 65, Message: "timeout when making HTTP request: " + err.Error()}
	}

	return &HealthCheckError{Code: 5, Message: "failure to make HTTP request: " + err.Error()}
}

// The below are a bit aggressive, but at least in the IPv4 case
// save you 2 allocs (3 vs. 2).

func IPString(ip net.IP) string {
	p := ip

	if len(ip) == 0 {
		return "<nil>"
	}

	// If IPv4, use dotted notation.
	if p4 := p.To4(); len(p4) == net.IPv4len {
		return convertIPv4(p4)
	}
	if len(p) != net.IPv6len {
		return "?" + hexString(ip)
	}

	return convertIPv6(ip)
}

func appendInt(a []byte, val uint) []byte {
	if val == 0 { // avoid string allocation
		return append(a, '0')
	}
	var buf [20]byte
	i := len(buf) - 1
	for val >= 10 {
		q := val / 10
		buf[i] = byte('0' + val - q*10)
		i--
		val = q
	}
	// val < 10
	buf[i] = byte('0' + val)
	return append(a, buf[i:]...)
}

func convertIPv4(ip net.IP) string {
	var a [83]byte
	b := a[:0]
	b = appendInt(b, uint(ip[0]))
	b = append(b, '.')
	b = appendInt(b, uint(ip[1]))
	b = append(b, '.')
	b = appendInt(b, uint(ip[2]))
	b = append(b, '.')
	b = appendInt(b, uint(ip[3]))
	return string(b)
}

func convertIPv6(ip net.IP) string {
	// Find longest run of zeros.
	e0 := -1
	e1 := -1
	for i := 0; i < net.IPv6len; i += 2 {
		j := i
		for j < net.IPv6len && ip[j] == 0 && ip[j+1] == 0 {
			j += 2
		}
		if j > i && j-i > e1-e0 {
			e0 = i
			e1 = j
			i = j
		}
	}
	// The symbol "::" MUST NOT be used to shorten just one 16 bit 0 field.
	if e1-e0 <= 2 {
		e0 = -1
		e1 = -1
	}

	var a [len("ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff")]byte
	b := a[:0]
	for i := 0; i < net.IPv6len; i += 2 {
		if i == e0 {
			b = append(b, ':', ':')
			i = e1
			if i >= net.IPv6len {
				break
			}
		} else if i > 0 {
			b = append(b, ':')
		}
		b = appendHex(b, (uint32(ip[i])<<8)|uint32(ip[i+1]))
	}
	return string(b)
}

const hexDigit = "0123456789abcdef"

func appendHex(dst []byte, i uint32) []byte {
	if i == 0 {
		return append(dst, '0')
	}
	for j := 7; j >= 0; j-- {
		v := i >> uint(j*4)
		if v > 0 {
			dst = append(dst, hexDigit[v&0xf])
		}
	}
	return dst
}

func hexString(b []byte) string {
	s := make([]byte, len(b)*2)
	for i, tn := range b {
		s[i*2], s[i*2+1] = hexDigit[tn>>4], hexDigit[tn&0xf]
	}
	return string(s)
}
