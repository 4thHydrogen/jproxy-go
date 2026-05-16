package util

import "testing"

func TestCountMergeAndTrimXML(t *testing.T) {
	xml1 := `<?xml version="1.0"?><rss><channel><item><title>a</title></item></channel></rss>`
	xml2 := `<?xml version="1.0"?><rss><channel><item><title>b</title></item><item><title>c</title></item></channel></rss>`

	if CountItems(xml1) != 1 {
		t.Fatalf("unexpected item count for xml1")
	}
	if CountItems(xml2) != 2 {
		t.Fatalf("unexpected item count for xml2")
	}

	merged := MergeXML(xml1, xml2)
	if CountItems(merged) != 3 {
		t.Fatalf("unexpected merged item count: %s", merged)
	}

	trimmed := RemoveOverflowItems(merged, 2)
	if CountItems(trimmed) != 2 {
		t.Fatalf("unexpected trimmed item count: %s", trimmed)
	}
}
