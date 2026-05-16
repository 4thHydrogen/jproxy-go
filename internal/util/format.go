package util

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	Placeholder          = " "
	PlaceholderSeparator = "/"
	TokenRegex           = `\{([^}]+)\}`
)

var (
	regexSeparator   = regexp.MustCompile(`[\\[\\]銆愩€慮]`)
	regexSpecialChar = regexp.MustCompile(`[\\$\\(\\)\\*\\+\\.\\?\\^\\{\\}\\|\\\\]`)
	regexArticle     = regexp.MustCompile(`(\b|\s)((?i)a|an|the)\s`)
	placeholders     = regexp.MustCompile(`\s+`)
	separators       = regexp.MustCompile(`/+`)
)

func CleanTitle(title, removeRegex string) string {
	title = regexSeparator.ReplaceAllString(title, PlaceholderSeparator)
	title = regexSpecialChar.ReplaceAllString(title, Placeholder)
	cleanTitle := regexp.MustCompile(removeRegex).ReplaceAllString(title, Placeholder)
	cleanTitle = regexArticle.ReplaceAllString(cleanTitle, Placeholder)
	if strings.TrimSpace(cleanTitle) == "" {
		cleanTitle = title
	}
	cleanTitle = separators.ReplaceAllString(cleanTitle, PlaceholderSeparator)
	cleanTitle = placeholders.ReplaceAllString(cleanTitle, Placeholder)
	return strings.ToLower(strings.TrimSpace(cleanTitle))
}

func RemoveYear(title string) string {
	return regexp.MustCompile(` (19|20)\d{2}$`).ReplaceAllString(title, "")
}

func RemoveEpisode(title string) string {
	return regexp.MustCompile(` 0*(\d{1,3}|1[0-8]\d{2})$`).ReplaceAllString(title, "")
}

func RemoveSeason(title string) string {
	return regexp.MustCompile(` (S\d+)$`).ReplaceAllString(title, "")
}

func RemoveSeasonEpisode(title string) string {
	return regexp.MustCompile(` (S\d+ |)\d+$`).ReplaceAllString(title, "")
}

func ReplaceToken(token, value, text string) string {
	return strings.ReplaceAll(text, "{"+token+"}", value)
}

func ReplaceTokenWithOffset(token, value, text string, offset int) string {
	return ReplaceToken(token, ExecuteOffset(value, offset), text)
}

func ExecuteOffset(value string, offset int) string {
	if offset == 0 {
		return value
	}
	return regexp.MustCompile(`(\d+)`).ReplaceAllStringFunc(value, func(match string) string {
		number, err := strconv.Atoi(match)
		if err != nil {
			return match
		}
		return fmt.Sprint(number + offset)
	})
}

func RemoveAllToken(text string) string {
	return regexp.MustCompile(TokenRegex).ReplaceAllString(text, "")
}
