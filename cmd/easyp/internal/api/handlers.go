package api

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"path"
	"path/filepath"

	"connectrpc.com/connect"
	"github.com/samber/lo"
	"golang.org/x/crypto/sha3"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/easyp-tech/server/cmd/easyp/internal/core"
	"github.com/easyp-tech/server/internal/logkey"
	v1alpha1 "github.com/easyp-tech/server/proto/buf/alpha/module/v1alpha1"
	registryv1alpha1 "github.com/easyp-tech/server/proto/buf/alpha/registry/v1alpha1"
)

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
				return &v1alpha1.ModulePin{
					Remote:     a.domain,
					Owner:      item.Owner,
					Repository: item.Repository,
					Branch:     item.Branch,
					Commit:     item.Commit,
					CreateTime: timestamppb.New(item.CreatedAt),
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

	repositories := make([]core.Repository, len(req.Msg.FullNames))
	for i := range req.Msg.FullNames {
		owner, repositoryName := filepath.Split(req.Msg.FullNames[i])
		repository, err := a.core.GetRepository(ctx, core.GetRequest{
			Owner:      owner,
			Repository: repositoryName,
		})
		if err != nil {
			return nil, fmt.Errorf("a.core.GetRepository: %w", err)
		}

		repositories = append(repositories, *repository)
	}

	return &connect.Response[registryv1alpha1.GetRepositoriesByFullNameResponse]{
		Msg: &registryv1alpha1.GetRepositoriesByFullNameResponse{
			Repositories: lo.Map(repositories, func(item core.Repository, index int) *registryv1alpha1.Repository {
				return &registryv1alpha1.Repository{
					Id:            path.Join(a.domain, item.Owner, item.Repository),
					CreateTime:    timestamppb.New(item.CreatedAt),
					UpdateTime:    timestamppb.New(item.UpdatedAt),
					Name:          item.Repository,
					Owner:         &registryv1alpha1.Repository_UserId{UserId: item.Owner},
					Visibility:    registryv1alpha1.Visibility_VISIBILITY_PUBLIC,
					OwnerName:     item.Owner,
					Description:   "", // TODO
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
	err = fs.WalkDir(repository, ".", func(path string, info fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return nil
		case info.IsDir():
			return nil
		case filepath.Ext(path) != ".proto":
			return nil
		}

		f, err := repository.Open(path)
		if err != nil {
			return fmt.Errorf("repository.Open: %w", err)
		}
		defer func() {
			err := f.Close()
			if err != nil {
				logkey.FromContext(ctx).Warn("f.Close", slog.String(logkey.Error, err.Error()))
			}
		}()

		buf, err := io.ReadAll(f)
		if err != nil {
			return fmt.Errorf("io.ReadAll: %w", err)
		}

		d := sha3.NewShake256()
		_, err = d.Write(buf)
		if err != nil {
			return fmt.Errorf("d.Write: %w", err)
		}

		hash := make([]byte, 64)
		_, err = d.Read(hash)
		if err != nil {
			return fmt.Errorf("d.Read: %w", err)
		}

		digest := fmt.Sprintf("shake256:%s  %s\n", hex.EncodeToString(hash), path)
		manifestB.WriteString(digest)

		blobs = append(blobs, &v1alpha1.Blob{
			Digest: &v1alpha1.Digest{
				DigestType: v1alpha1.DigestType_DIGEST_TYPE_SHAKE256,
				Digest:     hash,
			},
			Content: buf,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("fs.WalkDir: %w", err)
	}

	buf, err := io.ReadAll(bytes.NewBuffer(manifestB.Bytes()))
	if err != nil {
		return nil, fmt.Errorf("io.ReadAll: %w", err)
	}

	d := sha3.NewShake256()
	_, err = d.Write(buf)
	if err != nil {
		return nil, fmt.Errorf("d.Write: %w", err)
	}

	hash := make([]byte, 64)
	_, err = d.Read(hash)
	if err != nil {
		return nil, fmt.Errorf("d.Read: %w", err)
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
