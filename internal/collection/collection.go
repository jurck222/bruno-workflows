package collection

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Format int

const (
	FormatBru Format = iota
	FormatYAML
)

type Collection struct {
	Root     string
	Format   Format
	Name     string
	Defaults Defaults
}

func Open(startDir string) (*Collection, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, err
	}
	for {
		if fileExists(filepath.Join(dir, "opencollection.yml")) {
			return openYAMLRoot(dir)
		}
		if fileExists(filepath.Join(dir, "bruno.json")) {
			return openBruRoot(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("no bruno.json or opencollection.yml found above %s", startDir)
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func openBruRoot(dir string) (*Collection, error) {
	data, err := os.ReadFile(filepath.Join(dir, "bruno.json"))
	if err != nil {
		return nil, err
	}
	var meta struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("%s/bruno.json: %w", dir, err)
	}
	defaults, err := loadBruFolderDefaults(filepath.Join(dir, "collection.bru"))
	if err != nil {
		return nil, err
	}
	return &Collection{Root: dir, Format: FormatBru, Name: meta.Name, Defaults: defaults}, nil
}

func loadBruFolderDefaults(path string) (Defaults, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Defaults{}, nil
	}
	if err != nil {
		return Defaults{}, err
	}
	blocks := splitBruBlocks(string(data))
	var d Defaults
	for _, b := range blocks {
		switch b.name {
		case "headers":
			d.Headers = parseDict(b.body)
		case "vars":
			d.Vars = parseDict(b.body)
		}
	}
	auth, err := resolveBruAuth(blocks, "", path)
	if err != nil {
		return Defaults{}, err
	}
	d.Auth = auth
	return d, nil
}

func openYAMLRoot(dir string) (*Collection, error) {
	path := filepath.Join(dir, "opencollection.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var root struct {
		Info struct {
			Name string `yaml:"name"`
		} `yaml:"info"`
		Bundled bool `yaml:"bundled"`
		Request struct {
			Headers   []ycHeader     `yaml:"headers"`
			Auth      *ycAuth        `yaml:"auth"`
			Variables []yamlVariable `yaml:"variables"`
		} `yaml:"request"`
		Extends         yaml.Node `yaml:"extends"`
		ExternalSecrets yaml.Node `yaml:"externalSecrets"`
	}
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if root.Bundled {
		return nil, fmt.Errorf("%s: bundled collections are not supported", path)
	}
	if root.Extends.Kind != 0 {
		return nil, fmt.Errorf("%s: extends is not supported", path)
	}
	if root.ExternalSecrets.Kind != 0 {
		return nil, fmt.Errorf("%s: externalSecrets is not supported", path)
	}
	defaults, err := ycRequestDefaultsToDefaults(root.Request.Headers, root.Request.Auth, root.Request.Variables, path)
	if err != nil {
		return nil, err
	}
	return &Collection{Root: dir, Format: FormatYAML, Name: root.Info.Name, Defaults: defaults}, nil
}

func ycRequestDefaultsToDefaults(headers []ycHeader, auth *ycAuth, vars []yamlVariable, path string) (Defaults, error) {
	var d Defaults
	for _, h := range headers {
		if h.Disabled {
			continue
		}
		d.Headers = append(d.Headers, KV{K: h.Name, V: h.Value})
	}
	for _, v := range vars {
		if v.Disabled {
			continue
		}
		val, err := v.resolveValue()
		if err != nil {
			return Defaults{}, fmt.Errorf("%s: variable %q: %w", path, v.Name, err)
		}
		d.Vars = append(d.Vars, KV{K: v.Name, V: val})
	}
	a, err := auth.toAuth()
	if err != nil {
		return Defaults{}, fmt.Errorf("%s: %w", path, err)
	}
	d.Auth = a
	return d, nil
}

func (c *Collection) folderDefaults(dir string) (Defaults, error) {
	if c.Format == FormatBru {
		return loadBruFolderDefaults(filepath.Join(dir, "folder.bru"))
	}

	folderYML := filepath.Join(dir, "folder.yml")
	nestedOC := filepath.Join(dir, "opencollection.yml")
	path := ""
	if fileExists(folderYML) {
		path = folderYML
	} else if fileExists(nestedOC) {
		path = nestedOC
	} else {
		return Defaults{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Defaults{}, err
	}
	var f struct {
		Request struct {
			Headers   []ycHeader     `yaml:"headers"`
			Auth      *ycAuth        `yaml:"auth"`
			Variables []yamlVariable `yaml:"variables"`
		} `yaml:"request"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Defaults{}, fmt.Errorf("%s: %w", path, err)
	}
	return ycRequestDefaultsToDefaults(f.Request.Headers, f.Request.Auth, f.Request.Variables, path)
}

func (c *Collection) resolvePath(relPath string) (string, error) {
	stem := strings.TrimSuffix(strings.TrimSuffix(relPath, ".bru"), ".yml")
	bruPath := filepath.Join(c.Root, stem+".bru")
	ymlPath := filepath.Join(c.Root, stem+".yml")
	bruExists, ymlExists := fileExists(bruPath), fileExists(ymlPath)
	switch {
	case bruExists && ymlExists:
		return "", fmt.Errorf("%s: both %s.bru and %s.yml exist; the collection is in a broken half-migrated state", relPath, stem, stem)
	case bruExists:
		return bruPath, nil
	case ymlExists:
		return ymlPath, nil
	default:
		return "", fmt.Errorf("request %q not found", relPath)
	}
}

func (c *Collection) Load(relPath string) (*Request, error) {
	path, err := c.resolvePath(relPath)
	if err != nil {
		return nil, err
	}

	var req *Request
	if strings.HasSuffix(path, ".bru") {
		req, err = parseBru(path)
	} else {
		req, err = parseYAMLRequest(path)
	}
	if err != nil {
		return nil, err
	}

	chain, err := c.folderChain(filepath.Dir(path))
	if err != nil {
		return nil, err
	}

	headers := append([]KV{}, c.Defaults.Headers...)
	vars := append([]KV{}, c.Defaults.Vars...)
	authChain := []*Auth{c.Defaults.Auth}
	for _, d := range chain {
		headers = mergeKV(headers, d.Headers, true)
		vars = mergeKV(vars, d.Vars, false)
		authChain = append(authChain, d.Auth)
	}
	headers = mergeKV(headers, req.Headers, true)
	vars = mergeKV(vars, req.Vars, false)

	req.Headers = headers
	req.Vars = vars
	req.Auth = resolveAuthChain(authChain, req.Auth)

	return req, nil
}

func (c *Collection) folderChain(dir string) ([]Defaults, error) {
	var dirs []string
	for d := dir; ; d = filepath.Dir(d) {
		if d == c.Root {
			break
		}
		dirs = append([]string{d}, dirs...)
		if filepath.Dir(d) == d {
			return nil, fmt.Errorf("directory %s is not under collection root %s", dir, c.Root)
		}
	}
	var chain []Defaults
	for _, d := range dirs {
		fd, err := c.folderDefaults(d)
		if err != nil {
			return nil, err
		}
		chain = append(chain, fd)
	}
	return chain, nil
}

func mergeKV(dst []KV, src []KV, caseInsensitive bool) []KV {
	eq := func(a, b string) bool {
		if caseInsensitive {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	out := append([]KV{}, dst...)
	for _, kv := range src {
		found := false
		for i := range out {
			if eq(out[i].K, kv.K) {
				out[i].V = kv.V
				found = true
				break
			}
		}
		if !found {
			out = append(out, kv)
		}
	}
	return out
}

func resolveAuthChain(chain []*Auth, own *Auth) *Auth {
	if own == nil {
		return nil
	}
	if own.Type != "inherit" {
		return own
	}
	for i := len(chain) - 1; i >= 0; i-- {
		a := chain[i]
		if a != nil && a.Type != "inherit" {
			return a
		}
	}
	return nil
}

func (c *Collection) Envs() ([]string, error) {
	dir := filepath.Join(c.Root, "environments")
	if c.Format == FormatBru {
		return envNamesInDir(dir, ".bru")
	}
	return envNamesInDir(dir, ".yml")
}

func (c *Collection) Env(name string) (map[string]string, error) {
	if c.Format == FormatBru {
		return parseBruEnv(filepath.Join(c.Root, "environments", name+".bru"), c.Root)
	}
	return parseYAMLEnv(filepath.Join(c.Root, "environments", name+".yml"), c.Root)
}
