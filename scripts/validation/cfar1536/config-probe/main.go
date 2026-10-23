package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	grootConfig "code.cloudfoundry.org/guardian/grootfs/commands/config"
	"code.cloudfoundry.org/guardian/guardiancmd"
	flags "github.com/jessevdk/go-flags"
)

func main() {
	backend := flag.String("backend", "", "Expected stock or runsc backend")
	selfTest := flag.Bool("self-test", false, "Run parser controls without installed files")
	flag.Parse()
	if *backend != "stock" && *backend != "runsc" {
		panic("valid backend required")
	}
	for _, control := range []struct {
		input, runtime   string
		processes, valid bool
	}{
		{"[server]\ncontainerd-runtime-type=io.containerd.runsc.v1\nuse-containerd-for-processes=true\n", "io.containerd.runsc.v1", true, true},
		{"[server]\nuse-containerd-for-processes=false\n", "", false, true},
		{"[server]\ncontainerd-runtime-typo=io.containerd.runsc.v1\n", "", false, false},
		{"[server]\nuse-containerd-for-processes=maybe\n", "", false, false},
	} {
		cmd := &guardiancmd.GdnCommand{ServerCommand: &guardiancmd.ServerCommand{}}
		parser := flags.NewParser(cmd, flags.Default)
		parser.NamespaceDelimiter = "-"
		err := flags.NewIniParser(parser).Parse(strings.NewReader(control.input))
		if (err == nil) != control.valid {
			panic(fmt.Sprintf("parser control validity: %v", err))
		}
		if control.valid && (cmd.ServerCommand.Containerd.RuntimeType != control.runtime || cmd.ServerCommand.Containerd.UseContainerdForProcesses != control.processes) {
			panic("parser control values differ")
		}
	}
	if *selfTest {
		fmt.Println("PASS: four real-parser controls")
		return
	}
	cmd := &guardiancmd.GdnCommand{ServerCommand: &guardiancmd.ServerCommand{}}
	parser := flags.NewParser(cmd, flags.Default)
	parser.NamespaceDelimiter = "-"
	if err := flags.NewIniParser(parser).ParseFile("/var/vcap/jobs/garden/config/config.ini"); err != nil {
		panic(err)
	}
	containerd := cmd.ServerCommand.Containerd
	if *backend == "runsc" && (containerd.RuntimeType != "io.containerd.runsc.v1" || !containerd.UseContainerdForProcesses) {
		panic("runsc effective config mismatch")
	}
	if *backend == "stock" && (containerd.RuntimeType != "" || containerd.UseContainerdForProcesses) {
		panic("stock opt-in defaults changed")
	}
	quota := map[string]bool{}
	for _, name := range []string{"grootfs_config.yml", "privileged_grootfs_config.yml"} {
		builder, err := grootConfig.NewBuilder("/var/vcap/jobs/garden/config/" + name)
		if err != nil {
			panic(err)
		}
		config, err := builder.Build()
		if err != nil {
			panic(err)
		}
		quota[name] = config.Create.ExcludeImageFromQuota
		if config.Create.ExcludeImageFromQuota != (*backend == "runsc") {
			panic("effective quota exclusion differs from declared shard property")
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"backend": *backend, "runtime_type": containerd.RuntimeType, "containerd_processes": containerd.UseContainerdForProcesses, "quota_exclusion": quota, "parser_controls": 4}); err != nil {
		panic(err)
	}
	fmt.Println("PASS: exact candidate parsers accept installed config and reject invalid property/boolean controls; no server executed")
}
