package runner

import (
	"bytes"
	"context"
	"os"

	"github.com/dimiro1/banner"
)

type State int

const (
	StateNew State = iota
	StateStarting
	StateRunning
	StateDraining
	StateStopped
)

type Runner interface {
	Run(ctx context.Context) error
	Stop() error
	State() State
}

type Hooks struct {
	OnStart func()
	OnStop  func()
}

type Drainer interface {
	Drain() error
}

const EngineVersion = "dev"

func PrintBanner() {
	tpl := "{{ .Title \"RANYA\" \"\" 0 }}\nVersion: " + EngineVersion + "\n"
	banner.Init(os.Stdout, true, true, bytes.NewBufferString(tpl))
}
