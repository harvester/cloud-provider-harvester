package utils

import (
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
