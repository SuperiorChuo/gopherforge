package store

import (
	"reflect"
	"testing"

	"github.com/go-admin-kit/services/visibility/internal/model"
)

func TestCompareDuplicateQuestionsIsOrderIndependent(t *testing.T) {
	cur, prev := &model.GeoRun{ID: 2}, &model.GeoRun{ID: 1}
	before := []model.GeoResult{{Question: " q ", Engine: "selfhost", MentionBrand: true}}
	a := model.GeoResult{Question: " q ", Engine: "selfhost"}
	b := model.GeoResult{Question: "q", Engine: "selfhost", MentionBrand: true}
	first := BuildGeoRunCompare(cur, []model.GeoResult{a, b}, prev, before)
	second := BuildGeoRunCompare(cur, []model.GeoResult{b, a}, prev, before)
	if !reflect.DeepEqual(first, second) || first.StillMentioned != 1 || len(first.NewlyMissed) != 0 {
		t.Fatalf("duplicate question order changed report: first=%+v second=%+v", first, second)
	}
}
