// coverreport reports statement-weighted production coverage without enforcing a gate.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: coverreport coverage.out")
		os.Exit(1)
	}
	file, err := os.Open(os.Args[1])
	if err == nil {
		defer func() { _ = file.Close() }()
		err = report(file, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func report(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	if !scanner.Scan() || scanner.Text() != "mode: atomic" {
		return fmt.Errorf("expected an atomic Go coverage profile")
	}
	var covered, total int64
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return fmt.Errorf("invalid coverage record: %q", scanner.Text())
		}
		statements, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || statements < 0 {
			return fmt.Errorf("invalid statement count: %q", fields[1])
		}
		count, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || count < 0 {
			return fmt.Errorf("invalid execution count: %q", fields[2])
		}
		total += statements
		if count > 0 {
			covered += statements
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if total == 0 {
		return fmt.Errorf("coverage profile contains no statements")
	}
	if _, err := fmt.Fprintf(output, "Production coverage: %d/%d statements = %.2f%% (target: 80%%)\n", covered, total, 100*float64(covered)/float64(total)); err != nil {
		return err
	}
	if covered*100 < total*80 {
		_, err := fmt.Fprintln(output, "WARNING: production coverage is below 80%; advisory only, not a failing gate.")
		return err
	}
	return nil
}
