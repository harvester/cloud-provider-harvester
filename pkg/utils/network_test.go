package utils

import (
	"strings"
	"testing"
)

func Test_normalizeNetworkName(t *testing.T) {
	tests := []struct {
		name        string
		networkType string
		input       string
		want        string
		wantErr     bool
	}{
		{
			name:        "Empty input is an error",
			networkType: NetworkTypeManagement,
			input:       "",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Bare name is normalized to default namespace",
			networkType: NetworkTypeManagement,
			input:       "vlan-100",
			want:        "default/vlan-100",
			wantErr:     false,
		},
		{
			name:        "Already namespaced name is preserved",
			networkType: NetworkTypeLB,
			input:       "harvester-public/vlan-200",
			want:        "harvester-public/vlan-200",
			wantErr:     false,
		},
		{
			name:        "Normalized name in default namespace is preserved",
			networkType: NetworkTypeLB,
			input:       "default/vlan-100",
			want:        "default/vlan-100",
			wantErr:     false,
		},
		{
			name:        "Error: Too many slashes",
			networkType: NetworkTypeLB,
			input:       "too/many/slashes",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Error: Missing name part",
			networkType: NetworkTypeLB,
			input:       "namespace/",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Error: Missing namespace part",
			networkType: NetworkTypeLB,
			input:       "/name",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Error: Only a slash",
			networkType: NetworkTypeLB,
			input:       "/",
			want:        "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeNetworkName(tt.networkType, tt.input)

			// Check if we got an error when we expected one
			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeNetworkName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Check if the resulting string matches
			if got != tt.want {
				t.Errorf("NormalizeNetworkName() got = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateCIDRFilter_Comprehensive(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		errMsg  string
	}{
		// --- GOOD CASES ---
		{
			name:    "IPv4 Only Mode",
			input:   "192.168.10.0/24",
			wantErr: false,
		},
		{
			name:    "IPv6 Only Mode",
			input:   "2001:db8::/64",
			wantErr: false,
		},
		{
			name:    "Dual Stack Mode (Standard)",
			input:   "10.0.0.0/8, fd00::/64",
			wantErr: false,
		},
		{
			name:    "Dual Stack with Literal IPs",
			input:   "172.16.1.5, 2001:db8::fe21",
			wantErr: false,
		},
		{
			name:    "Complex Multi-Range (Legacy + Management + IPv6)",
			input:   "10.0.0.0/24, 192.168.1.0/24, 2001:db8:a::/48",
			wantErr: false,
		},
		{
			name:    "Empty Input (Allowed, means no filtering)",
			input:   "",
			wantErr: false,
		},

		// --- ERROR CASES ---
		{
			name:    "Malformed: Impossible IPv4 Mask",
			input:   "192.168.122.0/36",
			wantErr: true,
			errMsg:  "invalid CIDR",
		},
		{
			name:    "Malformed: Missing Octet",
			input:   "192.168.1/24",
			wantErr: true,
			errMsg:  "invalid CIDR",
		},
		{
			name:    "Logical: Loopback Block",
			input:   "127.0.0.1",
			wantErr: true,
			errMsg:  "loopback",
		},
		{
			name:    "Logical: Link-Local Block",
			input:   "fe80::/10",
			wantErr: true,
			errMsg:  "link-local",
		},
		{
			name:    "Logical: Multicast Block",
			input:   "224.0.0.1",
			wantErr: true,
			errMsg:  "unicast",
		},
		{
			name:    "Poisoned List: One bad CIDR in Dual Stack",
			input:   "10.0.0.0/24, 2001:db8::/129", // IPv6 mask too large
			wantErr: true,
			errMsg:  "invalid CIDR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCIDRFilter(tt.input)

			// 1. Check if we got an error when we expected one
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCIDRFilter(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}

			// 2. If an error occurred, verify the message content
			if tt.wantErr && err != nil {
				if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errMsg)) {
					t.Errorf("ValidateCIDRFilter(%q) error %q did not contain keyword %q", tt.input, err.Error(), tt.errMsg)
				}
			}
		})
	}
}
