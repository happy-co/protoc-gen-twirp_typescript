package generator

import (
	"bytes"
	"html/template"
	"path"

	"github.com/golang/protobuf/proto"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

const indexTemplate = `
{{- range .}}
export * from './{{.}}';
{{end}}
`

func CreatePackageIndex(outputPath string, files []*plugin.CodeGeneratorResponse_File) (*plugin.CodeGeneratorResponse_File, error) {
	var names []string

	for _, f := range files {
		filename := *f.Name

		// myModule.ts => myModule
		if path.Ext(filename) == ".ts" {
			base := path.Base(filename)
			moduleName := base[:len(base)-len(path.Ext(base))]
			names = append(names, moduleName)
		}
	}

	t, err := template.New("index.ts").Parse(indexTemplate)
	if err != nil {
		return nil, err
	}

	b := bytes.NewBufferString("")
	t.Execute(b, names)

	cf := &plugin.CodeGeneratorResponse_File{}
	cf.Name = proto.String(path.Join(outputPath, "index.ts"))
	cf.Content = proto.String(b.String())

	return cf, nil
}
