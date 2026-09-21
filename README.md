# brw — Bruno Workflow Runner

A single-binary CLI that runs a sequence of existing Bruno requests as one workflow,
passing values between them (JWT from login → every later request) and pausing for
manual out-of-band steps (e.g. "go read the SMS code").

It does not replace Bruno. The request files stay the source of truth and stay usable
in the Bruno GUI. The workflow file is a separate, additive artifact. Both Bruno
collection formats are supported: legacy `.bru` and the newer OpenCollection YAML.

## Install

```
go install ./...
```

or just `go run .` from this directory.

## Usage

```
brw run [workflow.yaml]     Run a workflow. Defaults to ./workflow.yaml.
  --env NAME                Environment (overrides the file's `env:`)
  --var K=V                 Set a variable; repeatable
  --collection DIR          Collection root (overrides the file's `collection:`)
  --from N / --to N         1-based step range
  --only N[,M…]             Run just these steps
  --yes                     Auto-continue pauses; error if a prompt is unanswerable
  --dry-run                 Resolve and print requests, send nothing
  --verbose                 Full requests and responses
  --continue-on-error       Keep going after a failed step (still exits 1)
  --insecure                Skip TLS verification
  --format human|json       Default human

brw check [workflow.yaml]   Load + validate only. Exit 0/1. No network.
brw ls [workflow.yaml]      Print steps with resolved paths and captures.
brw envs                    List the collection's environments.
brw info                    Print the collection root, detected format, and request count.
```

Exit codes: `0` ok, `1` step or validation failure, `2` usage error, `130` user aborted.

## Writing a workflow

```yaml
name: Mediation happy path
collection: ../my-collection   # optional; relative to the workflow file
env: Local

steps:
  - request: Auth/Login             # collection-relative path, extension optional
    name: Log in as claimant
    expect: { status: 200 }
    capture:
      AUTH_TOKEN: body.access_token # status | header.<Name> | body | body.<path>

  - request: Auth/Verify user phone
    pause: |
      Check the SMS inbox for the code.
    prompt:
      - var: SMS_CODE
        message: Enter the code from the SMS
```

### Request bodies become templates

Adopting this tool means editing a hardcoded value in a request body to a
`{{VARIABLE}}` placeholder — which stays valid in the Bruno GUI (it just resolves to
whatever that environment/variable currently holds there). The workflow file supplies
the value for headless runs. Existing `script:post-response` / `runtime.scripts`
blocks can stay as-is; Bruno's own GUI still runs them, this tool just ignores them
and expects the workflow's `capture:` to mirror what they'd have set.
