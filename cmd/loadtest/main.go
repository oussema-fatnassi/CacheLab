package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
)

// config describes one test run. Authentication is read only from the environment.
type config struct {
	URL          string
	Count        int
	ValueBytes   int
	Label        string
	ServerCommit string
	APIKey       string
	Timeout      time.Duration
	Seed         int64
	Concurrency  int
	Rate         float64
	Warmup       time.Duration
	Duration     time.Duration
	MissPercent  int
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	var cfg config
	var output string
	flag.StringVar(&cfg.URL, "url", "http://localhost:"+port, "CacheLab server URL")
	flag.IntVar(&cfg.Count, "count", 1000, "number of entries to insert and read")
	flag.IntVar(&cfg.ValueBytes, "value-bytes", 100, "size of each synthetic value in bytes")
	flag.StringVar(&cfg.Label, "label", "baseline", "configuration label, for example buckets-16")
	flag.StringVar(&cfg.ServerCommit, "server-commit", "", "tested server commit, if known")
	flag.DurationVar(&cfg.Timeout, "timeout", 10*time.Second, "timeout per HTTP request")
	flag.Int64Var(&cfg.Seed, "seed", 42, "seed for reproducible read selection")
	flag.IntVar(&cfg.Concurrency, "concurrency", 1, "maximum in-flight workload requests")
	flag.Float64Var(&cfg.Rate, "rate", 0, "offered reads/second; 0 means unpaced")
	flag.DurationVar(&cfg.Warmup, "warmup", 0, "read warm-up excluded from measured samples")
	flag.DurationVar(&cfg.Duration, "duration", 0, "measured read scheduling window; 0 means count reads")
	flag.IntVar(&cfg.MissPercent, "miss-percent", 0, "percentage probability of intentional missing-key reads")
	flag.StringVar(&output, "output", "", "JSON report path; defaults to a timestamped file in results/")
	flag.Parse()
	cfg.APIKey = os.Getenv("API_KEY")
	if err := cfg.validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = cfg.Concurrency
	transport.MaxIdleConns = max(100, cfg.Concurrency)
	transport.MaxConnsPerHost = cfg.Concurrency
	client := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
		// Never forward the credential to a redirect destination.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	defer client.CloseIdleConnections()
	result := run(client, cfg)
	if output == "" {
		output = "results/loadtest-" + time.Now().UTC().Format("20060102T150405.000000000Z") + ".json"
	}
	if err := saveReport(output, result); err != nil {
		fmt.Fprintln(os.Stderr, "Could not save the report:", err)
		os.Exit(1)
	}
	fmt.Printf("Configuration: %s | Entries: %d | Value: %d bytes\n", cfg.Label, cfg.Count, cfg.ValueBytes)
	if clock, ok := result.Metadata["latency_clock"]; ok {
		fmt.Println("Measurement clock:", clock)
	}
	for _, phase := range result.Phases {
		fmt.Printf("%-8s %d/%d successful", phase.Name, phase.Successes, phase.Attempts)
		if phase.Timed {
			fmt.Printf(" | %.3f ms total", phase.TotalMS)
		} else {
			fmt.Print(" | not timed")
			if phase.Name == "warmup" {
				fmt.Printf(" | unsent %d | unscheduled %d", phase.Unsent, phase.NotScheduled)
			}
		}
		if phase.AverageMS != nil {
			fmt.Printf(" | %.6f ms/op", *phase.AverageMS)
		}
		fmt.Println()
		if phase.Timed {
			fmt.Printf("         %.1f successful ops/s | offered %.1f req/s | unsent %d | delayed %d\n",
				phase.SuccessfulRPS, phase.OfferedRPS, phase.Unsent, phase.Delayed)
			for _, name := range []string{"POST", "GET_hit", "GET_miss"} {
				if group := phase.Operations[name]; group != nil && group.P50MS != nil {
					fmt.Printf("         %s: P50 %.3f | P95 %.3f | P99 %.3f ms\n", name, *group.P50MS, *group.P95MS, *group.P99MS)
				}
			}
		}
	}
	fmt.Println("Report:", output)
	for _, warning := range result.Warnings {
		fmt.Fprintln(os.Stderr, "Warning:", warning)
	}
	if !result.Success {
		fmt.Fprintln(os.Stderr, "Run failed:", result.Failure)
		os.Exit(1)
	}
}

func (c config) validate() error {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("url must be an HTTP(S) server address without credentials, path, query or fragment")
	}
	if c.Count < 1 || c.Count > 1000000 || c.ValueBytes < 1 || c.ValueBytes > 1048576 || c.Timeout <= 0 {
		return fmt.Errorf("count must be 1..1000000, value-bytes 1..1048576 and timeout positive")
	}
	if int64(c.Count)*int64(c.ValueBytes) > 256*1024*1024 {
		return fmt.Errorf("generated values must total at most 256 MiB; reduce count or value-bytes")
	}
	if err := c.validateWorkload(); err != nil {
		return err
	}
	if c.APIKey == "" || strings.ContainsAny(c.APIKey, " \t\r\n") {
		return fmt.Errorf("API_KEY must be supplied through the environment without whitespace")
	}
	return nil
}

func clientCommit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return "unknown"
}

// metadata records context without storing credentials or claiming to know server settings.
func metadata(c config) map[string]any {
	host, _ := os.Hostname()
	return map[string]any{
		"report_version": 3, "percentile_method": "nearest_rank_successful_responses",
		"started_at_utc": time.Now().UTC(), "server_url": strings.TrimRight(c.URL, "/"),
		"label": c.Label, "entries": c.Count, "value_bytes": c.ValueBytes,
		"concurrency": c.Concurrency, "timeout": c.Timeout.String(), "authentication": "Bearer",
		"seed": c.Seed, "offered_read_rps": c.Rate, "warmup": c.Warmup.String(),
		"read_duration": c.Duration.String(), "miss_percent": c.MissPercent,
		"client_commit": clientCommit(), "server_commit": c.ServerCommit,
		"go_version": runtime.Version(), "os": runtime.GOOS, "architecture": runtime.GOARCH,
		"logical_cpus": runtime.NumCPU(), "hostname": host,
		"logging_settings": "record server settings separately", "metrics_collected": false,
		"scope": "authenticated baseline loading, verification, warm-up and measured reads",
	}
}

func saveReport(path string, result report) error {
	// Encode only the explicit report fields, never the config containing API_KEY.
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return writeReportFile(path, append(data, '\n'))
}
