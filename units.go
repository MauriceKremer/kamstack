package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// The two units setup installs. systemd owns the processes; kamstack only
// writes the files and starts or stops them (DST-16, DST-17).
const kamUnitTemplate = `[Unit]
Description=KAM MeshCore node
Wants=network-online.target
After=network-online.target

[Service]
User=%[1]s
SupplementaryGroups=dialout
ExecStart=%[2]s -config %[3]s
Restart=on-failure
RestartSec=5
StateDirectory=kamstack
ProtectSystem=strict
ReadWritePaths=%[4]s %[5]s

[Install]
WantedBy=multi-user.target
`

const kammonUnitTemplate = `[Unit]
Description=KAMmon web monitor
Wants=network-online.target
After=network-online.target

[Service]
User=%[1]s
ExecStart=%[2]s -config %[3]s
Restart=on-failure
RestartSec=5
ProtectSystem=strict

[Install]
WantedBy=multi-user.target
`

func kamUnit(a *app) string {
	return fmt.Sprintf(kamUnitTemplate,
		serviceUser,
		a.binary("kam"),
		a.kamConfigPath(),
		a.configDir,
		a.dataDir,
	)
}

func kammonUnit(a *app) string {
	return fmt.Sprintf(kammonUnitTemplate,
		serviceUser,
		a.binary("kammon"),
		a.kammonConfigPath(),
	)
}

func writeUnits(a *app) error {
	units := map[string]string{
		kamService + ".service":    kamUnit(a),
		kammonService + ".service": kammonUnit(a),
	}
	for name, content := range units {
		path := filepath.Join(a.unitDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}
