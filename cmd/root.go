package cmd

import (
	"os"

	"github.com/linuxsuren/api-testing/pkg/version"
	fakeruntime "github.com/linuxsuren/go-fake-runtime"
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root command
func NewRootCmd(execer fakeruntime.Execer, gRPCServer gRPCServer) (c *cobra.Command) {
	c = &cobra.Command{
		Use:   "atest",
		Short: "API testing tool",
	}
	c.SetOut(os.Stdout)
	c.Version = version.GetVersion()
	c.AddCommand(createInitCommand(execer),
		createRunCommand(), createSampleCmd(),
		createServerCmd(gRPCServer), createJSONSchemaCmd(),
		createServiceCommand(execer))
	return
}

type printer interface {
	Println(i ...interface{})
}

func println(printer printer, err error, i ...interface{}) {
	if err != nil {
		printer.Println(i...)
	}
}
