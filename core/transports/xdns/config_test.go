package xdns

import (
	"encoding/json"
	"os"
	"testing"

	"tunnelbypass/core/layout"
	"tunnelbypass/core/types"
)

func useTempDataRoot(t *testing.T) {
	t.Helper()
	layout.SetDataRootOverride(t.TempDir())
	t.Cleanup(func() { layout.SetDataRootOverride("") })
}

func TestGenerateServerConfig(t *testing.T) {
	useTempDataRoot(t)

	opt := types.ConfigOptions{
		Transport:     "xdns",
		ServerAddr:    "127.0.0.1",
		Port:          53,
		UUID:          "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Host:          "test.example.com",
		KCPSeed:       "testseed123456789012345678901234",
		KCPMTU:        1350,
		KCPTTI:        20,
		KCPHeaderType: "none",
	}

	path, err := GenerateServerConfig(opt)
	if err != nil {
		t.Fatalf("GenerateServerConfig failed: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read server config: %v", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal server config: %v", err)
	}

	inbounds, ok := root["inbounds"].([]interface{})
	if !ok || len(inbounds) == 0 {
		t.Fatal("missing inbounds")
	}
	ib := inbounds[0].(map[string]interface{})

	if ib["port"] != float64(53) {
		t.Errorf("port = %v, want 53", ib["port"])
	}

	ss, ok := ib["streamSettings"].(map[string]interface{})
	if !ok {
		t.Fatal("missing streamSettings")
	}
	if ss["network"] != "kcp" {
		t.Errorf("network = %v, want kcp", ss["network"])
	}

	kcp, ok := ss["kcpSettings"].(map[string]interface{})
	if !ok {
		t.Fatal("missing kcpSettings")
	}
	if kcp["mtu"] != float64(opt.KCPMTU) {
		t.Errorf("mtu = %v, want %d", kcp["mtu"], opt.KCPMTU)
	}

	fm, ok := ss["finalmask"].(map[string]interface{})
	if !ok {
		t.Fatal("missing finalmask")
	}
	udpMasks, ok := fm["udp"].([]interface{})
	if !ok || len(udpMasks) == 0 {
		t.Fatal("missing udp masks")
	}
	firstMask := udpMasks[0].(map[string]interface{})
	if firstMask["type"] != "mkcp-aes128gcm" {
		t.Errorf("mask type = %v, want mkcp-aes128gcm", firstMask["type"])
	}

	meta, ok := root["_tunnelbypass"].(map[string]interface{})
	if !ok {
		t.Fatal("missing _tunnelbypass meta")
	}
	if meta["seed"] != opt.KCPSeed {
		t.Errorf("meta seed = %v, want %s", meta["seed"], opt.KCPSeed)
	}
}

func TestGenerateClientConfig(t *testing.T) {
	useTempDataRoot(t)

	opt := types.ConfigOptions{
		Transport:     "xdns",
		ServerAddr:    "192.0.2.1",
		Port:          53,
		UUID:          "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		Host:          "test.example.com",
		KCPSeed:       "testseed123456789012345678901234",
		KCPMTU:        1350,
		KCPTTI:        20,
		KCPHeaderType: "none",
	}

	path, err := GenerateClientConfig(opt)
	if err != nil {
		t.Fatalf("GenerateClientConfig failed: %v", err)
	}
	defer os.Remove(path)

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read client config: %v", err)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal client config: %v", err)
	}

	outbounds, ok := root["outbounds"].([]interface{})
	if !ok || len(outbounds) == 0 {
		t.Fatal("missing outbounds")
	}
	ob := outbounds[0].(map[string]interface{})

	ss, ok := ob["streamSettings"].(map[string]interface{})
	if !ok {
		t.Fatal("missing streamSettings")
	}
	if ss["network"] != "kcp" {
		t.Errorf("network = %v, want kcp", ss["network"])
	}

	kcp, ok := ss["kcpSettings"].(map[string]interface{})
	if !ok {
		t.Fatal("missing kcpSettings")
	}
	if kcp["mtu"] != float64(opt.KCPMTU) {
		t.Errorf("mtu = %v, want %d", kcp["mtu"], opt.KCPMTU)
	}

	fm, ok := ss["finalmask"].(map[string]interface{})
	if !ok {
		t.Fatal("missing finalmask")
	}
	udpMasks, ok := fm["udp"].([]interface{})
	if !ok || len(udpMasks) == 0 {
		t.Fatal("missing udp masks")
	}
	firstMask := udpMasks[0].(map[string]interface{})
	if firstMask["type"] != "mkcp-aes128gcm" {
		t.Errorf("mask type = %v, want mkcp-aes128gcm", firstMask["type"])
	}
}

func TestSeedMatchesBetweenServerAndClient(t *testing.T) {
	useTempDataRoot(t)

	opt := types.ConfigOptions{
		Transport:     "xdns",
		ServerAddr:    "192.0.2.1",
		Port:          53,
		UUID:          "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
		KCPSeed:       "matchingseed12345678901234567890",
		KCPMTU:        1350,
		KCPTTI:        20,
		KCPHeaderType: "none",
	}

	srvPath, err := GenerateServerConfig(opt)
	if err != nil {
		t.Fatalf("GenerateServerConfig failed: %v", err)
	}
	defer os.Remove(srvPath)

	cliPath, err := GenerateClientConfig(opt)
	if err != nil {
		t.Fatalf("GenerateClientConfig failed: %v", err)
	}
	defer os.Remove(cliPath)

	var srvRoot, cliRoot map[string]interface{}

	srvData, _ := os.ReadFile(srvPath)
	json.Unmarshal(srvData, &srvRoot)

	cliData, _ := os.ReadFile(cliPath)
	json.Unmarshal(cliData, &cliRoot)

	srvInbounds := srvRoot["inbounds"].([]interface{})
	srvIb := srvInbounds[0].(map[string]interface{})
	srvFm := srvIb["streamSettings"].(map[string]interface{})["finalmask"].(map[string]interface{})
	srvMasks := srvFm["udp"].([]interface{})
	srvFirst := srvMasks[0].(map[string]interface{})
	srvSettings := srvFirst["settings"].(map[string]interface{})
	srvPassword := srvSettings["password"]

	cliOutbounds := cliRoot["outbounds"].([]interface{})
	cliOb := cliOutbounds[0].(map[string]interface{})
	cliFm := cliOb["streamSettings"].(map[string]interface{})["finalmask"].(map[string]interface{})
	cliMasks := cliFm["udp"].([]interface{})
	cliFirst := cliMasks[0].(map[string]interface{})
	cliSettings := cliFirst["settings"].(map[string]interface{})
	cliPassword := cliSettings["password"]

	if srvPassword != cliPassword {
		t.Errorf("server password %q != client password %q", srvPassword, cliPassword)
	}
}
