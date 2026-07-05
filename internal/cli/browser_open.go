package cli

import (
	"runtime"
)

func browserOpenCommand(url string) (string, []string) {
	return browserOpenCommandForOS(runtime.GOOS, url)
}

func browserOpenCommandForOS(goos, url string) (string, []string) {
	switch goos {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
