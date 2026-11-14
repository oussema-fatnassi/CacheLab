package main

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestConfiguredTableSelectsEngine(t *testing.T) {
	for _, test := range []struct{ setting, name, kind string }{
		{"", "chained", "*cache.HashTable"},
		{"chained", "chained", "*cache.HashTable"},
		{"recursive", "recursive", "*cache.RecursiveTable"},
		{"go-map", "go-map", "*cache.GoMap"},
	} {
		t.Run(test.name+"/"+test.setting, func(t *testing.T) {
			t.Setenv("CACHE_ENGINE", test.setting)
			t.Setenv("CACHE_BUCKET_COUNT", "2")
			table, metadata, err := newConfiguredTable()
			if err != nil || fmt.Sprintf("%T", table) != test.kind || metadata.Name != test.name {
				t.Fatalf("configured engine = %T, %+v, %v", table, metadata, err)
			}
		})
	}
	t.Run("unset", func(t *testing.T) {
		t.Setenv("CACHE_ENGINE", "")
		t.Setenv("CACHE_BUCKET_COUNT", "")
		if err := os.Unsetenv("CACHE_ENGINE"); err != nil {
			t.Fatal(err)
		}
		table, metadata, err := newConfiguredTable()
		if err != nil || fmt.Sprintf("%T", table) != "*cache.HashTable" || metadata.Preallocation.InitialBucketCount == nil || *metadata.Preallocation.InitialBucketCount != 16 {
			t.Fatalf("default engine = %T, %+v, %v", table, metadata, err)
		}
	})
}

func TestConfiguredTableBucketSetting(t *testing.T) {
	for _, engine := range []string{"chained", "recursive", "go-map"} {
		for _, setting := range []string{"2", "0", "-1", "invalid"} {
			t.Run(engine+"/"+setting, func(t *testing.T) {
				t.Setenv("CACHE_ENGINE", engine)
				t.Setenv("CACHE_BUCKET_COUNT", setting)
				table, metadata, err := newConfiguredTable()
				if engine == "chained" && setting != "2" {
					if err == nil || table != nil {
						t.Fatal("invalid bucket count must prevent chained startup")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if engine == "chained" {
					if metadata.Preallocation.InitialBucketCount == nil || *metadata.Preallocation.InitialBucketCount != 2 {
						t.Fatalf("bucket count = %+v", metadata.Preallocation)
					}
				} else if metadata.Preallocation.InitialBucketCount != nil {
					t.Fatal("inactive bucket setting must not be reported")
				}
			})
		}
	}
}

func TestApplicationKeepsSelectedEngine(t *testing.T) {
	for _, engine := range []string{"chained", "recursive", "go-map"} {
		t.Run(engine, func(t *testing.T) {
			t.Setenv("CACHE_ENGINE", engine)
			t.Setenv("CACHE_BUCKET_COUNT", "2")
			t.Setenv("MAX_KEY_BYTES", "4")
			t.Setenv("MAX_VALUE_BYTES", "5")
			app, err := newApplication()
			if err != nil {
				t.Fatal(err)
			}
			if err := app.service.Create("key", "value"); err != nil {
				t.Fatal(err)
			}
			before := app.engine
			t.Setenv("CACHE_ENGINE", "invalid")
			t.Setenv("CACHE_BUCKET_COUNT", "invalid")
			if got, err := app.service.Read("key"); err != nil || got != "value" {
				t.Fatalf("existing service changed: %q, %v", got, err)
			}
			if app.engine.Name != engine || !reflect.DeepEqual(app.engine, before) {
				t.Fatalf("metadata changed after construction: %+v", app.engine)
			}
		})
	}
}

func TestExecutionMetadata(t *testing.T) {
	for _, test := range []struct{ engine, hash, policy string }{
		{"chained", "fnv1a-64", "initial-bucket-slice"},
		{"recursive", "fnv1a-64", "nodes-on-demand"},
		{"go-map", "go-runtime", "no-explicit-capacity-hint"},
	} {
		t.Run(test.engine, func(t *testing.T) {
			t.Setenv("CACHE_ENGINE", test.engine)
			t.Setenv("CACHE_BUCKET_COUNT", "2")
			t.Setenv("API_KEY", "metadata-test-secret")
			_, metadata, err := newConfiguredTable()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("CACHE_ENGINE", "changed-after-construction")
			t.Setenv("CACHE_BUCKET_COUNT", "999")
			var logs bytes.Buffer
			logExecutionMetadata(slog.New(slog.NewJSONHandler(&logs, nil)), metadata)
			events := logEvents(t, &logs)
			if len(events) != 1 {
				t.Fatalf("got %d events; want one", len(events))
			}
			event := events[0]
			for key, want := range map[string]any{
				"msg": "execution metadata", "engine": test.engine,
				"server_go_version": runtime.Version(), "hash": test.hash,
				"locking": "single-sync.Mutex-all-operations",
			} {
				if event[key] != want {
					t.Errorf("%s = %v; want %v", key, event[key], want)
				}
			}
			wantPreallocation := map[string]any{"policy": test.policy}
			if test.engine == "chained" {
				wantPreallocation["initial_bucket_count"] = float64(2)
			}
			if !reflect.DeepEqual(event["preallocation"], wantPreallocation) {
				t.Errorf("preallocation = %v; want %v", event["preallocation"], wantPreallocation)
			}
			if test.engine == "recursive" {
				if event["branch_count"] != float64(16) || event["max_depth"] != float64(16) {
					t.Fatalf("recursive configuration: %v", event)
				}
			} else if event["branch_count"] != nil || event["max_depth"] != nil {
				t.Fatal("inapplicable recursive configuration reported")
			}
			if strings.Contains(logs.String(), "metadata-test-secret") || strings.Contains(logs.String(), "API_KEY") {
				t.Fatal("metadata exposed credentials")
			}
		})
	}
}
