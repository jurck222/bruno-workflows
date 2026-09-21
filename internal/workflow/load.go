package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thecalda/brw/internal/collection"
	"gopkg.in/yaml.v3"
)

type LoadOptions struct {
	CollectionDir string
	Env           string
	Vars          map[string]string
	Yes           bool
}

func Load(path string, opts LoadOptions) (*Workflow, *collection.Collection, []error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, []error{err}
	}
	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, nil, []error{fmt.Errorf("%s: %w", path, err)}
	}
	wf.Path = path

	var problems []error
	if wf.Name == "" {
		problems = append(problems, fmt.Errorf("%s: missing required field 'name'", path))
	}
	if len(wf.Steps) == 0 {
		problems = append(problems, fmt.Errorf("%s: 'steps' must be non-empty", path))
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return &wf, nil, append(problems, err)
	}

	collDir := opts.CollectionDir
	if collDir == "" {
		collDir = wf.Collection
	}
	if collDir != "" && !filepath.IsAbs(collDir) {
		collDir = filepath.Join(filepath.Dir(absPath), collDir)
	}
	if collDir == "" {
		collDir = filepath.Dir(absPath)
	}

	col, err := collection.Open(collDir)
	if err != nil {
		return &wf, nil, append(problems, err)
	}

	envName := opts.Env
	if envName == "" {
		envName = wf.Env
	}
	envVars := map[string]string{}
	if envName != "" {
		v, err := col.Env(envName)
		if err != nil {
			problems = append(problems, fmt.Errorf("environment %q: %w", envName, err))
		} else {
			envVars = v
		}
	}

	known := map[string]bool{}
	known["process.env"] = true
	for k := range wf.Vars {
		known[k] = true
	}
	for k := range envVars {
		known[k] = true
	}
	for k := range opts.Vars {
		known[k] = true
	}

	for i := range wf.Steps {
		step := &wf.Steps[i]
		stepLabel := fmt.Sprintf("step %d %q", i+1, stepName(step))

		req, err := col.Load(step.Request)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: request %q: %w", stepLabel, step.Request, err))
		}

		for capVar, expr := range step.Capture {
			if err := validateCaptureExpr(expr); err != nil {
				problems = append(problems, fmt.Errorf("%s: capture %s: %w", stepLabel, capVar, err))
			}
		}

		for _, p := range step.Prompt {
			if p.Var == "" || p.Message == "" {
				problems = append(problems, fmt.Errorf("%s: prompt entries require 'var' and 'message'", stepLabel))
			}
		}

		if step.Expect != nil {
			if err := validateExpectStatus(step.Expect.Status); err != nil {
				problems = append(problems, fmt.Errorf("%s: expect.status: %w", stepLabel, err))
			}
		}

		hasPrompt := len(step.Prompt) > 0
		if opts.Yes && hasPrompt {
			for _, p := range step.Prompt {
				if _, ok := opts.Vars[p.Var]; !ok && !p.Optional {
					problems = append(problems, fmt.Errorf("%s: --yes cannot auto-answer prompt %q; supply --var %s=…", stepLabel, p.Var, p.Var))
				}
			}
		}

		if req != nil && !step.Skip {
			for _, name := range varNamesIn(req) {
				if strings.HasPrefix(name, "process.env.") {
					continue
				}
				if !known[name] {
					problems = append(problems, fmt.Errorf("%s: {{%s}} is never defined", stepLabel, name))
				}
			}
		}

		for k := range step.Capture {
			known[k] = true
		}
		for _, p := range step.Prompt {
			known[p.Var] = true
		}
	}

	return &wf, col, problems
}

func stepName(s *Step) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Request
}

func validateExpectStatus(s ExpectStatus) error {
	if len(s.Ints) == 0 && s.Class == "" {
		return fmt.Errorf("must be an int, a list of ints, or \"1xx\".. \"5xx\"")
	}
	if s.Class != "" {
		if matched, _ := regexp.MatchString(`^[1-5]xx$`, s.Class); !matched {
			return fmt.Errorf("invalid status class %q", s.Class)
		}
	}
	return nil
}

func validateCaptureExpr(expr string) error {
	switch {
	case expr == "status", expr == "body":
		return nil
	case strings.HasPrefix(expr, "header."):
		if strings.TrimPrefix(expr, "header.") == "" {
			return fmt.Errorf("invalid capture expression %q: missing header name", expr)
		}
		return nil
	case strings.HasPrefix(expr, "body."):
		if strings.TrimPrefix(expr, "body.") == "" {
			return fmt.Errorf("invalid capture expression %q: missing path", expr)
		}
		return nil
	default:
		return fmt.Errorf("invalid capture expression %q", expr)
	}
}

var varRefRe = regexp.MustCompile(`\{\{\s*([^{}\s]+)\s*\}\}`)

func varNamesIn(req *collection.Request) []string {
	var all []string
	add := func(s string) {
		for _, m := range varRefRe.FindAllStringSubmatch(s, -1) {
			all = append(all, m[1])
		}
	}
	add(req.URL)
	for _, kv := range req.Query {
		add(kv.V)
	}
	for _, kv := range req.PathParams {
		add(kv.V)
	}
	for _, kv := range req.Headers {
		add(kv.V)
	}
	add(req.Body.Raw)
	for _, p := range req.Body.Parts {
		add(p.Value)
	}
	if req.Auth != nil {
		add(req.Auth.Token)
		add(req.Auth.Username)
		add(req.Auth.Password)
		add(req.Auth.Value)
	}
	return all
}
