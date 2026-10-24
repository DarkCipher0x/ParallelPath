// Package admin is the operator's side channel to a running server: a random token in the data folder (readable
// only by the server's account and administrators) that unlocks the loopback-only admin endpoint.
package admin

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"pp/server/internal/config"
)

// TokenPath is the admin token file inside a data directory.
func TokenPath(data string) string { return filepath.Join(data, "admin.token") }

// EnsureToken creates the admin token if there isn't one yet.
func EnsureToken(data string) error {
	if _, err := os.Stat(TokenPath(data)); err == nil {
		return nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return err
	}
	f, err := os.OpenFile(TokenPath(data), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(base64.RawURLEncoding.EncodeToString(b))
	return errors.Join(err, f.Close())
}

// ReadToken reads the admin token.
func ReadToken(data string) ([]byte, error) {
	b, err := os.ReadFile(TokenPath(data))
	if err != nil {
		return nil, fmt.Errorf("can't read the admin token (run as the server's account or an administrator): %w", err)
	}
	return []byte(strings.TrimSpace(string(b))), nil
}

// Invite asks the running server for a one-time code that lets someone create an account.
func Invite(data string) (code string, expires time.Time, err error) {
	c, err := config.Load(data)
	if err != nil {
		return "", time.Time{}, err
	}
	tok, err := ReadToken(data)
	if err != nil {
		return "", time.Time{}, err
	}
	req, _ := http.NewRequest("POST", "http://"+c.AdminListen+"/admin/v1/invite", nil)
	req.Header.Set("X-PP-Admin", string(tok))
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("is the server running? %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("server said %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	parts := strings.SplitN(strings.TrimSpace(string(body)), "\n", 2)
	if len(parts) == 2 {
		expires, _ = time.Parse(time.RFC3339, parts[1])
	}
	return parts[0], expires, nil
}

// Link asks the running server for a one-time admin portal link: "setup" (create an admin account, 24 hours) or
// "local" (sign in on this machine, 2 minutes).
func Link(data, kind string) (string, error) {
	c, err := config.Load(data)
	if err != nil {
		return "", err
	}
	tok, err := ReadToken(data)
	if err != nil {
		return "", err
	}
	req, _ := http.NewRequest("POST", "http://"+c.AdminListen+"/admin/v1/link/"+kind, nil)
	req.Header.Set("X-PP-Admin", string(tok))
	res, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("is the server running? %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server said %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	return strings.TrimSpace(string(body)), nil
}
