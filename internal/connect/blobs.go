package connect

import (
	"bytes"
	"context"
	"fmt"

	"connectrpc.com/connect"

	module "github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	"github.com/easyp-tech/server/internal/content"
	"github.com/easyp-tech/server/internal/shake256"
)

func (a *api) DownloadManifestAndBlobs(
	_ context.Context,
	req *connect.Request[registry.DownloadManifestAndBlobsRequest],
) (
	*connect.Response[registry.DownloadManifestAndBlobsResponse],
	error,
) {
	_, files, err := a.repo.GetWithFiles(req.Msg.GetOwner(), req.Msg.GetRepository(), req.Msg.GetReference())
	if err != nil {
		return nil, fmt.Errorf("a.repo.GetRepository: %w", err)
	}

	var manifest bytes.Buffer

	blobs := buildBlobs(files)

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

func buildBlobs(in []content.File) []*module.Blob {
	out := make([]*module.Blob, 0, len(in))

	for _, file := range in {
		out = append(out, buildBlob(file.Hash, file.Data))
	}

	return out
}

func buildBlob(hash shake256.Hash, data []byte) *module.Blob {
	return &module.Blob{
		Digest: &module.Digest{
			DigestType: module.DigestType_DIGEST_TYPE_SHAKE256,
			Digest:     hash[:],
		},
		Content: data,
	}
}
