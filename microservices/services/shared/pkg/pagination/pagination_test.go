package pagination

import (
	"testing"
)

func TestGetPageRequest(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		req := PageRequest{
			Page:     1,
			PageSize: 10,
		}
		if req.Page != 1 {
			t.Errorf("Expected Page 1, got %d", req.Page)
		}
		if req.PageSize != 10 {
			t.Errorf("Expected PageSize 10, got %d", req.PageSize)
		}
	})
}

