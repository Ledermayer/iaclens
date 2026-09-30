// licensenotices collects redistribution notices for the release dependency graph.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// module records the source identity selected by the Go build tool.
type module struct {
	Path, Version, Dir string
	Main               bool
	Replace            *module
}

// dependency contains the files selected for one package on one release target.
type dependency struct {
	Dir, ImportPath string
	Standard        bool
	Module          *module
	GoFiles         []string
	SFiles          []string
}

// notice combines license texts and source attribution without local paths.
type notice struct {
	Source string
	Texts  map[string]string
}

var targetSystems = []string{"linux", "darwin", "windows"}
var targetArchitectures = []string{"amd64", "arm64"}
var noticeNames = []string{"LICENSE", "LICENSE.md", "LICENSE.txt", "LICENCE", "COPYING", "NOTICE", "NOTICE.txt", "PATENTS"}

// decodeDependencies reads the consecutive JSON objects emitted by go list.
func decodeDependencies(reader io.Reader) ([]dependency, error) {
	var dependencies []dependency
	decoder := json.NewDecoder(reader)
	for {
		var item dependency
		if err := decoder.Decode(&item); errors.Is(err, io.EOF) {
			return dependencies, nil
		} else if err != nil {
			return nil, fmt.Errorf("decode dependency: %w", err)
		}
		dependencies = append(dependencies, item)
	}
}

// proxyPath implements the Go module proxy's uppercase escaping convention.
func proxyPath(value string) string {
	var escaped strings.Builder
	for _, character := range value {
		if character >= 'A' && character <= 'Z' {
			escaped.WriteByte('!')
			escaped.WriteRune(character + ('a' - 'A'))
		} else {
			escaped.WriteRune(character)
		}
	}
	return escaped.String()
}

// addNoticeFiles collects the complete license and supplementary notice files.
func addNoticeFiles(root, directory string, texts map[string]string) (bool, error) {
	licensed := false
	for _, name := range noticeNames {
		filename := filepath.Join(directory, name)
		data, err := os.ReadFile(filename)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("read notice %s: %w", filename, err)
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return false, fmt.Errorf("empty notice: %s", filename)
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return false, err
		}
		texts[filepath.ToSlash(relative)] = string(data)
		licensed = licensed || strings.HasPrefix(name, "LICEN") || name == "COPYING"
	}
	return licensed, nil
}

// addSourceNotices preserves inline licenses, including Go's Sun and fiat notices.
func addSourceNotices(root string, item dependency, texts map[string]string) error {
	for _, name := range item.GoFiles {
		filename := filepath.Join(item.Dir, name)
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		positions := token.NewFileSet()
		parsed, err := parser.ParseFile(positions, filename, data, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parse source notices %s: %w", filename, err)
		}
		var selected strings.Builder
		for _, group := range parsed.Comments {
			text := strings.ToLower(group.Text())
			if strings.Contains(text, "copyright") || strings.Contains(text, "license") ||
				strings.Contains(text, "permission") || strings.Contains(text, "redistribution") || strings.Contains(text, "warranty") {
				selected.Write(data[positions.Position(group.Pos()).Offset:positions.Position(group.End()).Offset])
				selected.WriteByte('\n')
			}
		}
		if selected.Len() > 0 {
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			texts[filepath.ToSlash(relative)+" (source notices)"] = selected.String()
		}
	}
	return nil
}

// addAssemblyNotices retains complete comment text from selected assembly files.
func addAssemblyNotices(root string, item dependency, texts map[string]string) error {
	for _, name := range item.SFiles {
		filename := filepath.Join(item.Dir, name)
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		positions := token.NewFileSet()
		var lexer scanner.Scanner
		lexer.Init(positions.AddFile(filename, -1, len(data)), data, nil, scanner.ScanComments)
		var comments strings.Builder
		for {
			_, kind, literal := lexer.Scan()
			if kind == token.EOF {
				break
			}
			if kind == token.COMMENT {
				comments.WriteString(literal)
				comments.WriteByte('\n')
			}
		}
		if comments.Len() > 0 {
			relative, err := filepath.Rel(root, filename)
			if err != nil {
				return err
			}
			texts[filepath.ToSlash(relative)+" (assembly comments)"] = comments.String()
		}
	}
	return nil
}

// goDistributionNotice supports official Go archives and Homebrew's license layout.
func goDistributionNotice(goroot, goversion string) (*notice, error) {
	goNotice := &notice{Source: "https://go.dev/dl/" + goversion + ".src.tar.gz", Texts: map[string]string{}}
	licensed, err := addNoticeFiles(goroot, goroot, goNotice.Texts)
	if err != nil {
		return nil, err
	}
	if !licensed {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(goroot), "LICENSE"))
		if err != nil || len(bytes.TrimSpace(data)) == 0 {
			return nil, fmt.Errorf("Go distribution license is unavailable at %s or its parent", goroot)
		}
		goNotice.Texts["LICENSE"] = string(data)
	}
	return goNotice, nil
}

// dependencyNotice requires exact upstream source identity and a root license.
func dependencyNotice(item dependency, goroot string, goNotice *notice, notices map[string]*notice) (string, *notice, error) {
	if item.Standard {
		return goroot, goNotice, nil
	}
	if item.Module == nil || item.Module.Version == "" || item.Module.Replace != nil {
		return "", nil, fmt.Errorf("dependency %s requires a versioned, unreplaced module", item.ImportPath)
	}
	root := item.Module.Dir
	identity := item.Module.Path + "@" + item.Module.Version
	if entry := notices[identity]; entry != nil {
		return root, entry, nil
	}
	entry := &notice{Source: "https://proxy.golang.org/" + proxyPath(item.Module.Path) + "/@v/" + proxyPath(item.Module.Version) + ".zip", Texts: map[string]string{}}
	licensed, err := addNoticeFiles(root, root, entry.Texts)
	if err != nil {
		return "", nil, err
	}
	if !licensed {
		return "", nil, fmt.Errorf("dependency %s has no recognized root license", identity)
	}
	notices[identity] = entry
	return root, entry, nil
}

// addAncestorNotices preserves nested and Go-vendored licenses inside the source root.
func addAncestorNotices(root, directory string, texts map[string]string, seen map[string]bool) error {
	for ; directory != root; directory = filepath.Dir(directory) {
		relative, err := filepath.Rel(root, directory)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("dependency directory is outside its source root")
		}
		if seen[directory] {
			continue
		}
		if _, err := addNoticeFiles(root, directory, texts); err != nil {
			return err
		}
		seen[directory] = true
	}
	return nil
}

// collect merges target graphs and fails closed on unversioned/replaced sources.
func collect(dependencies []dependency, goroot, goversion string) (map[string]*notice, error) {
	goNotice, err := goDistributionNotice(goroot, goversion)
	if err != nil {
		return nil, err
	}
	notices := map[string]*notice{"Go " + goversion: goNotice}
	seen := map[string]bool{}
	for _, item := range dependencies {
		if item.Module != nil && item.Module.Main {
			continue
		}
		root, entry, err := dependencyNotice(item, goroot, goNotice, notices)
		if err != nil {
			return nil, err
		}
		if err := addAncestorNotices(root, item.Dir, entry.Texts, seen); err != nil {
			return nil, err
		}
		if err := addSourceNotices(root, item, entry.Texts); err != nil {
			return nil, err
		}
		if err := addAssemblyNotices(root, item, entry.Texts); err != nil {
			return nil, err
		}
	}
	return notices, nil
}

// render sorts every section so target ordering cannot change the notice bundle.
func render(notices map[string]*notice) ([]byte, error) {
	var output strings.Builder
	output.WriteString("IaCLens third-party redistribution notices\n\nThese notices cover the union of the six release targets. Source archives\nidentify the exact dependency and Go versions distributed in the binaries.\nUpstream licenses and notices below retain their original terms.\n\n")
	identities := make([]string, 0, len(notices))
	for identity := range notices {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	for _, identity := range identities {
		entry := notices[identity]
		fmt.Fprintf(&output, "===== %s =====\nSource: %s\n\n", identity, entry.Source)
		paths := make([]string, 0, len(entry.Texts))
		for path := range entry.Texts {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			fmt.Fprintf(&output, "--- %s ---\n%s\n\n", path, entry.Texts[path])
		}
	}
	text := output.String()
	if strings.Contains(text, "Apache License") && !strings.Contains(text, "END OF TERMS AND CONDITIONS") {
		return nil, fmt.Errorf("Apache-licensed dependencies require the complete Apache 2.0 text")
	}
	return []byte(text), nil
}

// run uses the same target settings as release.sh without adding build dependencies.
func run() error {
	output := flag.String("out", "THIRD_PARTY_NOTICES.txt", "Generated redistribution notice file")
	flag.Parse()
	settings, err := exec.Command("go", "env", "-json", "GOROOT", "GOVERSION").Output()
	if err != nil {
		return err
	}
	var environment struct{ GOROOT, GOVERSION string }
	if err := json.Unmarshal(settings, &environment); err != nil {
		return err
	}
	var dependencies []dependency
	for _, system := range targetSystems {
		for _, architecture := range targetArchitectures {
			command := exec.Command("go", "list", "-deps", "-json", "./cmd/iaclens")
			command.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+system, "GOARCH="+architecture)
			command.Stderr = os.Stderr
			data, err := command.Output()
			if err != nil {
				return fmt.Errorf("list %s/%s dependencies: %w", system, architecture, err)
			}
			items, err := decodeDependencies(bytes.NewReader(data))
			if err != nil {
				return err
			}
			dependencies = append(dependencies, items...)
		}
	}
	notices, err := collect(dependencies, environment.GOROOT, environment.GOVERSION)
	if err != nil {
		return err
	}
	data, err := render(notices)
	if err != nil {
		return err
	}
	return os.WriteFile(*output, data, 0644)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "licensenotices:", err)
		os.Exit(1)
	}
}
