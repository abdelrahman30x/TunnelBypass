package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// LinuxRollbackDir is the fixed directory for managed Linux rollback state (append-only script).
const LinuxRollbackDir = "/usr/local/etc/tunnelbypass"

// LinuxRollbackScriptPath returns the append-only rollback script path.
func LinuxRollbackScriptPath() string {
	return filepath.Join(LinuxRollbackDir, ".rollback.sh")
}

// AppendRollbackInverse appends one shell line that undoes a reversible change. Lines are executed
// in reverse order by RunLinuxRollback.
func AppendRollbackInverse(line string) error {
	line = strings.TrimSpace(line)
	if line == "" || runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	_ = os.MkdirAll(LinuxRollbackDir, 0755)
	path := LinuxRollbackScriptPath()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0755)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := fmt.Fprintf(f, "%s\n", line); err != nil {
		return err
	}
	return nil
}

// RunLinuxRollback executes inverse lines from the rollback file in reverse order (last change first).
// Best-effort: continues on individual line failure (no set -e) so partial undo still runs.
//
// Callers: installers invoke this on failed ApplyLinuxTransitNetworking (transactional undo). For
// per-service uninstall, use only when RemainingTunnelBypassSystemdUnitCount() == 0 so removing one
// inbound does not strip sysctl/iptables still needed by another TunnelBypass service (see ui_cleanup).
func RunLinuxRollback() {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return
	}
	path := LinuxRollbackScriptPath()
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return
	}
	lines := strings.Split(string(b), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_ = exec.Command("sh", "-c", line).Run()
	}
}
