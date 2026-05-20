package system

import (
	"reflect"
	"strings"
	"testing"
)

func TestPermissionRequestsExposeDescription(t *testing.T) {
	tests := []struct {
		name      string
		value     any
		fieldKind reflect.Kind
	}{
		{name: "create", value: CreatePermissionRequest{}, fieldKind: reflect.String},
		{name: "update", value: UpdatePermissionRequest{}, fieldKind: reflect.Ptr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, ok := reflect.TypeOf(tt.value).FieldByName("Description")
			if !ok {
				t.Fatal("request should expose a Description field")
			}
			if field.Type.Kind() != tt.fieldKind {
				t.Fatalf("Description kind = %s, want %s", field.Type.Kind(), tt.fieldKind)
			}
			if got := field.Tag.Get("json"); !strings.HasPrefix(got, "description") {
				t.Fatalf("Description json tag = %q, want description", got)
			}
		})
	}
}
