module pp/server

go 1.27

require (
	github.com/pion/turn/v4 v4.1.4
	go.etcd.io/bbolt v1.5.0
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0
	golang.org/x/term v0.47.0
	pp/core v0.0.0
	pp/engine v0.0.0-00010101000000-000000000000
)

require (
	github.com/pion/dtls/v3 v3.1.4 // indirect
	github.com/pion/logging v0.2.4 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/stun/v3 v3.1.5 // indirect
	github.com/pion/transport/v4 v4.0.2 // indirect
	github.com/wlynxg/anet v0.0.5 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace (
	pp/core => ../core
	pp/engine => ../engine
)
