package cfg

import "strings"

func ApplySpecFileToRunSpec(rspec *RunSpec, f SpecFile) {
	if strings.TrimSpace(f.Transport) != "" {
		rspec.Transport = strings.TrimSpace(f.Transport)
	}
	if f.Port != 0 {
		rspec.Port = f.Port
	}
	if strings.TrimSpace(f.SNI) != "" {
		rspec.SNI = strings.TrimSpace(f.SNI)
	}
	if strings.TrimSpace(f.ServerAddr) != "" {
		rspec.Server.Address = strings.TrimSpace(f.ServerAddr)
	}
	if strings.TrimSpace(f.Server) != "" && strings.TrimSpace(rspec.Server.Address) == "" {
		rspec.Server.Address = strings.TrimSpace(f.Server)
	}
	if strings.TrimSpace(f.UUID) != "" {
		rspec.Auth.UUID = strings.TrimSpace(f.UUID)
	}
	if strings.TrimSpace(f.SSHUser) != "" {
		rspec.Auth.SSHUser = strings.TrimSpace(f.SSHUser)
	}
	if strings.TrimSpace(f.SSHPassword) != "" {
		rspec.Auth.SSHPass = strings.TrimSpace(f.SSHPassword)
	}
	if f.SSHPort != 0 {
		rspec.SSH.Port = f.SSHPort
	}
	if f.UDPGWPort != 0 {
		rspec.UDPGW.Port = f.UDPGWPort
		rspec.UDPGW.Enabled = true
	}
	if strings.TrimSpace(f.DataDir) != "" {
		rspec.Paths.DataDir = strings.TrimSpace(f.DataDir)
	}
	if strings.TrimSpace(f.LogsDir) != "" {
		rspec.Paths.LogsDir = strings.TrimSpace(f.LogsDir)
	}
	if f.Daemon {
		rspec.Behavior.Daemon = true
	}
	if f.DryRun {
		rspec.Behavior.GenerateOnly = true
	}
	if f.NoElevate {
		rspec.Behavior.NoElevate = true
	}
	if f.AutoStart {
		rspec.Behavior.AutoStart = true
	}
	if f.LinuxOptimizeNet {
		rspec.Behavior.LinuxOptimizeNet = true
	}
	if f.LinuxDNSFix {
		rspec.Behavior.LinuxDNSFix = true
	}
	if f.LinuxRouter {
		rspec.Behavior.LinuxRouter = true
	}
	if f.LinuxNoAutoOptimize {
		rspec.Behavior.LinuxNoAutoOptimize = true
	}
}
