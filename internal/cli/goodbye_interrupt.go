package cli

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
)

var goodbyeOnInterruptOnce sync.Once

// setupGoodbyeOnInterrupt registers SIGINT (^C) and, on Unix, SIGTERM so we exit with a short friendly message.
// Used for the interactive wizard; sync.Once avoids duplicate handlers if runWizard were entered more than once.
func setupGoodbyeOnInterrupt() {
	goodbyeOnInterruptOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		sigs := []os.Signal{os.Interrupt}
		if runtime.GOOS != "windows" {
			sigs = append(sigs, syscall.SIGTERM)
		}
		signal.Notify(ch, sigs...)
		go func() {
			<-ch
			fmt.Fprintf(os.Stderr, "\n%sGoodbye! Stay safe out there.%s\n", ColorCyan, ColorReset)
			os.Exit(0)
		}()
	})
}
