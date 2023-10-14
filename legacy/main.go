package legacy

//
//import (
//	"bytes"
//	"context"
//	"encoding/hex"
//	"fmt"
//	"io"
//	"io/fs"
//	"net/http"
//	"os"
//	"path/filepath"
//	"strings"
//
//	"connectrpc.com/connect"
//	"golang.org/x/crypto/sha3"
//	"golang.org/x/net/http2"
//	"golang.org/x/net/http2/h2c"
//	"google.golang.org/protobuf/types/known/timestamppb"
//	"google.golang.org/protobuf/types/pluginpb"
//
//	v1alpha1 "github.com/sipki-tech/easyp/proto/buf/alpha/module/v1alpha1"
//	registryv1alpha1 "github.com/sipki-tech/easyp/proto/buf/alpha/registry/v1alpha1"
//	"github.com/sipki-tech/easyp/proto/buf/alpha/registry/v1alpha1/registryv1alpha1connect"
//)
//
//type api struct {
//	registryv1alpha1connect.UnimplementedRepositoryServiceHandler
//	registryv1alpha1connect.UnimplementedResolveServiceHandler
//	registryv1alpha1connect.UnimplementedDownloadServiceHandler
//}
//
//func (api) GenerateCode(context.Context, *connect.Request[registryv1alpha1.GenerateCodeRequest]) (*connect.Response[registryv1alpha1.GenerateCodeResponse], error) {
//	return &connect.Response[registryv1alpha1.GenerateCodeResponse]{
//		Msg: &registryv1alpha1.GenerateCodeResponse{
//			Responses: []*registryv1alpha1.PluginGenerationResponse{
//				{
//					Response: &pluginpb.CodeGeneratorResponse{
//						Error:             nil,
//						SupportedFeatures: nil,
//						File:              nil,
//					},
//				},
//			},
//		},
//	}, nil
//}
//
//func (api) GetModulePins(
//	ctx context.Context,
//	req *connect.Request[registryv1alpha1.GetModulePinsRequest],
//) (
//	*connect.Response[registryv1alpha1.GetModulePinsResponse],
//	error,
//) {
//	fmt.Println("aAAAA ", req.Msg.String())
//	return &connect.Response[registryv1alpha1.GetModulePinsResponse]{
//		Msg: &registryv1alpha1.GetModulePinsResponse{
//			ModulePins: []*v1alpha1.ModulePin{
//				{
//					Remote:     "localhost:8080",
//					Owner:      "googleapis",
//					Repository: "googleapis",
//					Branch:     "master",
//					Commit:     "e4fb9e3c97678646b17b0efb7ed75e35c9122aca",
//					//CreateTime:     nil,
//					//ManifestDigest: "",
//				},
//			},
//		},
//	}, nil
//}
//
//func (api) GetRepositoriesByFullName(
//	ctx context.Context,
//	req *connect.Request[registryv1alpha1.GetRepositoriesByFullNameRequest],
//) (
//	*connect.Response[registryv1alpha1.GetRepositoriesByFullNameResponse],
//	error,
//) {
//	fmt.Println("aAAAA ", req.Msg.String())
//	return &connect.Response[registryv1alpha1.GetRepositoriesByFullNameResponse]{
//		Msg: &registryv1alpha1.GetRepositoriesByFullNameResponse{
//			Repositories: []*registryv1alpha1.Repository{
//				{
//					Id:         "gawd",
//					CreateTime: timestamppb.Now(),
//					UpdateTime: timestamppb.Now(),
//					Name:       "name",
//					Owner: &registryv1alpha1.Repository_UserId{
//						UserId: "",
//					},
//					Visibility:         0,
//					Deprecated:         false,
//					DeprecationMessage: "",
//					OwnerName:          "owner",
//					Description:        "",
//					Url:                "",
//					DefaultBranch:      "",
//				},
//			},
//		},
//	}, nil
//}
//
//func (api) DownloadManifestAndBlobs(
//	context.Context,
//	*connect.Request[registryv1alpha1.DownloadManifestAndBlobsRequest],
//) (
//	*connect.Response[registryv1alpha1.DownloadManifestAndBlobsResponse],
//	error,
//) {
//
//	type fInfo struct {
//		path        string
//		digest      string
//		digestBytes []byte
//	}
//
//	manifestB := bytes.NewBuffer(nil)
//	// path => info
//	var files []fInfo
//	filepath.Walk("cache/repositories/googleapis", func(path string, info fs.FileInfo, err error) error {
//		if info.IsDir() ||
//			filepath.Ext(path) != ".proto" ||
//			!strings.Contains(path, "cache/repositories/googleapis/google") {
//			return nil
//		}
//
//		f, err := os.Open(path)
//		if err != nil {
//			return fmt.Errorf("os.Open: %w", err)
//		}
//		defer func() {
//			err := f.Close()
//			if err != nil {
//				fmt.Println(err)
//			}
//		}()
//
//		buf, err := io.ReadAll(f)
//		if err != nil {
//			return err
//		}
//
//		d := sha3.NewShake256()
//		d.Write(buf)
//		hash := make([]byte, 64)
//		_, err = d.Read(hash)
//		if err != nil {
//			return err
//		}
//
//		// пришлось символ пробела заменить даже
//		filePath, _ := strings.CutPrefix(path, "cache/repositories/googleapis/")
//		digest := fmt.Sprintf("shake256:%s  %s\n", hex.EncodeToString(hash), filePath)
//
//		files = append(files, fInfo{
//			path:        path,
//			digest:      digest,
//			digestBytes: hash,
//		})
//
//		manifestB.WriteString(digest)
//
//		return nil
//	})
//
//	blobs := make([]*v1alpha1.Blob, len(files))
//	for i := range files {
//
//		f, err := os.Open(files[i].path)
//		if err != nil {
//			return nil, fmt.Errorf("os.Open: %w", err)
//		}
//
//		bytes, err := io.ReadAll(f)
//		if err != nil {
//			f.Close()
//			return nil, fmt.Errorf("io.ReadAll: %w", err)
//		}
//		f.Close()
//
//		blobs[i] = &v1alpha1.Blob{
//			Digest: &v1alpha1.Digest{
//				DigestType: v1alpha1.DigestType_DIGEST_TYPE_SHAKE256,
//				Digest:     files[i].digestBytes,
//			},
//			Content: bytes,
//		}
//	}
//
//	manifestB2 := bytes.NewBuffer(manifestB.Bytes())
//
//	buf, err := io.ReadAll(manifestB2)
//	if err != nil {
//		return nil, err
//	}
//
//	d := sha3.NewShake256()
//	d.Write(buf)
//	hash := make([]byte, 64)
//	_, err = d.Read(hash)
//	if err != nil {
//		return nil, err
//	}
//
//	return &connect.Response[registryv1alpha1.DownloadManifestAndBlobsResponse]{
//		Msg: &registryv1alpha1.DownloadManifestAndBlobsResponse{
//			Manifest: &v1alpha1.Blob{
//				Digest: &v1alpha1.Digest{
//					DigestType: v1alpha1.DigestType_DIGEST_TYPE_SHAKE256,
//					Digest:     hash,
//				},
//				Content: manifestB.Bytes(),
//			},
//			Blobs: blobs,
//		},
//	}, nil
//}
//
////func main() {
////	g := grpc.NewServer()
////	registryv1alpha1.RegisterResolveServiceServer(g, &api{})
////
////	ln, err := net.Listen("tcp", ":8080")
////	if err != nil {
////		panic(err)
////	}
////
////	log.Fatal(g.Serve(ln))
////}
//
//func NewAuthInterceptor() connect.UnaryInterceptorFunc {
//	interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
//		return connect.UnaryFunc(func(
//			ctx context.Context,
//			req connect.AnyRequest,
//		) (connect.AnyResponse, error) {
//			fmt.Printf("GGGGG %s\n, %+v", req.Any(), ctx)
//			return next(ctx, req)
//		})
//	}
//	return connect.UnaryInterceptorFunc(interceptor)
//}
//
//func main() {
//	interceptors := connect.WithInterceptors(NewAuthInterceptor())
//	a := &api{}
//	mux := http.NewServeMux()
//	path, handler := registryv1alpha1connect.NewResolveServiceHandler(a, interceptors)
//	mux.Handle(path, handler)
//
//	path, handler = registryv1alpha1connect.NewRepositoryServiceHandler(a, interceptors)
//	mux.Handle(path, handler)
//
//	path, handler = registryv1alpha1connect.NewDownloadServiceHandler(a, interceptors)
//	mux.Handle(path, handler)
//
//	path, handler = registryv1alpha1connect.NewCodeGenerationServiceHandler(a, interceptors)
//	mux.Handle(path, handler)
//
//	http.ListenAndServe(
//		"localhost:8080",
//		// Use h2c so we can serve HTTP/2 without TLS.
//		h2c.NewHandler(mux, &http2.Server{}),
//	)
//}
