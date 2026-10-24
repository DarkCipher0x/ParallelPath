//go:build windows

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"

	"pp/server/internal/install"
)

func isWindowsService() bool {
	ok, err := svc.IsWindowsService()
	return err == nil && ok
}

// runWindowsService runs the server under the Service Control Manager. A service has no console, so it logs to
// logs\server.log in the data folder (the previous log is kept as server.log.1).
func runWindowsService(data string) error {
	dir := filepath.Join(data, "logs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "server.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 10<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	log := newLogger(f)
	return svc.Run(install.ServiceName, &service{data: data, log: log})
}

type service struct {
	data string
	log  *slog.Logger
}

func (s *service) Execute(_ []string, req <-chan svc.ChangeRequest, st chan<- svc.Status) (bool, uint32) {
	st <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serve(ctx, s.data, s.log) }()
	st <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-done:
			if errors.Is(err, errRestart) { // the service manager's recovery starts it again
				s.log.Info("restarting at an administrator's request")
				return true, restartExitCode
			}
			if err != nil {
				s.log.Error("server stopped", "err", err)
				return true, 1
			}
			return false, 0
		case c := <-req:
			switch c.Cmd {
			case svc.Interrogate:
				st <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				st <- svc.Status{State: svc.StopPending}
				cancel()
				if err := <-done; err != nil {
					s.log.Error("server stopped", "err", err)
				}
				return false, 0
			}
		}
	}
}
