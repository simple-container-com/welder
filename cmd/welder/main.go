package main

import (
	"os"

	"github.com/alecthomas/kingpin"
	"github.com/simple-container-com/welder/pkg/cli/build"
)

// Version is set during build (see Makefile).
var Version string

func main() {
	os.Exit(run(os.Args))
}

func run(args []string) int {
	app := kingpin.New("welder", "Welder allows to orchestrate day-to-day build tasks using containers").Version(Version)

	(&build.Init{}).Mount(app)
	(&build.Make{}).Mount(app)
	(&build.Deploy{}).Mount(app)
	(&build.Docker{}).Mount(app)
	(&build.All{}).Mount(app)
	(&build.Run{}).Mount(app)
	(&build.Version{}).Mount(app)
	(&build.Volumes{}).Mount(app)
	(&build.Mutagen{}).Mount(app)

	// The `mutagen` command passes all arguments to the underlying `mutagen` command directly
	// All other commands will go through to our kingpin application which we can manage directly here.

	if len(args) > 1 && args[1] == "mutagen" {
		kingpin.MustParse(app.Parse(args[1:2]))
		return 0
	}

	kingpin.MustParse(app.Parse(args[1:]))
	return 0
}
