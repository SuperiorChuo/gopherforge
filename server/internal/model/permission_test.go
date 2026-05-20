package model

import (
	"reflect"
	"strings"
	"testing"
)

func TestPermissionModelExposesDescription(t *testing.T) {
	field, ok := reflect.TypeOf(Permission{}).FieldByName("Description")
	if !ok {
		t.Fatal("Permission should expose a Description field")
	}
	if field.Type.Kind() != reflect.String {
		t.Fatalf("Description kind = %s, want string", field.Type.Kind())
	}
	if got := field.Tag.Get("json"); !strings.HasPrefix(got, "description") {
		t.Fatalf("Description json tag = %q, want description", got)
	}
}
