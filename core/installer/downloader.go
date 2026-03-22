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

	if os.Getenv("TB_BIN_FORCE_REFRESH") == "1" {
		_ = os.Remove(targetPath)
	}

	if _, err := os.Stat(targetPath); err == nil {
		if name == "wstunnel" && !isWstunnelVersionUsable(targetPath) {
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
	out := []string{buildXrayZipURL(ver)}
	if m := strings.TrimSpace(os.Getenv("TB_XRAY_MIRROR_URLS")); m != "" {
		for _, u := range strings.Split(m, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				out = append(out, u)
			}
		}
	}
	return out
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
	out := []string{buildHysteriaURL(ver)}
	if m := strings.TrimSpace(os.Getenv("TB_HYSTERIA_MIRROR_URLS")); m != "" {
		for _, u := range strings.Split(m, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				out = append(out, u)
			}
		}
	}
	return out
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
	} {
		if _, err := exec.LookPath(pkg[0]); err != nil {
			continue
		}
		if err := exec.Command(pkg[0], pkg[1:]...).Run(); err == nil {
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
