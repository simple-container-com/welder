package render

import (
	"bytes"
	"fmt"
)

type ArgList struct {
	b *bytes.Buffer
}

func NewArgList() *ArgList {
	return &ArgList{&bytes.Buffer{}}
}

func (a *ArgList) Add(flag, value string) *ArgList {
	if a.b.Len() > 0 {
		fmt.Fprint(a.b, " ")
	}
	fmt.Fprint(a.b, flag, " ", value)
	return a
}

func (a *ArgList) Append(args string) *ArgList {
	if len(args) == 0 {
		return a
	}
	if a.b.Len() > 0 {
		fmt.Fprint(a.b, " ")
	}
	fmt.Fprint(a.b, args)
	return a
}

func (a *ArgList) String() string {
	return a.b.String()
}
