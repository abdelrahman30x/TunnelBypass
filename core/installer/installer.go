// Package installer: paths, TLS, downloads, OS services.
package installer

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kardianos/service"

	"tunnelbypass/core/binmgr"
	"tunnelbypass/core/layout"
	"tunnelbypass/core/svcman"
	"tunnelbypass/internal/supervisor"
)

const (
	XrayVersion              = "v24.11.11"
	WstunnelVersion          = "v10.5.2"
	HysteriaVersion          = "v2.6.1"
	WireGuardMSIVersion      = "0.5.3"
	WinSWVersion             = "v2.12.0"
	OpenSSLWin64LightVersion = "3_5_0"
	OpenSSLUnixMinVersion    = "1.1.1"
	StunnelVersion           = "latest"
)

// Delegates to layout.SetDataRootOverride.
func SetDataRootOverride(dir string) { layout.SetDataRootOverride(dir) }

// Delegates to layout.SetLogsRootOverride.
func SetLogsRootOverride(dir string) { layout.SetLogsRootOverride(dir) }

// Delegates to layout.GetLogsDir.
func GetLogsDir() string { return layout.GetLogsDir() }

// Delegates to layout.DataRootOverride.
func DataRootOverride() string { return layout.DataRootOverride() }

// Delegates to layout.PortableDefaultDataDir.
func PortableDefaultDataDir() string { return layout.PortableDefaultDataDir() }

// Delegates to layout.GetBaseDir.
func GetBaseDir() string { return layout.GetBaseDir() }

func svcmanDeps() svcman.Deps {
	return svcman.Deps{
		BaseDir:     GetBaseDir,
		EnsureWinSW: EnsureWinSW,
		CopyFile:    copyFile,
		Logf: func(format string, args ...any) {
			slog.Info(fmt.Sprintf(format, args...))
		},
	}
}

func GetConfigDir(transport string) string { return layout.GetConfigDir(transport) }

func GetSystemBinaryDir(name string) string {
	return binmgr.SystemBinaryDir(name, GetBaseDir(), layout.PortableLayoutActive())
}

func isTempExecutablePath(p string) bool {
	low := strings.ToLower(p)
	if strings.Contains(low, `\appdata\local\temp\`) && strings.Contains(low, `\go-build`) {
		return true
	}
	return strings.Contains(low, `/tmp/`) && strings.Contains(low, `go-build`)
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	_ = os.MkdirAll(filepath.Dir(dst), 0755)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	_ = out.Chmod(perm)
	return nil
}

func serviceExecutable() string {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return exe
	}
	if !isTempExecutablePath(exe) {
		return exe
	}

	baseDir := GetBaseDir()
	binDir := filepath.Join(baseDir, "bin")
	target := filepath.Join(binDir, "tunnelbypass-service")
	if runtime.GOOS == "windows" {
		target += ".exe"
	}

	if err := copyFile(exe, target, 0755); err == nil {
		return target
	}
	return exe
}

func EnsureSelfSignedCert(certPath, keyPath, commonName string) error {
	if _, err := os.Stat(certPath); err == nil {
		if _, err2 := os.Stat(keyPath); err2 == nil {
			return nil // both files exist
		}
	}
	if commonName == "" {
		commonName = "localhost"
	}
	_ = os.MkdirAll(filepath.Dir(certPath), 0755)

	openssl := EnsureOpenSSL()
	if openssl == "" {
		openssl, _ = exec.LookPath("openssl")
	}
	if openssl != "" {
		cmd := exec.Command(
			openssl,
			"req", "-x509", "-newkey", "rsa:2048",
			"-nodes",
			"-keyout", keyPath,
			"-out", certPath,
			"-days", "365",
			"-subj", "/CN="+commonName,
		)
		if err := cmd.Run(); err == nil {
			if _, err1 := os.Stat(certPath); err1 == nil {
				if _, err2 := os.Stat(keyPath); err2 == nil {
					return nil
				}
			}
		}
		_ = os.Remove(certPath)
		_ = os.Remove(keyPath)
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-5 * time.Minute),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{commonName},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	cf, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer cf.Close()
	if err := pem.Encode(cf, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return err
	}

	kf, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer kf.Close()
	return pem.Encode(kf, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
}

func EnsureOpenSSL() string {
	binDir := GetSystemBinaryDir("openssl")
	_ = os.MkdirAll(binDir, 0755)

	exeName := "openssl"
	if runtime.GOOS == "windows" {
		exeName = "openssl.exe"
	}
	targetPath := filepath.Join(binDir, exeName)
	if _, err := os.Stat(targetPath); err == nil {
		return targetPath
	}

	if p, err := exec.LookPath("openssl"); err == nil && p != "" {
		if err := copyFile(p, targetPath, 0755); err == nil {
			return targetPath
		}
		return p
	}

	if runtime.GOOS == "windows" {
		installerURL := fmt.Sprintf("https://slproweb.com/download/Win64OpenSSL_Light-%s.exe", OpenSSLWin64LightVersion)
		installerPath := filepath.Join(binDir, "openssl-installer.exe")
		if err := downloadFileWithProgress(installerURL, installerPath); err == nil {
			installDir := filepath.Join(filepath.VolumeName(binDir)+`\`, "OpenSSL")
			cmd := exec.Command(
				installerPath,
				"/verysilent", "/sp-", "/suppressmsgboxes", "/norestart",
				"/DIR="+installDir,
			)
			_ = cmd.Run()
			candidates := []string{
				filepath.Join(installDir, "bin", "openssl.exe"),
				filepath.Join(os.Getenv("ProgramFiles"), "OpenSSL-Win64", "bin", "openssl.exe"),
				filepath.Join(os.Getenv("ProgramFiles(x86)"), "OpenSSL-Win32", "bin", "openssl.exe"),
			}
			for _, c := range candidates {
				if c == "" {
					continue
				}
				if _, err := os.Stat(c); err == nil {
					_ = copyFile(c, targetPath, 0755)
					if _, err := os.Stat(targetPath); err == nil {
						return targetPath
					}
					return c
				}
			}
		}
	}

	if runtime.GOOS != "windows" {
		_ = ensureOpenSSLUnix()
		if p, err := exec.LookPath("openssl"); err == nil && p != "" {
			if err := copyFile(p, targetPath, 0755); err == nil {
				return targetPath
			}
			return p
		}
	}

	return ""
}

func ensureOpenSSLUnix() error {
	if runtime.GOOS == "windows" {
		return nil
	}
	cmds := [][]string{
		{"apt-get", "update"},
		{"apt-get", "install", "-y", "openssl"},
		{"dnf", "install", "-y", "openssl"},
		{"yum", "install", "-y", "openssl"},
		{"apk", "add", "--no-cache", "openssl"},
		{"pacman", "-Sy", "--noconfirm", "openssl"},
		{"zypper", "--non-interactive", "install", "openssl"},
		{"brew", "install", "openssl"},
	}
	for _, c := range cmds {
		if _, err := exec.LookPath(c[0]); err != nil {
			continue
		}
		_ = exec.Command(c[0], c[1:]...).Run()
		if _, err := exec.LookPath("openssl"); err == nil {
			return nil
		}
	}
	return fmt.Errorf("openssl install failed or unsupported package manager (need >= %s)", OpenSSLUnixMinVersion)
}

func GetXrayServiceConfig(name, configPath string) *service.Config {
	exe := serviceExecutable()
	baseDir := GetBaseDir()

	absConfig := configPath
	if !filepath.IsAbs(configPath) {
		absConfig = filepath.Join(baseDir, configPath)
	}

	return &service.Config{
		Name:             name,
		DisplayName:      name + " (TunnelBypass)",
		Description:      name + " (TunnelBypass)",
		Executable:       exe,
		Arguments:        []string{"xray-svc", "-config", absConfig},
		WorkingDirectory: baseDir,
		Option: service.KeyValue{
			"Restart": "always",
		},
	}
}

// GetXrayService wraps Xray for kardianos/service.
func GetXrayService(name, config string) (service.Service, error) {
	conf := GetXrayServiceConfig(name, config)
	return service.New(&XrayServiceWrapper{ConfigPath: config, Binary: "xray"}, conf)
}

func GetHysteriaServiceConfig(name, configPath string) *service.Config {
	exe := serviceExecutable()
	baseDir := GetBaseDir()

	absConfig := configPath
	if !filepath.IsAbs(configPath) {
		absConfig = filepath.Join(baseDir, configPath)
	}

	return &service.Config{
		Name:             name,
		DisplayName:      name + " (TunnelBypass)",
		Description:      name + " (TunnelBypass)",
		Executable:       exe,
		Arguments:        []string{"hysteria-svc", "-config", absConfig},
		WorkingDirectory: baseDir,
		Option: service.KeyValue{
			"Restart": "always",
		},
	}
}

func GetHysteriaService(name, config string) (service.Service, error) {
	conf := GetHysteriaServiceConfig(name, config)
	return service.New(&XrayServiceWrapper{ConfigPath: config, Binary: "hysteria"}, conf)
}

type XrayServiceWrapper struct {
	ConfigPath string
	Binary     string
	cmd        *exec.Cmd
	stopCh     chan struct{}
	stopOnce   sync.Once
}

func (p *XrayServiceWrapper) Start(s service.Service) error {
	if p.stopCh == nil {
		p.stopCh = make(chan struct{})
	}
	go p.run()
	return nil
}

func (p *XrayServiceWrapper) run() {
	baseDir := GetBaseDir()
	pol := supervisor.DefaultPolicy()
	log := slog.Default().With("sub", p.Binary+"-svc")

	logsDir := filepath.Join(baseDir, "logs")
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		log.Error("cannot create logs directory", "path", logsDir, "err", err,
			"hint", "check disk space, permissions, and that the data directory is writable")
	}

	logFile, err := os.OpenFile(filepath.Join(logsDir, "TunnelBypass-Service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Error("failed to open service log file", "err", err,
			"hint", "disk full, read-only filesystem, or permission denied on logs/")
	} else {
		defer func() { _ = logFile.Sync(); _ = logFile.Close() }()
	}

	absConfig := p.ConfigPath
	if !filepath.IsAbs(p.ConfigPath) {
		absConfig = filepath.Join(baseDir, p.ConfigPath)
	}

	var backoff time.Duration
	var shortStreak uint32

	for {
		select {
		case <-p.stopCh:
			log.Info("service shutdown requested")
			return
		default:
		}

		binPath, err := EnsureBinary(p.Binary)
		if err != nil {
			log.Error("cannot locate or download binary", "binary", p.Binary, "err", err,
				"hint", "check network, DNS, proxy, and that the install/bin directory is writable")
			line := fmt.Sprintf("[%s] binary ensure failed: %v\n", time.Now().Format(time.RFC3339), err)
			if logFile != nil {
				_, _ = logFile.WriteString(line)
			}
			backoff = supervisor.NextBackoff(backoff, pol)
			select {
			case <-p.stopCh:
				return
			case <-time.After(backoff):
			}
			continue
		}
		backoff = 0

		if err := ValidateTunnelConfigFile(absConfig); err != nil {
			log.Error("config validation failed", "path", absConfig, "err", err,
				"hint", "fix JSON/YAML or restore from backup")
			line := fmt.Sprintf("[%s] config error: %v\n", time.Now().Format(time.RFC3339), err)
			if logFile != nil {
				_, _ = logFile.WriteString(line)
			}
			backoff = supervisor.NextBackoff(backoff, pol)
			select {
			case <-p.stopCh:
				return
			case <-time.After(backoff):
			}
			continue
		}
		backoff = 0

		log.Info("starting tunnel child", "exe", binPath, "config", absConfig)
		if logFile != nil {
			_, _ = logFile.WriteString(fmt.Sprintf("\n[%s] Starting %s\n", time.Now().Format(time.RFC3339), p.Binary))
		}

		if p.Binary == "hysteria" {
			p.cmd = exec.Command(binPath, "server", "-c", absConfig)
		} else {
			p.cmd = exec.Command(binPath, "run", "-config", absConfig)
		}

		if logFile != nil {
			p.cmd.Stdout = logFile
			p.cmd.Stderr = logFile
		} else {
			p.cmd.Stdout = os.Stdout
			p.cmd.Stderr = os.Stderr
		}

		start := time.Now()
		runErr := p.cmd.Run()
		dur := time.Since(start)

		select {
		case <-p.stopCh:
			log.Info("stopped after child exit (shutdown in progress)")
			return
		default:
		}

		kind := supervisor.ClassifyExit(runErr)
		if kind == supervisor.ExitManual {
			return
		}
		if kind == supervisor.ExitClean {
			log.Warn("child exited cleanly; will restart", "binary", p.Binary, "duration", dur)
			if logFile != nil {
				_, _ = logFile.WriteString(fmt.Sprintf("[%s] %s exited cleanly after %v\n", time.Now().Format(time.RFC3339), p.Binary, dur))
			}
			shortStreak = 0
			backoff = 0
		} else {
			log.Error("child crashed or failed", "binary", p.Binary, "duration", dur, "err", runErr,
				"hint", "inspect logs above; check ports, certs, memory, and upstream connectivity")
			if logFile != nil {
				_, _ = logFile.WriteString(fmt.Sprintf("[%s] %s error: %v\n", time.Now().Format(time.RFC3339), p.Binary, runErr))
			}
			if dur < pol.ShortRun {
				shortStreak++
				if pol.MaxCrashLoops > 0 && shortStreak >= uint32(pol.MaxCrashLoops) {
					log.Error("crash loop limit reached; halting restarts",
						"consecutive_short_runs", shortStreak, "threshold", pol.ShortRun, "limit", pol.MaxCrashLoops,
						"hint", "fix configuration then restart the OS service or run tunnelbypass manually")
					if logFile != nil {
						_, _ = logFile.WriteString(fmt.Sprintf("[%s] HALT: crash loop (%d short runs)\n", time.Now().Format(time.RFC3339), shortStreak))
					}
					return
				}
			} else {
				shortStreak = 0
				backoff = 0
			}
		}

		backoff = supervisor.NextBackoff(backoff, pol)
		log.Info("backoff before restart", "sleep", backoff)
		select {
		case <-p.stopCh:
			return
		case <-time.After(backoff):
		}
	}
}

func (p *XrayServiceWrapper) Stop(s service.Service) error {
	p.stopOnce.Do(func() {
		if p.stopCh == nil {
			p.stopCh = make(chan struct{})
		}
		close(p.stopCh)
	})
	if p.cmd != nil && p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return nil
}
