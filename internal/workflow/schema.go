package workflow

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Expect struct {
	Status ExpectStatus `yaml:"status"`
}

type ExpectStatus struct {
	Ints  []int
	Class string
}

func (e *ExpectStatus) UnmarshalYAML(node *yaml.Node) error {
	switch node.Tag {
	case "!!int":
		var v int
		if err := node.Decode(&v); err != nil {
			return err
		}
		e.Ints = []int{v}
		return nil
	case "!!seq":
		return node.Decode(&e.Ints)
	case "!!str":
		e.Class = node.Value
		return nil
	default:
		return fmt.Errorf("expect.status: unsupported value %q", node.Value)
	}
}

type Prompt struct {
	Var      string `yaml:"var"`
	Message  string `yaml:"message"`
	Optional bool   `yaml:"optional"`
}

type Step struct {
	Request string            `yaml:"request"`
	Name    string            `yaml:"name"`
	Capture map[string]string `yaml:"capture"`
	Expect  *Expect           `yaml:"expect"`
	Vars    map[string]string `yaml:"vars"`
	Pause   string            `yaml:"pause"`
	Prompt  []Prompt          `yaml:"prompt"`
	Skip    bool              `yaml:"skip"`
}

type Workflow struct {
	Name       string            `yaml:"name"`
	Collection string            `yaml:"collection"`
	Env        string            `yaml:"env"`
	Vars       map[string]string `yaml:"vars"`
	Steps      []Step            `yaml:"steps"`

	Path string `yaml:"-"`
}
