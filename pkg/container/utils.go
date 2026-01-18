package container

import (
	"fmt"
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
