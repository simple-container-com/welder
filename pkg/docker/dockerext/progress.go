/*
*
This entire package is a copy of Docker sources to publicize and use the ability to
run exec from within the command line seamlessly
*/
package dockerext

import "github.com/docker/docker/pkg/progress"

// lastProgressOutput is the same as progress.Output except
// that it only output with the last update. It is used in
// non terminal scenarios to suppress verbose messages
type LastProgressOutput struct {
	Output progress.Output
}

// WriteProgress formats progress information from a ProgressReader.
func (out *LastProgressOutput) WriteProgress(prog progress.Progress) error {
	if !prog.LastUpdate {
		return nil
	}

	return out.Output.WriteProgress(prog)
}
