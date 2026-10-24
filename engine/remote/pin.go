package remote

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"
)

var errPin = errors.New("the server's certificate has changed - this could be an attack; ask the server's owner")

// checkPin accepts the connection when the pinned certificate is the server's own (a self-signed server), or when
// it's the server's own certificate authority (tls_mode local-ca) and the server certificate is a valid one it
// issued for this host - so the CA's routine renewals of the server certificate need nothing from anyone.
func checkPin(cs tls.ConnectionState, host string, pin []byte) error {
	chain := cs.PeerCertificates
	if len(chain) == 0 {
		return errors.New("no server certificate")
	}
	if sum := sha256.Sum256(chain[0].Raw); bytes.Equal(sum[:], pin) {
		return nil
	}
	for _, c := range chain[1:] {
		if sum := sha256.Sum256(c.Raw); !bytes.Equal(sum[:], pin) || !c.IsCA {
			continue
		}
		roots := x509.NewCertPool()
		roots.AddCert(c)
		_, err := chain[0].Verify(x509.VerifyOptions{DNSName: host, Roots: roots, CurrentTime: time.Now(),
			KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
		if err == nil {
			return nil
		}
	}
	return errPin
}

// pinOf is what to pin from a first connection: the server's own CA when it sent one, else its certificate.
func pinOf(chain []*x509.Certificate) []byte {
	pick := chain[0]
	if n := len(chain); n > 1 && chain[n-1].IsCA {
		pick = chain[n-1]
	}
	sum := sha256.Sum256(pick.Raw)
	return sum[:]
}
