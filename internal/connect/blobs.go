package connect

import (
	"bytes"
	"context"
	"encoding/hex"
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

func buildBlob(hash shake256.Hash, data []byte) *module.Blob {
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
