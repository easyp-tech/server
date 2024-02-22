package connect

import (
	"bytes"
	"context"
	"fmt"

	"connectrpc.com/connect"

	module "github.com/easyp-tech/server/gen/proto/buf/alpha/module/v1alpha1"
	registry "github.com/easyp-tech/server/gen/proto/buf/alpha/registry/v1alpha1"
	"github.com/easyp-tech/server/internal/shake256"
)

const digestFormat = "shake256:%s  %s\n"

func (a *api) DownloadManifestAndBlobs(
	ctx context.Context,
	req *connect.Request[registry.DownloadManifestAndBlobsRequest],
) (
	*connect.Response[registry.DownloadManifestAndBlobsResponse],
	error,
) {
	files, err := a.repo.GetFiles(ctx, req.Msg.GetOwner(), req.Msg.GetRepository(), req.Msg.GetReference())
	if err != nil {
		return nil, fmt.Errorf("a.repo.GetRepository: %w", err)
	}

	var (
		manifest bytes.Buffer
		blobs    = make([]*module.Blob, 0, len(files))
	)

	for _, file := range files {
		fmt.Fprintf(&manifest, digestFormat, file.Hash.String(), file.Path)
		blobs = append(blobs, buildBlob(file.Hash, file.Data))
	}

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

func buildBlob(hash shake256.Hash, data []byte) *module.Blob {
	return &module.Blob{
		Digest: &module.Digest{
			DigestType: module.DigestType_DIGEST_TYPE_SHAKE256,
			Digest:     hash[:],
		},
		Content: data,
	}
}
