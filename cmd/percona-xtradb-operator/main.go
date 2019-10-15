package main

import (
	"log"

	"kubedb.dev/percona-xtradb/pkg/cmds"

	"kmodules.xyz/client-go/logs"
)

func main() {
	logs.InitLogs()
	defer logs.FlushLogs()
	if err := cmds.NewRootCmd(Version).Execute(); err != nil {
		log.Fatal(err)
	}
}
