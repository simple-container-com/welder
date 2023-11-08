package util

import (
	"hash/fnv"
	"strconv"
)

func Hash(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return strconv.Itoa(int(h.Sum32()))
}

func LastNChars(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
