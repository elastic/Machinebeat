package cmd

import (
	"github.com/elastic/beats/libbeat/cmd/instance"
	"github.com/felix-lessoer/machinebeat/beater"

	cmd "github.com/elastic/beats/libbeat/cmd"
)

// Name of this beat
var Name = "machinebeat"

// RootCmd to handle beats cli
var RootCmd = cmd.GenRootCmdWithSettings(beater.New, instance.Settings{Name: Name})
