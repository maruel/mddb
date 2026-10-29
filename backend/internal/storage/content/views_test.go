// Tests for view types.

package content

import (
	"testing"

	"github.com/maruel/ksid"
)

func TestView(t *testing.T) {
	t.Run("Validate", func(t *testing.T) {
		t.Run("valid view", func(t *testing.T) {
			v := &View{
				ID:   ksid.NewID(),
				Name: "My View",
				Type: ViewTypeTable,
			}
			if err := v.Validate(); err != nil {
				t.Errorf("Validate() unexpected error: %v", err)
			}
		})

		t.Run("missing ID", func(t *testing.T) {
			v := &View{
				Name: "My View",
				Type: ViewTypeTable,
			}
			if err := v.Validate(); err == nil {
				t.Error("Validate() expected error for missing ID")
			}
		})

		t.Run("missing name", func(t *testing.T) {
			v := &View{
				ID:   ksid.NewID(),
				Type: ViewTypeTable,
			}
			if err := v.Validate(); err == nil {
				t.Error("Validate() expected error for missing name")
			}
		})
	})

	t.Run("GetID", func(t *testing.T) {
		id := ksid.NewID()
		v := &View{ID: id}
		if v.GetID() != id {
			t.Errorf("GetID() = %v, want %v", v.GetID(), id)
		}
	})
}
