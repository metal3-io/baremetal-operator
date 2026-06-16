//go:build patcher

/*

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Patches pkg/provisioner/demo/demo.go to import go.starlark.net/starlark
// and rewrite (*demoProvisioner).GetHealth so it evaluates a tiny Starlark
// expression. Used by Dockerfile.plugin-test to verify that an out-of-tree
// plugin can pull in deps outside BMO's go.mod and still build and work.
//
// Behind the `patcher` build tag so `go build ./...` skips it.
package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
)

const newBodySrc = `package demo

func _() string {
	thread := &starlark.Thread{Name: "health"}
	val, err := starlark.Eval(thread, "health.star", ` + "`\"healthy\"`" + `, nil)
	if err != nil {
		return ""
	}
	return val.String()
}
`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: patcher <demo.go path>")
		os.Exit(2)
	}

	path := os.Args[1]
	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		die("parse %s: %v", path, err)
	}

	if !addImport(file, "go.starlark.net/starlark") {
		die("no import block in %s", path)
	}

	bodyFile, err := parser.ParseFile(fset, "body.go", newBodySrc, 0)
	if err != nil {
		die("parse new body: %v", err)
	}

	newBody := bodyFile.Decls[0].(*ast.FuncDecl).Body

	if !replaceMethodBody(file, "demoProvisioner", "GetHealth", newBody) {
		die("did not find (*demoProvisioner).GetHealth in %s", path)
	}

	out, err := os.Create(path)
	if err != nil {
		die("open %s: %v", path, err)
	}
	defer out.Close()

	if err := format.Node(out, fset, file); err != nil {
		die("format: %v", err)
	}
}

func addImport(file *ast.File, path string) bool {
	quoted := fmt.Sprintf("%q", path)

	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}

		for _, spec := range gd.Specs {
			if is, ok := spec.(*ast.ImportSpec); ok && is.Path.Value == quoted {
				return true
			}
		}

		gd.Specs = append(gd.Specs, &ast.ImportSpec{
			Path: &ast.BasicLit{Kind: token.STRING, Value: quoted},
		})

		gd.Lparen = gd.Specs[0].Pos()

		return true
	}

	return false
}

func replaceMethodBody(file *ast.File, recvType, methodName string, body *ast.BlockStmt) bool {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != methodName {
			continue
		}

		if !isPointerReceiver(fn.Recv, recvType) {
			continue
		}

		fn.Body = body

		return true
	}

	return false
}

func isPointerReceiver(recv *ast.FieldList, name string) bool {
	if recv == nil || len(recv.List) == 0 {
		return false
	}

	star, ok := recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}

	id, ok := star.X.(*ast.Ident)
	return ok && id.Name == name
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}
