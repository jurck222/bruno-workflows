package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/thecalda/brw/internal/collection"
	"github.com/thecalda/brw/internal/run"
	"github.com/thecalda/brw/internal/workflow"
)

func main() {
	os.Exit(mainErr())
}

func mainErr() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "run":
		return cmdRun(args)
	case "check":
		return cmdCheck(args)
	case "ls":
		return cmdLs(args)
	case "envs":
		return cmdEnvs(args)
	case "info":
		return cmdInfo(args)
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "brw: unknown command %q\n", cmd)
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `brw — Bruno workflow runner

Usage:
  brw run [workflow.yaml]     Run a workflow. Defaults to ./workflow.yaml.
  brw check [workflow.yaml]   Load + validate only. Exit 0/1. No network.
  brw ls [workflow.yaml]      Print steps with resolved paths and captures.
  brw envs                    List the collection's environments.
  brw info                    Print the collection root, detected format, and request count.`)
}

type varFlags map[string]string

func (v varFlags) String() string { return "" }
func (v varFlags) Set(s string) error {
	k, val, ok := strings.Cut(s, "=")
	if !ok {
		return fmt.Errorf("--var must be K=V, got %q", s)
	}
	v[k] = val
	return nil
}

func commonFlags(fs *flag.FlagSet) (*string, *string, varFlags) {
	env := fs.String("env", "", "environment (overrides the file's env:)")
	collectionDir := fs.String("collection", "", "collection root (overrides the file's collection:)")
	vars := varFlags{}
	fs.Var(vars, "var", "set a variable K=V; repeatable")
	return env, collectionDir, vars
}

func workflowPathArg(fs *flag.FlagSet) string {
	if fs.NArg() > 0 {
		return fs.Arg(0)
	}
	return "workflow.yaml"
}

func reorder(fs *flag.FlagSet, args []string) []string {
	isBool := func(name string) bool {
		f := fs.Lookup(name)
		if f == nil {
			return false
		}
		b, ok := f.Value.(interface{ IsBoolFlag() bool })
		return ok && b.IsBoolFlag()
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			continue
		}
		if !isBool(name) && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, positional...)
}

func cmdCheck(args []string) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	env, collectionDir, vars := commonFlags(fs)
	if err := fs.Parse(reorder(fs, args)); err != nil {
		return 2
	}
	_, _, problems := workflow.Load(workflowPathArg(fs), workflow.LoadOptions{
		Env: *env, CollectionDir: *collectionDir, Vars: vars,
	})
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "✗", p)
		}
		return 1
	}
	fmt.Println("ok")
	return 0
}

func cmdLs(args []string) int {
	fs := flag.NewFlagSet("ls", flag.ContinueOnError)
	env, collectionDir, vars := commonFlags(fs)
	if err := fs.Parse(reorder(fs, args)); err != nil {
		return 2
	}
	wf, _, problems := workflow.Load(workflowPathArg(fs), workflow.LoadOptions{
		Env: *env, CollectionDir: *collectionDir, Vars: vars,
	})
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "✗", p)
		}
		return 1
	}
	for i, s := range wf.Steps {
		name := s.Name
		if name == "" {
			name = s.Request
		}
		fmt.Printf("%2d  %-40s %s", i+1, name, s.Request)
		if s.Skip {
			fmt.Print("  SKIPPED")
		}
		fmt.Println()
		for capVar, expr := range s.Capture {
			fmt.Printf("      captures %s = %s\n", capVar, expr)
		}
	}
	return 0
}

func cmdEnvs(args []string) int {
	fs := flag.NewFlagSet("envs", flag.ContinueOnError)
	dir := fs.String("collection", ".", "collection root")
	if err := fs.Parse(reorder(fs, args)); err != nil {
		return 2
	}
	col, err := collection.Open(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		return 1
	}
	names, err := col.Envs()
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		return 1
	}
	for _, n := range names {
		fmt.Println(n)
	}
	return 0
}

func cmdInfo(args []string) int {
	fs := flag.NewFlagSet("info", flag.ContinueOnError)
	dir := fs.String("collection", ".", "collection root")
	if err := fs.Parse(reorder(fs, args)); err != nil {
		return 2
	}
	col, err := collection.Open(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		return 1
	}
	format := "bru"
	if col.Format == collection.FormatYAML {
		format = "yml"
	}
	fmt.Printf("root:   %s\n", col.Root)
	fmt.Printf("name:   %s\n", col.Name)
	fmt.Printf("format: %s\n", format)
	return 0
}

func cmdRun(args []string) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	env, collectionDir, vars := commonFlags(fs)
	yes := fs.Bool("yes", false, "auto-continue pauses")
	dryRun := fs.Bool("dry-run", false, "resolve and print requests, send nothing")
	verbose := fs.Bool("verbose", false, "full requests and responses")
	continueOnError := fs.Bool("continue-on-error", false, "keep going after a failed step")
	insecure := fs.Bool("insecure", false, "skip TLS verification")
	format := fs.String("format", "human", "output format: human|json")
	from := fs.Int("from", 0, "1-based first step to run")
	to := fs.Int("to", 0, "1-based last step to run")
	only := fs.String("only", "", "run just these steps, e.g. 2,4,5")
	if err := fs.Parse(reorder(fs, args)); err != nil {
		return 2
	}

	wf, col, problems := workflow.Load(workflowPathArg(fs), workflow.LoadOptions{
		Env: *env, CollectionDir: *collectionDir, Vars: vars, Yes: *yes,
	})
	if len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "✗", p)
		}
		return 1
	}

	result, err := run.Run(wf, col, run.Options{
		Env: *env, Vars: vars, Yes: *yes, DryRun: *dryRun, Verbose: *verbose,
		ContinueOnError: *continueOnError, Insecure: *insecure, Format: *format,
		From: *from, To: *to, Only: *only,
		Out: os.Stdout, In: os.Stdin,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
	}
	if result.Aborted {
		return 130
	}
	if !result.Success {
		return 1
	}
	return 0
}
