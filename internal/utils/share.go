package utils

import (
	"fmt"
	"runtime"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// Labeled share URL or raw config text.
type ShareLink struct {
	Label string
	URL   string
}

// Small terminal QR (unicode blocks); fine for quick scans, not print quality.
func PrintQRCodeANSI(label, content string) error {
	if content == "" {
		return nil
	}

	cols, rows := terminalSize()

	type choice struct {
		level qrcode.RecoveryLevel
		quiet int
	}

	// Try higher scan reliability first, then shrink until it fits.
	candidates := []choice{
		{level: qrcode.Medium, quiet: 1},
		{level: qrcode.Medium, quiet: 0},
		{level: qrcode.Low, quiet: 1},
		{level: qrcode.Low, quiet: 0},
	}

	var bestQR *qrcode.QRCode
	bestQuiet := 0

	for _, c := range candidates {
		qr, err := qrcode.New(content, c.level)
		if err != nil {
			continue
		}
		matrix := qr.Bitmap()
		h := len(matrix)
		w := 0
		if h > 0 {
			w = len(matrix[0])
		}

		outW := w + 2*c.quiet
		outH := (h + 2*c.quiet + 1) / 2 // 2 QR rows per terminal line

		if cols == 0 || rows == 0 {
			bestQR = qr
			bestQuiet = c.quiet
			break
		}

		neededRows := outH
		if strings.TrimSpace(label) != "" {
			neededRows += 1
		}

		// Leave a tiny margin for prompts/output below.
		if outW <= cols-2 && neededRows <= rows-2 {
			bestQR = qr
			bestQuiet = c.quiet
			break
		}
	}

	if bestQR == nil {
		qr, err := qrcode.New(content, qrcode.Low)
		if err != nil {
			return nil
		}
		bestQR = qr
		bestQuiet = 0
	}

	matrix := bestQR.Bitmap()
	h := len(matrix)
	w := 0
	if h > 0 {
		w = len(matrix[0])
	}

	var b strings.Builder
	// Print label only if we have room; many screens are short.
	if strings.TrimSpace(label) != "" && (rows == 0 || rows >= 12) {
		b.WriteString(fmt.Sprintf("%s\n", label))
	}

	// Windows: tighter vertical (no quiet border).
	if runtime.GOOS == "windows" {
		bestQuiet = 0
	}

	for y := -bestQuiet; y < h+bestQuiet; y += 2 {
		for x := -bestQuiet; x < w+bestQuiet; x++ {
			top := false
			bot := false

			if y >= 0 && y < h && x >= 0 && x < w {
				top = matrix[y][x]
			}
			if y+1 >= 0 && y+1 < h && x >= 0 && x < w {
				bot = matrix[y+1][x]
			}

			switch {
			case top && bot:
				b.WriteRune('█')
			case top && !bot:
				b.WriteRune('▀')
			case !top && bot:
				b.WriteRune('▄')
			default:
				b.WriteRune(' ')
			}
		}
		b.WriteRune('\n')
	}

	fmt.Print("\n" + b.String())
	return nil
}

// SaveQRCodePNG writes a QR PNG to disk if path is non-empty.
func SaveQRCodePNG(path, content string, size int) error {
	if path == "" || content == "" {
		return nil
	}
	if size <= 0 {
		size = 256
	}
	if err := qrcode.WriteFile(content, qrcode.Medium, size, path); err != nil {
		return err
	}
	return nil
}
