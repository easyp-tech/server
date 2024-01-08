package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"

	"connectrpc.com/connect"
	"github.com/samber/lo"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"
	registryv1alpha1 "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	"github.com/easyp-tech/server/internal/core"
)

const hashLen = 64

func (a *api) GetModulePins(
	ctx context.Context,
	req *connect.Request[registryv1alpha1.GetModulePinsRequest],
) (
	*connect.Response[registryv1alpha1.GetModulePinsResponse],
	error,
) {
	repositories := make([]core.Repository, len(req.Msg.ModuleReferences))

	for i := range req.Msg.ModuleReferences {
		repository, err := a.core.GetRepository(ctx, core.GetRequest{
			Owner:      req.Msg.ModuleReferences[i].Owner,
			Repository: req.Msg.ModuleReferences[i].Repository,
			Branch:     req.Msg.ModuleReferences[i].Reference,
		})
		if err != nil {
			return nil, fmt.Errorf("a.core.GetRepository: %w", err)
		}

		repositories[i] = *repository
	}

	return &connect.Response[registryv1alpha1.GetModulePinsResponse]{
		Msg: &registryv1alpha1.GetModulePinsResponse{
			ModulePins: lo.Map(repositories, func(item core.Repository, index int) *v1alpha1.ModulePin {
				return &v1alpha1.ModulePin{ //nolint:exhaustruct
					Remote:     a.domain,
					Owner:      item.Owner,
					Repository: item.Repository,
					Commit:     item.Commit,
				}
			}),
		},
	}, nil
}

func (a *api) GetRepositoriesByFullName(
	ctx context.Context,
	req *connect.Request[registryv1alpha1.GetRepositoriesByFullNameRequest],
) (
	*connect.Response[registryv1alpha1.GetRepositoriesByFullNameResponse],
	error,
) {
	repositories := make([]core.Repository, 0, len(req.Msg.FullNames))

	for _, name := range req.Msg.FullNames {
		owner, repositoryName := filepath.Split(name)

		repository, err := a.core.GetRepository(
			ctx,
			core.GetRequest{ //nolint:exhaustruct
				Owner:      owner,
				Repository: repositoryName,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("a.core.GetRepository: %w", err)
		}

		repositories = append(repositories, *repository)
	}

	return &connect.Response[registryv1alpha1.GetRepositoriesByFullNameResponse]{
		Msg: &registryv1alpha1.GetRepositoriesByFullNameResponse{
			Repositories: lo.Map(repositories, func(item core.Repository, index int) *registryv1alpha1.Repository {
				return &registryv1alpha1.Repository{ //nolint:exhaustruct
					Id:            path.Join(a.domain, item.Owner, item.Repository),
					CreateTime:    timestamppb.New(item.CreatedAt),
					UpdateTime:    timestamppb.New(item.UpdatedAt),
					Name:          item.Repository,
					Owner:         &registryv1alpha1.Repository_UserId{UserId: item.Owner},
					Visibility:    registryv1alpha1.Visibility_VISIBILITY_PUBLIC,
					OwnerName:     item.Owner,
					Description:   "", // TODO //nolint:godox
					Url:           path.Join(a.domain, item.Owner, item.Repository),
					DefaultBranch: item.Branch,
				}
			}),
		},
	}, nil
}

func (a *api) DownloadManifestAndBlobs(
	ctx context.Context,
	req *connect.Request[registryv1alpha1.DownloadManifestAndBlobsRequest],
) (
	*connect.Response[registryv1alpha1.DownloadManifestAndBlobsResponse],
	error,
) {
	repository, err := a.core.GetRepository(ctx, core.GetRequest{
		Owner:      req.Msg.Owner,
		Repository: req.Msg.Repository,
		Branch:     req.Msg.Reference,
	})
	if err != nil {
		return nil, fmt.Errorf("a.core.GetRepository: %w", err)
	}

	manifestB := bytes.NewBuffer(nil)

	var blobs []*v1alpha1.Blob

	err = fs.WalkDir(
		repository,
		".",
		func(path string, info fs.DirEntry, err error) error {
			digest, blob, errInner := processDirEntry(repository, path, info, err)
			if errInner != nil {
				return errInner
			}

			if blob == nil {
				return nil
			}
			manifestB.WriteString(digest)
			blobs = append(blobs, blob)

			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("fs.WalkDir: %w", err)
	}

	hash, err := sha3Shake256(manifestB.Bytes())
	if err != nil {
		return nil, fmt.Errorf("calculating manifest hash: %w", err)
	}

	return &connect.Response[registryv1alpha1.DownloadManifestAndBlobsResponse]{
		Msg: &registryv1alpha1.DownloadManifestAndBlobsResponse{
			Manifest: &v1alpha1.Blob{
				Digest: &v1alpha1.Digest{
					DigestType: v1alpha1.DigestType_DIGEST_TYPE_SHAKE256,
					Digest:     hash,
				},
				Content: manifestB.Bytes(),
			},
			Blobs: blobs,
		},
	}, nil
}

func processDirEntry(
	repository *core.Repository,
	path string,
	info fs.DirEntry,
	err error,
) (string, *v1alpha1.Blob, error) {
	switch {
	case err != nil:
		return "", nil, nil //nolint:nilerr
	case info.IsDir():
		return "", nil, nil
	case filepath.Ext(path) != ".proto":
		return "", nil, nil
	}

	buf, err := fs.ReadFile(repository, path)
	if err != nil {
		return "", nil, fmt.Errorf("reading %q: %w", path, err)
	}

	hash, err := sha3Shake256(buf)
	if err != nil {
		return "", nil, fmt.Errorf("hashing %q: %w", path, err)
	}

	return buildDigest(path, hash), buildBlob(hash, buf), nil
}

func buildDigest(path string, hash []byte) string {
	return fmt.Sprintf("shake256:%s  %s\n", hex.EncodeToString(hash), path)
}

func buildBlob(hash []byte, data []byte) *v1alpha1.Blob {
	return &v1alpha1.Blob{
		Digest: &v1alpha1.Digest{
			DigestType: v1alpha1.DigestType_DIGEST_TYPE_SHAKE256,
			Digest:     hash,
		},
		Content: data,
	}
}

func sha3Shake256(data []byte) ([]byte, error) {
	d := sha3.NewShake256()

	if _, err := d.Write(data); err != nil {
		return nil, fmt.Errorf("calculating hash: %w", err)
	}

	hash := make([]byte, hashLen)

	if _, err := d.Read(hash); err != nil {
		return nil, fmt.Errorf("extracting hash: %w", err)
	}

	return hash, nil
}
