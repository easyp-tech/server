package proto

//go:generate rm -rf ./buf
//go:generate cp -r ../_third_party/buf/proto/buf ./
//go:generate rm -rf ../../gen
//go:generate buf generate
