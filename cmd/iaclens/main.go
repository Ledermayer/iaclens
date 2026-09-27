package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Ledermayer/iaclens/internal/jev"
	"github.com/Ledermayer/iaclens/internal/scan"
)

var (
	version   = "dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func run() error {
	showVersion := flag.Bool("version", false, "Print version, commit and build date")
	source := flag.String("source", ".", "Terraform repository directory")
	out := flag.String("out", "module.yaml", "YAML output; existing content outside IACLENS markers is preserved")
	mode := flag.String("mode", "offline", "offline, prepare (write request), or jev (live API)")
	requestPath := flag.String("request-out", "request.json", "prepared Jev request path")
	model := flag.String("model", "jev-1.13.0", "TypeSafe model ID")
	provider := flag.String("provider", "typesafe", "typesafe or llmgateway")
	flag.Parse()
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
	inv, err := scan.Read(*source)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(inv)
	digest := sha256.Sum256(encoded)
	analysis := map[string]any{"status": "not_run", "question_version": "1"}
	doc := map[string]any{"iaclens": map[string]any{"schema_version": 1, "source_digest": hex.EncodeToString(digest[:]), "modules": inv.Modules, "analysis_jev": analysis}}
	req := jev.Build(inv, *model)
	if *mode == "prepare" {
		b, err := json.MarshalIndent(req, "", "  ")
		if err != nil {
			return err
		}
		if err = write(*requestPath, append(b, '\n')); err != nil {
			return err
		}
		analysis["status"] = "prepared"
	}
	if *mode == "jev" {
		key := os.Getenv(keyEnv)
		if key == "" {
			return fmt.Errorf("%s is required for --mode jev", keyEnv)
		}
		r, err := jev.Evaluate(context.Background(), endpoint, key, req)
		if err != nil {
			return err
		}
		analysis["status"] = "review_required"
		analysis["result"] = r
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
	fmt.Printf("Scanned %d Terraform files across %d module directories → %s (Jev: %s)\n", len(inv.Files), len(inv.Modules), *out, analysis["status"])
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
