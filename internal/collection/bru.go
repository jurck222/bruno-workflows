package collection

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var bruBlockOpen = regexp.MustCompile(`^(\S+) \{$`)

type bruBlock struct {
	name string
	body string
}

func splitBruBlocks(src string) []bruBlock {
	lines := strings.Split(src, "\n")
	var blocks []bruBlock
	i := 0
	for i < len(lines) {
		m := bruBlockOpen.FindStringSubmatch(lines[i])
		if m == nil {
			i++
			continue
		}
		name := m[1]
		start := i + 1
		j := start
		for j < len(lines) && lines[j] != "}" {
			j++
		}
		blocks = append(blocks, bruBlock{name: name, body: strings.Join(lines[start:j], "\n")})
		i = j + 1
	}
	return blocks
}

func parseDict(body string) []KV {
	var out []KV
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "~") {
			continue
		}
		k, v, ok := strings.Cut(line, ": ")
		if !ok {
			k, v, ok = strings.Cut(line, ":")
			if !ok {
				continue
			}
		}
		out = append(out, KV{K: strings.TrimSpace(k), V: strings.TrimSpace(v)})
	}
	return out
}

func dictGet(kvs []KV, key string) (string, bool) {
	for _, kv := range kvs {
		if kv.K == key {
			return kv.V, true
		}
	}
	return "", false
}

func dedent2(body string) string {
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimPrefix(l, "  ")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

var jsonCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

var httpMethods = map[string]bool{
	"get": true, "post": true, "put": true, "patch": true,
	"delete": true, "options": true, "head": true,
}

var bodySelBlock = map[string]string{
	"json":           "body:json",
	"text":           "body:text",
	"xml":            "body:xml",
	"sparql":         "body:sparql",
	"formUrlEncoded": "body:form-urlencoded",
	"multipartForm":  "body:multipart-form",
}

func resolveBruAuth(blocks []bruBlock, initialMode, path string) (*Auth, error) {
	mode := initialMode
	var bearerToken, basicUser, basicPass string
	for _, b := range blocks {
		switch b.name {
		case "auth":
			if m, ok := dictGet(parseDict(b.body), "mode"); ok {
				mode = m
			}
		case "auth:bearer":
			bearerToken, _ = dictGet(parseDict(b.body), "token")
		case "auth:basic":
			d := parseDict(b.body)
			basicUser, _ = dictGet(d, "username")
			basicPass, _ = dictGet(d, "password")
		}
	}
	switch mode {
	case "", "none":
		return nil, nil
	case "inherit":
		return &Auth{Type: "inherit"}, nil
	case "bearer":
		return &Auth{Type: "bearer", Token: bearerToken}, nil
	case "basic":
		return &Auth{Type: "basic", Username: basicUser, Password: basicPass}, nil
	default:
		return nil, fmt.Errorf("%s: unsupported auth mode %q", path, mode)
	}
}

func parseBru(path string) (*Request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	blocks := splitBruBlocks(string(data))

	req := &Request{Path: path, TimeoutMS: -1}
	var inlineAuthMode, bodySel string

	for _, b := range blocks {
		switch {
		case b.name == "meta":
			d := parseDict(b.body)
			if name, ok := dictGet(d, "name"); ok {
				req.Name = name
			}
			if typ, ok := dictGet(d, "type"); ok && typ != "http" {
				return nil, fmt.Errorf("%s: unsupported request type %q", path, typ)
			}
			if seq, ok := dictGet(d, "seq"); ok {
				fmt.Sscanf(seq, "%d", &req.Seq)
			}

		case httpMethods[b.name]:
			req.Method = strings.ToUpper(b.name)
			d := parseDict(b.body)
			if url, ok := dictGet(d, "url"); ok {
				req.URL = url
			}
			if body, ok := dictGet(d, "body"); ok {
				bodySel = body
			}

			if auth, ok := dictGet(d, "auth"); ok {
				inlineAuthMode = auth
			}

		case b.name == "params:query":
			req.Query = parseDict(b.body)

		case b.name == "params:path":
			req.PathParams = parseDict(b.body)

		case b.name == "headers":
			req.Headers = parseDict(b.body)

		case b.name == "vars":
			req.Vars = parseDict(b.body)

		case b.name == "body:json" && bodySelBlock[bodySel] == b.name:
			raw := jsonCommentRe.ReplaceAllString(dedent2(b.body), "")
			req.Body = Body{Kind: "json", Raw: strings.TrimSpace(raw)}

		case b.name == "body:text" && bodySelBlock[bodySel] == b.name:
			req.Body = Body{Kind: "text", Raw: dedent2(b.body)}

		case b.name == "body:xml" && bodySelBlock[bodySel] == b.name:
			req.Body = Body{Kind: "xml", Raw: dedent2(b.body)}

		case b.name == "body:form-urlencoded" && bodySelBlock[bodySel] == b.name:
			var parts []Part
			for _, kv := range parseDict(b.body) {
				parts = append(parts, Part{Name: kv.K, Value: kv.V})
			}
			req.Body = Body{Kind: "form-urlencoded", Parts: parts}

		case b.name == "body:multipart-form" && bodySelBlock[bodySel] == b.name:
			var parts []Part
			for _, kv := range parseDict(b.body) {
				if strings.HasPrefix(kv.V, "@file(") && strings.HasSuffix(kv.V, ")") {
					fp := kv.V[len("@file(") : len(kv.V)-1]
					parts = append(parts, Part{Name: kv.K, IsFile: true, Value: fp})
				} else {
					parts = append(parts, Part{Name: kv.K, Value: kv.V})
				}
			}
			req.Body = Body{Kind: "multipart-form", Parts: parts}

		case b.name == "settings":
			d := parseDict(b.body)
			if t, ok := dictGet(d, "timeout"); ok {
				var secs int
				fmt.Sscanf(t, "%d", &secs)
				req.TimeoutMS = secs * 1000
			}
		}
	}

	auth, err := resolveBruAuth(blocks, inlineAuthMode, path)
	if err != nil {
		return nil, err
	}
	req.Auth = auth

	return req, nil
}
