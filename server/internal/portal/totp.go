package portal

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP (RFC 6238) is the portal's second factor: a six-digit code from an authenticator app, changing every 30
// seconds. It uses HMAC-SHA1 because that is what every authenticator app supports; with a 160-bit secret and
// codes that last seconds, it stays strong even against quantum attacks.

const totpStep = 30

// newTOTPSecret makes a 160-bit secret.
func newTOTPSecret() []byte {
	b := make([]byte, 20)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// setupKey is the secret as people type it into an authenticator app, in groups of four.
func setupKey(secret []byte) string {
	s := b32.EncodeToString(secret)
	var parts []string
	for len(s) > 4 {
		parts, s = append(parts, s[:4]), s[4:]
	}
	return strings.Join(append(parts, s), " ")
}

// otpURI is the otpauth:// link authenticator apps understand.
func otpURI(secret []byte, account string) string {
	label := url.PathEscape("ParallelPath Server:" + account)
	return "otpauth://totp/" + label + "?secret=" + b32.EncodeToString(secret) + "&issuer=" + url.QueryEscape("ParallelPath Server") +
		"&algorithm=SHA1&digits=6&period=30"
}

func totpAt(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	m := hmac.New(sha1.New, secret)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", v%1_000_000)
}

// checkTOTP accepts the code for now or one step either side (clock drift), and never a step already used
// (lastUsed), so an overheard code can't be replayed. It returns the step that matched.
func checkTOTP(secret []byte, code string, now time.Time, lastUsed int64) (int64, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != 6 {
		return 0, false
	}
	step := now.Unix() / totpStep
	for _, s := range []int64{step, step - 1, step + 1} {
		if s > lastUsed && hmac.Equal([]byte(totpAt(secret, s)), []byte(code)) {
			return s, true
		}
	}
	return 0, false
}
