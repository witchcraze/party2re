package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type BenchResult struct {
	Name        string
	Iterations  int64
	NsPerOp     float64
	BytesPerOp  int64
	AllocsPerOp int64
}

var benchRegex = regexp.MustCompile(`^(Benchmark\S+)\s+(\d+)\s+([\d\.]+)\s+ns/op(?:\s+(\d+)\s+B/op)?(?:\s+(\d+)\s+allocs/op)?`)

func parseBenchFile(path string) (map[string]BenchResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	results := make(map[string]BenchResult)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		matches := benchRegex.FindStringSubmatch(line)
		if len(matches) >= 4 {
			name := matches[1]
			// Normalize name (strip -<GOMAXPROCS> suffix for stable matching)
			baseName := name
			if idx := strings.LastIndex(name, "-"); idx != -1 {
				baseName = name[:idx]
			}

			iters, _ := strconv.ParseInt(matches[2], 10, 64)
			ns, _ := strconv.ParseFloat(matches[3], 64)
			var bPerOp, allocsPerOp int64
			if len(matches) >= 5 && matches[4] != "" {
				bPerOp, _ = strconv.ParseInt(matches[4], 10, 64)
			}
			if len(matches) >= 6 && matches[5] != "" {
				allocsPerOp, _ = strconv.ParseInt(matches[5], 10, 64)
			}

			results[baseName] = BenchResult{
				Name:        baseName,
				Iterations:  iters,
				NsPerOp:     ns,
				BytesPerOp:  bPerOp,
				AllocsPerOp: allocsPerOp,
			}
		}
	}
	return results, scanner.Err()
}

func formatDelta(oldVal, newVal float64) string {
	if oldVal == 0 {
		return "N/A"
	}
	delta := ((newVal - oldVal) / oldVal) * 100.0
	if math.Abs(delta) < 0.05 {
		return "  0.0%"
	}
	sign := "+"
	if delta < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%.1f%%", sign, delta)
}

func formatTime(ns float64) string {
	if ns < 1000 {
		return fmt.Sprintf("%.1fns", ns)
	} else if ns < 1000000 {
		return fmt.Sprintf("%.2fµs", ns/1000.0)
	} else if ns < 1000000000 {
		return fmt.Sprintf("%.2fms", ns/1000000.0)
	}
	return fmt.Sprintf("%.2fs", ns/1000000000.0)
}

func main() {
	threshold := flag.Float64("threshold", 30.0, "Allowed regression threshold percentage (e.g. 30.0 for 30%)")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		fmt.Println("Usage: benchcompare [-threshold <percent>] <old_file> <new_file>")
		os.Exit(1)
	}

	oldResults, err := parseBenchFile(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading baseline benchmark file: %v\n", err)
		os.Exit(1)
	}

	newResults, err := parseBenchFile(args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading current benchmark file: %v\n", err)
		os.Exit(1)
	}

	names := make([]string, 0, len(newResults))
	for k := range newResults {
		names = append(names, k)
	}
	sort.Strings(names)

	fmt.Printf("%-38s %12s %12s %8s %12s %12s\n", "Benchmark", "Old ns/op", "New ns/op", "Delta", "Old allocs", "New allocs")
	fmt.Println(strings.Repeat("-", 100))

	hasRegression := false
	for _, name := range names {
		curr := newResults[name]
		prev, ok := oldResults[name]
		if !ok {
			fmt.Printf("%-38s %12s %12s %8s %12s %12d (new)\n", name, "-", formatTime(curr.NsPerOp), "-", "-", curr.AllocsPerOp)
			continue
		}

		deltaPercent := ((curr.NsPerOp - prev.NsPerOp) / prev.NsPerOp) * 100.0
		deltaStr := formatDelta(prev.NsPerOp, curr.NsPerOp)
		allocDeltaStr := ""
		if curr.AllocsPerOp != prev.AllocsPerOp {
			allocDeltaStr = fmt.Sprintf(" (%+d)", curr.AllocsPerOp-prev.AllocsPerOp)
		}

		warning := ""
		if deltaPercent > *threshold {
			warning = " [REGRESSION]"
			hasRegression = true
		}

		fmt.Printf("%-38s %12s %12s %8s %12d %12d%s%s\n",
			name,
			formatTime(prev.NsPerOp),
			formatTime(curr.NsPerOp),
			deltaStr,
			prev.AllocsPerOp,
			curr.AllocsPerOp,
			allocDeltaStr,
			warning,
		)
	}
	fmt.Println(strings.Repeat("-", 100))

	if hasRegression {
		fmt.Fprintf(os.Stderr, "\n[WARNING] Performance regression exceeded threshold (%.1f%%)!\n", *threshold)
		os.Exit(2)
	} else {
		fmt.Println("All benchmarks are within acceptable performance thresholds.")
	}
}
