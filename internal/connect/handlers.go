package connect

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"path"
	"path/filepath"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	module "github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	"github.com/easyp-tech/server/internal/content"
	"github.com/easyp-tech/server/internal/shake256"
)

const hashLen = 64

func (a *api) GetModulePins(
	_ context.Context,
	req *connect.Request[registry.GetModulePinsRequest],
) (
	*connect.Response[registry.GetModulePinsResponse],
	error,
) {
	modulePins, err := sliceMap(
		req.Msg.ModuleReferences,
		func(_ int, v *module.ModuleReference) (*module.ModulePin, error) {
			repo, err := a.repo.Get(v.Owner, v.Repository, v.Reference)
			if err != nil {
				return nil, fmt.Errorf("investigating %q/%q:%q: %w", v.Owner, v.Repository, v.Reference, err)
			}

			return &module.ModulePin{ //nolint:exhaustruct
				Remote:     a.domain,
				Owner:      v.Owner,
				Repository: v.Repository,
				Commit:     repo.Commit,
			}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("gewtting repository: %w", err)
	}

	return &connect.Response[registry.GetModulePinsResponse]{
		Msg: &registry.GetModulePinsResponse{ModulePins: modulePins},
	}, nil
}

func (a *api) GetRepositoriesByFullName(
	ctx context.Context,
	req *connect.Request[registry.GetRepositoriesByFullNameRequest],
) (
	*connect.Response[registry.GetRepositoriesByFullNameResponse],
	error,
) {
	repositories, err := sliceMap(
		req.Msg.FullNames,
		func(_ int, v string) (*registry.Repository, error) {
			owner, repositoryName := filepath.Split(v)

			repo, err := a.repo.Get(owner, repositoryName, "")
			if err != nil {
				return nil, fmt.Errorf("investigating %q: %w", v, err)
			}

			return &registry.Repository{ //nolint:exhaustruct
				Id:            path.Join(a.domain, owner, repositoryName),
				CreateTime:    timestamppb.New(repo.CreatedAt),
				UpdateTime:    timestamppb.New(repo.UpdatedAt),
				Name:          repositoryName,
				Owner:         &registry.Repository_UserId{UserId: owner},
				Visibility:    registry.Visibility_VISIBILITY_PUBLIC,
				OwnerName:     owner,
				Description:   "", // TODO //nolint:godox
				Url:           path.Join(a.domain, owner, repositoryName),
				DefaultBranch: repo.DefaultBranch,
			}, nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("getting repositories: %w", err)
	}

	return &connect.Response[registry.GetRepositoriesByFullNameResponse]{
		Msg: &registry.GetRepositoriesByFullNameResponse{Repositories: repositories},
	}, nil
}

func (a *api) DownloadManifestAndBlobs(
	ctx context.Context,
	req *connect.Request[registry.DownloadManifestAndBlobsRequest],
) (
	*connect.Response[registry.DownloadManifestAndBlobsResponse],
	error,
) {
	_, files, err := a.repo.GetWithFiles(req.Msg.Owner, req.Msg.Repository, req.Msg.Reference)
	if err != nil {
		return nil, fmt.Errorf("a.repo.GetRepository: %w", err)
	}

	var manifest bytes.Buffer

	blobs := must(sliceMap(files, func(_ int, v content.File) (*module.Blob, error) {
		must(fmt.Fprintf(&manifest, "shake256:%s  %s\n", hex.EncodeToString(v.Hash[:]), v.Path))

		return buildBlob(v.Hash, v.Data), nil
	}))

	manifestHash, err := shake256.SHA3Shake256(manifest.Bytes())
	if err != nil {
		return nil, fmt.Errorf("calculating manifest hash: %w", err)
	}

	return &connect.Response[registry.DownloadManifestAndBlobsResponse]{
		Msg: &registry.DownloadManifestAndBlobsResponse{
			Manifest: &module.Blob{
				Digest: &module.Digest{
					DigestType: module.DigestType_DIGEST_TYPE_SHAKE256,
					Digest:     manifestHash[:],
				},
				Content: manifest.Bytes(),
			},
			Blobs: blobs,
		},
	}, nil
}

func buildBlob(hash [shake256.HashLen]byte, data []byte) *module.Blob {
	return &module.Blob{
		Digest: &module.Digest{
			DigestType: module.DigestType_DIGEST_TYPE_SHAKE256,
			Digest:     hash[:],
		},
		Content: data,
	}
}

func sliceMap[T any, R any](in []T, f func(i int, v T) (R, error)) ([]R, error) {
	out := make([]R, 0, len(in))

	for i, item := range in {
		v, err := f(i, item)
		if err != nil {
			return out, fmt.Errorf("iterating %d of %d: %w", i, len(in), err)
		}

		out = append(out, v)
	}

	return out, nil
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}

	return v
}
