package mdns

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
)

// GenerateSharingPackage creates client sharing files for MasterDnsVPN.
func GenerateSharingPackage(opt types.ConfigOptions, encryptKey string, clientConfigPath, resolversPath string) error {
	configsDir := installer.GetConfigDir("mdns")
	_ = os.MkdirAll(configsDir, 0755)

	// Read client config for base64 encoding
	clientData, err := os.ReadFile(clientConfigPath)
	if err != nil {
		return fmt.Errorf("read client config: %w", err)
	}

	b64Config := base64.StdEncoding.EncodeToString(clientData)

	// sharing-link.txt with instructions
	shareContent := fmt.Sprintf(`# Tunnel — MasterDnsVPN (DNS Tunnel)
# Domain: %s
# Encryption: AES-128-GCM
#
# Client setup:
# 1. Install masterdnsvpn-client binary
# 2. Copy client_config.toml, client_resolvers.txt, and encrypt_key.txt
# 3. Run: masterdnsvpn-client -config client_config.toml
#
# Base64-encoded client config (for QR/import):
%s
`, opt.MDNSDomain, b64Config)

	sharePath := filepath.Join(configsDir, "sharing-link.txt")
	if err := os.WriteFile(sharePath, []byte(shareContent), 0644); err != nil {
		return err
	}

	// QR code of base64 config
	qrPath := filepath.Join(configsDir, "qr-mdns.png")
	if err := utils.SaveQRCodePNG(qrPath, b64Config, 320); err != nil {
		// QR generation is optional; don't fail
		_ = err
	}

	return nil
}

// GenerateBase64ClientConfig returns a base64-encoded client config string.
func GenerateBase64ClientConfig(opt types.ConfigOptions, encryptKey string, resolvers []string) (string, error) {
	clientPath, _, err := GenerateClientConfig(opt, encryptKey, resolvers)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(clientPath)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
