package installer

import (
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{"  v26.3.27  ", "26.3.27"},
		{"v", ""},
		{"", ""},
		{" v2.6.1\n", "2.6.1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeVersion(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		output   string
		expected string
	}{
		{
			name:     "Xray normal output",
			tool:     "xray",
			output:   "Xray 26.3.27 (Xray, Penetrates Everything.)\nA unified platform for anti-censorship.",
			expected: "v26.3.27",
		},
		{
			name:     "Xray empty output",
			tool:     "xray",
			output:   "",
			expected: "",
		},
		{
			name:     "Hysteria normal output",
			tool:     "hysteria",
			output:   "hysteria2 v2.6.1\napp version: v2.6.1\n",
			expected: "v2.6.1",
		},
		{
			name:     "Wstunnel normal output",
			tool:     "wstunnel",
			output:   "wstunnel 10.5.2\n",
			expected: "v10.5.2",
		},
		{
			name:     "Shadowsocks normal output",
			tool:     "shadowsocks",
			output:   "shadowsocks 1.24.0\n",
			expected: "v1.24.0",
		},
		{
			name:     "V2ray-plugin normal output",
			tool:     "v2ray-plugin",
			output:   "v2ray-plugin v5.49.0\n",
			expected: "v5.49.0",
		},
		{
			name:     "Stunnel always returns latest",
			tool:     "stunnel",
			output:   "stunnel 5.72 on x86_64-pc-linux-gnu platform",
			expected: "latest",
		},
		{
			name:     "Unknown tool",
			tool:     "unknown",
			output:   "version 1.0",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseVersionOutput(tt.tool, tt.output)
			if result != tt.expected {
				t.Errorf("ParseVersionOutput(%q, %q) = %q; want %q", tt.tool, tt.output, result, tt.expected)
			}
		})
	}
}
