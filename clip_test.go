package clip

import (
	"reflect"
	"testing"
)

func TestParams_AddFrom(t *testing.T) {
	t.Run("it works", func(t *testing.T) {
		p := &Params{
			Query:     new(string),
			Grayscale: new(bool),
		}
		*p.Query = "q"
		*p.Grayscale = true
		o := &Params{
			Query:        new(string),
			CustomStyles: new(string),
		}
		*o.CustomStyles = "s"
		want := &Params{
			Query:        new(string),
			Grayscale:    new(bool),
			CustomStyles: new(string),
		}
		*want.Query = "q"
		*want.Grayscale = true
		*want.CustomStyles = "s"
		p.AddFrom(o)
		*o.CustomStyles = "sss"
		if !reflect.DeepEqual(p, want) {
			t.Errorf("AddFrom(): want: %v, got: %v", want, p)
		}
	})
}
