package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Package struct {
	Source      string    `json:"source"`
	InstalledAt time.Time `json:"installed_at"`
}

type Config struct {
	BinDir   string             `json:"bin_dir"`
	Packages map[string]Package `json:"packages"`
}

var (
	configPath string
	verbose    bool
)

func init() {
	home, _ := os.UserHomeDir()
	configPath = filepath.Join(home, ".config", "tuna", "config.json")
}

func main() {
	// 1. Define Flags
	installCmd := flag.NewFlagSet("install", flag.ExitOnError)
	binDirFlag := installCmd.String("dir", "", "Override destination directory")
	vFlag := installCmd.Bool("v", false, "Verbose output")

	if len(os.Args) < 2 {
		printUsage()
		return
	}

	cfg := loadConfig()

	switch os.Args[1] {
	case "install":
		installCmd.Parse(os.Args[2:])
		verbose = *vFlag
		
		args := installCmd.Args()
		if len(args) < 2 {
			fmt.Println("❌ Error: 'install' requires <name> and <source>")
			installCmd.Usage()
			return
		}

		if *binDirFlag != "" {
			cfg.BinDir = *binDirFlag
		}
		
		install(cfg, args[0], args[1])

	case "remove":
		if len(os.Args) < 3 {
			fmt.Println("❌ Error: 'remove' requires a package name")
			return
		}
		remove(cfg, os.Args[2])

	case "list":
		list(cfg)

	default:
		printUsage()
	}
}

func install(cfg *Config, name, source string) {
	fmt.Printf("🎣 Fishing for %s...\n", name)
	dest := filepath.Join(cfg.BinDir, name)

	var reader io.ReadCloser
	var err error

	// 2. Local vs Remote Check
	if strings.HasPrefix(source, "http") {
		if verbose {
			fmt.Printf("🌐 Source is remote: %s\n", source)
		}
		resp, err := http.Get(source)
		if err != nil {
			fmt.Printf("❌ Download failed: %v\n", err)
			return
		}
		reader = resp.Body
	} else {
		if verbose {
			fmt.Printf("🏠 Source is local: %s\n", source)
		}
		file, err := os.Open(source)
		if err != nil {
			fmt.Printf("❌ Failed to open local file: %v\n", err)
			return
		}
		reader = file
	}
	defer reader.Close()

	// 3. Handle Archiving or Plain Binary
	if strings.HasSuffix(source, ".tar.gz") || strings.HasSuffix(source, ".tgz") {
		err = extractBinary(reader, dest)
	} else {
		err = savePlainBinary(reader, dest)
	}

	if err != nil {
		fmt.Printf("❌ Installation failed: %v\n", err)
		return
	}

	os.Chmod(dest, 0755)
	cfg.Packages[name] = Package{Source: source, InstalledAt: time.Now()}
	saveConfig(cfg)
	fmt.Printf("✅ Success! %s is now at %s\n", name, dest)
}

// --- HELPER LOGIC ---

func savePlainBinary(r io.Reader, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

func extractBinary(r io.Reader, dest string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	_, err = tr.Next()
	if err != nil {
		return err
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, tr)
	return err
}

// --- CONFIG & HELP ---

func loadConfig() *Config {
	os.MkdirAll(filepath.Dir(configPath), 0755)
	cfg := &Config{BinDir: "/usr/local/bin", Packages: make(map[string]Package)}
	file, err := os.ReadFile(configPath)
	if err == nil {
		json.Unmarshal(file, cfg)
	}
	return cfg
}

func saveConfig(cfg *Config) {
	data, _ := json.MarshalIndent(cfg, "", "  ")
	os.WriteFile(configPath, data, 0644)
}

func list(cfg *Config) {
	if len(cfg.Packages) == 0 {
		fmt.Println("No fish in the bucket. (Zero packages installed)")
		return
	}
	fmt.Printf("%-15s %-20s %s\n", "NAME", "INSTALLED", "SOURCE")
	for name, pkg := range cfg.Packages {
		fmt.Printf("%-15s %-20s %s\n", name, pkg.InstalledAt.Format("2006-01-02"), pkg.Source)
	}
}

func remove(cfg *Config, name string) {
	if _, exists := cfg.Packages[name]; !exists {
		fmt.Printf("❌ %s is not installed.\n", name)
		return
	}
	os.Remove(filepath.Join(cfg.BinDir, name))
	delete(cfg.Packages, name)
	saveConfig(cfg)
	fmt.Printf("🗑️ Released %s back into the wild.\n", name)
}

func printUsage() {
	fmt.Println(`tuna - the binary package manager that just works

USAGE:
  tuna <command> [arguments]

COMMANDS:
  install   Download/copy a binary to your bin folder
  remove    Delete an installed binary
  list      Show all binaries managed by tuna

INSTALL FLAGS:
  -v        Enable verbose output
  -dir      Override the installation directory (default: /usr/local/bin)

EXAMPLES:
  tuna install mytool ./path/to/binary
  tuna install gh https://github.com/cli/cli/releases/.../gh_linux_amd64.tar.gz`)
}
