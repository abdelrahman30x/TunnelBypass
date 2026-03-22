package transport

// Result summarizes written artifacts after provisioning.
type Result struct {
	Transport        string
	ServerConfigPath string
	ClientConfigPath string
	InstructionPath  string
	SharingLink      string
	ListenPort       int
	SSHPort          int
}
