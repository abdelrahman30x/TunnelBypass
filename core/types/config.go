package types

type ConfigOptions struct {
	Transport    string   `json:"transport"`
	ServerAddr   string   `json:"server"`
	Port         int      `json:"port"`
	UUID         string   `json:"uuid"`
	Sni          string   `json:"sni"`
	ExtraSNIs    []string `json:"extra_snis"`
	ServiceName  string   `json:"service_name"`
	PrivateKey   string   `json:"private_key"`
	PublicKey    string   `json:"public_key"`
	ShortIds     []string `json:"short_ids"`
	RealityDest  string   `json:"reality_dest"`
	Host         string   `json:"host"`
	ObfsPassword string   `json:"obfs_password"`

	SSHUser           string `json:"ssh_user"`
	SSHPassword       string `json:"ssh_password"`
	SSHWelcomeMessage string `json:"ssh_welcome_message"`
	SSHIsAdmin        bool   `json:"ssh_is_admin"`
}

// XrayServerConfig is used for parsing server-side configuration
type XrayServerConfig struct {
	Inbounds []struct {
		Port     int `json:"port"`
		Settings struct {
			Clients []struct {
				ID   string `json:"id"`
				Flow string `json:"flow"`
			} `json:"clients"`
		} `json:"settings"`
		StreamSettings struct {
			Network         string `json:"network"`
			Security        string `json:"security"`
			RealitySettings struct {
				PrivateKey  string   `json:"privateKey"`
				PublicKey   string   `json:"publicKey"`
				ShortIds    []string `json:"shortIds"`
				ServerNames []string `json:"serverNames"`
			} `json:"realitySettings"`
		} `json:"streamSettings"`
	} `json:"inbounds"`
}

const (
	TransportReality  = "reality"
	TransportGRPC     = "grpc"
	TransportVLESS    = "vless-tcp"
	TransportUDP      = "udp"
	TransportHysteria = "hysteria"
)
