package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBrowserOpenCommandForOS(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		url      string
		wantName string
		wantArgs []string
	}{
		{
			name:     "macOS",
			goos:     "darwin",
			url:      "http://localhost:4000",
			wantName: "open",
			wantArgs: []string{"http://localhost:4000"},
		},
		{
			name:     "linux",
			goos:     "linux",
			url:      "http://localhost:4000",
			wantName: "xdg-open",
			wantArgs: []string{"http://localhost:4000"},
		},
		{
			name:     "windows",
			goos:     "windows",
			url:      "http://localhost:4000",
			wantName: "rundll32",
			wantArgs: []string{"url.dll,FileProtocolHandler", "http://localhost:4000"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args := browserOpenCommandForOS(tt.goos, tt.url)

			require.Equal(t, tt.wantName, name)
			require.Equal(t, tt.wantArgs, args)
		})
	}
}
