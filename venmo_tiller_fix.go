package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Match ($...number...) or (number) with optional $ and spaces; ASCII or fullwidth parens （）
var parenAmountRe = regexp.MustCompile(`^\s*[\(\x{FF08}]\s*\$?\s*([0-9,]+\.[0-9]+)\s*[\)\x{FF09}]\s*$`)

func fixAmount(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	// Normalize: replace fullwidth parens and optional BOM so regex can match
	s = strings.TrimPrefix(s, "\uFEFF")
	s = strings.TrimSpace(s)
	if m := parenAmountRe.FindStringSubmatch(s); m != nil {
		return "-$" + m[1]
	}
	return s
}

func rowHasAccountLabel(row []string) bool {
	for _, cell := range row {
		c := strings.TrimSpace(cell)
		if c == "Account Statement" || c == "Account Activity" {
			return true
		}
	}
	return false
}

func at(row []string, col int) string {
	if col < 0 || col >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[col])
}

func transformRow(row []string, headerLen int, amountCol, fromCol, toCol, typeCol, destCol int) []string {
	newRow := make([]string, headerLen+1)
	for i := 0; i < headerLen; i++ {
		if i < len(row) {
			newRow[i] = fixAmount(row[i])
		}
	}
	from := at(row, fromCol)
	if from == "" {
		from = at(row, typeCol)
	}
	to := at(row, toCol)
	if to == "" {
		to = at(row, destCol)
	}
	newRow[headerLen] = "[" + from + "] => [" + to + "]"
	return newRow
}

func processFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	r := csv.NewReader(strings.NewReader(string(data)))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("parse csv: %w", err)
	}
	if len(records) == 0 {
		return fmt.Errorf("empty file")
	}

	// Find header row (file may have "Account Statement" / "Account Activity" lines first)
	headerRow := -1
	for idx, row := range records {
		for _, h := range row {
			if strings.TrimSpace(h) == "Amount (total)" || strings.TrimSpace(h) == "Amount" {
				headerRow = idx
				break
			}
		}
		if headerRow >= 0 {
			break
		}
	}
	if headerRow < 0 {
		return fmt.Errorf("column 'Amount (total)' or 'Amount' not found")
	}

	header := records[headerRow]
	amountCol := -1
	fromCol := -1
	toCol := -1
	typeCol := -1
	destCol := -1
	idCol := -1

	for i, h := range header {
		switch strings.TrimSpace(h) {
		case "Amount (total)", "Amount":
			amountCol = i
		case "From":
			fromCol = i
		case "To":
			toCol = i
		case "Type":
			typeCol = i
		case "Destination":
			destCol = i
		case "ID":
			idCol = i
		}
	}

	if amountCol == -1 {
		return fmt.Errorf("column 'Amount (total)' or 'Amount' not found")
	}
	if fromCol == -1 {
		return fmt.Errorf("column 'From' not found")
	}
	if toCol == -1 {
		return fmt.Errorf("column 'To' not found")
	}

	// Build output: header + data rows (skip "Account Statement" / "Account Activity" rows)
	newHeader := make([]string, len(header)+1)
	copy(newHeader, header)
	newHeader[amountCol] = "Amount"
	newHeader[len(header)] = "Hint"

	out := [][]string{newHeader}
	for _, row := range records[headerRow+1:] {
		if rowHasAccountLabel(row) {
			continue
		}
		if idCol >= 0 && at(row, idCol) == "" {
			continue
		}
		out = append(out, transformRow(row, len(header), amountCol, fromCol, toCol, typeCol, destCol))
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.WriteAll(out); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	w.Flush()
	return w.Error()
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: venmo_fix <pattern> [pattern ...]")
		os.Exit(1)
	}

	var paths []string
	for _, pattern := range os.Args[1:] {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid pattern %q: %v\n", pattern, err)
			os.Exit(1)
		}
		paths = append(paths, matches...)
	}

	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "no files matched")
		os.Exit(1)
	}

	exitCode := 0
	for _, p := range paths {
		fmt.Printf("processing %s ... ", p)
		if err := processFile(p); err != nil {
			fmt.Printf("ERROR: %v\n", err)
			exitCode = 1
		} else {
			fmt.Println("ok")
		}
	}
	os.Exit(exitCode)
}
