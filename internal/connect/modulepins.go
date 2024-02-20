package connect

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	module "github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
)

func (a *api) GetModulePins(
	_ context.Context,
	req *connect.Request[registry.GetModulePinsRequest],
) (
	*connect.Response[registry.GetModulePinsResponse],
	error,
) {
	modulePins, err := a.resolveModulePins(req.Msg.GetModuleReferences())
	if err != nil {
		return nil, fmt.Errorf("getting repository: %w", err)
	}

	return &connect.Response[registry.GetModulePinsResponse]{
		Msg: &registry.GetModulePinsResponse{ModulePins: modulePins},
	}, nil
}

func (a *api) resolveModulePins(in []*module.ModuleReference) ([]*module.ModulePin, error) {
	out := make([]*module.ModulePin, 0, len(in))

	for i, m := range in {
		v, err := a.resolveModulePin(m)
		if err != nil {
			return out, fmt.Errorf("iterating %d of %d: %w", i, len(in), err)
		}

		out = append(out, v)
	}

	return out, nil
}

func (a *api) resolveModulePin(v *module.ModuleReference) (*module.ModulePin, error) {
	repo, err := a.repo.Get(v.GetOwner(), v.GetRepository(), v.GetReference())
	if err != nil {
		return nil, fmt.Errorf("resolving %q/%q:%q: %w", v.GetOwner(), v.GetRepository(), v.GetReference(), err)
	}

	return &module.ModulePin{ //nolint:exhaustruct
		Remote:     a.domain,
		Owner:      v.GetOwner(),
		Repository: v.GetRepository(),
		Commit:     repo.Commit,
	}, nil
}
