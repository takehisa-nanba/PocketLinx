package container

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

func downloadFile(url string, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

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

// ProgressProxy tracks bytes written and displays a progress bar
type ProgressProxy struct {
	Total      int64
	Processed  int64
	Label      string
	StartTime  time.Time
	LastUpdate time.Time
}

func NewProgressProxy(total int64, label string) *ProgressProxy {
	return &ProgressProxy{
		Total:      total,
		Label:      label,
		StartTime:  time.Now(),
		LastUpdate: time.Now(),
	}
}

func (p *ProgressProxy) Write(b []byte) (int, error) {
	n := len(b)
	p.Processed += int64(n)
	if time.Since(p.LastUpdate) > 100*time.Millisecond {
		p.Display()
		p.LastUpdate = time.Now()
	}
	return n, nil
}

func (p *ProgressProxy) Display() {
	percent := 0.0
	if p.Total > 0 {
		percent = float64(p.Processed) / float64(p.Total) * 100
	}
	if percent > 100 {
		percent = 100
	}

	const width = 20
	done := int(percent / (100 / width))
	bar := strings.Repeat("=", done)
	if done < width {
		bar += ">" + strings.Repeat(" ", width-done-1)
	} else {
		bar += "="
	}

	fmt.Printf("\x1b[2K\r%s [%s] %.1f%% (%s/%s) %ds",
		p.Label, bar, percent, formatSize(p.Processed), formatSize(p.Total),
		int(time.Since(p.StartTime).Seconds()))
}

func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(b)/float64(div), "KMGTPE"[exp])
}
