package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/thecalda/brw/internal/collection"
	"github.com/thecalda/brw/internal/workflow"
)

type Options struct {
	Env             string
	Vars            map[string]string
	Yes             bool
	DryRun          bool
	Verbose         bool
	Insecure        bool
	ContinueOnError bool
	Format          string
	From, To        int
	Only            string
	Out             io.Writer
	In              io.Reader
}

type StepResult struct {
	N          int               `json:"n"`
	Name       string            `json:"name"`
	Request    string            `json:"request"`
	Method     string            `json:"method,omitempty"`
	URL        string            `json:"url,omitempty"`
	Status     int               `json:"status,omitempty"`
	DurationMS int64             `json:"duration_ms,omitempty"`
	Captured   map[string]string `json:"captured,omitempty"`
	Error      string            `json:"error,omitempty"`
	Skipped    bool              `json:"skipped,omitempty"`
	Body       string            `json:"body,omitempty"`
}

type Result struct {
	Name    string       `json:"name"`
	Env     string       `json:"env,omitempty"`
	Format  string       `json:"format"`
	Steps   []StepResult `json:"steps"`
	Success bool         `json:"-"`
	Aborted bool         `json:"-"`
}

var secretNameRe = regexp.MustCompile(`(?i)token|secret|password|key|auth`)

func selectedSteps(n int, opts Options) (map[int]bool, error) {
	sel := map[int]bool{}
	if opts.Only != "" {
		for _, tok := range strings.Split(opts.Only, ",") {
			i, err := strconv.Atoi(strings.TrimSpace(tok))
			if err != nil {
				return nil, fmt.Errorf("--only: invalid step %q", tok)
			}
			sel[i] = true
		}
		return sel, nil
	}
	from, to := 1, n
	if opts.From > 0 {
		from = opts.From
	}
	if opts.To > 0 {
		to = opts.To
	}
	for i := from; i <= to; i++ {
		sel[i] = true
	}
	return sel, nil
}

func Run(wf *workflow.Workflow, col *collection.Collection, opts Options) (Result, error) {
	sel, err := selectedSteps(len(wf.Steps), opts)
	if err != nil {
		return Result{}, err
	}

	envName := opts.Env
	if envName == "" {
		envName = wf.Env
	}
	envVars := map[string]string{}
	if envName != "" {
		v, err := col.Env(envName)
		if err != nil {
			return Result{}, err
		}
		envVars = v
	}

	runVars := map[string]string{}
	for k, v := range envVars {
		runVars[k] = v
	}
	for k, v := range wf.Vars {
		runVars[k] = v
	}
	for k, v := range opts.Vars {
		runVars[k] = v
	}

	dotenv := LoadDotenv(col.Root)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if opts.Insecure {
		client.Transport = insecureTransport()
	}

	out := opts.Out
	format := opts.Format
	if format == "" {
		format = "human"
	}
	format2 := "bru"
	if col.Format == collection.FormatYAML {
		format2 = "yml"
	}

	if format == "human" {
		fmt.Fprintf(out, "▸ %s   env=%s   collection=%s [%s]\n\n", wf.Name, envName, filepath.Base(col.Root), format2)
	}

	result := Result{Name: wf.Name, Env: envName, Format: format2, Success: true}
	runStart := time.Now()

	for i := range wf.Steps {
		n := i + 1
		step := &wf.Steps[i]
		name := step.Name
		if name == "" {
			name = step.Request
		}
		label := fmt.Sprintf("step %d %q", n, name)

		if !sel[n] || step.Skip {
			if format == "human" {
				fmt.Fprintf(out, "  %d/%d  %-40s SKIPPED\n", n, len(wf.Steps), name)
			}
			result.Steps = append(result.Steps, StepResult{N: n, Name: name, Request: step.Request, Skipped: true})
			continue
		}

		req, err := col.Load(step.Request)
		if err != nil {
			return result, fmt.Errorf("%s: %w", label, err)
		}

		stepVars := map[string]string{}
		for k, v := range runVars {
			stepVars[k] = v
		}
		for k, v := range step.Vars {
			stepVars[k] = v
		}

		httpReq, sendBody, err := buildRequest(req, stepVars, dotenv)
		if err != nil {
			result.Success = false
			result.Steps = append(result.Steps, StepResult{N: n, Name: name, Request: step.Request, Error: err.Error()})
			if format == "human" {
				fmt.Fprintf(out, "  %d/%d  %-40s FAILED\n       %s\n", n, len(wf.Steps), name, err)
			}
			if !opts.ContinueOnError {
				return result, fmt.Errorf("%s: %w", label, err)
			}
			continue
		}

		if opts.DryRun {
			if format == "human" {
				printDryRun(out, httpReq, sendBody)
			}
			result.Steps = append(result.Steps, StepResult{N: n, Name: name, Request: step.Request, Method: httpReq.Method, URL: httpReq.URL.String(), Body: sendBody})
			continue
		}

		timeout := 30 * time.Second
		if req.TimeoutMS > 0 {
			timeout = time.Duration(req.TimeoutMS) * time.Millisecond
		}
		client.Timeout = timeout

		start := time.Now()
		httpResp, err := client.Do(httpReq)
		dur := time.Since(start)
		if err != nil {
			result.Success = false
			result.Steps = append(result.Steps, StepResult{N: n, Name: name, Request: step.Request, Method: httpReq.Method, URL: httpReq.URL.String(), Error: err.Error()})
			if format == "human" {
				fmt.Fprintf(out, "  %d/%d  %-40s ERROR\n       %s\n", n, len(wf.Steps), name, err)
			}
			if !opts.ContinueOnError {
				return result, fmt.Errorf("%s: %w", label, err)
			}
			continue
		}
		bodyBytes, _ := io.ReadAll(httpResp.Body)
		httpResp.Body.Close()
		resp := &Response{Status: httpResp.StatusCode, Header: httpResp.Header, Body: string(bodyBytes)}

		sr := StepResult{N: n, Name: name, Request: step.Request, Method: httpReq.Method, URL: httpReq.URL.String(), Status: resp.Status, DurationMS: dur.Milliseconds()}

		if format == "human" {
			printHumanStepHeader(out, n, len(wf.Steps), name, httpReq.Method, resp.Status, dur)
		}

		stepErr := ""
		if step.Expect != nil {
			if !statusMatches(step.Expect.Status, resp.Status) {
				stepErr = fmt.Sprintf("expected status %s, got %d", describeExpect(step.Expect.Status), resp.Status)
			}
		}

		showBody := step.Pause != "" || stepErr != "" || opts.Verbose || len(resp.Body) <= 400
		if format == "human" && showBody && resp.Body != "" {
			printBody(out, resp.Body, len(resp.Body) > 400 && !opts.Verbose && stepErr == "" && step.Pause == "")
		}

		captured := map[string]string{}
		if stepErr == "" {
			for capVar, expr := range step.Capture {
				val, err := Capture(expr, resp)
				if err != nil {
					stepErr = fmt.Sprintf("capture %s: %s: %v", capVar, expr, err)
					break
				}
				captured[capVar] = val
				runVars[capVar] = val
				if format == "human" {
					fmt.Fprintf(out, "       captured %s = %s\n", capVar, displayValue(capVar, val))
				}
			}
		}

		if stepErr != "" {
			sr.Error = stepErr
			result.Success = false
			if format == "human" {
				fmt.Fprintf(out, "       %s\n", stepErr)
			}
		} else {
			sr.Captured = captured
		}
		if opts.Verbose {
			sr.Body = resp.Body
		}
		result.Steps = append(result.Steps, sr)

		if stepErr == "" && (step.Pause != "" || len(step.Prompt) > 0) {
			aborted, err := runPause(step, opts, out, runVars)
			if aborted {
				result.Aborted = true
				fmt.Fprintf(out, "\n✗ aborted at step %d/%d %q after %s\n", n, len(wf.Steps), name, time.Since(runStart).Round(time.Millisecond))
				return result, nil
			}
			if err != nil {
				return result, err
			}
		}

		if stepErr != "" {
			if !opts.ContinueOnError {
				fmt.Fprintf(out, "\n✗ failed at step %d/%d %q after %s\n", n, len(wf.Steps), name, time.Since(runStart).Round(time.Millisecond))
				if format == "json" {
					printJSONResult(out, result)
				}
				return result, nil
			}
		}
	}

	if format == "json" {
		printJSONResult(out, result)
	} else if result.Success {
		fmt.Fprintf(out, "\n✓ done in %s\n", time.Since(runStart).Round(time.Millisecond))
	}

	return result, nil
}

func printJSONResult(out io.Writer, r Result) {
	data, _ := json.MarshalIndent(r, "", "  ")
	out.Write(data)
	fmt.Fprintln(out)
}

func displayValue(name, val string) string {
	if secretNameRe.MatchString(name) && len(val) > 12 {
		return val[:12] + "…"
	}
	return val
}

func printHumanStepHeader(out io.Writer, n, total int, name, method string, status int, dur time.Duration) {
	fmt.Fprintf(out, "  %d/%d  %-40s %-4s %d  %dms\n", n, total, name, method, status, dur.Milliseconds())
}

func printBody(out io.Writer, body string, truncate bool) {
	if truncate {
		fmt.Fprintf(out, "       (body: %.1f KB, use --verbose)\n", float64(len(body))/1024)
		return
	}
	var v any
	if json.Unmarshal([]byte(body), &v) == nil {
		var buf bytes.Buffer
		if json.Indent(&buf, []byte(body), "       ", "  ") == nil {
			fmt.Fprintf(out, "       %s\n", buf.String())
			return
		}
	}
	fmt.Fprintf(out, "       %s\n", body)
}

func printDryRun(out io.Writer, req *http.Request, body string) {
	fmt.Fprintf(out, "%s %s\n", req.Method, req.URL.String())
	for k, vs := range req.Header {
		for _, v := range vs {
			fmt.Fprintf(out, "  %s: %s\n", k, v)
		}
	}
	if body != "" {
		fmt.Fprintf(out, "\n%s\n", body)
	}
}

func statusMatches(exp workflow.ExpectStatus, got int) bool {
	if exp.Class != "" {
		return got/100 == int(exp.Class[0]-'0')
	}
	for _, s := range exp.Ints {
		if s == got {
			return true
		}
	}
	return false
}

func describeExpect(exp workflow.ExpectStatus) string {
	if exp.Class != "" {
		return exp.Class
	}
	parts := make([]string, len(exp.Ints))
	for i, s := range exp.Ints {
		parts[i] = strconv.Itoa(s)
	}
	return strings.Join(parts, ",")
}

func buildRequest(req *collection.Request, vars map[string]string, dotenv map[string]string) (*http.Request, string, error) {
	interp := func(s string) (string, error) { return Interpolate(s, vars, dotenv) }

	url, err := interp(req.URL)
	if err != nil {
		return nil, "", err
	}
	for _, kv := range req.PathParams {
		v, err := interp(kv.V)
		if err != nil {
			return nil, "", err
		}
		url = strings.ReplaceAll(url, ":"+kv.K, v)
	}

	u, err := addQuery(url, req.Query, interp)
	if err != nil {
		return nil, "", err
	}

	var bodyReader io.Reader
	var bodyStr string
	contentType := ""

	switch req.Body.Kind {
	case "json":
		s, err := interp(req.Body.Raw)
		if err != nil {
			return nil, "", err
		}
		bodyReader, bodyStr, contentType = strings.NewReader(s), s, "application/json"
	case "text":
		s, err := interp(req.Body.Raw)
		if err != nil {
			return nil, "", err
		}
		bodyReader, bodyStr, contentType = strings.NewReader(s), s, "text/plain"
	case "xml":
		s, err := interp(req.Body.Raw)
		if err != nil {
			return nil, "", err
		}
		bodyReader, bodyStr, contentType = strings.NewReader(s), s, "application/xml"
	case "form-urlencoded":
		vals := make([]string, 0, len(req.Body.Parts))
		for _, p := range req.Body.Parts {
			v, err := interp(p.Value)
			if err != nil {
				return nil, "", err
			}
			vals = append(vals, p.Name+"="+v)
		}
		s := strings.Join(vals, "&")
		bodyReader, bodyStr, contentType = strings.NewReader(s), s, "application/x-www-form-urlencoded"
	case "multipart-form":
		buf := &bytes.Buffer{}
		mw := multipart.NewWriter(buf)
		for _, p := range req.Body.Parts {
			v, err := interp(p.Value)
			if err != nil {
				return nil, "", err
			}
			if p.IsFile {
				path := v
				if !filepath.IsAbs(path) {
					path = filepath.Join(filepath.Dir(req.Path), path)
				}
				f, err := os.Open(path)
				if err != nil {
					return nil, "", fmt.Errorf("multipart field %s: %w", p.Name, err)
				}
				fw, err := mw.CreateFormFile(p.Name, filepath.Base(path))
				if err != nil {
					f.Close()
					return nil, "", err
				}
				if _, err := io.Copy(fw, f); err != nil {
					f.Close()
					return nil, "", err
				}
				f.Close()
			} else {
				if err := mw.WriteField(p.Name, v); err != nil {
					return nil, "", err
				}
			}
		}
		mw.Close()
		bodyReader, bodyStr, contentType = buf, "(multipart)", mw.FormDataContentType()
	case "file":
		path := req.Body.File
		if !filepath.IsAbs(path) {
			path = filepath.Join(filepath.Dir(req.Path), path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", err
		}
		bodyReader, bodyStr = bytes.NewReader(data), fmt.Sprintf("(file: %s)", path)
	}

	httpReq, err := http.NewRequest(req.Method, u, bodyReader)
	if err != nil {
		return nil, "", err
	}
	if contentType != "" {
		httpReq.Header.Set("Content-Type", contentType)
	}
	for _, kv := range req.Headers {
		v, err := interp(kv.V)
		if err != nil {
			return nil, "", err
		}
		httpReq.Header.Set(kv.K, v)
	}

	if err := applyAuth(httpReq, req.Auth, interp); err != nil {
		return nil, "", err
	}

	return httpReq, bodyStr, nil
}

func addQuery(rawURL string, query []collection.KV, interp func(string) (string, error)) (string, error) {
	if len(query) == 0 {
		return rawURL, nil
	}
	base, qs, hasQuery := strings.Cut(rawURL, "?")
	values := map[string][]string{}
	var order []string
	if hasQuery {
		for _, pair := range strings.Split(qs, "&") {
			if pair == "" {
				continue
			}
			k, v, _ := strings.Cut(pair, "=")
			if _, seen := values[k]; !seen {
				order = append(order, k)
			}
			values[k] = append(values[k], v)
		}
	}
	for _, kv := range query {
		v, err := interp(kv.V)
		if err != nil {
			return "", err
		}
		if _, seen := values[kv.K]; !seen {
			order = append(order, kv.K)
		}
		values[kv.K] = []string{v}
	}
	var parts []string
	for _, k := range order {
		for _, v := range values[k] {
			parts = append(parts, k+"="+v)
		}
	}
	if len(parts) == 0 {
		return base, nil
	}
	return base + "?" + strings.Join(parts, "&"), nil
}

func applyAuth(req *http.Request, auth *collection.Auth, interp func(string) (string, error)) error {
	if auth == nil || req.Header.Get("Authorization") != "" {
		return nil
	}
	switch auth.Type {
	case "bearer":
		tok, err := interp(auth.Token)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
	case "basic":
		u, err := interp(auth.Username)
		if err != nil {
			return err
		}
		p, err := interp(auth.Password)
		if err != nil {
			return err
		}
		req.SetBasicAuth(u, p)
	case "apikey":
		k, err := interp(auth.Key)
		if err != nil {
			return err
		}
		v, err := interp(auth.Value)
		if err != nil {
			return err
		}
		if auth.Placement == "query" {
			q := req.URL.Query()
			q.Set(k, v)
			req.URL.RawQuery = q.Encode()
		} else {
			req.Header.Set(k, v)
		}
	}
	return nil
}
