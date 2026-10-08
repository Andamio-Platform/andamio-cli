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

	// Struct names can collide across the packages now being scanned, so
	// sort on Source too. Labels can still repeat within one file (two
	// functions each binding an anonymous struct to `resp`, say), so the sort
	// is stable: ties keep source order rather than depending on
	// sort-algorithm internals.
	sort.SliceStable(structs, func(i, j int) bool {
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
//
// Structs are found at any depth, not just top-level type declarations:
// many --output json envelopes are declared inside the function that prints
// them (`type authStatus struct` in a RunE, or `payload := struct{...}{...}`),
// and a top-level-only walk silently left those out of the golden. Anonymous
// structs that decode API responses are collected too; the scanner can't
// tell them apart from output envelopes, and pinning them is harmless.
func parseFiles(files []string) ([]structSnapshot, error) {

	var allStructs []structSnapshot
	fset := token.NewFileSet()

	for _, f := range files {
		parsedFile, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("could not parse file %s: %w", f, err)
		}

		for _, decl := range parsedFile.Decls {
			// Top-level types keep their bare name; anything inside a
			// function is prefixed with it, so two functions' local
			// `verifyResult`s stay distinguishable in the golden.
			prefix := ""
			if fn, ok := decl.(*ast.FuncDecl); ok {
				prefix = funcLabel(fn) + "."
			}
			allStructs = append(allStructs, collectStructs(decl, prefix, f)...)
		}
	}

	return allStructs, nil
}

// funcLabel names a function for golden labels: `runAuthStatus`, or
// `Client.Do` for a method (pointer receivers drop the `*`).
func funcLabel(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	return types.ExprString(recv) + "." + fn.Name.Name
}

// collectStructs walks root and snapshots every json-tagged struct type in
// it, labelling each by how it's bound (see structLabel).
func collectStructs(root ast.Node, prefix, source string) []structSnapshot {
	var out []structSnapshot
	labels := map[*ast.StructType]string{}
	var stack []ast.Node
	anon := 0

	ast.Inspect(root, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}

		if st, ok := n.(*ast.StructType); ok {
			label := structLabel(stack, labels, prefix)
			if label == "" {
				anon++
				label = fmt.Sprintf("%sanon#%d", prefix, anon)
			}
			labels[st] = label

			if fields, hasJSONTag := extractFields(st); hasJSONTag {
				out = append(out, structSnapshot{Name: label, Source: source, Fields: fields})
			}
		}

		stack = append(stack, n)
		return true
	})

	return out
}

// structLabel names a struct type from its ancestors (stack, innermost
// last), or returns "" if nothing nameable binds it:
//
//	type X struct{...}           -> X
//	var x struct{...}            -> x
//	x := struct{...}{...}        -> x   (also []struct{...}{...}, &struct{...}{...})
//	Outer{ F struct{...} }       -> Outer.F   (nested anonymous field)
func structLabel(stack []ast.Node, labels map[*ast.StructType]string, prefix string) string {
	i := len(stack) - 1

	// Look through []T, *T and map[K]T wrappers to what actually binds T.
	for i >= 0 {
		switch stack[i].(type) {
		case *ast.ArrayType, *ast.StarExpr, *ast.MapType:
			i--
			continue
		}
		break
	}
	if i < 0 {
		return ""
	}

	switch p := stack[i].(type) {
	case *ast.TypeSpec:
		return prefix + p.Name.Name
	case *ast.ValueSpec:
		return prefix + p.Names[0].Name
	case *ast.Field:
		// Field -> FieldList -> StructType for a field of an enclosing struct;
		// anything else (a func parameter, say) isn't nameable here.
		if i >= 2 && len(p.Names) > 0 {
			if outer, ok := stack[i-2].(*ast.StructType); ok {
				return labels[outer] + "." + p.Names[0].Name
			}
		}
	case *ast.CompositeLit:
		return compositeLitLabel(stack[:i+1], prefix)
	}
	return ""
}

// compositeLitLabel names a struct literal by the variable it's assigned to.
// stack ends with the *ast.CompositeLit.
func compositeLitLabel(stack []ast.Node, prefix string) string {
	lit := ast.Node(stack[len(stack)-1])
	i := len(stack) - 2
	if i >= 0 {
		if u, ok := stack[i].(*ast.UnaryExpr); ok && u.Op == token.AND {
			lit = u
			i--
		}
	}
	if i < 0 {
		return ""
	}

	switch p := stack[i].(type) {
	case *ast.AssignStmt:
		for j, rhs := range p.Rhs {
			if rhs == lit && j < len(p.Lhs) {
				if id, ok := p.Lhs[j].(*ast.Ident); ok {
					return prefix + id.Name
				}
			}
		}
	case *ast.ValueSpec:
		for j, v := range p.Values {
			if v == lit && j < len(p.Names) {
				return prefix + p.Names[j].Name
			}
		}
	}
	return ""
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
