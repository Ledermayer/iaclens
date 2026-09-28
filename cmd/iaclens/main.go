package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Ledermayer/iaclens/internal/analyze"
	"github.com/Ledermayer/iaclens/internal/config"
	"github.com/Ledermayer/iaclens/internal/jev"
	"github.com/Ledermayer/iaclens/internal/scan"
	"github.com/Ledermayer/iaclens/rules"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func run() error {
	showVersion := flag.Bool("version", false, "Print version, commit and build date")
	source := flag.String("source", ".", "Git repository or a directory inside it; analysis uses the Git root")
	out := flag.String("out", "module.yaml", "YAML output; existing content outside IACLENS markers is preserved")
	mode := flag.String("mode", "offline", "offline, prepare (write request), or jev (live API)")
	requestPath := flag.String("request-out", "request.json", "prepared Jev request path")
	model := flag.String("model", "jev-1.13.0", "TypeSafe model ID")
	provider := flag.String("provider", "typesafe", "typesafe or llmgateway")
	configPath := flag.String("config", "", "Ruleset YAML; defaults to embedded rules/default.yaml")
	kind := flag.String("kind", "auto", "Root directory kind: auto, module, deployment, unknown")
	format := flag.String("format", "yaml", "Output format: yaml or json")
	printConfig := flag.Bool("print-default-config", false, "Print the default ruleset YAML and exit")
	flag.Parse()
	if *printConfig {
		fmt.Print(string(rules.Default))
		return nil
	}
	if *format != "yaml" && *format != "json" {
		return fmt.Errorf("invalid --format %q", *format)
	}
	if *showVersion {
		fmt.Printf("iaclens %s (commit %s, built %s)\n", version, commit, buildDate)
		return nil
	}
	endpoint, keyEnv := "https://api.typesafe.ai/v1/systemone", "TYPESAFE_API_KEY"
	if *provider == "llmgateway" {
		endpoint, keyEnv = "https://api.llmgateway.io/v1/systemone", "LLM_GATEWAY_API_KEY"
	} else if *provider != "typesafe" {
		return fmt.Errorf("invalid provider %q", *provider)
	}
	if *mode != "offline" && *mode != "prepare" && *mode != "jev" {
		return fmt.Errorf("invalid mode %q", *mode)
	}
	root, err := repositoryRoot(*source)
	if err != nil {
		return err
	}
	if *configPath == "" {
		local := filepath.Join(root, ".iaclens.yaml")
		if _, err := os.Stat(local); err == nil {
			*configPath = local
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	inv, err := scan.Read(root, cfg.Parse)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(inv)
	digest := sha256.Sum256(encoded)
	eval := func(ctx context.Context, req jev.Request) (jev.Response, error) {
		key := os.Getenv(keyEnv)
		if key == "" {
			return jev.Response{}, fmt.Errorf("%s is required for unresolved Jev decisions", keyEnv)
		}
		return jev.Evaluate(ctx, endpoint, key, req)
	}
	analysis, err := analyze.Run(context.Background(), inv, cfg, *mode, *model, *kind, eval)
	if err != nil {
		return err
	}
	doc := map[string]any{"iaclens": map[string]any{
		"schema_version": 2, "source_digest": hex.EncodeToString(digest[:]),
		"repository": map[string]string{"root": root, "name": filepath.Base(root)},
		"ruleset":    map[string]any{"name": cfg.Name, "version": cfg.Version, "digest": cfg.Digest},
		"units":      inv.Modules, "analysis": analysis,
	}}
	if *mode == "prepare" {
		prepared := map[string]any{"ruleset_digest": cfg.Digest, "requests": analysis.Requests, "note": "Requests are per directory. Profile-dependent Jev checks for unresolved directories are planned only after classification in live mode."}
		b, err := json.MarshalIndent(prepared, "", "  ")
		if err != nil {
			return err
		}
		if err = write(*requestPath, append(b, '\n')); err != nil {
			return err
		}
	}
	if *format == "json" {
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		if err = write(*out, append(data, '\n')); err != nil {
			return err
		}
		fmt.Printf("Scanned %d Terraform files across %d directories → %s (kind: %s, Jev calls: %d)\n", len(inv.Files), len(inv.Modules), *out, analysis.CodebaseKind, len(analysis.Calls))
		return nil
	}
	existing, err := os.ReadFile(*out)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := scan.Render(existing, doc)
	if err != nil {
		return err
	}
	if err = write(*out, data); err != nil {
		return err
	}
	fmt.Printf("Scanned %d Terraform files across %d directories → %s (kind: %s, Jev calls: %d)\n", len(inv.Files), len(inv.Modules), *out, analysis.CodebaseKind, len(analysis.Calls))
	return nil
}
func write(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".iaclens-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "iaclens:", err)
		os.Exit(1)
	}
}

// Git locates the repository boundary only. No history, remotes, policies or
// hosting-service settings participate in Terraform analysis.
func repositoryRoot(source string) (string, error) {
	cmd := exec.Command("git", "-C", source, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("--source must be inside a Git worktree: %w", err)
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}
