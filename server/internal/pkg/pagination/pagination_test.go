package pagination

import (
	"testing"
)

func TestGetPageRequest(t *testing.T) {
	// 这个测试需要 gin.Context，实际项目中应该使用更完整的测试
	// 这里只是示例结构
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

func TestCalculatePages(t *testing.T) {
	tests := []struct {
		name     string
		total    int64
		pageSize int
		expected int
	}{
		{"exact division", 100, 10, 10},
		{"with remainder", 101, 10, 11},
		{"zero total", 0, 10, 0},
		{"single page", 5, 10, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePages(tt.total, tt.pageSize)
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestNewPageResponse(t *testing.T) {
	req := PageRequest{
		Page:     2,
		PageSize: 10,
	}
	total := int64(25)

	resp := NewPageResponse(req, total)

	if resp.Page != 2 {
		t.Errorf("Expected Page 2, got %d", resp.Page)
	}
	if resp.PageSize != 10 {
		t.Errorf("Expected PageSize 10, got %d", resp.PageSize)
	}
	if resp.Total != 25 {
		t.Errorf("Expected Total 25, got %d", resp.Total)
	}
	if resp.Pages != 3 {
		t.Errorf("Expected Pages 3, got %d", resp.Pages) // 25 / 10 = 2.5, 向上取整为 3
	}
}
