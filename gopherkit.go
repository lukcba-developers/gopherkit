package gopherkit

import (
	"fmt"
)

type Kit struct {
	Name    string
	Version string
}

func New(name string) *Kit {
	return &Kit{
		Name:    name,
		Version: "v0.1.0",
	}
}

func (k *Kit) Greet() string {
	return fmt.Sprintf("Hello from %s!", k.Name)
}

func (k *Kit) GetInfo() string {
	return fmt.Sprintf("GopherKit: %s (version %s)", k.Name, k.Version)
}