package utils

import (
	"net/netip"
	"testing"
)

func Test_NormalizeNetworkName(t *testing.T) {
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
			networkType: NetworkTypeManagement,
			input:       "harvester-public/vlan-200",
			want:        "harvester-public/vlan-200",
			wantErr:     false,
		},
		{
			name:        "Normalized name in default namespace is preserved",
			networkType: NetworkTypeManagement,
			input:       "default/vlan-100",
			want:        "default/vlan-100",
			wantErr:     false,
		},
		{
			name:        "Error: Too many slashes",
			networkType: NetworkTypeManagement,
			input:       "too/many/slashes",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Error: Missing name part",
			networkType: NetworkTypeManagement,
			input:       "namespace/",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Error: Missing namespace part",
			networkType: NetworkTypeManagement,
			input:       "/name",
			want:        "",
			wantErr:     true,
		},
		{
			name:        "Error: Only a slash",
			networkType: NetworkTypeManagement,
			input:       "/",
			want:        "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeNetworkName(tt.networkType, tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("NormalizeNetworkName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if got != tt.want {
				t.Errorf("NormalizeNetworkName() got = %q, want %q", got, tt.want)
			}
		})
	}
}

func Test_ConvertAndFilterIPs(t *testing.T) {
	tests := []struct {
		name        string
		input       []string
		expected    []netip.Addr
		expectError bool
	}{
		{
			name:  "Valid IPv4 and IPv6",
			input: []string{"10.0.0.1", "2001:db8::1"},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.1"),
				netip.MustParseAddr("2001:db8::1"),
			},
			expectError: false,
		},
		{
			name:  "Mixed Valid and Invalid",
			input: []string{"10.0.0.1", "not-an-ip", "2001:db8::1"},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.1"),
				netip.MustParseAddr("2001:db8::1"),
			},
			expectError: true,
		},
		{
			name:        "Only Invalid",
			input:       []string{"malformed"},
			expected:    nil,
			expectError: true,
		},
		{
			name:  "Filter Loopback",
			input: []string{"127.0.0.1", "::1", "192.168.1.1"},
			expected: []netip.Addr{
				netip.MustParseAddr("192.168.1.1"),
			},
			expectError: false,
		},
		{
			name:  "Filter Link-Local",
			input: []string{"169.254.10.1", "fe80::1", "10.0.0.5"},
			expected: []netip.Addr{
				netip.MustParseAddr("10.0.0.5"),
			},
			expectError: false,
		},
		{
			name:  "Filter Multicast and Broadcast",
			input: []string{"224.0.0.1", "255.255.255.255", "8.8.8.8"},
			expected: []netip.Addr{
				netip.MustParseAddr("8.8.8.8"),
			},
			expectError: false,
		},
		{
			name:        "Empty or Nil Input",
			input:       []string{},
			expected:    nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ConvertAndFilterIPs(tt.input)

			if (err != nil) != tt.expectError {
				t.Fatalf("ConvertAndFilterIPs() error = %v, expectError %v", err, tt.expectError)
			}
			if tt.expectError {
				if actual != nil {
					t.Errorf("Expected nil return on error, but got %v", actual)
				}
				return
			}

			if len(actual) != len(tt.expected) {
				t.Fatalf("Length mismatch: got %d, want %d", len(actual), len(tt.expected))
			}

			for i := range actual {
				if actual[i] != tt.expected[i] {
					t.Errorf("At index %d: got %s, want %s", i, actual[i], tt.expected[i])
				}
			}
		})
	}
}
