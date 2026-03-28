package installer

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"tunnelbypass/internal/utils"
)

func EnsureSSHServer() error {
	return EnsureSSHServerWithAuth("", "")
}

func ensureOpenSSHServerInstall() error {
	if PortListening(22) {
		return nil
	}

	if runtime.GOOS != "windows" {
		for _, pkg := range [][]string{
			{"apt-get", "install", "-y", "openssh-server"},
			{"yum", "install", "-y", "openssh-server"},
			{"dnf", "install", "-y", "openssh-server"},
			{"apk", "add", "--no-cache", "openssh"},
			{"pacman", "-Sy", "--noconfirm", "openssh"},
		} {
			if _, err := exec.LookPath(pkg[0]); err != nil {
				continue
			}
			cmd := exec.Command(pkg[0], pkg[1:]...)
			cmd.Env = envForPkgManager(pkg[0])
			_ = cmd.Run()
			if PortListening(22) {
				return nil
			}
			_ = exec.Command("systemctl", "enable", "sshd").Run()
			_ = exec.Command("systemctl", "start", "sshd").Run()
			if PortListening(22) {
				return nil
			}
		}
		return fmt.Errorf("sshd not found and auto-install failed")
	}

	url := "https://github.com/PowerShell/Win32-OpenSSH/releases/latest/download/OpenSSH-Win64.zip"
	extractDir := filepath.Join(systemDriveRoot(), "OpenSSH")
	zipPath := `"$env:TEMP\openssh-win64.zip"`

	ps := strings.Join([]string{
		"$ErrorActionPreference='Stop'",
		"function Test-PortListening {",
		"  param([int]$P)",
		"  $listening = Get-NetTCPConnection -LocalPort $P -State Listen -ErrorAction SilentlyContinue",
		"  return [bool]$listening",
		"}",
		"$url = '" + url + "'",
		"$zipPathResolved = " + zipPath,
		"try {",
		"  [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12",
		"  Invoke-WebRequest -Uri $url -OutFile $zipPathResolved -UseBasicParsing -TimeoutSec 120",
		"} catch { throw $_ }",
		"if (Test-Path '" + extractDir + "') { Remove-Item '" + extractDir + "' -Recurse -Force -ErrorAction SilentlyContinue }",
		"New-Item -ItemType Directory -Force -Path '" + extractDir + "' | Out-Null",
		"Expand-Archive -Path $zipPathResolved -DestinationPath '" + extractDir + "' -Force",
		"$installScript = Join-Path '" + extractDir + "' 'OpenSSH-Win64\\install-sshd.ps1'",
		"if (-not (Test-Path $installScript)) { throw 'OpenSSH install-sshd.ps1 not found after extraction' }",
		"& powershell.exe -ExecutionPolicy Bypass -File $installScript",
		"try { Start-Service sshd -ErrorAction SilentlyContinue } catch {}",
		"try { Set-Service sshd -StartupType Automatic -ErrorAction SilentlyContinue } catch {}",
		"try { Start-Service sshd -ErrorAction SilentlyContinue } catch {}",
		"try { if (-not (Get-NetFirewallRule -Name 'sshd' -ErrorAction SilentlyContinue)) { New-NetFirewallRule -Name 'sshd' -DisplayName 'SSH' -Enabled True -Direction Inbound -Protocol TCP -Action Allow -LocalPort 22 | Out-Null } } catch {}",
		"Start-Sleep -Seconds 2",
		"$status = (Get-Service sshd).Status",
		"if ($status -ne 'Running') { Start-Service sshd; Start-Sleep -Seconds 2 }",
		"if (-not (Test-PortListening -P 22)) { throw 'sshd is not listening on port 22 after install/start' }",
		"return 0",
	}, ";")

	cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("windows ssh server ensure failed: %v", err)
	}
	return nil
}

// PortListening reports whether something accepts TCP connections on loopback for this port.
// Uses an actual connect (not parsing ss/netstat) so minimal images without iproute2 still work.
func PortListening(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}
	s := strconv.Itoa(port)
	for _, host := range []string{"127.0.0.1", "::1"} {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, s), 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
	}
	return false
}

func EnsureFreeTCPPort(preferred int, label string) int {
	p := utils.AllocatePort("tcp", preferred)
	if p == 0 {
		fmt.Printf("    [!] Warning: could not allocate free TCP port for %s. Keeping %d (may fail).\n", label, preferred)
		return preferred
	}
	// preferred <= 0 means "pick any free port" — do not print "port 0 is busy".
	if preferred > 0 && p != preferred {
		fmt.Printf("    [!] Port %d is busy for %s. Using free port %d.\n", preferred, label, p)
	}
	return p
}

const UDPGWServiceName = "TunnelBypass-UDPGW"

func udpgwServiceExists() bool {
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/etc/systemd/system/TunnelBypass-UDPGW.service"); err == nil {
			return true
		}
	}
	return false
}

// EnsureSSHUDPGW installs TunnelBypass-UDPGW (`run udpgw`) so it appears as its own service (uninstall UX).
// TunnelBypass-SSH should be installed with --external-udpgw so SSH does not bind UDPGW twice.
// Returns the TCP port UDPGW listens on (for matching --udpgw-port on the SSH service).
func EnsureSSHUDPGW(preferred int) (int, error) {
	if preferred <= 0 {
		preferred = 7300
	}
	if udpgwServiceExists() {
		fmt.Printf("    [*] UDPGW service already exists, skipping installation\n")
		if err := waitUDPGWListen(preferred, 30*time.Second); err != nil {
			return preferred, err
		}
		return preferred, nil
	}
	if PortListening(preferred) {
		fmt.Printf("    [*] UDPGW already listening on port %d, skipping service creation\n", preferred)
		return preferred, nil
	}

	port := EnsureFreeTCPPort(preferred, "UDPGW")

	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("failed to get executable path for UDPGW service: %w", err)
	}
	args := []string{"run", "udpgw", "--udpgw-port", strconv.Itoa(port)}

	if err := CreateService(
		UDPGWServiceName,
		UDPGWServiceName+" (TunnelBypass UDPGW)",
		exe,
		args,
		GetBaseDir(),
	); err != nil {
		return 0, err
	}

	_ = OpenFirewallPort(port, "tcp", UDPGWServiceName)
	fmt.Printf("    [*] UDPGW service installed on port %d\n", port)
	if err := waitUDPGWListen(port, 45*time.Second); err != nil {
		return port, err
	}
	return port, nil
}

func waitUDPGWListen(port int, total time.Duration) error {
	deadline := time.Now().Add(total)
	for time.Now().Before(deadline) {
		if PortListening(port) {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for UDPGW to listen on %d", port)
}
