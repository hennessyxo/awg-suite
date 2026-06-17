// Package amneziawg embeds shared repo assets so binaries can be fully
// self-contained. Currently it carries the installer script, which the SSH
// deploy tool (cmd/awg-deploy) and the GUI pipe to a remote server.
package amneziawg

import (
	_ "embed"
	"strings"
)

//go:embed amneziawg-install.sh
var installerScriptRaw string

// InstallerScript is the installer with CR stripped. A Windows checkout can
// introduce CRLF line endings, which break `bash -s` on the Linux server with
// `$'\r': command not found`; normalizing here makes every consumer safe
// regardless of how the file was checked out at build time.
var InstallerScript = strings.ReplaceAll(installerScriptRaw, "\r", "")
