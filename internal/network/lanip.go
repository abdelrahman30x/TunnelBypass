package network

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
)

type lanCandidate struct {
	iface string
	ip    string
	score int
}

func isRFC1918IPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil || ip.IsLoopback() {
		return false
	}
	switch {
	case ip[0] == 10:
		return true
	case ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31:
		return true
	case ip[0] == 192 && ip[1] == 168:
		return true
	default:
		return false
	}
}

// ifaceNameScore returns a sort priority (higher is better) or -1 to skip the interface.
func ifaceNameScore(name string) int {
	ln := strings.ToLower(strings.TrimSpace(name))
	if ln == "" || ln == "lo" {
		return -1
	}
	skipSubstr := []string{"veth", "vmnet", "wsl", "virbr", "loopback"}
	for _, s := range skipSubstr {
		if strings.Contains(ln, s) {
			return -1
		}
	}
	if strings.HasPrefix(ln, "br-") || strings.HasPrefix(ln, "tun") || strings.HasPrefix(ln, "tap") {
		return -1
	}
	if strings.Contains(ln, "vpn") {
		return -1
	}

	if strings.Contains(ln, "eth") || ln == "en0" || ln == "en1" ||
		strings.Contains(ln, "local area connection") || strings.Contains(ln, "ethernet") {
		return 3
	}
	if strings.Contains(ln, "wlan") || strings.Contains(ln, "wi-fi") || strings.Contains(ln, "wifi") ||
		strings.Contains(ln, "wireless") || strings.HasPrefix(ln, "wl") {
		return 2
	}
	return 1
}

func firstLANIPOnInterface(ifName string) string {
	iface, err := net.InterfaceByName(ifName)
	if err != nil {
		return ""
	}
	if iface.Flags&net.FlagUp == 0 {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP.To4()
		case *net.IPAddr:
			ip = v.IP.To4()
		}
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		if isRFC1918IPv4(ip) {
			return ip.String()
		}
	}
	return ""
}

func defaultGatewayInterface() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return defaultGatewayLinux()
	case "darwin", "freebsd", "openbsd", "netbsd":
		return defaultGatewayBSD()
	case "windows":
		return defaultGatewayWindows()
	default:
		return "", fmt.Errorf("default gateway: unsupported GOOS %q", runtime.GOOS)
	}
}

func defaultGatewayLinux() (string, error) {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "", err
	}
	s := bufio.NewScanner(bytes.NewReader(b))
	if !s.Scan() {
		return "", fmt.Errorf("empty /proc/net/route")
	}
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 1 {
			continue
		}
		iface := fields[0]
		dest := ""
		if len(fields) > 1 {
			dest = fields[1]
		}
		if dest == "00000000" && iface != "Iface" {
			return iface, nil
		}
	}
	return "", fmt.Errorf("no default route in /proc/net/route")
}

func defaultGatewayBSD() (string, error) {
	cmd := exec.Command("route", "-n", "get", "0.0.0.0")
	out, err := cmd.Output()
	if err != nil {
		out, err = exec.Command("route", "-n", "get", "default").Output()
		if err != nil {
			return "", err
		}
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "interface:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1], nil
			}
		}
	}
	return "", fmt.Errorf("parse route get: no interface line")
}

func defaultGatewayWindows() (string, error) {
	ps := "(Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).InterfaceAlias"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	iface := strings.TrimSpace(string(out))
	if iface == "" {
		return "", fmt.Errorf("empty InterfaceAlias from Get-NetRoute")
	}
	return iface, nil
}

func collectRankedLANInterfaces(log *slog.Logger, trace bool) []lanCandidate {
	ifaces, err := net.Interfaces()
	if err != nil {
		if trace && log != nil {
			log.Debug("[network] list interfaces failed", "err", err)
		}
		return nil
	}
	var out []lanCandidate
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			if trace && log != nil {
				log.Debug("[network] excluded: loopback adapter", "iface", iface.Name)
			}
			continue
		}
		score := ifaceNameScore(iface.Name)
		if score < 0 {
			if trace && log != nil {
				log.Debug("[network] excluded: virtual adapter", "iface", iface.Name)
			}
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP.To4()
			case *net.IPAddr:
				ip = v.IP.To4()
			}
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}
			if !isRFC1918IPv4(ip) {
				continue
			}
			out = append(out, lanCandidate{iface: iface.Name, ip: ip.String(), score: score})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].ip < out[j].ip
	})
	return out
}

// detectLANIP returns (ip, interfaceName, source, error).
// source is one of "gateway-interface" or "name-ranked".
func detectLANIP(log *slog.Logger, trace bool) (ip, iface, source string, err error) {
	if trace && log != nil {
		log.Debug("[network] step-3 trying default gateway interface...")
	}
	gwIface, gwErr := defaultGatewayInterface()
	if gwErr != nil && trace && log != nil {
		log.Debug("[network] default gateway iface lookup failed", "err", gwErr)
	}
	if gwErr == nil && gwIface != "" {
		if ip := firstLANIPOnInterface(gwIface); ip != "" {
			if log != nil {
				log.Info("[network] using gateway interface", "iface", gwIface, "ip", ip)
			}
			if trace && log != nil {
				log.Debug("[network] gateway interface detected", "iface", gwIface)
				log.Debug("[network] "+gwIface+" → "+ip+" (score=3, source=gateway-interface) ✓ selected")
				logRankedExcluding(log, gwIface, ip, trace)
			}
			return ip, gwIface, "gateway-interface", nil
		}
	}

	candidates := collectRankedLANInterfaces(log, trace)
	if len(candidates) == 0 {
		return "", "", "", fmt.Errorf("no LAN interface with RFC-1918 address found")
	}
	best := candidates[0]
	if log != nil {
		log.Info("[network] LAN IP from ranked interface scan", "iface", best.iface, "ip", best.ip, "score", best.score)
	}
	if trace && log != nil {
		log.Debug("[network] "+best.iface+" → "+best.ip+fmt.Sprintf(" (score=%d, source=name-ranked) ✓ selected", best.score))
		for i := 1; i < len(candidates); i++ {
			c := candidates[i]
			log.Debug("[network] excluded: "+c.iface, "score", c.score, "reason", "lower priority in name-ranked list")
		}
	}
	return best.ip, best.iface, "name-ranked", nil
}

func logRankedExcluding(log *slog.Logger, selectedIface, selectedIP string, trace bool) {
	if !trace || log == nil {
		return
	}
	all := collectRankedLANInterfaces(log, false)
	for _, c := range all {
		if c.iface == selectedIface && c.ip == selectedIP {
			continue
		}
		log.Debug("[network] excluded: "+c.iface, "score", c.score, "ip", c.ip, "reason", "lower priority than gateway")
	}
}
