package app

import (
	"regexp"
	"strings"
)

func rewriteItems(xml string, rewrite func(title, description string) string) string {
	itemRe := regexp.MustCompile(`(?s)<item>(.*?)</item>`)
	titleRe := regexp.MustCompile(`(?s)<title>(.*?)</title>`)
	descRe := regexp.MustCompile(`(?s)<description>(.*?)</description>`)

	return itemRe.ReplaceAllStringFunc(xml, func(item string) string {
		titleMatch := titleRe.FindStringSubmatch(item)
		if len(titleMatch) < 2 {
			return item
		}
		description := ""
		descMatch := descRe.FindStringSubmatch(item)
		if len(descMatch) >= 2 {
			description = descMatch[1]
		}
		newTitle := rewrite(strings.TrimSpace(titleMatch[1]), strings.TrimSpace(description))
		return titleRe.ReplaceAllString(item, "<title>"+newTitle+"</title>")
	})
}
