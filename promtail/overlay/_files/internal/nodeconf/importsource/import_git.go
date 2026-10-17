package importsource

import (
	"context"
	"errors"

	"github.com/grafana/alloy/internal/component"
	"github.com/grafana/alloy/syntax/vm"
)

// ErrGitUnsupported rejects import.git: go-git is not included in alloy-promtail.
var ErrGitUnsupported = errors.New("import.git is not supported by alloy-promtail")

// ImportGit is the import.git source.
type ImportGit struct{}

var _ ImportSource = (*ImportGit)(nil)

// NewImportGit returns an import.git source.
func NewImportGit(component.Options, *vm.Evaluator, func(map[string]string)) *ImportGit {
	return &ImportGit{}
}

// Evaluate implements ImportSource.
//
// Returns ErrGitUnsupported.
func (*ImportGit) Evaluate(*vm.Scope) error {
	return ErrGitUnsupported
}

// Run implements ImportSource.
func (*ImportGit) Run(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

// CurrentHealth implements ImportSource.
func (*ImportGit) CurrentHealth() component.Health {
	return component.Health{Health: component.HealthTypeUnhealthy, Message: ErrGitUnsupported.Error()}
}

// SetEval implements ImportSource.
func (*ImportGit) SetEval(*vm.Evaluator) {}

// ModulePath implements ImportSource.
func (*ImportGit) ModulePath() string {
	return ""
}

// InheritsModulePath implements ImportSource.
func (*ImportGit) InheritsModulePath() bool {
	return false
}
