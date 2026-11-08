// Command catalog writes a Silo plugin repository index (catalog.json) for
// one release of this plugin, so Silo can install and update it from
// Admin -> Plugins -> Repositories.
//
// The index has the shape Silo reads: {"plugins": [{"manifest", "repo_url",
// "checksums_url", "binaries"}]}. The manifest is decoded the way Silo decodes
// a plugin manifest and written with encoding/json, as the Silo-Community
// catalog writes it, so Silo reads it back into the same struct.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	pluginv1 "github.com/Silo-Server/silo-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type platformBinary struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum"`
}

type catalogPackage struct {
	Manifest     *pluginv1.PluginManifest  `json:"manifest"`
	RepoURL      string                    `json:"repo_url,omitempty"`
	ChecksumsURL string                    `json:"checksums_url,omitempty"`
	Binaries     map[string]platformBinary `json:"binaries"`
}

type repositoryIndex struct {
	Plugins []catalogPackage `json:"plugins"`
}

func main() {
	manifestPath := flag.String("manifest", "manifest.json", "plugin manifest")
	checksumsPath := flag.String("checksums", "", "release checksums.txt (\"<sha256>  plugin-<os>-<arch>\" per line)")
	repo := flag.String("repo", "", "GitHub repository, owner/name")
	tag := flag.String("tag", "", "release tag, such as v0.3.0")
	out := flag.String("out", "catalog.json", "output file")
	flag.Parse()
	if err := run(*manifestPath, *checksumsPath, *repo, *tag, *out); err != nil {
		fmt.Fprintln(os.Stderr, "catalog:", err)
		os.Exit(1)
	}
}

func run(manifestPath, checksumsPath, repo, tag, out string) error {
	if repo == "" || tag == "" || checksumsPath == "" {
		return fmt.Errorf("-repo, -tag and -checksums are required")
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var manifest pluginv1.PluginManifest
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("decode %s: %w", manifestPath, err)
	}
	manifest.Version = strings.TrimPrefix(tag, "v")
	// The manifest carries "__CHECKSUM__" until a build substitutes it; the
	// index gives each platform's checksum instead.
	manifest.Checksum = ""

	download := "https://github.com/" + repo + "/releases/download/" + tag + "/"
	binaries, err := readChecksums(checksumsPath, download)
	if err != nil {
		return err
	}
	index := repositoryIndex{Plugins: []catalogPackage{{
		Manifest:     &manifest,
		RepoURL:      "https://github.com/" + repo,
		ChecksumsURL: download + "checksums.txt",
		Binaries:     binaries,
	}}}
	encoded, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, append(encoded, '\n'), 0o644)
}

func readChecksums(path, download string) (map[string]platformBinary, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	binaries := map[string]platformBinary{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		sum, name := fields[0], strings.TrimPrefix(fields[1], "*")
		platform := strings.TrimPrefix(name, "plugin-")
		osName, arch, ok := strings.Cut(platform, "-")
		if !ok || platform == name || len(sum) != 64 {
			continue
		}
		binaries[osName+"/"+arch] = platformBinary{URL: download + name, Checksum: sum}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(binaries) == 0 {
		return nil, fmt.Errorf("%s lists no plugin binaries", path)
	}
	return binaries, nil
}
