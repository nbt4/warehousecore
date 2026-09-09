package services

import (
	"strings"
	"testing"
)

func TestRenderPackingListHTMLEscapesContentAndIndentsAccessories(t *testing.T) {
	html, err := RenderPackingListHTML(&PackingList{
		JobID:       12,
		JobCode:     "JOB0012",
		Title:       `<script>alert("x")</script>`,
		CompanyName: "Tsunami & Events",
		BarcodeURI:  "data:image/png;base64,dGVzdA==",
		Items: []PackingListItem{
			{Name: "Lautsprecher", Quantity: 2, Unit: "Stück"},
			{Name: "Kabel <lang>", Quantity: 1.5, Unit: "m", Depth: 1, Accessory: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`&lt;script&gt;alert`,
		`Tsunami &amp; Events`,
		`Kabel &lt;lang&gt;`,
		`padding-left: 6.0mm`,
		`1.5 m`,
		`data:image/png;base64,dGVzdA==`,
	} {
		if !strings.Contains(html, expected) {
			t.Errorf("rendered HTML does not contain %q", expected)
		}
	}
	if strings.Contains(html, `<script>alert("x")</script>`) {
		t.Error("job title was not escaped")
	}
}
