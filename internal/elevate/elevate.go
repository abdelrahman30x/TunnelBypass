package elevate

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func IsAdmin() bool {
	if runtime.GOOS == "windows" {
		_, err := exec.Command("net", "session").Output()
		return err == nil
	}
	return os.Getuid() == 0
}

func psSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func Elevate() error {
	if IsAdmin() {
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	args := os.Args[1:]

	if runtime.GOOS == "windows" {
		// UAC decline returns an error; don't os.Exit(0) without telling the user.
		exeQ := psSingleQuote(exe)
		cwdQ := psSingleQuote(cwd)
		var b strings.Builder
		b.WriteString("$ErrorActionPreference='Stop'; try { Start-Process -FilePath ")
		b.WriteString(exeQ)
		if len(args) > 0 {
			b.WriteString(" -ArgumentList @(")
			for i, a := range args {
				if i > 0 {
					b.WriteString(",")
				}
				b.WriteString(psSingleQuote(a))
			}
			b.WriteString(")")
		}
		b.WriteString(" -Verb RunAs -WorkingDirectory ")
		b.WriteString(cwdQ)
		b.WriteString(" -WindowStyle Normal; exit 0 } catch { exit 1 }")

		cmd := exec.Command("powershell", "-NoProfile", "-Command", b.String())
		cmd.Stdin = os.Stdin
		out, runErr := cmd.CombinedOutput()
		if runErr != nil {
			if len(out) > 0 {
				return fmt.Errorf("%w: %s", runErr, strings.TrimSpace(string(out)))
			}
			return fmt.Errorf("elevation cancelled or failed: %w", runErr)
		}
		os.Exit(0)
	} else {
		sudoArgs := append([]string{"sudo", exe}, args...)
		cmd := exec.Command(sudoArgs[0], sudoArgs[1:]...)
		cmd.Dir = cwd
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return err
		}
		os.Exit(0)
	}

	return nil
}
