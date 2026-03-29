package cli

import (
	"bufio"
	"fmt"
	"strings"
	"time"
)

func printLogo() {
	fmt.Printf("%s%s", ColorBold, ColorCyan)
	logo := `  _____                      _ ____                                
 |_   _|   _ _ __  _ __   ___| | __ ) _   _ _ __   __ _ ___ ___    
   | || | | | '_ \| '_ \ / _ \ |  _ \| | | | '_ \ / _` + "`" + ` / __/ __|   
   | || |_| | | | | | | |  __/ | |_) | |_| | |_) | (_| \__ \__ \   
   |_| \__,_|_| |_|_| |_|\___|_|____/ \__, | .__/ \__,_|___/___/   
                                      |___/|_|                     `
	fmt.Println(logo)
	fmt.Printf("%s", ColorReset)
	const bar = "  ========================================================"
	fmt.Printf("%s%s%s\n", ColorTeal+ColorBold, bar, ColorReset)
	fmt.Printf("   %sTunnelBypass CLI%s  -  %s%s%s  -  %sDPI Bypass Tool%s\n",
		ColorBold+ColorWhite, ColorReset,
		ColorBold+ColorOrange, version, ColorReset,
		ColorGray, ColorReset)
	fmt.Printf("%s%s%s\n", ColorTeal+ColorBold, bar, ColorReset)
}

func prompt(r *bufio.Reader, label string) string {
	fmt.Print(label)
	input, _ := r.ReadString('\n')
	return strings.TrimSpace(input)
}

func flushInput(reader *bufio.Reader) {
	time.Sleep(100 * time.Millisecond)
	if reader.Buffered() > 0 {
		reader.Discard(reader.Buffered())
	}
}
