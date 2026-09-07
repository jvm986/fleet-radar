// Command tsgen writes the TypeScript the web client compiles against, from the Go types that are the
// contract. The direction of generation matches the direction of authority: the backend consumes
// events and derives state, and the client consumes the backend's output (ADR-0001 §1.4).
//
// It starts from the three message kinds and emits only what is reachable from them, so a type reaches
// the client only if the client can actually be sent it. Inbound events are absent by construction
// rather than by exclusion — nothing has to remember to leave them out.
//
// Output goes to stdout, so `make check` can diff a fresh generation against the committed file.
// Go-as-the-source-of-truth holds only while drift is detectable, and a committed file with no gate is
// a file that silently stops being generated (ADR-0001 consequences, ADR-0009 §9.7).
//
// Usage: go run ./cmd/tsgen, from the backend module root.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// sourceDir is the package that is the contract. Relative, because this is run by the Makefile from
// the module root and nowhere else.
const sourceDir = "contract"

// roots are the client's whole vocabulary: the three messages it is sent, and the kinds that name
// them. Everything else in the output is reachable from these.
var roots = []string{"MessageKind", "Config", "Routes", "Snapshot"}

// closedMarker says a string type's constants are the complete set, so it can be emitted as a union.
// Without it a string type becomes an alias for string — which is right for zone ids, whose values
// come from the checked-in geometry rather than from Go, and where a union would be a lie.
const closedMarker = "tsgen:closed"

func main() {
	if err := run(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "tsgen: %v\n", err)
		os.Exit(1)
	}
}

// declaration is one named type from the contract, with the constants declared for it.
type declaration struct {
	name      string
	doc       string
	spec      ast.Expr
	closed    bool
	constants []constant
}

type constant struct {
	name  string
	value string
}

func run(out *os.File) error {
	files, err := parse()
	if err != nil {
		return err
	}

	declarations, order := collect(files)
	reachable, err := walk(declarations, roots)
	if err != nil {
		return err
	}

	fmt.Fprint(out, header)
	for _, name := range order {
		if !reachable[name] {
			continue
		}
		rendered, err := emit(declarations[name])
		if err != nil {
			return err
		}
		fmt.Fprint(out, rendered)
	}
	return nil
}

// parse reads the contract's source, by file name so the output order is stable.
func parse() ([]*ast.File, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", sourceDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	fileSet := token.NewFileSet()
	files := make([]*ast.File, 0, len(names))
	for _, name := range names {
		file, err := parser.ParseFile(fileSet, filepath.Join(sourceDir, name), nil, parser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		files = append(files, file)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no source in %s", sourceDir)
	}
	return files, nil
}

// collect reads every named type and constant, in source order, so the TypeScript reads in the same
// order as the Go rather than in whatever order a map produced.
func collect(files []*ast.File) (map[string]*declaration, []string) {
	declarations := map[string]*declaration{}
	var order []string

	for _, file := range files {
		for _, decl := range file.Decls {
			general, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}

			switch general.Tok {
			case token.TYPE:
				for _, spec := range general.Specs {
					typeSpec := spec.(*ast.TypeSpec)
					doc := typeSpec.Doc
					if doc == nil {
						doc = general.Doc
					}
					declarations[typeSpec.Name.Name] = &declaration{
						name:   typeSpec.Name.Name,
						doc:    text(doc),
						spec:   typeSpec.Type,
						closed: strings.Contains(rawText(doc), closedMarker),
					}
					order = append(order, typeSpec.Name.Name)
				}

			case token.CONST:
				for _, spec := range general.Specs {
					valueSpec, ok := spec.(*ast.ValueSpec)
					if !ok || valueSpec.Type == nil || len(valueSpec.Values) != 1 {
						continue
					}
					named, ok := valueSpec.Type.(*ast.Ident)
					if !ok {
						continue
					}
					literal, ok := valueSpec.Values[0].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					owner, known := declarations[named.Name]
					if !known {
						continue
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil {
						continue
					}
					owner.constants = append(owner.constants, constant{name: valueSpec.Names[0].Name, value: value})
				}
			}
		}
	}
	return declarations, order
}

// walk finds everything reachable from the roots, so the output is complete without being wider than
// the client's actual vocabulary.
func walk(declarations map[string]*declaration, from []string) (map[string]bool, error) {
	reachable := map[string]bool{}

	var visit func(string) error
	visit = func(name string) error {
		if reachable[name] {
			return nil
		}
		declaration, known := declarations[name]
		if !known {
			return fmt.Errorf("%s is referenced but not declared in the contract", name)
		}
		reachable[name] = true

		structure, ok := declaration.spec.(*ast.StructType)
		if !ok {
			return nil
		}
		for _, field := range structure.Fields.List {
			for _, dependency := range references(field.Type) {
				if _, known := declarations[dependency]; !known {
					continue
				}
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		return nil
	}

	for _, name := range from {
		if err := visit(name); err != nil {
			return nil, err
		}
	}
	return reachable, nil
}

func emit(declaration *declaration) (string, error) {
	var out strings.Builder
	out.WriteString("\n")
	out.WriteString(comment(declaration.doc, ""))

	switch spec := declaration.spec.(type) {
	case *ast.StructType:
		out.WriteString(fmt.Sprintf("export interface %s {\n", declaration.name))
		for i, field := range spec.Fields.List {
			name, optional, err := jsonName(field)
			if err != nil {
				return "", fmt.Errorf("%s: %w", declaration.name, err)
			}
			rendered, err := typeOf(field.Type)
			if err != nil {
				return "", fmt.Errorf("%s.%s: %w", declaration.name, name, err)
			}

			if doc := comment(text(field.Doc), "  "); doc != "" {
				if i > 0 {
					out.WriteString("\n")
				}
				out.WriteString(doc)
			}
			out.WriteString(fmt.Sprintf("  %s%s: %s;\n", name, optionalMark(optional), rendered))
		}
		out.WriteString("}\n")

	case *ast.Ident:
		if spec.Name != "string" {
			return "", fmt.Errorf("%s: only string is supported as a named scalar, not %s", declaration.name, spec.Name)
		}
		if declaration.closed {
			if len(declaration.constants) == 0 {
				return "", fmt.Errorf("%s is marked closed but declares no constants", declaration.name)
			}
			values := make([]string, len(declaration.constants))
			for i, member := range declaration.constants {
				values[i] = strconv.Quote(member.value)
			}
			out.WriteString(fmt.Sprintf("export type %s = %s;\n", declaration.name, strings.Join(values, " | ")))
			break
		}

		// Open: the values are not all declared here, so a union would be a lie. The constants that
		// are declared here still travel, because the client comparing against a literal of its own
		// is the drift this whole file exists to prevent.
		out.WriteString(fmt.Sprintf("export type %s = string;\n", declaration.name))
		for _, member := range declaration.constants {
			out.WriteString(fmt.Sprintf("export const %s: %s = %s;\n", member.name, declaration.name, strconv.Quote(member.value)))
		}

	case *ast.ArrayType:
		element, err := typeOf(spec.Elt)
		if err != nil {
			return "", fmt.Errorf("%s: %w", declaration.name, err)
		}
		length, ok := spec.Len.(*ast.BasicLit)
		if !ok {
			return "", fmt.Errorf("%s: only fixed-length arrays are supported as named types", declaration.name)
		}
		size, err := strconv.Atoi(length.Value)
		if err != nil {
			return "", fmt.Errorf("%s: %w", declaration.name, err)
		}
		out.WriteString(fmt.Sprintf("export type %s = [%s];\n", declaration.name, strings.Repeat(element+", ", size-1)+element))

	default:
		return "", fmt.Errorf("%s: unsupported declaration", declaration.name)
	}
	return out.String(), nil
}

// typeOf is the whole of the Go-to-TypeScript mapping. Anything not listed here is an error rather
// than a guess: a contract that grew a shape this does not understand should fail the build, not
// produce TypeScript that quietly disagrees with the wire.
func typeOf(expr ast.Expr) (string, error) {
	switch node := expr.(type) {
	case *ast.Ident:
		switch node.Name {
		case "string":
			return "string", nil
		case "bool":
			return "boolean", nil
		case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "float32", "float64":
			return "number", nil
		default:
			return node.Name, nil
		}

	case *ast.SelectorExpr:
		qualified := fmt.Sprintf("%s.%s", node.X, node.Sel)
		switch qualified {
		// Timestamps travel as RFC 3339 strings, which is what encoding/json produces.
		case "time.Time":
			return "string", nil
		// Raw JSON is passed through untouched — the service-area geometry, which the client hands
		// straight to the map.
		case "json.RawMessage":
			return "unknown", nil
		default:
			return "", fmt.Errorf("no mapping for %s", qualified)
		}

	case *ast.ArrayType:
		element, err := typeOf(node.Elt)
		if err != nil {
			return "", err
		}
		if node.Len == nil {
			return element + "[]", nil
		}
		size, err := strconv.Atoi(node.Len.(*ast.BasicLit).Value)
		if err != nil {
			return "", err
		}
		return "[" + strings.Repeat(element+", ", size-1) + element + "]", nil

	// Go-shaped optionality reaches the client as it is: a pointer is a value that may be absent.
	case *ast.StarExpr:
		inner, err := typeOf(node.X)
		if err != nil {
			return "", err
		}
		return inner + " | null", nil

	default:
		return "", fmt.Errorf("unsupported type")
	}
}

// references lists the named types a field's type mentions.
func references(expr ast.Expr) []string {
	switch node := expr.(type) {
	case *ast.Ident:
		return []string{node.Name}
	case *ast.ArrayType:
		return references(node.Elt)
	case *ast.StarExpr:
		return references(node.X)
	default:
		return nil
	}
}

func jsonName(field *ast.Field) (string, bool, error) {
	if field.Tag == nil {
		return "", false, fmt.Errorf("no json tag, so the wire name is not stated")
	}
	tag, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return "", false, err
	}

	_, rest, found := strings.Cut(tag, `json:"`)
	if !found {
		return "", false, fmt.Errorf("no json tag, so the wire name is not stated")
	}
	value, _, _ := strings.Cut(rest, `"`)
	name, options, _ := strings.Cut(value, ",")
	if name == "" || name == "-" {
		return "", false, fmt.Errorf("json tag %q does not name a field", value)
	}
	return name, strings.Contains(options, "omitempty"), nil
}

func optionalMark(optional bool) string {
	if optional {
		return "?"
	}
	return ""
}

// comment carries the Go documentation through, so the reasoning arrives where the client developer
// reads it. It cannot drift, because it is generated.
func comment(doc, indent string) string {
	if doc == "" {
		return ""
	}

	var out strings.Builder
	out.WriteString(indent + "/**\n")
	for _, line := range strings.Split(doc, "\n") {
		out.WriteString(strings.TrimRight(indent+" * "+line, " ") + "\n")
	}
	out.WriteString(indent + " */\n")
	return out.String()
}

func text(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimRight(group.Text(), "\n"), "\n") {
		if strings.Contains(line, closedMarker) {
			continue
		}
		lines = append(lines, line)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func rawText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	var out strings.Builder
	for _, line := range group.List {
		out.WriteString(line.Text)
	}
	return out.String()
}

const header = `// Code generated by cmd/tsgen from the Go contract. DO NOT EDIT.
//
// Regenerate with ` + "`make generate`" + `. ` + "`make check`" + ` fails if this file is out of date, because
// Go being the single source of truth holds only while drift is visible.
`
