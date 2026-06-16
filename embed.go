// Package amneziawg embeds shared repo assets so binaries can be fully
// self-contained. Currently it carries the installer script, which the SSH
// deploy tool (cmd/awg-deploy) pipes to a remote server.
package amneziawg

import _ "embed"

// InstallerScript is the contents of amneziawg-install.sh.
//
//go:embed amneziawg-install.sh
var InstallerScript string
