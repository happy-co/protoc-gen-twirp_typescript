package generator

import (
	"bytes"
	"fmt"
	"log"
	"path"
	"strings"
	"text/template"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	plugin "github.com/golang/protobuf/protoc-gen-go/plugin"
)

const apiTemplate = `
import {resolve} from 'url';
import {createTwirpRequest, throwTwirpError, Fetch} from './twirp';

export interface Dictionary<V> {
    [index: string]: V
}

{{range .Enums -}}
export type {{.Name}} = {{range $index, $value := .Values}}{{if gt $index 0}} | {{end}}"{{$value}}"{{end}}
{{- end}}

{{range .Models -}}
{{if not .Primitive -}}
{{if .Map -}}
interface {{.Map.Name}}JSON {
    [index: string]: {{.Map.ValueField.JSONType}}
}

{{else -}}
export interface {{.Name}} {
{{- range .Fields}}
    {{.Name}}{{if not .IsRepeated}}?{{end}}: {{if .MapType}}{{.MapType}}{{else}}{{.Type}}{{end}};
{{- end}}
}

interface {{.Name}}JSON {
{{- range .Fields}}
    {{.JSONName}}{{if not .IsRepeated}}?{{end}}: {{.JSONType}};
{{- end}}
}

{{end -}}

{{if .CanMarshal -}}
{{if .Map -}}
const {{.Map.Name}}MapToJSON = (map: Dictionary<{{.Map.ValueField.Type}}>): {{.Map.Name}}JSON => {
    const obj:{{.Map.Name}}JSON = {}
    Object.keys(map).forEach(key => {
        const m = {value: map[key]}
        obj[key] = {{stringify .Map.ValueField}}
    })
    return obj
}

{{else -}}
const {{.Name}}ToJSON = ({{if .Fields}}m{{else}}_{{end}}?: {{.Name}}): {{.Name}}JSON => {
    return {
{{- range .Fields}}
        {{.JSONName}}: (!!m && m.hasOwnProperty('{{.Name}}')) && m.{{.Name}} !== undefined ? {{stringify .}} : {{if .MapType}}{}{{else if .IsRepeated}}[]{{else}}undefined{{end}},
{{- end}}
    };
};

{{end -}}
{{end -}}

{{if .CanUnmarshal -}}
{{if .Map -}}
const JSONTo{{.Map.Name}}Map = (entries: {{.Map.Name}}JSON): Dictionary<{{.Map.ValueField.Type}}> => {
    const obj:Dictionary<{{.Map.ValueField.Type}}> = {}
    Object.keys(entries).forEach(key => {
        const m = {value: entries[key]}
        obj[key] = {{parse .Map.ValueField}}
    })
    return obj
}

{{else -}}
const JSONTo{{.Name}} = ({{if .Fields}}m{{else}}_{{end}}?: {{.Name}}JSON): {{.Name}} => {
    return {
{{- range .Fields}}
        {{.Name}}: (!!m && m.hasOwnProperty('{{.JSONName}}')) && m.{{.JSONName}} !== undefined && m.{{.JSONName}} !== null ? {{parse .}} : {{if .MapType}}{}{{else if .IsRepeated}}[]{{else}}undefined{{end}},
{{- end}}
    };
};

{{end -}}
{{end -}}
{{end -}}
{{end -}}

{{range .Services -}}
export interface {{.Name}} {
{{range .Methods -}}
    {{.Name}}: ({{.InputArg}}: {{.InputType}}) => Promise<{{.OutputType}}>;
{{end -}}
}

export class {{.Name}}Client implements {{.Name}} {
    private _hostname: string;
    private _fetch: Fetch;
    private _pathPrefix = "/twirp/{{.Package}}.{{.Name}}/";

    constructor(hostname: string, fetch: Fetch) {
        this._hostname = hostname;
        this._fetch = fetch;
    }

    {{range .Methods -}}
    {{.Name}}({{.InputArg}}: {{.InputType}}): Promise<{{.OutputType}}> {
        const url = resolve(this._hostname, this._pathPrefix + "{{.Path}}");
        return this._fetch(createTwirpRequest(url, {{.InputType}}ToJSON({{.InputArg}}))).then((resp) => {
            if (!resp.ok) {
                return throwTwirpError(resp);
            }

            return resp.json().then(JSONTo{{.OutputType}});
        });
    }
    {{end}}
}

{{end -}}
`

type Model struct {
	Name         string
	Primitive    bool
	Fields       []ModelField
	Map          *MapDetails
	CanMarshal   bool
	CanUnmarshal bool
}

type ModelField struct {
	Name       string
	Type       string
	JSONName   string
	JSONType   string
	IsMessage  bool
	IsRepeated bool
	MapType    *string
}

type MapDetails struct {
	Name       string
	KeyField   ModelField
	ValueField ModelField
}

type Enum struct {
	Name   string
	Values []string
}

type Service struct {
	Name    string
	Package string
	Methods []ServiceMethod
}

type ServiceMethod struct {
	Name       string
	Path       string
	InputArg   string
	InputType  string
	OutputType string
}

func NewAPIContext() APIContext {
	ctx := APIContext{}
	ctx.modelLookup = make(map[string]*Model)

	return ctx
}

type APIContext struct {
	Models      []*Model
	Enums       []*Enum
	Services    []*Service
	modelLookup map[string]*Model
}

func (ctx *APIContext) AddModel(m *Model) {
	ctx.Models = append(ctx.Models, m)
	ctx.modelLookup[m.Name] = m
}

// ApplyMarshalFlags will inspect the CanMarshal and CanUnmarshal flags for models where
// the flags are enabled and recursively set the same values on all the models that are field types.
func (ctx *APIContext) ApplyMarshalFlags() {
	for _, m := range ctx.Models {
		for _, f := range m.Fields {
			// skip primitive types and WKT Timestamps
			if !f.IsMessage || f.Type == "Date" {
				continue
			}

			baseType := f.Type
			if f.IsRepeated {
				baseType = strings.Trim(baseType, "[]")
			}

			if m.CanMarshal {
				ctx.enableMarshal(ctx.modelLookup[baseType])
			}

			if m.CanUnmarshal {
				m, ok := ctx.modelLookup[baseType]
				if !ok {
					log.Fatalf("could not find model of type %s for field %s", baseType, f.Name)
				}
				ctx.enableUnmarshal(m)
			}
		}
	}
}

func (ctx *APIContext) enableMarshal(m *Model) {
	if m.CanMarshal {
		return
	}

	m.CanMarshal = true

	for _, f := range m.Fields {
		// skip primitive types and WKT Timestamps
		if !f.IsMessage || f.Type == "Date" {
			continue
		}

		baseType := f.Type
		if f.IsRepeated {
			baseType = strings.Trim(baseType, "[]")
		}

		mm, ok := ctx.modelLookup[baseType]
		if !ok {
			log.Fatalf("could not find model of type %s for field %s", baseType, f.Name)
		}
		ctx.enableMarshal(mm)
	}
}

func (ctx *APIContext) enableUnmarshal(m *Model) {
	if m.CanUnmarshal {
		return
	}

	m.CanUnmarshal = true

	for _, f := range m.Fields {
		// skip primitive types and WKT Timestamps
		if !f.IsMessage || f.Type == "Date" {
			continue
		}

		baseType := f.Type
		if f.IsRepeated {
			baseType = strings.Trim(baseType, "[]")
		}

		mm, ok := ctx.modelLookup[baseType]
		if !ok {
			log.Fatalf("could not find model of type %s for field %s", baseType, f.Name)
		}
		ctx.enableUnmarshal(mm)
	}
}

func CreateClientAPI(outputPath string, d *descriptor.FileDescriptorProto) (*plugin.CodeGeneratorResponse_File, error) {
	ctx := NewAPIContext()
	pkg := d.GetPackage()

	// Parse all Messages for generating typescript interfaces
	for _, m := range d.GetMessageType() {
		addMessageType(m, "", pkg, &ctx)
	}

	// Parse all Enums for generating typescript interfaces
	for _, e := range d.GetEnumType() {
		addEnumType(e, pkg, &ctx)
	}

	// Parse all Services for generating typescript method interfaces and default client implementations
	for _, s := range d.GetService() {
		service := &Service{
			Name:    s.GetName(),
			Package: pkg,
		}

		for _, m := range s.GetMethod() {
			methodPath := m.GetName()
			methodName := strings.ToLower(methodPath[0:1]) + methodPath[1:]
			in := removePkg(m.GetInputType(), pkg)
			arg := strings.ToLower(in[0:1]) + in[1:]

			method := ServiceMethod{
				Name:       methodName,
				Path:       methodPath,
				InputArg:   arg,
				InputType:  in,
				OutputType: removePkg(m.GetOutputType(), pkg),
			}

			service.Methods = append(service.Methods, method)
		}

		ctx.Services = append(ctx.Services, service)
	}

	// Only include the custom 'ToJSON' and 'JSONTo' methods in generated code
	// if the Model is part of an rpc method input arg or return type.
	for _, m := range ctx.Models {
		for _, s := range ctx.Services {
			for _, sm := range s.Methods {
				if m.Name == sm.InputType {
					m.CanMarshal = true
				}

				if m.Name == sm.OutputType {
					m.CanUnmarshal = true
				}
			}
		}
	}

	ctx.AddModel(&Model{
		Name:      "Date",
		Primitive: true,
	})

	ctx.ApplyMarshalFlags()

	funcMap := template.FuncMap{
		"stringify": stringify,
		"parse":     parse,
	}

	t, err := template.New("client_api").Funcs(funcMap).Parse(apiTemplate)
	if err != nil {
		return nil, err
	}

	b := bytes.NewBufferString("")
	err = t.Execute(b, ctx)
	if err != nil {
		return nil, err
	}

	cf := &plugin.CodeGeneratorResponse_File{}
	cf.Name = proto.String(path.Join(outputPath, tsModuleFilename(d)))
	cf.Content = proto.String(b.String())

	return cf, nil
}

func addMessageType(m *descriptor.DescriptorProto, prefix, pkg string, ctx *APIContext) {
	model := &Model{
		Name: strings.Replace(prefix, ".", "", -1) + m.GetName(),
	}
	var keyField, valueField *ModelField
	for _, f := range m.GetField() {
		field := newField(f, m, pkg)
		model.Fields = append(model.Fields, field)
		if f.GetName() == "key" {
			keyField = &field
		}
		if f.GetName() == "value" {
			valueField = &field
		}
	}
	ctx.AddModel(model)

	if m.Options.GetMapEntry() {
		model.Map = &MapDetails{Name: strings.TrimSuffix(model.Name, "Entry"), KeyField: *keyField, ValueField: *valueField}
	}

	for _, n := range m.GetNestedType() {
		addMessageType(n, prefix+"."+m.GetName(), pkg, ctx)
	}
}

func addEnumType(e *descriptor.EnumDescriptorProto, pkg string, ctx *APIContext) {
	enum := &Enum{
		Name: e.GetName(),
	}
	for _, v := range e.GetValue() {
		enum.Values = append(enum.Values, v.GetName())
	}
	ctx.Enums = append(ctx.Enums, enum)
}

func newField(f *descriptor.FieldDescriptorProto, m *descriptor.DescriptorProto, pkg string) ModelField {
	tsType, jsonType := protoToTSType(f, pkg)
	jsonName := f.GetName()
	name := camelCase(jsonName)

	field := ModelField{
		Name:     name,
		Type:     tsType,
		JSONName: jsonName,
		JSONType: jsonType,
	}

	field.IsMessage = f.GetType() == descriptor.FieldDescriptorProto_TYPE_MESSAGE
	field.IsRepeated = isRepeated(f)
	field.MapType = mapType(f, m, pkg)

	if field.MapType != nil {
		field.Type = strings.TrimSuffix(field.Type, "[]")
		field.JSONType = strings.TrimSuffix(field.JSONType, "EntryJSON[]") + "JSON"
	}

	return field
}

// generates the (Type, JSONType) tuple for a ModelField so marshal/unmarshal functions
// will work when converting between TS interfaces and protobuf JSON.
func protoToTSType(f *descriptor.FieldDescriptorProto, pkg string) (string, string) {
	tsType, jsonType := types(f, pkg)

	if isRepeated(f) {
		tsType = tsType + "[]"
		jsonType = jsonType + "[]"
	}

	return tsType, jsonType
}

func types(f *descriptor.FieldDescriptorProto, pkg string) (tsType string, jsonType string) {
	tsType = "string"
	jsonType = "string"

	switch f.GetType() {
	case descriptor.FieldDescriptorProto_TYPE_DOUBLE,
		descriptor.FieldDescriptorProto_TYPE_FIXED32,
		descriptor.FieldDescriptorProto_TYPE_FIXED64,
		descriptor.FieldDescriptorProto_TYPE_INT32,
		descriptor.FieldDescriptorProto_TYPE_INT64:
		tsType = "number"
		jsonType = "number"
	case descriptor.FieldDescriptorProto_TYPE_STRING:
		tsType = "string"
		jsonType = "string"
	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		tsType = "boolean"
		jsonType = "boolean"
	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		tsType = removePkg(f.GetTypeName(), pkg)
		jsonType = tsType
	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		name := f.GetTypeName()

		// Google WKT Timestamp is a special case here:
		//
		// Currently the value will just be left as jsonpb RFC 3339 string.
		// JSON.stringify already handles serializing Date to its RFC 3339 format.
		//
		if name == ".google.protobuf.Timestamp" {
			tsType = "Date"
			jsonType = "string"
		} else {
			tsType = removePkg(name, pkg)
			jsonType = removePkg(name, pkg) + "JSON"
		}
	}

	return
}

func isRepeated(f *descriptor.FieldDescriptorProto) bool {
	return f.GetLabel() == descriptor.FieldDescriptorProto_LABEL_REPEATED
}

func mapType(f *descriptor.FieldDescriptorProto, m *descriptor.DescriptorProto, pkg string) *string {
	typeName := f.GetTypeName()
	if typeName == "" {
		return nil
	}

	splits := strings.Split(typeName, ".")
	simpleName := splits[len(splits)-1]
	for _, n := range m.GetNestedType() {
		if n.GetName() == simpleName && n.GetOptions().GetMapEntry() {
			var valType string
			for _, e := range n.GetField() {
				if e.GetName() == "value" {
					valType, _ = types(e, pkg)
				}
			}
			s := "Dictionary<" + valType + ">"
			return &s
		}
	}
	return nil
}

func removePkg(s string, pkg string) string {
	return strings.Replace(strings.TrimPrefix(s, "."+pkg+"."), ".", "", -1)
}

func camelCase(s string) string {
	parts := strings.Split(s, "_")

	for i, p := range parts {
		if i == 0 {
			parts[i] = strings.ToLower(p)
		} else {
			parts[i] = strings.ToUpper(p[0:1]) + strings.ToLower(p[1:])
		}
	}

	return strings.Join(parts, "")
}

func stringify(f ModelField) string {
	if f.IsRepeated {
		if f.Type == "Date" {
			return fmt.Sprintf("m.%s.map((n) => n.toISOString())", f.Name)
		} else if f.MapType != nil {
			return fmt.Sprintf("%sMapToJSON(m.%s)", strings.TrimSuffix(f.Type, "Entry"), f.Name)
		} else if f.IsMessage {
			return fmt.Sprintf("m.%s.map(%sToJSON)", f.Name, strings.TrimSuffix(f.Type, "[]"))
		}
	}

	if f.Type == "Date" {
		return fmt.Sprintf("m.%[1]s.toISOString()", f.Name)
	}

	if f.IsMessage {
		return fmt.Sprintf("%sToJSON(m.%s)", f.Type, f.Name)
	}

	return "m." + f.Name
}

func parse(f ModelField) string {
	if f.IsRepeated {
		if f.Type == "Date" {
			return fmt.Sprintf("m.%s.map((n) => new Date(n))", f.JSONName)
		} else if f.MapType != nil {
			return fmt.Sprintf("JSONTo%sMap(m.%s)", strings.TrimSuffix(f.Type, "Entry"), f.JSONName)
		} else if f.IsMessage {
			return fmt.Sprintf("m.%s.map(JSONTo%s)", f.JSONName, strings.TrimSuffix(f.Type, "[]"))
		}
	}

	if f.Type == "Date" {
		return fmt.Sprintf("new Date(m.%[1]s)", f.JSONName)
	}

	if f.IsMessage {
		return fmt.Sprintf("JSONTo%s(m.%s)", f.Type, f.JSONName)
	}

	return "m." + f.JSONName
}
