package collection

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func parseBruEnv(path, root string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	var secrets []string
	for _, b := range splitBruBlocks(string(data)) {
		switch b.name {
		case "vars":
			for _, kv := range parseDict(b.body) {
				out[kv.K] = kv.V
			}
		case "vars:secret":
			for _, line := range strings.Split(b.body, "\n") {
				line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), ","))
				if line != "" {
					secrets = append(secrets, line)
				}
			}
		}
	}
	dotenv := loadDotenv(root)
	for _, name := range secrets {
		if v, ok := os.LookupEnv(name); ok {
			out[name] = v
		} else if v, ok := dotenv[name]; ok {
			out[name] = v
		} else {
			out[name] = ""
		}
	}
	return out, nil
}

func loadDotenv(root string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}

func parseYAMLEnv(path, root string) (map[string]string, error) {
	name := strings.TrimSuffix(filepath.Base(path), ".yml")
	out := map[string]string{}

	if inline, err := inlineYAMLEnv(root, name); err == nil {
		for k, v := range inline {
			out[k] = v
		}
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if len(out) == 0 {
			return nil, err
		}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	var env struct {
		Variables []yamlVariable `yaml:"variables"`
	}
	if err := yaml.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	dotenv := loadDotenv(root)
	for _, v := range env.Variables {
		if v.Disabled {
			continue
		}
		if v.Secret {
			if val, ok := os.LookupEnv(v.Name); ok {
				out[v.Name] = val
			} else if val, ok := dotenv[v.Name]; ok {
				out[v.Name] = val
			} else {
				out[v.Name] = ""
			}
			continue
		}
		val, err := v.resolveValue()
		if err != nil {
			return nil, fmt.Errorf("%s: variable %q: %w", path, v.Name, err)
		}
		out[v.Name] = val
	}
	return out, nil
}

func inlineYAMLEnv(root, name string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "opencollection.yml"))
	if err != nil {
		return nil, err
	}
	var root2 struct {
		Config struct {
			Environments []struct {
				Name      string         `yaml:"name"`
				Variables []yamlVariable `yaml:"variables"`
			} `yaml:"environments"`
		} `yaml:"config"`
	}
	if err := yaml.Unmarshal(data, &root2); err != nil {
		return nil, err
	}
	for _, e := range root2.Config.Environments {
		if e.Name != name {
			continue
		}
		out := map[string]string{}
		for _, v := range e.Variables {
			if v.Disabled {
				continue
			}
			val, err := v.resolveValue()
			if err != nil {
				return nil, err
			}
			out[v.Name] = val
		}
		return out, nil
	}
	return nil, fmt.Errorf("no inline environment %q", name)
}

func envNamesInDir(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ext) {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ext))
	}
	return names, nil
}
