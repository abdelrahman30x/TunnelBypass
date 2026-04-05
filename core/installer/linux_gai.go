package installer

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const gaiIPv4Line = "precedence  ::ffff:0:0/96  100"

// ensureGaiConfIPv4Preference appends a single IPv4-preference line to /etc/gai.conf when
// TUNNELBYPASS_LINUX_GAI=1 (last-resort; queryStrategy/sysctl should be tried first).
func ensureGaiConfIPv4Preference() error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil
	}
	if strings.TrimSpace(os.Getenv("TUNNELBYPASS_LINUX_GAI")) != "1" {
		return nil
	}
	path := "/etc/gai.conf"
	fi, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		fmt.Fprintf(os.Stderr, "[!] Skipped %s (symlink).\n", path)
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if bytes.Contains(b, []byte("::ffff:0:0/96")) {
		return nil
	}
	ts := time.Now().UTC().Format("20060102T150405.000000000")
	bak := fmt.Sprintf("%s.bak.%s", path, ts)
	if err := os.WriteFile(bak, b, fi.Mode().Perm()); err != nil {
		return err
	}
	fmt.Printf("[*] Backed up %s to %s\n", path, filepath.Base(bak))
	nb := append(bytes.TrimSuffix(b, []byte("\n")), []byte("\n\n# TunnelBypass — prefer IPv4 mapped addresses\n"+gaiIPv4Line+"\n")...)
	if err := os.WriteFile(path, nb, fi.Mode().Perm()); err != nil {
		return err
	}
	fmt.Printf("[*] Appended IPv4 precedence hint to %s\n", path)
	_ = AppendRollbackInverse(fmt.Sprintf("cp -f %q %q", bak, path))
	return nil
}
