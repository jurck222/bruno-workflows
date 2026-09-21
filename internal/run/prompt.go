package run

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/thecalda/brw/internal/workflow"
)

func insecureTransport() http.RoundTripper {
	return &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
}

func runPause(step *workflow.Step, opts Options, out io.Writer, vars map[string]string) (aborted bool, err error) {
	reader := bufio.NewReader(opts.In)

	if step.Pause != "" {
		fmt.Fprintln(out, strings.Repeat("─", 58))
		fmt.Fprintln(out, "  MANUAL STEP")
		fmt.Fprintf(out, "  %s\n", strings.TrimSpace(step.Pause))
		fmt.Fprintln(out, strings.Repeat("─", 58))
	}

	for _, p := range step.Prompt {
		if v, ok := opts.Vars[p.Var]; ok {
			vars[p.Var] = v
			continue
		}
		fmt.Fprintf(out, "%s: ", p.Message)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" && !p.Optional {

			vars[p.Var] = ""
			continue
		}
		vars[p.Var] = line
	}

	if opts.Yes {
		fmt.Fprintln(out, "(auto-continued)")
		return false, nil
	}

	fmt.Fprint(out, "Press Enter to continue (q to abort) ▸ ")
	line, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(line)) == "q" {
		return true, nil
	}
	return false, nil
}
