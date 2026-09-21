package collection

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type scalarOrSeq []string

func (s *scalarOrSeq) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.SequenceNode {
		var out []string
		for _, n := range node.Content {
			out = append(out, n.Value)
		}
		*s = out
		return nil
	}
	*s = []string{node.Value}
	return nil
}

type ycHeader struct {
	Name        string `yaml:"name"`
	Value       string `yaml:"value"`
	Disabled    bool   `yaml:"disabled"`
	Description string `yaml:"description"`
}

type ycParam struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Type     string `yaml:"type"`
	Disabled bool   `yaml:"disabled"`
}

type ycMultipartField struct {
	Name        string      `yaml:"name"`
	Type        string      `yaml:"type"`
	Value       scalarOrSeq `yaml:"value"`
	ContentType string      `yaml:"contentType"`
	Disabled    bool        `yaml:"disabled"`
}

type ycFormField struct {
	Name     string `yaml:"name"`
	Value    string `yaml:"value"`
	Disabled bool   `yaml:"disabled"`
}

type ycFileVariant struct {
	FilePath    string `yaml:"filePath"`
	ContentType string `yaml:"contentType"`
	Selected    bool   `yaml:"selected"`
}

type ycBody struct {
	Type string    `yaml:"type"`
	Data yaml.Node `yaml:"data"`
}

type ycBodyField struct {
	body     *ycBody
	variants []struct {
		Selected bool   `yaml:"selected"`
		Body     ycBody `yaml:"body"`
	}
}

func (f *ycBodyField) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.SequenceNode {
		return node.Decode(&f.variants)
	}
	var b ycBody
	if err := node.Decode(&b); err != nil {
		return err
	}
	f.body = &b
	return nil
}

func (f *ycBodyField) resolve() *ycBody {
	if f == nil {
		return nil
	}
	if f.body != nil {
		return f.body
	}
	for _, v := range f.variants {
		if v.Selected {
			b := v.Body
			return &b
		}
	}
	if len(f.variants) > 0 {
		b := f.variants[0].Body
		return &b
	}
	return nil
}

type ycAuth struct {
	scalar string
	obj    *ycAuthObj
}

type ycAuthObj struct {
	Type      string `yaml:"type"`
	Token     string `yaml:"token"`
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	Key       string `yaml:"key"`
	Value     string `yaml:"value"`
	Placement string `yaml:"placement"`
}

func (a *ycAuth) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		a.scalar = node.Value
		return nil
	}
	var o ycAuthObj
	if err := node.Decode(&o); err != nil {
		return err
	}
	a.obj = &o
	return nil
}

func (a *ycAuth) toAuth() (*Auth, error) {
	if a == nil {
		return nil, nil
	}
	if a.scalar == "inherit" {
		return &Auth{Type: "inherit"}, nil
	}
	if a.scalar != "" && a.scalar != "none" {
		return nil, fmt.Errorf("unsupported auth %q", a.scalar)
	}
	if a.obj == nil {
		return nil, nil
	}
	switch a.obj.Type {
	case "bearer":
		return &Auth{Type: "bearer", Token: a.obj.Token}, nil
	case "basic":
		return &Auth{Type: "basic", Username: a.obj.Username, Password: a.obj.Password}, nil
	case "apikey":
		return &Auth{Type: "apikey", Key: a.obj.Key, Value: a.obj.Value, Placement: a.obj.Placement}, nil
	default:
		return nil, fmt.Errorf("auth type %q is not supported", a.obj.Type)
	}
}

type ycHTTP struct {
	Method  string       `yaml:"method"`
	URL     string       `yaml:"url"`
	Headers []ycHeader   `yaml:"headers"`
	Params  []ycParam    `yaml:"params"`
	Body    *ycBodyField `yaml:"body"`
	Auth    *ycAuth      `yaml:"auth"`
}

type ycSettings struct {
	Timeout yaml.Node `yaml:"timeout"`
}

type ycRequestFile struct {
	Info struct {
		Name string `yaml:"name"`
		Type string `yaml:"type"`
		Seq  int    `yaml:"seq"`
	} `yaml:"info"`
	HTTP     ycHTTP     `yaml:"http"`
	Settings ycSettings `yaml:"settings"`
}

func parseYAMLRequest(path string) (*Request, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rf ycRequestFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if rf.Info.Type != "" && rf.Info.Type != "http" {
		return nil, fmt.Errorf("%s: unsupported request type %q", path, rf.Info.Type)
	}

	req := &Request{
		Path:      path,
		Name:      rf.Info.Name,
		Seq:       rf.Info.Seq,
		Method:    rf.HTTP.Method,
		URL:       rf.HTTP.URL,
		TimeoutMS: -1,
	}

	for _, h := range rf.HTTP.Headers {
		if h.Disabled {
			continue
		}
		req.Headers = append(req.Headers, KV{K: h.Name, V: h.Value})
	}
	for _, p := range rf.HTTP.Params {
		if p.Disabled {
			continue
		}
		kv := KV{K: p.Name, V: p.Value}
		if p.Type == "path" {
			req.PathParams = append(req.PathParams, kv)
		} else {
			req.Query = append(req.Query, kv)
		}
	}

	body, err := decodeYAMLBody(rf.HTTP.Body.resolve())
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	req.Body = body

	auth, err := rf.HTTP.Auth.toAuth()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	req.Auth = auth

	if t, ok := timeoutMSFromNode(rf.Settings.Timeout); ok {
		req.TimeoutMS = t
	}

	return req, nil
}

func timeoutMSFromNode(n yaml.Node) (int, bool) {
	if n.Kind == 0 {
		return 0, false
	}
	if n.Tag == "!!str" {
		return 0, false
	}
	var ms int
	if err := n.Decode(&ms); err != nil {
		return 0, false
	}
	return ms, true
}

func decodeYAMLBody(b *ycBody) (Body, error) {
	if b == nil || b.Type == "" {
		return Body{}, nil
	}
	switch b.Type {
	case "json", "text", "xml", "sparql":
		var s string
		if err := b.Data.Decode(&s); err != nil {
			return Body{}, err
		}
		return Body{Kind: b.Type, Raw: s}, nil

	case "form-urlencoded":
		var fields []ycFormField
		if err := b.Data.Decode(&fields); err != nil {
			return Body{}, err
		}
		var parts []Part
		for _, f := range fields {
			if f.Disabled {
				continue
			}
			parts = append(parts, Part{Name: f.Name, Value: f.Value})
		}
		return Body{Kind: "form-urlencoded", Parts: parts}, nil

	case "multipart-form":
		var fields []ycMultipartField
		if err := b.Data.Decode(&fields); err != nil {
			return Body{}, err
		}
		var parts []Part
		for _, f := range fields {
			if f.Disabled {
				continue
			}
			val := ""
			if len(f.Value) > 0 {
				val = f.Value[0]
			}
			parts = append(parts, Part{Name: f.Name, IsFile: f.Type == "file", Value: val, ContentType: f.ContentType})
		}
		return Body{Kind: "multipart-form", Parts: parts}, nil

	case "file":
		var variants []ycFileVariant
		if err := b.Data.Decode(&variants); err != nil {
			return Body{}, err
		}
		for _, v := range variants {
			if v.Selected {
				return Body{Kind: "file", File: v.FilePath}, nil
			}
		}
		if len(variants) > 0 {
			return Body{Kind: "file", File: variants[0].FilePath}, nil
		}
		return Body{Kind: "file"}, nil

	default:
		return Body{}, fmt.Errorf("unsupported body type %q", b.Type)
	}
}

type yamlVariable struct {
	Name     string    `yaml:"name"`
	Value    yaml.Node `yaml:"value"`
	Secret   bool      `yaml:"secret"`
	Disabled bool      `yaml:"disabled"`
}

func (v yamlVariable) resolveValue() (string, error) {
	n := v.Value
	if n.Kind == 0 {
		return "", nil
	}
	switch n.Kind {
	case yaml.ScalarNode:
		return n.Value, nil
	case yaml.MappingNode:
		var typed struct {
			Data string `yaml:"data"`
		}
		if err := n.Decode(&typed); err != nil {
			return "", err
		}
		return typed.Data, nil
	case yaml.SequenceNode:
		var variants []struct {
			Selected bool      `yaml:"selected"`
			Value    yaml.Node `yaml:"value"`
		}
		if err := n.Decode(&variants); err != nil {
			return "", err
		}
		for _, variant := range variants {
			if variant.Selected {
				vv := variant
				return vv.Value.Value, nil
			}
		}
		if len(variants) > 0 {
			return variants[0].Value.Value, nil
		}
		return "", nil
	}
	return "", nil
}

func atoiOrZero(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
