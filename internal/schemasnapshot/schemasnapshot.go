package schemasnapshot

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

type fieldSnapshot struct {
	Name string
	Type string
	Tag  string
}

type structSnapshot struct {
	Name   string
	Source string
	Fields []fieldSnapshot
}

func Generate(srcDirs []string, outPath string) error {

	var filteredFilePaths []string

	for _, root := range srcDirs {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			if strings.Contains(d.Name(), "_test") {
				return nil
			}

			filteredFilePaths = append(filteredFilePaths, path)
			return nil
		})
		if err != nil {
			return fmt.Errorf("walking %s: %w", root, err)
		}
	}

	structs, err := parseFiles(filteredFilePaths)
	if err != nil {
		return err
	}

	// Struct names can collide across the packages now being scanned, and
	// sort.Slice isn't stable — sorting on Source too gives every entry a
	// unique key so tie-breaking can't depend on sort-algorithm internals.
	sort.Slice(structs, func(i, j int) bool {
		if structs[i].Source != structs[j].Source {
			return structs[i].Source < structs[j].Source
		}
		return structs[i].Name < structs[j].Name
	})

	var b strings.Builder
	for _, s := range structs {
		for _, f := range s.Fields {
			fmt.Fprintf(&b, "%s.%s %s json:%q\n", s.Name, f.Name, f.Type, f.Tag)
		}
	}

	if err := os.WriteFile(outPath, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("writing snapshot to %s: %w", outPath, err)
	}

	return nil
}

// parseFiles parses each file and collects every struct that has at least
// one json-tagged field. Structs with no json tags at all are assumed to
// never be marshalled to JSON and are skipped.
func parseFiles(files []string) ([]structSnapshot, error) {

	var allStructs []structSnapshot
	fset := token.NewFileSet()

	for _, f := range files {
		parsedFile, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("could not parse file %s: %w", f, err)
		}

		for _, decl := range parsedFile.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				fields, hasJSONTag := extractFields(structType)
				if !hasJSONTag {
					continue
				}

				allStructs = append(allStructs, structSnapshot{
					Name:   typeSpec.Name.Name,
					Source: f,
					Fields: fields,
				})
			}
		}
	}

	return allStructs, nil
}

// extractFields returns every exported field of structType (embedded fields
// included, keyed by their type name) plus whether at least one field
// carries a json tag. Fields tagged json:"-" are still recorded, since
// excluding a field from output is part of the JSON shape too.
func extractFields(structType *ast.StructType) ([]fieldSnapshot, bool) {
	var fields []fieldSnapshot
	hasJSONTag := false

	for _, field := range structType.Fields.List {
		typeStr := types.ExprString(field.Type)

		tag := ""
		if field.Tag != nil {
			unquoted, err := strconv.Unquote(field.Tag.Value)
			if err == nil {
				tag = reflect.StructTag(unquoted).Get("json")
			}
		}
		if tag != "" {
			hasJSONTag = true
		}

		if len(field.Names) == 0 {
			// embedded field, e.g. `SomeType` with no explicit name
			fields = append(fields, fieldSnapshot{Name: typeStr, Type: typeStr, Tag: tag})
			continue
		}

		for _, name := range field.Names {
			if !name.IsExported() {
				continue
			}
			fields = append(fields, fieldSnapshot{Name: name.Name, Type: typeStr, Tag: tag})
		}
	}

	return fields, hasJSONTag
}
