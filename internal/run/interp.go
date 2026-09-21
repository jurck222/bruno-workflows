package run

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var varRe = regexp.MustCompile(`\{\{\s*([^{}\s]+)\s*\}\}`)

func Interpolate(s string, vars map[string]string, dotenv map[string]string) (string, error) {
	var firstErr error
	out := varRe.ReplaceAllStringFunc(s, func(m string) string {
		if firstErr != nil {
			return m
		}
		name := varRe.FindStringSubmatch(m)[1]
		if rest, ok := strings.CutPrefix(name, "process.env."); ok {
			if v, ok := os.LookupEnv(rest); ok {
				return v
			}
			if v, ok := dotenv[rest]; ok {
				return v
			}
			firstErr = fmt.Errorf("{{%s}} is unresolved (not in process env or .env)", name)
			return m
		}
		if v, ok := vars[name]; ok {
			return v
		}
		firstErr = fmt.Errorf("{{%s}} is unresolved", name)
		return m
	})
	if firstErr != nil {
		return "", firstErr
	}
	return out, nil
}

func LoadDotenv(root string) map[string]string {
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
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}
