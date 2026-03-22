package installer

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

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
			_ = exec.Command(pkg[0], pkg[1:]...).Run()
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

// PortListening checks if a local port is listening (Windows/Linux).
func PortListening(port int) bool {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("netstat", "-ano")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return false
		}
		s := string(out)
		pStr := fmt.Sprintf(":%d", port)
		return strings.Contains(s, pStr) && strings.Contains(strings.ToUpper(s), "LISTENING")
	}
	out, err := exec.Command("ss", "-tunl").CombinedOutput()
	if err != nil {
		out, err = exec.Command("netstat", "-tunl").CombinedOutput()
	}
	if err != nil {
		return false
	}
	pStr := fmt.Sprintf(":%d", port)
	return strings.Contains(string(out), pStr)
}

func EnsureFreeTCPPort(preferred int, label string) int {
	p := utils.AllocatePort("tcp", preferred)
	if p == 0 {
		fmt.Printf("    [!] Warning: could not allocate free TCP port for %s. Keeping %d (may fail).\n", label, preferred)
		return preferred
	}
	if p != preferred {
		fmt.Printf("    [!] Port %d is busy for %s. Using free port %d.\n", preferred, label, p)
	}
	return p
}

const UDPGWServiceName = "TunnelBypass-UDPGW"

func EnsureSSHUDPGW(port int) error {
	if port <= 0 {
		port = 7300
	}
	port = EnsureFreeTCPPort(port, "UDPGW")

	serviceName := UDPGWServiceName
	args := []string{"udpgw-svc", "-udpgw-port", strconv.Itoa(port)}

	if err := CreateService(
		serviceName,
		serviceName+" (TunnelBypass UDPGW)",
		serviceExecutable(),
		args,
		GetBaseDir(),
	); err != nil {
		return err
	}

	_ = OpenFirewallPort(port, "tcp", serviceName)
	return nil
}
