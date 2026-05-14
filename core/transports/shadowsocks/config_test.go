package shadowsocks

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tunnelbypass/core/installer"
	"tunnelbypass/core/layout"
	"tunnelbypass/core/types"
	"tunnelbypass/tools/host_catalog"
)

func TestGeneratePasswordSS2022KeyLength(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		method  string
		wantRaw int
		wantB64 int // standard base64 length for raw key (with padding)
	}{
		{"2022-blake3-aes-256-gcm", 32, 44},
		{"2022-blake3-aes-128-gcm", 16, 24},
		{"2022-blake3-chacha20-poly1305", 32, 44},
	} {
		pw := GeneratePassword(tc.method)
		raw, err := base64.StdEncoding.DecodeString(pw)
		if err != nil {
			t.Fatalf("%s: decode: %v", tc.method, err)
		}
		if len(raw) != tc.wantRaw {
			t.Fatalf("%s: decoded key length %d, want %d", tc.method, len(raw), tc.wantRaw)
		}
		if len(pw) != tc.wantB64 {
			t.Fatalf("%s: base64 string length %d, want %d (openssl rand -base64 %d compatible)", tc.method, len(pw), tc.wantB64, tc.wantRaw)
		}
	}
}

func TestNormalizeV2rayPluginCertKeyPaths(t *testing.T) {
	in := "server;tls;host=x;cert=C:\\TunnelBypass\\configs\\shadowsocks\\cert.crt;key=C:\\TunnelBypass\\configs\\shadowsocks\\private.key"
	want := "server;tls;host=x;cert=C:/TunnelBypass/configs/shadowsocks/cert.crt;key=C:/TunnelBypass/configs/shadowsocks/private.key"
	if got := normalizeV2rayPluginCertKeyPaths(in); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestClientPluginOptsVsServerOpts(t *testing.T) {
	opt := types.ConfigOptions{
		SSPlugin:     "v2ray-plugin",
		SSPluginOpts: "server;tls;host=pubgmobile.com;cert=/tmp/c.crt;key=/tmp/k.pem",
		Sni:          "pubgmobile.com",
		ServerAddr:   "1.2.3.4",
		Port:         443,
	}
	co, err := clientPluginOpts(opt)
	if err != nil {
		t.Fatal(err)
	}
	if co != "tls;host=pubgmobile.com;allowInsecure" {
		t.Fatalf("client plugin_opts got %q", co)
	}
	got, err := GenerateShadowsocksURLForSNI(opt, "pubgmobile.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "tls") || !strings.Contains(got, "host%3Dpubgmobile.com") || !strings.Contains(got, "allowInsecure") {
		t.Fatalf("url should include tls, host, allowInsecure: %s", got)
	}
	// Broken importers / hand-edited URIs leave raw ";" in the query; clients then drop tls opts.
	if i := strings.Index(got, "?plugin="); i >= 0 {
		q := got[i:]
		end := strings.Index(q, "#")
		if end < 0 {
			end = len(q)
		}
		frag := q[:end]
		if strings.Contains(frag, ";") {
			t.Fatalf("plugin query must not contain raw semicolons (use %%3B): %s", frag)
		}
	}
}

func mustV2rayRaw(t *testing.T, certPath string) string {
	t.Helper()
	s, err := V2rayPluginCertRawFromPEMFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestV2rayPluginClientOptsCertRaw(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.crt")
	keyPath := filepath.Join(dir, "private.key")
	if err := installer.EnsureSelfSignedCert(certPath, keyPath, "pubgmobile.com"); err != nil {
		t.Fatal(err)
	}
	opt := types.ConfigOptions{
		SSPlugin:             "v2ray-plugin",
		SSV2rayClientCertRaw: mustV2rayRaw(t, certPath),
		SSV2rayPluginWSPath:  "/tb-pathexample",
		Sni:                  "pubgmobile.com",
		ServerAddr:           "1.2.3.4",
		Port:                 443,
	}
	co, err := clientPluginOpts(opt)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(co, "tls;host=pubgmobile.com;path=/tb-pathexample;certRaw=") || strings.Contains(co, "allowInsecure") {
		t.Fatalf("expected path+certRaw pin, got %q", co)
	}
	u, err := GenerateShadowsocksURLForSNI(opt, "pubgmobile.com")
	if err != nil {
		t.Fatal(err)
	}
	uu, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	plugin := uu.Query().Get("plugin")
	if plugin == "" {
		t.Fatal("missing plugin query")
	}
	if !strings.Contains(plugin, "certRaw=") {
		t.Fatalf("decoded plugin missing certRaw: %s", plugin)
	}
	if !strings.Contains(plugin, "path=/tb-pathexample") {
		t.Fatalf("decoded plugin missing path: %s", plugin)
	}
}

func TestV2rayPluginPathFromServerOpts(t *testing.T) {
	in := "server;tls;host=x;path=/tb-Ab12;cert=/a;key=/b"
	if got := V2rayPluginPathFromServerOpts(in); got != "/tb-Ab12" {
		t.Fatalf("got %q", got)
	}
	if V2rayPluginPathFromServerOpts("server;tls;host=x") != "" {
		t.Fatal("want empty")
	}
}

func TestServerJSONMetaIncludesV2rayCertRaw(t *testing.T) {
	dir := t.TempDir()
	layout.SetDataRootOverride(dir)
	t.Cleanup(func() { layout.SetDataRootOverride("") })

	certPath := filepath.Join(dir, "configs", "shadowsocks", "cert.crt")
	keyPath := filepath.Join(dir, "configs", "shadowsocks", "private.key")
	if err := os.MkdirAll(filepath.Dir(certPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := installer.EnsureSelfSignedCert(certPath, keyPath, "example.com"); err != nil {
		t.Fatal(err)
	}
	raw, err := V2rayPluginCertRawFromPEMFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	opt := types.ConfigOptions{
		SSPlugin:             "v2ray-plugin",
		SSPluginOpts:         "server;tls;host=example.com;cert=" + certPath + ";key=" + keyPath,
		Sni:                  "example.com",
		SSV2rayClientCertRaw: raw,
		SSV2rayPluginWSPath:  "/tb-fixedtest",
		ServerAddr:           "203.0.113.1",
		Port:                 443,
	}
	srvPath, _, err := GenerateShadowsocksConfig(opt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(srvPath)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatal(err)
	}
	meta, ok := root[host_catalog.MetaKey].(map[string]interface{})
	if !ok {
		t.Fatal("missing meta")
	}
	got, ok := meta[MetaV2rayClientCertRawKey].(string)
	if !ok || got != raw {
		t.Fatalf("meta %s: got %q want %q", MetaV2rayClientCertRawKey, got, raw)
	}
	gotPath, ok := meta[MetaV2rayWSPathKey].(string)
	if !ok || gotPath != "/tb-fixedtest" {
		t.Fatalf("meta %s: got %q want /tb-fixedtest", MetaV2rayWSPathKey, gotPath)
	}
}
