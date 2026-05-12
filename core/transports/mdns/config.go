package mdns

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/types"
	"tunnelbypass/internal/utils"
)

// Default public DNS resolvers for maximum reachability.
var DefaultResolvers = []string{
	"8.8.8.8",
	"8.8.4.4",
	"1.1.1.1",
	"1.0.0.1",
	"9.9.9.9",
	"149.112.112.112",
	"208.67.222.222",
	"208.67.220.220",
	"94.140.14.14",
	"94.140.15.15",
	"185.228.168.9",
	"185.228.169.9",
	"76.76.19.19",
	"76.223.122.150",
}

// GenerateEncryptionKey creates a random 32-byte hex key for MasterDnsVPN.
func GenerateEncryptionKey() string {
	return utils.GenerateRandomString(32)
}

// GenerateServerConfig creates the MasterDnsVPN server TOML config.
func GenerateServerConfig(opt types.ConfigOptions, encryptKey string) (string, error) {
	domain := opt.MDNSDomain
	if domain == "" {
		domain = opt.Host
	}
	if domain == "" {
		domain = opt.Sni
	}
	if domain == "" {
		return "", fmt.Errorf("mdns: tunnel domain is required (set --mdns-domain or --sni)")
	}

	encMethod := opt.MDNSEncryptionMethod
	if encMethod < 0 || encMethod > 5 {
		encMethod = 3 // AES-128-GCM
	}

	port := opt.Port
	if port == 0 {
		port = types.DefaultMDNSListenPort
	}

	config := fmt.Sprintf(`DOMAIN = ["%s"]
PROTOCOL_TYPE = "SOCKS5"

UDP_HOST = "0.0.0.0"
UDP_PORT = %d

DATA_ENCRYPTION_METHOD = %d
ENCRYPTION_KEY_FILE = "encrypt_key.txt"

DNS_UPSTREAM_SERVERS = ["1.1.1.1:53", "8.8.8.8:53", "1.0.0.1:53", "9.9.9.9:53"]

MAX_ALLOWED_CLIENT_PACKETS_PER_BATCH = 64
MIN_ALLOWED_CLIENT_COMPRESSION_MIN_SIZE = 50

SUPPORTED_UPLOAD_COMPRESSION_TYPES = [0, 1, 2, 3]
SUPPORTED_DOWNLOAD_COMPRESSION_TYPES = [0, 1, 2, 3]

LOG_LEVEL = "INFO"
`, domain, port, encMethod)

	configsDir := installer.GetConfigDir("mdns")
	_ = os.MkdirAll(configsDir, 0755)

	targetPath := filepath.Join(configsDir, "server_config.toml")
	if err := os.WriteFile(targetPath, []byte(config), 0644); err != nil {
		return "", err
	}

	// Write encryption key file next to server config
	keyPath := filepath.Join(configsDir, "encrypt_key.txt")
	if err := os.WriteFile(keyPath, []byte(encryptKey), 0644); err != nil {
		return "", err
	}

	return targetPath, nil
}

// GenerateClientConfig creates the MasterDnsVPN client TOML config and returns
// the path to the client config and the resolver list file.
func GenerateClientConfig(opt types.ConfigOptions, encryptKey string, resolvers []string) (string, string, error) {
	domain := opt.MDNSDomain
	if domain == "" {
		domain = opt.Host
	}
	if domain == "" {
		domain = opt.Sni
	}
	if domain == "" {
		return "", "", fmt.Errorf("mdns: tunnel domain is required")
	}

	encMethod := opt.MDNSEncryptionMethod
	if encMethod < 0 || encMethod > 5 {
		encMethod = 3 // AES-128-GCM
	}

	if len(resolvers) == 0 {
		resolvers = DefaultResolvers
	}

	localDNS := "false"
	if opt.MDNSLocalDNS {
		localDNS = "true"
	}

	// Stealth-optimized defaults for DPI-heavy networks
	config := fmt.Sprintf(`PROTOCOL_TYPE = "SOCKS5"
DOMAINS = ["%s"]
DATA_ENCRYPTION_METHOD = %d
ENCRYPTION_KEY = "%s"

LISTEN_IP = "127.0.0.1"
LISTEN_PORT = 18000
SOCKS5_AUTH = false
SOCKS5_USER = "tunnelbypass"
SOCKS5_PASS = "tunnelbypass"

LOCAL_DNS_ENABLED = %s
LOCAL_DNS_IP = "127.0.0.1"
LOCAL_DNS_PORT = 53
LOCAL_DNS_CACHE_MAX_RECORDS = 10000
LOCAL_DNS_CACHE_TTL_SECONDS = 14400.0
LOCAL_DNS_PENDING_TIMEOUT_SECONDS = 300.0
LOCAL_DNS_CACHE_PERSIST_TO_FILE = true
LOCAL_DNS_CACHE_FLUSH_INTERVAL_SECONDS = 60.0

RESOLVER_BALANCING_STRATEGY = 5
PACKET_DUPLICATION_COUNT = 2
SETUP_PACKET_DUPLICATION_COUNT = 2
STREAM_RESOLVER_FAILOVER_RESEND_THRESHOLD = 2
STREAM_RESOLVER_FAILOVER_COOLDOWN = 2.5
RECHECK_INACTIVE_SERVERS_ENABLED = true
AUTO_DISABLE_TIMEOUT_SERVERS = true
AUTO_DISABLE_TIMEOUT_WINDOW_SECONDS = 30.0
BASE_ENCODE_DATA = true

UPLOAD_COMPRESSION_TYPE = 0
DOWNLOAD_COMPRESSION_TYPE = 0
COMPRESSION_MIN_SIZE = 50

MIN_UPLOAD_MTU = 38
MIN_DOWNLOAD_MTU = 100
MAX_UPLOAD_MTU = 150
MAX_DOWNLOAD_MTU = 500
MTU_TEST_RETRIES = 2
MTU_TEST_TIMEOUT = 2.0
MTU_TEST_PARALLELISM = 16

RX_TX_WORKERS = 4
TUNNEL_PROCESS_WORKERS = 6
TUNNEL_PACKET_TIMEOUT_SECONDS = 10.0
DISPATCHER_IDLE_POLL_INTERVAL_SECONDS = 0.020
RX_CHANNEL_SIZE = 4096
SOCKS_UDP_ASSOCIATE_READ_TIMEOUT_SECONDS = 30.0
CLIENT_TERMINAL_STREAM_RETENTION_SECONDS = 45.0
CLIENT_CANCELLED_SETUP_RETENTION_SECONDS = 120.0

SESSION_INIT_RETRY_BASE_SECONDS = 1.0
SESSION_INIT_RETRY_STEP_SECONDS = 1.0
SESSION_INIT_RETRY_LINEAR_AFTER = 5
SESSION_INIT_RETRY_MAX_SECONDS = 60.0
SESSION_INIT_BUSY_RETRY_INTERVAL_SECONDS = 60.0
SESSION_INIT_RACING_COUNT = 3

PING_AGGRESSIVE_INTERVAL_SECONDS = 0.100
PING_LAZY_INTERVAL_SECONDS = 0.750
PING_COOLDOWN_INTERVAL_SECONDS = 2.0
PING_COLD_INTERVAL_SECONDS = 15.0
PING_WARM_THRESHOLD_SECONDS = 8.0
PING_COOL_THRESHOLD_SECONDS = 20.0
PING_COLD_THRESHOLD_SECONDS = 30.0

MAX_PACKETS_PER_BATCH = 32
ARQ_WINDOW_SIZE = 600
ARQ_INITIAL_RTO_SECONDS = 1.0
ARQ_MAX_RTO_SECONDS = 5.0
ARQ_CONTROL_INITIAL_RTO_SECONDS = 0.5
ARQ_CONTROL_MAX_RTO_SECONDS = 3.0
ARQ_MAX_CONTROL_RETRIES = 400
ARQ_INACTIVITY_TIMEOUT_SECONDS = 1800.0
ARQ_DATA_PACKET_TTL_SECONDS = 2400.0
ARQ_CONTROL_PACKET_TTL_SECONDS = 1200.0
ARQ_MAX_DATA_RETRIES = 1200
ARQ_DATA_NACK_MAX_GAP = 16
ARQ_DATA_NACK_INITIAL_DELAY_SECONDS = 0.1
ARQ_DATA_NACK_REPEAT_SECONDS = 1.0
ARQ_TERMINAL_DRAIN_TIMEOUT_SECONDS = 120.0
ARQ_TERMINAL_ACK_WAIT_TIMEOUT_SECONDS = 90.0

DNS_RESPONSE_FRAGMENT_TIMEOUT_SECONDS = 60.0

LOG_LEVEL = "INFO"
`, domain, encMethod, encryptKey, localDNS)

	configsDir := installer.GetConfigDir("mdns")
	_ = os.MkdirAll(configsDir, 0755)

	clientPath := filepath.Join(configsDir, "client_config.toml")
	if err := os.WriteFile(clientPath, []byte(config), 0644); err != nil {
		return "", "", err
	}

	resolversPath := filepath.Join(configsDir, "client_resolvers.txt")
	resolverContent := GenerateResolverList(resolvers)
	if err := os.WriteFile(resolversPath, []byte(resolverContent), 0644); err != nil {
		return "", "", err
	}

	return clientPath, resolversPath, nil
}

// GenerateResolverList returns a newline-separated resolver list.
func GenerateResolverList(resolvers []string) string {
	if len(resolvers) == 0 {
		resolvers = DefaultResolvers
	}
	return strings.Join(resolvers, "\n") + "\n"
}
