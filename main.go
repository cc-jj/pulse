package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	Version           = "0.0.3"
	DefaultConfigPath = "pulse.json"
)

type Config struct {
	MainFile      string   `json:"main_file"`
	BinaryName    string   `json:"binary_name"`
	WatchDir      string   `json:"watch_dir"`
	WatchExts     []string `json:"watch_exts"`
	WatchInterval string   `json:"watch_interval"`
	MaxWatchers   int      `json:"max_watchers"`
}

// Default configuration
var config = Config{
	MainFile:      "main.go",
	BinaryName:    "app",
	WatchDir:      ".",
	WatchExts:     []string{".go", ".mod", ".sum"},
	WatchInterval: "1s",
	MaxWatchers:   100,
}

var (
	errCh   = make(chan error, 1)
	buildCh = make(chan bool)
	done    = make(chan bool)
	cmd     *exec.Cmd
)

func main() {
	versionFlag := flag.Bool("v", false, "Print version information and exit")
	initFlag := flag.Bool("init", false, "Initialize a new pulse.json configuration file")
	configFlag := flag.String("c", DefaultConfigPath, "Specify the configuration file path")
	flag.Parse()

	if *versionFlag {
		fmt.Printf("Go Pulse v%s\n", Version)
		return
	}

	if *initFlag {
		data, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			fmt.Printf("Error creating default config: %s\n", err)
			return
		}
		path := "pulse.json"
		if err := os.WriteFile(path, data, 0644); err != nil {
			fmt.Printf("Error writing config file: %s\n", err)
			return
		}
		fmt.Printf("Configuration file created at %s\n", path)
		return
	}

	fmt.Println("🚀 Go Pulse started")

	loadConfig(*configFlag)

	fmt.Printf("📋 Configuration:\n")
	fmt.Printf("   Main file:      %s\n", config.MainFile)
	fmt.Printf("   Binary name:    %s\n", config.BinaryName)
	fmt.Printf("   Watch dir:      %s\n", config.WatchDir)
	fmt.Printf("   Watch exts:     %v\n", config.WatchExts)
	fmt.Printf("   Watch interval: %s\n", config.WatchInterval)
	fmt.Printf("   Max watchers:   %d\n", config.MaxWatchers)
	fmt.Println("👀 Watching for file changes...")

	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	setupSignalHandling(cancelCtx)

	go watchFiles(cancelCtx)

	// Initial build and run
	buildAndRun()

	exitCode := 0

loop:
	for {
		select {
		case <-buildCh:
			stopProcess()
			buildAndRun()
		case err := <-errCh:
			fmt.Printf("❌ %v\n", err)
			exitCode = 1
			break loop
		case <-done:
			fmt.Println("💤 Go Pulse shutting down...")
			break loop
		}
	}

	cancel()
	stopProcess()
	os.Exit(exitCode)
}

// Load the configuration from pulse.json if it exists
func loadConfig(configPath string) {
	fmt.Printf("📄 Loading configuration from: %s\n", configPath)

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("⚠️ Warning: Could not read config file: %s\n", err)
		fmt.Println("   Using default configuration")
		return
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		fmt.Printf("⚠️ Warning: Could not parse config file: %s\n", err)
		fmt.Println("   Using default configuration")
		return
	}

	if config.MainFile == "" {
		config.MainFile = "main.go"
	}
	if config.BinaryName == "" {
		config.BinaryName = "app"
	}
	if config.WatchDir == "" {
		config.WatchDir = "."
	}
	if len(config.WatchExts) == 0 {
		config.WatchExts = []string{".go", ".mod", ".sum"}
	}
	if config.MaxWatchers < 1 {
		fmt.Printf("⚠️ Warning: Invalid max_watchers, using default of 100\n")
		config.MaxWatchers = 100
	} else if config.MaxWatchers > 500 {
		fmt.Printf("⚠️ Warning: max_watchers cannot exceed 500\n")
		config.MaxWatchers = 500
	}

	// Validate and parse the watch interval
	duration, err := time.ParseDuration(config.WatchInterval)
	if err != nil || config.WatchInterval == "" {
		fmt.Println("⚠️ Warning: Invalid watch_interval, using default of 1s")
		config.WatchInterval = "1s"
		duration = 1 * time.Second
	}

	// Enforce minimum interval (500ms)
	minInterval := 500 * time.Millisecond
	if duration < minInterval {
		fmt.Printf("⚠️ Warning: Watch interval too short, using minimum of 500ms\n")
		config.WatchInterval = "500ms"
		duration = minInterval
	}

	// Enforce maximum interval (1 hour)
	maxInterval := 1 * time.Hour
	if duration > maxInterval {
		fmt.Printf("⚠️ Warning: Watch interval too long, using maximum of 1h\n")
		config.WatchInterval = "1h"
		duration = maxInterval
	}

}

func setupSignalHandling(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-ctx.Done():
			return
		case sig := <-sigCh:
			fmt.Printf("\n🛑 Received signal: %v\n", sig)
			done <- true
			return
		}
	}()
}

func watchFiles(ctx context.Context) {

	lastModified := make(map[string]time.Time)

	// Get initial file list and modification times
	err := filepath.Walk(config.WatchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !shouldWatch(path) {
			return nil
		}

		lastModified[path] = info.ModTime()
		if len(lastModified) > config.MaxWatchers {
			return fmt.Errorf("Exceeded max watchers limit: %d", config.MaxWatchers)
		}
		return nil
	})

	if err != nil {
		errCh <- err
		return
	}

	duration, err := time.ParseDuration(config.WatchInterval)
	if err != nil {
		errCh <- fmt.Errorf("Invalid watch interval: %s", config.WatchInterval)
		return
	}

	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("🛑 Stopping file watcher...")
			return
		case <-ticker.C:
			changes := false

			err := filepath.Walk(config.WatchDir, func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if info.IsDir() || !shouldWatch(path) {
					return nil
				}

				// Check if file is new or modified
				modTime := info.ModTime()
				lastMod, exists := lastModified[path]

				if !exists || modTime.After(lastMod) {
					changes = true
					lastModified[path] = modTime
					fmt.Printf("📝 File changed: %s\n", path)
				}

				if !exists {
					if len(lastModified) >= config.MaxWatchers {
						return fmt.Errorf("Exceeded max watchers limit: %d", config.MaxWatchers)
					}
				}

				return nil
			})

			if err != nil {
				errCh <- err
				return
			}

			if changes {
				buildCh <- true
			}
		}
	}
}

func shouldWatch(filename string) bool {
	for _, ext := range config.WatchExts {
		if strings.HasSuffix(filename, ext) {
			return true
		}
	}
	return false
}

func buildAndRun() {
	fmt.Println("🔨 Building...")

	// Build the program
	buildCmd := exec.Command("go", "build", "-o", config.BinaryName, config.MainFile)
	buildCmd.Stderr = os.Stderr

	if err := buildCmd.Run(); err != nil {
		fmt.Printf("❌ Build failed: %s\n", err)
		return
	}

	fmt.Println("✅ Build successful")
	fmt.Println("🚀 Running program...")

	// Run the compiled program
	cmd = exec.Command("./" + config.BinaryName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("❌ Error starting program: %s\n", err)
		return
	}

	fmt.Println("✅ Program is running...")
}

func stopProcess() {
	if cmd != nil && cmd.Process != nil {
		fmt.Println("🛑 Stopping previous process...")
		cmd.Process.Kill()
		cmd.Wait()
	}
}
