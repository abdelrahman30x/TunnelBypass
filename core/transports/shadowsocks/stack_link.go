package shadowsocks

import (
	"strings"
	"time"

	"github.com/sagernet/sing-shadowsocks/shadowimpl"
)

// validatePasswordWithSingStack checks method+password using sing-shadowsocks (sing-box family, pinned in go.mod).
// Non-2022 ciphers skip this path and remain validated by shadowsocks-rust at runtime.
func validatePasswordWithSingStack(method, password string) error {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(method)), "2022-") {
		return nil
	}
	_, err := shadowimpl.FetchMethod(method, password, time.Now)
	return err
}
