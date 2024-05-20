package connect

import (
	"context"
	"fmt"
	"path"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
)

func (a *api) GetRepositoriesByFullName(
	ctx context.Context,
	req *connect.Request[registry.GetRepositoriesByFullNameRequest],
) (
	*connect.Response[registry.GetRepositoriesByFullNameResponse],
	error,
) {
	repositories, err := a.resolveReposByFullNames(ctx, req.Msg.GetFullNames())
	if err != nil {
		return nil, fmt.Errorf("getting repositories: %w", err)
	}

	return &connect.Response[registry.GetRepositoriesByFullNameResponse]{
		Msg: &registry.GetRepositoriesByFullNameResponse{Repositories: repositories},
	}, nil
}

func (a *api) GetRepositoryByFullName(
	ctx context.Context,
	req *connect.Request[registry.GetRepositoryByFullNameRequest],
) (
	*connect.Response[registry.GetRepositoryByFullNameResponse],
	error,
) {
	repository, err := a.resolveRepoByFullName(ctx, req.Msg.GetFullName())
	if err != nil {
		return nil, fmt.Errorf("getting repositories: %w", err)
	}

	return &connect.Response[registry.GetRepositoryByFullNameResponse]{
		Msg: &registry.GetRepositoryByFullNameResponse{Repository: repository}, //nolint:exhaustruct
	}, nil
}

func (a *api) resolveReposByFullNames(ctx context.Context, in []string) ([]*registry.Repository, error) {
	out := make([]*registry.Repository, 0, len(in))

	for i, name := range in {
		v, err := a.resolveRepoByFullName(ctx, name)
		if err != nil {
			return out, fmt.Errorf("iterating %d of %d: %w", i, len(in), err)
		}

		out = append(out, v)
	}

	return out, nil
}

func (a *api) resolveRepoByFullName(ctx context.Context, name string) (*registry.Repository, error) {
	owner, repositoryName := splitRepoName(name)

	repo, err := a.repo.GetMeta(ctx, owner, repositoryName, "")
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", name, err)
	}

	//nolint:godox,exhaustruct
	return &registry.Repository{
		Id:            path.Join(a.domain, owner, repositoryName),
		CreateTime:    timestamppb.New(repo.CreatedAt),
		UpdateTime:    timestamppb.New(repo.UpdatedAt),
		Name:          repositoryName,
		Owner:         &registry.Repository_UserId{UserId: owner},
		Visibility:    registry.Visibility_VISIBILITY_PUBLIC,
		OwnerName:     owner,
		Description:   "", // TODO
		Url:           path.Join(a.domain, owner, repositoryName),
		DefaultBranch: repo.DefaultBranch,
	}, nil
}

func splitRepoName(name string) (string, string) {
	fields := strings.Split(name, "/")

	return fields[0], fields[1]
}
