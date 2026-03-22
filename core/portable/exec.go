package portable

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
)

func defaultConfigPath(transport, filename string, override string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}
	return filepath.Join(installer.GetConfigDir(transport), filename)
}

func runForeground(ctx context.Context, log *slog.Logger, name, exe string, args []string) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	log.Info("portable: starting process", "tool", name, "exe", exe, "args", args)
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		return nil
	}
}
