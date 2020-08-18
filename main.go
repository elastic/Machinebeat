package main

import (
	"os"

	"github.com/elastic/machinebeat/cmd"

	// Make sure all your modules and metricsets are linked in this file
	_ "github.com/elastic/machinebeat/include"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
