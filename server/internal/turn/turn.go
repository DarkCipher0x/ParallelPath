// Package turn is ppserver's built-in TURN relay, so calls connect even when both people are behind mobile or home
// NAT. It only ever forwards media that is already end-to-end encrypted (the frame keys come from the family's
// post-quantum keys), so the relay - like the rest of the server - can't see or hear a call.
//
// Access is by short-lived credentials the server hands to signed-in devices (svc.TurnCredentials), and the relay
// refuses to forward to this machine or to non-unicast addresses, so it can't be used to reach the server's own
// services.
package turn

import (
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"

	"github.com/pion/turn/v4"
)

// Config is the relay's settings.
type Config struct {
	Listen    string // UDP and TCP, e.g. ":3478"
	RelayIP   string // the address peers reach the relay on (this server's public or LAN IP)
	PortRange string // relay ports, e.g. "49160-49200"
	Realm     string
	Valid     func(user string) bool   // the username hasn't expired and its person is allowed in
	Password  func(user string) string // the password that goes with a username
	Log       *slog.Logger
}

// Server is a running relay.
type Server struct {
	srv *turn.Server
	udp net.PacketConn
	tcp net.Listener
}

// Start starts the relay.
func Start(c Config) (*Server, error) {
	relayIP := net.ParseIP(c.RelayIP)
	if relayIP == nil {
		return nil, fmt.Errorf("turn: relay address %q isn't an IP address", c.RelayIP)
	}
	minP, maxP, err := ports(c.PortRange)
	if err != nil {
		return nil, err
	}
	udp, err := net.ListenPacket("udp", c.Listen)
	if err != nil {
		return nil, err
	}
	tcp, err := net.Listen("tcp", c.Listen)
	if err != nil {
		udp.Close()
		return nil, err
	}
	own := ownAddresses()
	permit := func(_ net.Addr, peer net.IP) bool {
		if peer.IsLoopback() || peer.IsUnspecified() || peer.IsMulticast() || peer.IsLinkLocalUnicast() || peer.IsLinkLocalMulticast() {
			return false
		}
		return !own[peer.String()] || peer.Equal(relayIP) // never the server's own other interfaces
	}
	bind := "0.0.0.0" // relay sockets listen where the relay does (e.g. 127.0.0.1 for a test server)
	if h, _, err := net.SplitHostPort(c.Listen); err == nil && h != "" {
		bind = h
	}
	gen := func() turn.RelayAddressGenerator {
		return &guardedGenerator{RelayAddressGeneratorPortRange: &turn.RelayAddressGeneratorPortRange{RelayAddress: relayIP, Address: bind,
			MinPort: minP, MaxPort: maxP}, self: relayIP, minPort: int(minP), maxPort: int(maxP)}
	}
	srv, err := turn.NewServer(turn.ServerConfig{
		Realm: c.Realm,
		AuthHandler: func(user, realm string, _ net.Addr) ([]byte, bool) {
			if !c.Valid(user) {
				return nil, false
			}
			return turn.GenerateAuthKey(user, realm, c.Password(user)), true
		},
		PacketConnConfigs: []turn.PacketConnConfig{{PacketConn: udp, RelayAddressGenerator: gen(), PermissionHandler: permit}},
		ListenerConfigs:   []turn.ListenerConfig{{Listener: tcp, RelayAddressGenerator: gen(), PermissionHandler: permit}},
	})
	if err != nil {
		udp.Close()
		tcp.Close()
		return nil, err
	}
	if c.Log != nil {
		c.Log.Info("call relay (TURN) running", "listen", c.Listen, "relay_ip", relayIP.String(), "ports", c.PortRange)
	}
	return &Server{srv: srv, udp: udp, tcp: tcp}, nil
}

// guardedGenerator hands out relay sockets that can only reach this server's own address on the relay's own
// ports. Two callers relaying through the same server need to reach each other's relay port, so the server's
// address can't simply be refused - but a TURN permission covers a whole IP, and without this a signed-in device
// could send UDP through the relay to any other service on this machine (DNS, NTP, VPNs...), arriving from the
// server itself.
type guardedGenerator struct {
	*turn.RelayAddressGeneratorPortRange
	self             net.IP
	minPort, maxPort int
}

func (g *guardedGenerator) AllocatePacketConn(network string, requestedPort int) (net.PacketConn, net.Addr, error) {
	c, addr, err := g.RelayAddressGeneratorPortRange.AllocatePacketConn(network, requestedPort)
	if err != nil {
		return nil, nil, err
	}
	return &guardedConn{PacketConn: c, g: g}, addr, nil
}

// allowed reports whether a relay socket may send to addr.
func (g *guardedGenerator) allowed(addr net.Addr) bool {
	u, ok := addr.(*net.UDPAddr)
	if !ok {
		return false
	}
	return !u.IP.Equal(g.self) || (u.Port >= g.minPort && u.Port <= g.maxPort)
}

type guardedConn struct {
	net.PacketConn
	g *guardedGenerator
}

func (c *guardedConn) WriteTo(p []byte, addr net.Addr) (int, error) {
	if !c.g.allowed(addr) {
		return len(p), nil // dropped silently, as an unreachable peer would be
	}
	return c.PacketConn.WriteTo(p, addr)
}

// Close stops the relay.
func (s *Server) Close() error { return s.srv.Close() }

func ports(r string) (uint16, uint16, error) {
	a, b, ok := strings.Cut(r, "-")
	lo, err1 := strconv.Atoi(strings.TrimSpace(a))
	hi, err2 := strconv.Atoi(strings.TrimSpace(b))
	if !ok || err1 != nil || err2 != nil || lo < 1024 || hi > 65535 || hi < lo {
		return 0, 0, fmt.Errorf("turn: relay ports must be a range like 49160-49200, got %q", r)
	}
	return uint16(lo), uint16(hi), nil
}

func ownAddresses() map[string]bool {
	own := map[string]bool{}
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok {
			own[n.IP.String()] = true
		}
	}
	return own
}

// GuessRelayIP picks this server's address for the relay when none is configured: the first host that's an IP, or
// the address of the interface used to reach the internet (or the LAN).
func GuessRelayIP(hosts []string) string {
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil && !ip.IsLoopback() {
			return ip.String()
		}
	}
	if c, err := net.Dial("udp", "192.0.2.1:9"); err == nil { // no packet is sent: this only picks a route
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).IP.String()
	}
	return "127.0.0.1"
}
