package monitor

import "github.com/atotto/clipboard"

var clipboardWriteAll = clipboard.WriteAll

func copyToClipboard(text string) error {
	return clipboardWriteAll(text)
}
