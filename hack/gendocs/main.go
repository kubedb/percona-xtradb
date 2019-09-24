package main

import (
	"fmt"
	"log"
	"os"

	"github.com/appscode/go/runtime"
	"github.com/spf13/cobra/doc"
	"kubedb.dev/percona-xtradb/pkg/cmds"
)

// ref: https://github.com/spf13/cobra/blob/master/doc/md_docs.md
func main() {
	rootCmd := cmds.NewRootCmd("")
	dir := runtime.GOPath() + "/src/kubedb.dev/percona-xtradb/docs/reference"
	fmt.Printf("Generating cli markdown tree in: %v\n", dir)
	err := os.RemoveAll(dir)
	if err != nil {
		log.Fatal(err)
	}
	err = os.MkdirAll(dir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	err = doc.GenMarkdownTree(rootCmd, dir)
	if err != nil {
		log.Fatal(err)
	}
}
