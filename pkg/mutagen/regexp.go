package mutagen

import (
	"regexp"
)

func MatchGroupsWithNames(expr *regexp.Regexp, text string) map[string]string {
	match := expr.FindStringSubmatch(text)
	result := make(map[string]string)
	if match == nil {
		return result
	}
	for i, name := range expr.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = match[i]
		}
	}
	return result
}
