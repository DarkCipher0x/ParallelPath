package turn

import (
	"net"
	"testing"
	"time"

	"github.com/pion/turn/v4"
)

// The relay may reach its own address only on its own relay ports (another caller's relayed address).
func TestGuardedRelaySockets(t *testing.T) {
	g := &guardedGenerator{self: net.ParseIP("203.0.113.7"), minPort: 49160, maxPort: 49200}
	for addr, want := range map[string]bool{
		"203.0.113.7:49170": true,  // another caller's relayed address on this server
		"203.0.113.7:53":    false, // a DNS server on this machine
		"203.0.113.7:51820": false, // a VPN on this machine
		"198.51.100.20:53":  true,  // somebody else's address: an ordinary peer
	} {
		a, _ := net.ResolveUDPAddr("udp", addr)
		if got := g.allowed(a); got != want {
			t.Errorf("%s: allowed=%v, want %v", addr, got, want)
		}
	}
}

// Wrong or expired credentials get no allocation, and the relay refuses permissions for loopback peers.
func TestRelayAccess(t *testing.T) {
	srv, err := Start(Config{Listen: "127.0.0.1:0", RelayIP: "127.0.0.1", PortRange: "49160-49200", Realm: "test",
		Valid:    func(u string) bool { return u == "good" },
		Password: func(string) string { return "secret" }})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	addr := srv.udp.LocalAddr().String()
	dial := func(user, pass string) (*turn.Client, net.PacketConn) {
		conn, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		c, err := turn.NewClient(&turn.ClientConfig{STUNServerAddr: addr, TURNServerAddr: addr, Conn: conn, Username: user,
			Password: pass, Realm: "test", RTO: 200 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Listen(); err != nil {
			t.Fatal(err)
		}
		return c, conn
	}
	for _, cred := range [][2]string{{"good", "wrong"}, {"expired", "secret"}} {
		c, conn := dial(cred[0], cred[1])
		if _, err := c.Allocate(); err == nil {
			t.Errorf("allocation with %s/%s worked", cred[0], cred[1])
		}
		c.Close()
		conn.Close()
	}
	c, conn := dial("good", "secret")
	defer conn.Close()
	defer c.Close()
	relay, err := c.Allocate()
	if err != nil {
		t.Fatalf("a valid allocation failed: %v", err)
	}
	defer relay.Close()
	target, _ := net.ResolveUDPAddr("udp", "127.0.0.1:9")
	if err := c.CreatePermission(target); err == nil {
		t.Error("the relay agreed to forward to this machine's loopback")
	}
}
