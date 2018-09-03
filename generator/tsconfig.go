package generator

import (
	"fmt"
	"path"

	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

func CreateTSConfig(outputPath string) *plugin.CodeGeneratorResponse_File {
	content := fmt.Sprintf(`{
  "compilerOptions": {
    "target": "es6",
    "module": "commonjs",
    "declaration": true,
    "importHelpers": true,
    "strict": true,
    "noUnusedParameters": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
    "esModuleInterop": true
  }
}
`)

	fileName := path.Join(outputPath, "tsconfig.json")
	cf := &plugin.CodeGeneratorResponse_File{}
	cf.Name = &fileName
	cf.Content = &content

	return cf
}
