package rackpic

import (
	"strings"
	"testing"
)

func TestAPatchChangesOnlyWhatItNames(t *testing.T) {
	p, err := ParsePicture(`{"gen":3,"w":100,"h":50,"items":[{"x":0,"y":0,"w":10,"h":10,"b":255},null,{"x":5,"y":5,"w":1,"h":1,"t":"a","f":1}],"ctls":{"k":[1,2,3,4]}}`)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ParsePatch(`{"gen":3,"items":{"1":{"x":1,"y":1,"w":2,"h":2,"t":"new","f":16711680},"2":null,"4":{"x":9,"y":9,"w":1,"h":1,"b":0}}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Apply(d) {
		t.Fatal("a patch with items changed nothing")
	}
	if p.Items[0] == nil || *p.Items[0].Fill != 255 {
		t.Error("an item the patch did not name was touched")
	}
	if p.Items[1] == nil || p.Items[1].Text != "new" {
		t.Error("a named item was not replaced")
	}
	if p.Items[2] != nil {
		t.Error("an item the patch took away is still there")
	}
	// Black is a fill like any other, and an element new since the
	// picture extends it.
	if len(p.Items) != 5 || p.Items[4] == nil || p.Items[4].Fill == nil || *p.Items[4].Fill != 0 {
		t.Errorf("the new element is %+v", p.Items)
	}
}

func TestAWholePictureInAPatchReplacesIt(t *testing.T) {
	p := &Picture{Gen: 1, Items: []*Item{{}}}
	d, err := ParsePatch(`{"full":{"gen":2,"w":1,"h":1,"items":[]}}`)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Apply(d) || p.Gen != 2 || len(p.Items) != 0 {
		t.Errorf("after a full patch: %+v", p)
	}
}

func TestThePageSaysWhatWentWrong(t *testing.T) {
	if _, err := ParsePicture(`{"err":"the page has no rack panel"}`); err == nil {
		t.Error("an error from the page parsed as a picture")
	}
	if err := ParseAct(`{"err":"the panel changed; try again"}`); err == nil {
		t.Error("an error from the page parsed as done")
	}
}

// The script is stamped with its own version, so a page holding an older one
// replaces it.
func TestTheScriptCarriesItsVersion(t *testing.T) {
	if strings.Contains(Script, "__RACKPIC_VERSION__") || !strings.Contains(Script, `var V = "`+version(script)+`"`) {
		t.Error("the script was not stamped")
	}
}
