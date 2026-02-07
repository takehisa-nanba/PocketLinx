package container

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// PrintTable はデータをテーブル形式で表示します。
// headers: 見出しのリスト
// rows: 各行のデータのリスト
func PrintTable(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}

	// 1. 各列の最大幅を計算
	colWidths := make([]int, len(headers))
	for i, h := range headers {
		colWidths[i] = len(h)
	}

	for _, row := range rows {
		for i, val := range row {
			if i < len(colWidths) && len(val) > colWidths[i] {
				colWidths[i] = len(val)
			}
		}
	}

	// 2. ヘッダーを印刷
	printSeparator(colWidths)
	printRow(headers, colWidths)
	printSeparator(colWidths)

	// 3. データを印刷
	for _, row := range rows {
		printRow(row, colWidths)
	}
	printSeparator(colWidths)
}

func printRow(row []string, widths []int) {
	fmt.Print("|")
	for i, val := range row {
		if i < len(widths) {
			fmt.Printf(" %-*s |", widths[i], val)
		}
	}
	fmt.Println()
}

func printSeparator(widths []int) {
	fmt.Print("+")
	for _, w := range widths {
		fmt.Print(strings.Repeat("-", w+2) + "+")
	}
	fmt.Println()
}

// CheckRequirements checks if the system meets the requirements for PocketLinx.
func CheckRequirements() error {
	if runtime.GOOS == "linux" {
		return nil
	}

	// 1. Check BIOS Virtualization (Windows only) - Warning only
	cmd := exec.Command("powershell.exe", "-Command", "(Get-WmiObject Win32_Processor).VirtualizationFirmwareEnabled")
	out, err := cmd.Output()
	if err == nil {
		if strings.TrimSpace(string(out)) == "False" {
			fmt.Println("Warning: VirtualizationFirmwareEnabled reported False. If WSL2 works, you can ignore this.")
		}
	}

	// 2. Check WSL command availability
	cmd = exec.Command("wsl.exe", "--status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("WSL2 is not installed or enabled. Please install WSL2 first (run 'wsl --install')")
	}

	return nil
}
