package build

import (
	"context"
	"os"
	"strings"

	"github.com/alecthomas/kingpin"
	"github.com/pkg/errors"
	"github.com/simple-container-com/welder/pkg/mutagen"
	"github.com/simple-container-com/welder/pkg/util"
)

type Mutagen struct{}

func (o *Mutagen) Mount(a *kingpin.Application) *kingpin.CmdClause {
	cmd := a.Command("mutagen", "Run mutagen commands (delegate to mutagen.io)")
	appVersion = a.Model().Version
	cmd.Action(registerAction(o.Mutagen))
	return cmd
}

func (o *Mutagen) Mutagen() error {
	goCtx, _ := context.WithCancel(context.Background())
	logger := util.NewPrefixLogger("[mutagen]", true)
	mutagenProc, err := mutagen.New(goCtx, logger)
	if err != nil {
		return errors.Wrapf(err, "failed to init mutagen")
	}
	return mutagenProc.RunCommand(strings.Join(os.Args[2:], " "))
}
