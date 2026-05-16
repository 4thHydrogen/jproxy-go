package util

import "strings"

func CountItems(xml string) int {
	count := 0
	needle := "<item>"
	index := 0
	for {
		found := strings.Index(xml[index:], needle)
		if found == -1 {
			return count
		}
		count++
		index += found + len(needle)
	}
}

func MergeXML(current, incoming string) string {
	if current == "" {
		return incoming
	}
	if incoming == "" {
		return current
	}
	start := strings.Index(current, "<item>")
	if start == -1 {
		return incoming
	}
	incomingStart := strings.Index(incoming, "<item>")
	if incomingStart == -1 {
		return current
	}
	return current[:strings.Index(current, "</channel>")] + incoming[incomingStart:]
}

func RemoveOverflowItems(xml string, limit int) string {
	count := 0
	index := 0
	itemPrefix := "<item>"
	itemSuffix := "</item>"
	for count <= limit {
		next := strings.Index(xml[index+1:], itemPrefix)
		if next == -1 {
			return xml
		}
		index += next + 1
		count++
	}
	last := strings.LastIndex(xml, itemSuffix)
	if last == -1 {
		return xml
	}
	return xml[:index] + xml[last+len(itemSuffix):]
}
