package proto

//go:generate rm -rf ./buf
//go:generate cp -r ../_third_party/buf-v1.69.0/proto/buf ./
//go:generate rm -rf ../../gen
//go:generate buf generate
