package system

import (
	"regexp"
	"strings"

	"jproxy/core-proxy/internal/util"
)

func testRule(pattern, replacement, example string, offset int) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, line := range strings.Split(example, "\n") {
		value := re.ReplaceAllString(line, replacement)
		value = util.ExecuteOffset(value, offset)
		builder.WriteString(value)
		builder.WriteString("\n")
	}
	return builder.String(), nil
}
