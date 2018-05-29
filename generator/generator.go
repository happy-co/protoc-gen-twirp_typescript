package generator

import (
	"path"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

func tsModuleFilename(f *descriptor.FileDescriptorProto) string {
	name := *f.Name

	if ext := path.Ext(name); ext == ".proto" || ext == ".protodevel" {
		name = name[:len(name)-len(path.Ext(name))]
	}

	name += ".ts"

	return name
}
