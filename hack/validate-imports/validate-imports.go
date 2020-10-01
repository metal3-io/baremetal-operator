package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const local = "github.com/metal3-io/baremetal-operator"

type importSpecs []*ast.ImportSpec

func (is importSpecs) Len() int           { return len(is) }
func (is importSpecs) Less(i, j int) bool { return is[i].Path.Value < is[j].Path.Value }
func (is importSpecs) Swap(i, j int)      { is[i], is[j] = is[j], is[i] }

var _ sort.Interface = importSpecs{}

type importType int

// at most one import group of each type may exist in a validated source file,
// specifically in the order declared below
const (
	importStd   importType = 1 << iota // go standard library
	importDot                          // "." imports (ginkgo and gomega)
	importOther                        // non-local imports
	importLocal                        // local imports
)

func typeForImport(imp *ast.ImportSpec) importType {
	path := strings.Trim(imp.Path.Value, `"`)

	//fmt.Printf("typeForImport %s\n", path)
	switch {
	case imp.Name != nil && imp.Name.Name == ".":
		return importDot
	case strings.HasPrefix(path, local+"/"):
		return importLocal
	case strings.ContainsRune(path, '.'):
		return importOther
	default:
		return importStd
	}
}

func validateImport(imp *ast.ImportSpec) (errs []error) {
	path := strings.Trim(imp.Path.Value, `"`)

	switch typeForImport(imp) {
	case importDot:
		switch path {
		case "github.com/onsi/ginkgo",
			"github.com/onsi/gomega",
			"github.com/onsi/gomega/gstruct":
		default:
			errs = append(errs, fmt.Errorf("invalid . import %s", imp.Path.Value))
		}
	}

	return
}

func check(path string) (errs []error) {
	var fset token.FileSet

	f, err := parser.ParseFile(&fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return []error{err}
	}

	var groups [][]*ast.ImportSpec

	for i, imp := range f.Imports {
		// if there's more than one line between this and the previous import,
		// break open a new import group
		if i == 0 || fset.Position(f.Imports[i].Pos()).Line-fset.Position(f.Imports[i-1].Pos()).Line > 1 {
			groups = append(groups, []*ast.ImportSpec{})
		}

		groups[len(groups)-1] = append(groups[len(groups)-1], imp)
	}

	// seenTypes holds a bitmask of the importTypes seen up to this point, so
	// that we can detect duplicate groups.  We can also detect misordered
	// groups, because when we set a bit (say 0b0100), we actually set all the
	// trailing bits (0b0111) as sentinels
	var seenTypes importType

	for groupnum, group := range groups {
		if !sort.IsSorted(importSpecs(group)) {
			errs = append(errs, fmt.Errorf("group %d: imports are not sorted", groupnum+1))
		}

		groupImportType := typeForImport(group[0])
		//fmt.Printf("groupImportType %d: %s\n", groupImportType, group[0].Path.Value)

		if (seenTypes & groupImportType) != 0 { // check if single bit is already set...
			errs = append(errs, fmt.Errorf("group %d(%d): duplicate group or invalid group ordering", groupnum+1, groupImportType))
		}
		seenTypes |= groupImportType<<1 - 1 // ...but set all trailing bits

		for _, imp := range group {
			errs = append(errs, validateImport(imp)...)
		}

		for _, imp := range group {
			if typeForImport(imp) != groupImportType {
				errs = append(errs, fmt.Errorf("group %d: mixed import type", groupnum+1))
				break
			}
		}
	}

	return
}

func main() {
	var rv int
	for _, path := range os.Args[1:] {
		if err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && strings.HasSuffix(path, ".go") {
				for _, err := range check(path) {
					fmt.Printf("%s: %v\n", path, err)
					rv = 1
				}
			}

			return nil
		}); err != nil {
			panic(err)
		}
	}
	os.Exit(rv)
}
