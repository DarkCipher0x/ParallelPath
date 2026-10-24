package install

import (
	"strings"
	"text/template"
)

// Linux layout.
const (
	linuxBin        = "/usr/local/bin/ppserver"
	linuxUser       = "parallelpath"
	linuxUnit       = "parallelpath.service"
	linuxVolumeUnit = "parallelpath-volume.service"
	linuxUnitDir    = "/etc/systemd/system"
	linuxCredDir    = "/etc/credstore.encrypted"
	linuxMasterCred = linuxCredDir + "/parallelpath-master_key"
	linuxVolumeCred = linuxCredDir + "/parallelpath-volume_key"
	linuxMarker     = "/etc/parallelpath/install.json"
	linuxMapper     = "parallelpath-data"
)

type unitData struct {
	Bin, DataDir, BackupDir   string
	Container                 bool
	ContainerFile             string
	Cryptsetup, Mount, Umount string
	LowPort                   bool // a port below 1024 needs CAP_NET_BIND_SERVICE
}

// The server runs as its own unprivileged user in a tight sandbox: no capabilities (except binding a low port when
// asked), a read-only system, no home directories, no devices, only the syscalls a normal service makes, and write
// access to nothing but its data and backup folders. The master key comes from a systemd encrypted credential.
var mainUnit = template.Must(template.New("main").Parse(`# Written by ppserver install - changes are replaced on upgrade.
[Unit]
Description=ParallelPath server (post-quantum co-parenting)
After=network-online.target{{if .Container}} ` + linuxVolumeUnit + `{{end}}
Wants=network-online.target
{{- if .Container}}
Requires=` + linuxVolumeUnit + `
{{- end}}

[Service]
Type=simple
User=` + linuxUser + `
Group=` + linuxUser + `
ExecStart={{.Bin}} run --data {{.DataDir}}
LoadCredentialEncrypted=master_key:` + linuxMasterCred + `
Restart=on-failure
RestartSec=5
UMask=0077
{{- if .LowPort}}
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
{{- else}}
AmbientCapabilities=
CapabilityBoundingSet=
{{- end}}
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths={{.DataDir}}{{if .BackupDir}} {{.BackupDir}}{{end}}
PrivateTmp=yes
PrivateDevices=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
ProtectProc=invisible
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX AF_NETLINK
RestrictNamespaces=yes
RestrictRealtime=yes
RestrictSUIDSGID=yes
LockPersonality=yes
MemoryDenyWriteExecute=yes
RemoveIPC=yes
SystemCallArchitectures=native
SystemCallFilter=@system-service
SystemCallFilter=~@privileged @resources
SystemCallErrorNumber=EPERM
ProcSubset=pid
{{- if not .LowPort}}
PrivateUsers=yes
{{- end}}

[Install]
WantedBy=multi-user.target
`))

// The optional encrypted container: a LUKS2 file whose key is a second systemd encrypted credential, opened and
// mounted on the data folder before the server starts.
var volumeUnit = template.Must(template.New("volume").Parse(`# Written by ppserver install - changes are replaced on upgrade.
[Unit]
Description=ParallelPath encrypted data volume
After=local-fs.target
Before=` + linuxUnit + `
ConditionPathExists={{.ContainerFile}}

[Service]
Type=oneshot
RemainAfterExit=yes
LoadCredentialEncrypted=volume_key:` + linuxVolumeCred + `
ExecStart={{.Cryptsetup}} open --type luks2 --key-file ${CREDENTIALS_DIRECTORY}/volume_key {{.ContainerFile}} ` + linuxMapper + `
ExecStart={{.Mount}} -o nodev,nosuid,noexec /dev/mapper/` + linuxMapper + ` {{.DataDir}}
ExecStop={{.Umount}} {{.DataDir}}
ExecStop={{.Cryptsetup}} close ` + linuxMapper + `

[Install]
WantedBy=multi-user.target
`))

func render(t *template.Template, d unitData) string {
	var b strings.Builder
	if err := t.Execute(&b, d); err != nil {
		panic(err) // the templates are fixed; a failure is a programming error
	}
	return b.String()
}
