package installer

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"tunnelbypass/core/binmgr"
	"tunnelbypass/internal/debug"
)

// Large release archives can stall on slow links; keep timeout generous.
var downloadHTTPClient = &http.Client{
	Timeout: 45 * time.Minute,
}

func verifyEmbeddedChecksum(tool, path string) error {
	var ver string
	switch tool {
	case "xray":
		ver = EffectiveXrayVersion()
	case "hysteria":
		ver = EffectiveHysteriaVersion()
	case "wstunnel":
		ver = EffectiveWstunnelVersion()
	case "stunnel":
		ver = EffectiveStunnelVersion()
	case "shadowsocks":
		ver = EffectiveShadowsocksVersion()
	case "v2ray-plugin":
		ver = EffectiveV2rayPluginVersion()
	case "masterdnsvpn-server", "masterdnsvpn-client":
		ver = EffectiveMasterDnsVPNVersion()
	default:
		return nil
	}
	want := binmgr.ExpectedSHA256(tool, runtime.GOOS, runtime.GOARCH, ver)
	if want == "" {
		return nil
	}
	return binmgr.VerifyFile(path, want)
}

// Path to tool binary (xray, hysteria, …), downloading if missing.
func EnsureBinary(name string) (string, error) {
	debug.Logf("EnsureBinary: %q", name)
	binDir := GetSystemBinaryDir(name)
	_ = os.MkdirAll(binDir, 0755)

	exeName := binmgr.ExecutableFilename(name)
	targetPath := filepath.Join(binDir, exeName)

	if _, err := os.Stat(targetPath); err == nil {
		if !IsVersionMatch(name, targetPath) {
			installedVer := QueryInstalledVersion(name, targetPath)
			requiredVer := versionForTool(name)
			slog.Info("version mismatch detected", "tool", name, "installed", installedVer, "required", requiredVer)

			runningServices := FindRunningServices(name)
			processes, _ := FindRunningProcesses(name)

			if len(runningServices) > 0 || len(processes) > 0 {
				if len(runningServices) > 0 {
					slog.Info("running services found", "tool", name, "services", runningServices)
				}
				if len(processes) > 0 {
					slog.Info("running processes found", "tool", name, "count", len(processes))
				}

				if UpgradePromptFunc != nil {
					if !UpgradePromptFunc(name, installedVer, requiredVer, runningServices, processes) {
						slog.Info("user declined upgrade", "tool", name)
						return targetPath, nil
					}
				}

				_ = StopServicesForTool(name)
				_ = KillProcessesByName(name)
				time.Sleep(1 * time.Second)
			}
			_ = os.Remove(targetPath)
		} else if name == "wstunnel" && !isWstunnelVersionUsable(targetPath) {
			_ = os.Remove(targetPath)
		} else if err := verifyEmbeddedChecksum(name, targetPath); err != nil {
			slog.Info("checksum mismatch, redownloading", "tool", name)
			_ = os.Remove(targetPath)
		} else {
			return targetPath, nil
		}
	}

	var err error

	switch name {
	case "xray":
		err = ensureXrayBinary(binDir, targetPath)
	case "wstunnel":
		slog.Info("downloading wstunnel", "version", EffectiveWstunnelVersion())
		err = ensureWstunnelBinary(binDir, targetPath)
	case "hysteria":
		err = ensureHysteriaBinary(binDir, targetPath)
	case "stunnel":
		slog.Info("ensuring stunnel", "version", EffectiveStunnelVersion())
		err = ensureStunnelBinary(binDir, targetPath)
	case "shadowsocks":
		slog.Info("downloading shadowsocks-rust", "version", EffectiveShadowsocksVersion())
		err = ensureShadowsocksBinary(binDir, targetPath)
	case "v2ray-plugin":
		slog.Info("downloading v2ray-plugin", "version", EffectiveV2rayPluginVersion())
		err = ensureV2rayPluginBinary(binDir, targetPath)
	case "masterdnsvpn-server", "masterdnsvpn-client":
		slog.Info("downloading masterdnsvpn", "version", EffectiveMasterDnsVPNVersion(), "component", name)
		err = ensureMasterDnsVPNBinary(binDir, targetPath, name)
	default:
		return "", fmt.Errorf("unknown binary: %s", name)
	}

	if err != nil {
		return "", fmt.Errorf("failed to ensure %s: %w", name, err)
	}

	if err := verifyEmbeddedChecksum(name, targetPath); err != nil {
		_ = os.Remove(targetPath)
		writeFetchMetaFail(binDir, name, "", versionForTool(name), err.Error())
		return "", fmt.Errorf("checksum verify failed for %s: %w", name, err)
	}

	writeFetchMetaOK(binDir, name, "", versionForTool(name))
	return targetPath, nil
}

func versionForTool(name string) string {
	switch name {
	case "xray":
		return EffectiveXrayVersion()
	case "hysteria":
		return EffectiveHysteriaVersion()
	case "wstunnel":
		return EffectiveWstunnelVersion()
	case "stunnel":
		return EffectiveStunnelVersion()
	case "shadowsocks":
		return EffectiveShadowsocksVersion()
	case "v2ray-plugin":
		return EffectiveV2rayPluginVersion()
	case "masterdnsvpn-server", "masterdnsvpn-client":
		return EffectiveMasterDnsVPNVersion()
	default:
		return ""
	}
}

func ensureXrayBinary(binDir, targetPath string) error {
	ver := EffectiveXrayVersion()
	slog.Info("downloading xray", "version", ver)
	var lastErr error
	for _, url := range getXrayDownloadURLs() {
		// Xray runtime requires geoip.dat/geosite.dat alongside xray executable.
		// Extract the full archive into binDir instead of only matching "xray".
		if err := downloadAndExtractZip(url, binDir, ""); err != nil {
			writeFetchMetaFail(binDir, "xray", url, ver, err.Error())
			lastErr = err
			continue
		}
		if err := verifyEmbeddedChecksum("xray", targetPath); err != nil {
			writeFetchMetaFail(binDir, "xray", url, ver, err.Error())
			lastErr = err
			_ = os.Remove(targetPath)
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no download URLs")
	}
	return fmt.Errorf("xray: %w", lastErr)
}

func ensureHysteriaBinary(binDir, targetPath string) error {
	ver := EffectiveHysteriaVersion()
	slog.Info("downloading hysteria", "version", ver)
	var lastErr error
	for _, url := range getHysteriaDownloadURLs() {
		if err := downloadFileWithProgress(url, targetPath); err != nil {
			writeFetchMetaFail(binDir, "hysteria", url, ver, err.Error())
			lastErr = err
			continue
		}
		if err := verifyEmbeddedChecksum("hysteria", targetPath); err != nil {
			writeFetchMetaFail(binDir, "hysteria", url, ver, err.Error())
			lastErr = err
			_ = os.Remove(targetPath)
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no download URLs")
	}
	return fmt.Errorf("hysteria: %w", lastErr)
}

func EnsureXrayCore() (string, error) {
	return EnsureBinary("xray")
}

func isWstunnelVersionUsable(binPath string) bool {
	out, err := exec.Command(binPath, "--version").CombinedOutput()
	if err != nil {
		return false
	}
	want := strings.TrimPrefix(EffectiveWstunnelVersion(), "v")
	return strings.Contains(strings.ToLower(string(out)), strings.ToLower(want))
}

func buildXrayZipURL(ver string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	xrayArch := arch
	if arch == "amd64" {
		xrayArch = "64"
	}
	if osName == "windows" {
		return fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/%s/Xray-windows-%s.zip", ver, xrayArch)
	}
	return fmt.Sprintf("https://github.com/XTLS/Xray-core/releases/download/%s/Xray-%s-%s.zip", ver, strings.ToTitle(string(osName[0]))+osName[1:], xrayArch)
}

func getXrayDownloadURLs() []string {
	ver := EffectiveXrayVersion()
	return []string{buildXrayZipURL(ver)}
}

func buildHysteriaURL(ver string) string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	ext := ""
	if osName == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("https://github.com/apernet/hysteria/releases/download/app%%2F%s/hysteria-%s-%s%s", ver, osName, arch, ext)
}

func getHysteriaDownloadURLs() []string {
	ver := EffectiveHysteriaVersion()
	return []string{buildHysteriaURL(ver)}
}

func getWstunnelDownloadURLs() []string {
	ver := EffectiveWstunnelVersion()
	osName := runtime.GOOS
	arch := runtime.GOARCH
	var out []string

	ext := ""
	if osName == "windows" {
		ext = ".exe"
	}
	out = append(out, fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/%s/wstunnel_%s_%s%s", ver, osName, arch, ext))

	verNoPrefix := strings.TrimPrefix(ver, "v")
	out = append(out,
		fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/%s/wstunnel_%s_%s_%s.tar.gz", ver, verNoPrefix, osName, arch),
		fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/%s/wstunnel_%s_%s_%s%s", ver, verNoPrefix, osName, arch, ext),
	)

	const fallbackVersion = "v10.5.2"
	fallbackNoPrefix := strings.TrimPrefix(fallbackVersion, "v")
	out = append(out,
		fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/%s/wstunnel_%s_%s%s", fallbackVersion, osName, arch, ext),
		fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/%s/wstunnel_%s_%s.tar.gz", fallbackVersion, osName, arch),
		fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/%s/wstunnel_%s_%s_%s.tar.gz", fallbackVersion, fallbackNoPrefix, osName, arch),
		fmt.Sprintf("https://github.com/erebe/wstunnel/releases/download/%s/wstunnel_%s_%s_%s%s", fallbackVersion, fallbackNoPrefix, osName, arch, ext),
	)

	return out
}

func ensureWstunnelBinary(binDir, targetPath string) error {
	tmpDir := filepath.Join(binDir, "_dl")
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	var lastErr error
	for _, u := range getWstunnelDownloadURLs() {
		if strings.HasSuffix(strings.ToLower(u), ".tar.gz") {
			archivePath := filepath.Join(tmpDir, "wstunnel.tar.gz")
			if err := downloadFileWithProgress(u, archivePath); err != nil {
				lastErr = err
				continue
			}
			if err := extractTarGzBinary(archivePath, targetPath, "wstunnel"); err != nil {
				lastErr = err
				continue
			}
			return nil
		}

		if err := downloadFileWithProgress(u, targetPath); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no download URLs available")
	}
	return fmt.Errorf("wstunnel download failed from all known URLs: %w", lastErr)
}

func extractTarGzBinary(archivePath, targetPath, binBase string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr == nil {
			continue
		}

		base := strings.ToLower(filepath.Base(hdr.Name))
		want := binBase
		if runtime.GOOS == "windows" {
			want = binBase + ".exe"
		}
		if base != strings.ToLower(want) {
			continue
		}

		out, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return err
		}
		if err := out.Close(); err != nil {
			return err
		}
		if runtime.GOOS != "windows" {
			_ = os.Chmod(targetPath, 0755)
		}
		return nil
	}

	return fmt.Errorf("binary not found in archive: %s", archivePath)
}

func downloadAndExtractZip(url, targetDir, filterName string) error {
	tmpZip := filepath.Join(os.TempDir(), "temp.zip")
	if err := downloadFileWithProgress(url, tmpZip); err != nil {
		return err
	}
	defer os.Remove(tmpZip)

	zipReader, err := zip.OpenReader(tmpZip)
	if err != nil {
		return err
	}
	defer zipReader.Close()

	for _, f := range zipReader.File {
		fpath := filepath.Join(targetDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}
		if filterName != "" && !strings.Contains(strings.ToLower(f.Name), strings.ToLower(filterName)) {
			continue
		}
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// ProgressReader counts bytes for download progress.
type ProgressReader struct {
	io.Reader
	Total      int64
	Downloaded int64
}

func (pr *ProgressReader) Read(p []byte) (n int, err error) {
	n, err = pr.Reader.Read(p)
	pr.Downloaded += int64(n)
	return
}

func downloadFileWithProgress(url, dst string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w (check network, DNS, firewall, and proxy)", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	progress := &ProgressReader{
		Reader: resp.Body,
		Total:  resp.ContentLength,
	}

	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			if progress.Total > 0 {
				percent := float64(progress.Downloaded) / float64(progress.Total) * 100
				fmt.Printf("\r    [>] Downloading: %.1f%% (%d/%d MB)", percent, progress.Downloaded/1024/1024, progress.Total/1024/1024)
			} else {
				fmt.Printf("\r    [>] Downloading: %d MB", progress.Downloaded/1024/1024)
			}
		}
	}()

	_, copyErr := io.Copy(out, progress)
	fmt.Println()
	if copyErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("download failed writing %s: %w (disk full, antivirus lock, or network drop?)", dst, copyErr)
	}

	return os.Chmod(dst, 0755)
}

// Stunnel binary path; on Windows prefers tstunnel.exe next to stunnel if present.
func EnsureStunnel() (string, error) {
	p, err := EnsureBinary("stunnel")
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		tstunnel := filepath.Join(filepath.Dir(p), "tstunnel.exe")
		if _, err := os.Stat(tstunnel); err == nil {
			return tstunnel, nil
		}
	}
	return p, nil
}

func ensureStunnelBinary(binDir, targetPath string) error {
	_ = os.MkdirAll(binDir, 0755)

	if runtime.GOOS == "windows" {
		sv := EffectiveStunnelVersion()
		urls := []string{
			fmt.Sprintf("https://www.stunnel.org/downloads/stunnel-%s-win64-installer.exe", sv),
			fmt.Sprintf("https://github.com/stunnel/static-stunnel/releases/download/%s/stunnel-%s-win-x86_64.zip", sv, sv),
		}
		for _, u := range urls {
			lower := strings.ToLower(u)
			if strings.HasSuffix(lower, ".zip") {
				tmpZip := filepath.Join(binDir, "stunnel.zip")
				if err := downloadFileWithProgress(u, tmpZip); err != nil {
					continue
				}
				if err := extractStunnelFromZip(tmpZip, targetPath); err == nil {
					os.Remove(tmpZip)
					return nil
				}
				os.Remove(tmpZip)
			} else if strings.HasSuffix(lower, ".exe") {
				tmpExe := filepath.Join(binDir, "stunnel-installer.exe")
				if err := downloadFileWithProgress(u, tmpExe); err != nil {
					continue
				}
				cmd := exec.Command(tmpExe, "/S")
				_ = cmd.Run()
				os.Remove(tmpExe)

				drive := os.Getenv("SystemDrive")
				if drive == "" {
					drive = "C:"
				}
				progFiles := []string{
					os.Getenv("ProgramFiles(x86)"),
					os.Getenv("ProgramFiles"),
					filepath.Join(drive, "Program Files (x86)"),
					filepath.Join(drive, "Program Files"),
				}
				for _, pf := range progFiles {
					if pf == "" {
						continue
					}
					stunnelBinDir := filepath.Join(pf, "stunnel", "bin")
					if _, err := os.Stat(filepath.Join(stunnelBinDir, "stunnel.exe")); err == nil {
						files, _ := os.ReadDir(stunnelBinDir)
						for _, file := range files {
							name := strings.ToLower(file.Name())
							if strings.HasSuffix(name, ".dll") || strings.HasSuffix(name, ".exe") {
								_ = copyFile(filepath.Join(stunnelBinDir, file.Name()), filepath.Join(binDir, file.Name()), 0755)
							}
						}
						if _, err := os.Stat(targetPath); err == nil {
							return nil
						}
					}
				}
			}
		}
		if p, err := exec.LookPath("stunnel.exe"); err == nil {
			return copyFile(p, targetPath, 0755)
		}
		return fmt.Errorf("stunnel download failed — install stunnel manually from https://www.stunnel.org")
	}

	// Linux: use package manager
	for _, pkg := range [][]string{
		{"apt-get", "install", "-y", "stunnel4"},
		{"apt-get", "install", "-y", "stunnel"},
		{"yum", "install", "-y", "stunnel"},
		{"dnf", "install", "-y", "stunnel"},
		{"apk", "add", "--no-cache", "stunnel"},
		{"pacman", "-Sy", "--noconfirm", "stunnel"},
	} {
		if _, err := exec.LookPath(pkg[0]); err != nil {
			continue
		}
		c := exec.Command(pkg[0], pkg[1:]...)
		c.Env = envForPkgManager(pkg[0])
		if err := c.Run(); err == nil {
			if p, err := exec.LookPath("stunnel"); err == nil {
				_ = copyFile(p, targetPath, 0755)
				return nil
			}
		}
	}
	if p, err := exec.LookPath("stunnel"); err == nil {
		_ = copyFile(p, targetPath, 0755)
		return nil
	}
	return fmt.Errorf("stunnel not found — install with: apt install stunnel4")
}

func extractStunnelFromZip(zipPath, targetPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		if strings.ToLower(filepath.Base(f.Name)) == "stunnel.exe" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			out, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("stunnel.exe not found in zip")
}

func shadowsocksTargetTriple() string {
	osName := runtime.GOOS
	arch := runtime.GOARCH
	switch {
	case osName == "linux" && arch == "amd64":
		return "x86_64-unknown-linux-gnu"
	case osName == "linux" && arch == "arm64":
		return "aarch64-unknown-linux-gnu"
	case osName == "windows" && arch == "amd64":
		return "x86_64-pc-windows-msvc"
	case osName == "windows" && arch == "arm64":
		return "aarch64-pc-windows-msvc"
	default:
		return fmt.Sprintf("%s-%s-unknown", arch, osName)
	}
}

func getShadowsocksDownloadURLs() []string {
	ver := EffectiveShadowsocksVersion()
	triple := shadowsocksTargetTriple()
	var out []string
	// Primary: zip archive (works for both Linux and Windows on GitHub releases)
	out = append(out, fmt.Sprintf(
		"https://github.com/shadowsocks/shadowsocks-rust/releases/download/%s/shadowsocks-%s.%s.zip",
		ver, ver, triple))
	// Fallback: tar.xz for Linux
	if runtime.GOOS != "windows" {
		out = append(out, fmt.Sprintf(
			"https://github.com/shadowsocks/shadowsocks-rust/releases/download/%s/shadowsocks-%s.%s.tar.xz",
			ver, ver, triple))
	}
	return out
}

func ensureShadowsocksBinary(binDir, targetPath string) error {
	ver := EffectiveShadowsocksVersion()
	var lastErr error
	for _, u := range getShadowsocksDownloadURLs() {
		lower := strings.ToLower(u)
		if strings.HasSuffix(lower, ".zip") {
			if err := downloadAndExtractZip(u, binDir, "ssserver"); err != nil {
				writeFetchMetaFail(binDir, "shadowsocks", u, ver, err.Error())
				lastErr = err
				continue
			}
			if _, err := os.Stat(targetPath); err == nil {
				return nil
			}
			lastErr = fmt.Errorf("ssserver not found after zip extraction")
			continue
		}
		if strings.HasSuffix(lower, ".tar.xz") || strings.HasSuffix(lower, ".tar.gz") {
			archivePath := filepath.Join(binDir, "_dl_ss_archive")
			_ = os.MkdirAll(filepath.Dir(archivePath), 0755)
			if err := downloadFileWithProgress(u, archivePath); err != nil {
				lastErr = err
				continue
			}
			if err := extractTarGzBinary(archivePath, targetPath, "ssserver"); err != nil {
				_ = os.Remove(archivePath)
				lastErr = err
				continue
			}
			_ = os.Remove(archivePath)
			return nil
		}
		// Direct binary download
		if err := downloadFileWithProgress(u, targetPath); err != nil {
			writeFetchMetaFail(binDir, "shadowsocks", u, ver, err.Error())
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no download URLs")
	}
	return fmt.Errorf("shadowsocks: %w", lastErr)
}

func getV2rayPluginDownloadURLs() []string {
	ver := EffectiveV2rayPluginVersion()
	osName := runtime.GOOS
	arch := runtime.GOARCH
	filename := fmt.Sprintf("v2ray-plugin-%s-%s-%s.tar.gz", osName, arch, ver)
	return []string{
		fmt.Sprintf("https://github.com/teddysun/v2ray-plugin/releases/download/%s/%s", ver, filename),
	}
}

func ensureV2rayPluginBinary(binDir, targetPath string) error {
	ver := EffectiveV2rayPluginVersion()
	var lastErr error
	for _, u := range getV2rayPluginDownloadURLs() {
		archivePath := filepath.Join(binDir, "_dl_v2p_archive")
		_ = os.MkdirAll(filepath.Dir(archivePath), 0755)
		if err := downloadFileWithProgress(u, archivePath); err != nil {
			writeFetchMetaFail(binDir, "v2ray-plugin", u, ver, err.Error())
			lastErr = err
			continue
		}

		// The tar.gz contains "v2ray-plugin_windows_amd64.exe" or "v2ray-plugin_linux_amd64"
		osName := runtime.GOOS
		arch := runtime.GOARCH
		expectedName := fmt.Sprintf("v2ray-plugin_%s_%s", osName, arch)

		if err := extractTarGzBinary(archivePath, targetPath, expectedName); err != nil {
			_ = os.Remove(archivePath)
			lastErr = err
			continue
		}
		_ = os.Remove(archivePath)
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no download URLs")
	}
	return fmt.Errorf("v2ray-plugin: %w", lastErr)
}


func masterDnsVPNOSName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "MacOS"
	case "linux":
		return "Linux"
	default:
		return strings.Title(runtime.GOOS)
	}
}

func masterDnsVPNArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "AMD64"
	case "arm64":
		return "ARM64"
	case "386":
		return "X86"
	case "arm":
		return "ARMV7"
	default:
		return strings.ToUpper(runtime.GOARCH)
	}
}

func getMasterDnsVPNDownloadURLs(component string) []string {
	ver := EffectiveMasterDnsVPNVersion()
	osName := masterDnsVPNOSName()
	arch := masterDnsVPNArch()
	// component is "masterdnsvpn-server" or "masterdnsvpn-client"
	shortComp := strings.TrimPrefix(component, "masterdnsvpn-")
	shortComp = strings.Title(shortComp)
	var out []string
	out = append(out, fmt.Sprintf(
		"https://github.com/masterking32/MasterDnsVPN/releases/download/%s/MasterDnsVPN_%s_%s_%s.zip",
		ver, shortComp, osName, arch))
	out = append(out, fmt.Sprintf(
		"https://github.com/masterking32/MasterDnsVPN/releases/download/%s/MasterDnsVPN_%s_%s_%s.tar.gz",
		ver, shortComp, osName, arch))
	return out
}

func ensureMasterDnsVPNBinary(binDir, targetPath, component string) error {
	ver := EffectiveMasterDnsVPNVersion()
	var lastErr error
	for _, u := range getMasterDnsVPNDownloadURLs(component) {
		lower := strings.ToLower(u)
		if strings.HasSuffix(lower, ".zip") {
			tmpZip := filepath.Join(binDir, "_dl_mdns.zip")
			if err := downloadFileWithProgress(u, tmpZip); err != nil {
				writeFetchMetaFail(binDir, component, u, ver, err.Error())
				lastErr = err
				continue
			}
			if err := extractMasterDnsVPNFromZip(tmpZip, targetPath, component); err != nil {
				_ = os.Remove(tmpZip)
				lastErr = err
				continue
			}
			_ = os.Remove(tmpZip)
			return nil
		}
		if strings.HasSuffix(lower, ".tar.gz") {
			archivePath := filepath.Join(binDir, "_dl_mdns_archive")
			_ = os.MkdirAll(filepath.Dir(archivePath), 0755)
			if err := downloadFileWithProgress(u, archivePath); err != nil {
				writeFetchMetaFail(binDir, component, u, ver, err.Error())
				lastErr = err
				continue
			}
			if err := extractMasterDnsVPNFromTarGz(archivePath, targetPath, component); err != nil {
				_ = os.Remove(archivePath)
				lastErr = err
				continue
			}
			_ = os.Remove(archivePath)
			return nil
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no download URLs")
	}
	return fmt.Errorf("masterdnsvpn: %w", lastErr)
}

func extractMasterDnsVPNFromZip(zipPath, targetPath, component string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	// MasterDnsVPN release archives contain files like:
	//   MasterDnsVPN_Server_Windows_AMD64_v2026.05.04.123456-38b73de.exe
	//   MasterDnsVPN_Client_Linux_AMD64_v2026.05.04.123456-38b73de
	shortComp := strings.TrimPrefix(component, "masterdnsvpn-")
	shortComp = strings.Title(shortComp)
	prefix := "MasterDnsVPN_" + shortComp + "_"

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		if strings.HasPrefix(base, prefix) {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			out, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			if err != nil {
				return err
			}
			if runtime.GOOS != "windows" {
				_ = os.Chmod(targetPath, 0755)
			}
			return nil
		}
	}
	return fmt.Errorf("no file matching %s found in zip", prefix)
}

func extractMasterDnsVPNFromTarGz(archivePath, targetPath, component string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	shortComp := strings.TrimPrefix(component, "masterdnsvpn-")
	shortComp = strings.Title(shortComp)
	prefix := "MasterDnsVPN_" + shortComp + "_"

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if hdr == nil || hdr.Typeflag == tar.TypeDir {
			continue
		}

		base := filepath.Base(hdr.Name)
		if strings.HasPrefix(base, prefix) {
			out, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
			if runtime.GOOS != "windows" {
				_ = os.Chmod(targetPath, 0755)
			}
			return nil
		}
	}
	return fmt.Errorf("no file matching %s found in tar.gz", prefix)
}
