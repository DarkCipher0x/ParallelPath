module pp/engine

go 1.27

require (
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/mobile v0.0.0-20261011034528-197770444867 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
)

require (
	go.etcd.io/bbolt v1.5.0
	pp/core v0.0.0
)

replace pp/core => ../core

tool golang.org/x/mobile/cmd/gobind
