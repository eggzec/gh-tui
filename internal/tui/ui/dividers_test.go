package ui

import "testing"

func TestDividers(t *testing.T) {
	t.Parallel()
	d := Dividers{Step: 3}
	if d.Moved() {
		t.Fatal("the zero value is moved")
	}
	d.Files.Move(3)
	if !d.Moved() {
		t.Error("a moved Files is not moved")
	}
	d.Reset()
	d.List.Move(3)
	if !d.Moved() || d.Files.Moved() {
		t.Error("Reset kept Files or dropped List")
	}
	d.Reset()
	if d.Moved() || d.Step != 3 {
		t.Errorf("Reset left %+v", d)
	}
}
