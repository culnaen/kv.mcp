package index

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/culnaen/kv.mcp/internal/kv"
	"golang.org/x/tools/go/packages"
)

const maxDepends = 50

// IndexRoot indexes all Go packages under root, writing ExtractedFunction
// records to the store. Uses packages.Load with NeedName|NeedFiles|NeedSyntax|
// NeedTypes|NeedTypesInfo on default build context (host GOOS/GOARCH only).
// Errors per package are logged to stderr; indexing continues.
// Returns total count of indexed functions.
func IndexRoot(root string, store kv.Store, verbose bool, tags string) (int, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return 0, fmt.Errorf("resolve root: %w", err)
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Dir:   absRoot,
		Tests: true,
	}
	if tags != "" {
		cfg.BuildFlags = []string{"-tags=" + tags}
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return 0, fmt.Errorf("packages.Load: %w", err)
	}

	if err := store.ClearExtracted(); err != nil {
		return 0, fmt.Errorf("clear extracted: %w", err)
	}

	// Collected records before write, so we can perform the test-attachment
	// second pass before persisting. Keyed by qualified name.
	records := map[string]*kv.ExtractedFunction{}
	// Track each function's short name (last path segment) for the test heuristic.
	shortNames := map[string][]string{} // shortName -> []qualifiedName
	// Track test function locations: TestXxx body idents -> loc.
	type testRef struct {
		idents map[string]struct{}
		loc    string
	}
	var testRefs []testRef

	// Deterministic pkg iteration.
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].PkgPath < pkgs[j].PkgPath })

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				fmt.Fprintf(os.Stderr, "package %s: %s\n", pkg.PkgPath, e)
			}
			continue
		}
		if pkg.TypesInfo == nil || pkg.Fset == nil {
			continue
		}
		// Skip synthetic test-binary packages produced when Tests: true.
		if strings.HasSuffix(pkg.PkgPath, ".test") || pkg.PkgPath == "command-line-arguments" {
			continue
		}

		for _, file := range pkg.Syntax {
			fname := pkg.Fset.File(file.Pos()).Name()
			isTestFile := strings.HasSuffix(fname, "_test.go")
			relFname, err := filepath.Rel(absRoot, fname)
			if err != nil {
				relFname = fname
			}
			relFname = filepath.ToSlash(relFname)

			for _, decl := range file.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok {
					continue
				}
				if fd.Name == nil {
					continue
				}

				qname := qualifiedName(pkg.Name, fd)
				startPos := pkg.Fset.Position(fd.Pos())
				endPos := pkg.Fset.Position(fd.End())
				loc := formatLoc(relFname, startPos.Line, endPos.Line)

				// Test second-pass collection: only look at TestXxx in _test.go.
				if isTestFile && strings.HasPrefix(fd.Name.Name, "Test") && fd.Recv == nil {
					idents := map[string]struct{}{}
					if fd.Body != nil {
						ast.Inspect(fd.Body, func(n ast.Node) bool {
							if id, ok := n.(*ast.Ident); ok {
								idents[id.Name] = struct{}{}
							}
							return true
						})
					}
					testRefs = append(testRefs, testRef{idents: idents, loc: loc})
					// Fall through: still index the test function itself.
				}

				godoc := ""
				if fd.Doc != nil {
					txt := fd.Doc.Text()
					if txt != "" {
						if idx := strings.IndexByte(txt, '\n'); idx >= 0 {
							godoc = strings.TrimSpace(txt[:idx])
						} else {
							godoc = strings.TrimSpace(txt)
						}
					}
				}

				depends := extractDepends(fd, pkg)

				rec := &kv.ExtractedFunction{
					Name:      qname,
					Loc:       loc,
					Depends:   depends,
					GodocStub: godoc,
				}
				records[qname] = rec
				short := shortName(qname)
				shortNames[short] = append(shortNames[short], qname)
			}
		}
	}

	// Second pass: attach tests heuristically.
	for _, tr := range testRefs {
		// For each ident in the test body, find matching short names.
		seen := map[string]struct{}{}
		for ident := range tr.idents {
			for _, qname := range shortNames[ident] {
				if _, ok := seen[qname]; ok {
					continue
				}
				rec, ok := records[qname]
				if !ok {
					continue
				}
				// Avoid self-referencing: if the record's own loc == tr.loc, skip.
				if rec.Loc == tr.loc {
					continue
				}
				rec.Test = append(rec.Test, tr.loc)
				seen[qname] = struct{}{}
			}
		}
	}

	// Sort test/depends slices for determinism and de-dup.
	for _, rec := range records {
		rec.Depends = uniqSorted(rec.Depends)
		if len(rec.Depends) > maxDepends {
			rec.Depends = rec.Depends[:maxDepends]
		}
		rec.Test = uniqSorted(rec.Test)
	}

	// Persist in sorted order for determinism.
	names := make([]string, 0, len(records))
	for n := range records {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := store.PutExtracted(*records[n]); err != nil {
			return 0, fmt.Errorf("put extracted %s: %w", n, err)
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "indexed %s\n", n)
		}
	}

	return len(records), nil
}

// qualifiedName builds the qualified name for a function declaration.
// Top-level: pkgName.FuncName
// Method (value receiver): pkgName.(ReceiverType).MethodName
// Method (pointer receiver): pkgName.(*ReceiverType).MethodName
func qualifiedName(pkgName string, fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return pkgName + "." + fd.Name.Name
	}
	recv := fd.Recv.List[0].Type
	switch t := recv.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.(*%s).%s", pkgName, id.Name, fd.Name.Name)
		}
		// Generic pointer receiver: *T[U]
		if ix, ok := t.X.(*ast.IndexExpr); ok {
			if id, ok := ix.X.(*ast.Ident); ok {
				return fmt.Sprintf("%s.(*%s).%s", pkgName, id.Name, fd.Name.Name)
			}
		}
		if ix, ok := t.X.(*ast.IndexListExpr); ok {
			if id, ok := ix.X.(*ast.Ident); ok {
				return fmt.Sprintf("%s.(*%s).%s", pkgName, id.Name, fd.Name.Name)
			}
		}
	case *ast.Ident:
		return fmt.Sprintf("%s.(%s).%s", pkgName, t.Name, fd.Name.Name)
	case *ast.IndexExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.(%s).%s", pkgName, id.Name, fd.Name.Name)
		}
	case *ast.IndexListExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.(%s).%s", pkgName, id.Name, fd.Name.Name)
		}
	}
	return pkgName + "." + fd.Name.Name
}

// shortName returns the last dot-separated segment (method or function name).
func shortName(qname string) string {
	if idx := strings.LastIndex(qname, "."); idx >= 0 {
		return qname[idx+1:]
	}
	return qname
}

// formatLoc renders a loc string: "path:N" for single-line, "path:N-M" for multi-line.
func formatLoc(relFile string, start, end int) string {
	if end <= start {
		return fmt.Sprintf("%s:%d", relFile, start)
	}
	return fmt.Sprintf("%s:%d-%d", relFile, start, end)
}

// extractDepends walks the body of fd, collecting references to functions and
// named types declared in non-stdlib packages, formatted as "pkgname.Symbol".
func extractDepends(fd *ast.FuncDecl, pkg *packages.Package) []string {
	if fd.Body == nil {
		return nil
	}
	var out []string
	seen := map[string]struct{}{}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pkg.TypesInfo.Uses[id]
		if obj == nil {
			return true
		}
		var opkg *types.Package
		var sym string
		switch o := obj.(type) {
		case *types.Func:
			opkg = o.Pkg()
			sym = o.Name()
		case *types.TypeName:
			opkg = o.Pkg()
			sym = o.Name()
		default:
			return true
		}
		if opkg == nil {
			return true
		}
		if isStdlib(opkg.Path()) {
			return true
		}
		// Skip self-reference: same function name in the same package.
		if opkg.Path() == pkg.PkgPath && fd.Name != nil && sym == fd.Name.Name {
			return true
		}
		entry := opkg.Name() + "." + sym
		if _, ok := seen[entry]; ok {
			return true
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
		return true
	})
	return out
}

// isStdlib reports whether a package path belongs to the Go standard library.
// Heuristic: stdlib import paths contain no dot in their first segment.
func isStdlib(path string) bool {
	if path == "" {
		return true
	}
	first := path
	if i := strings.IndexByte(path, '/'); i >= 0 {
		first = path[:i]
	}
	return !strings.Contains(first, ".")
}

// uniqSorted returns a sorted, de-duplicated copy of in.
func uniqSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(in))
	for _, s := range in {
		m[s] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

