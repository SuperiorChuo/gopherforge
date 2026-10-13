package api

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/go-admin-kit/services/crm/internal/model"
	store "github.com/go-admin-kit/services/crm/internal/store/crm"
)

func TestMergeChecksTheLastAllowedParticipantBeforeAnyWrite(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			r, srv := newTestRouterWithServer(t)
			seedScope(t, srv, 5, "self", 0)
			rows := make([]model.Customer, store.MaxMergeSourceIDs+1)
			for i := range rows {
				rows[i] = model.Customer{TenantID: 1, Name: fmt.Sprintf("customer-%d", i), OwnerUserID: 5}
			}
			rows[len(rows)-1].OwnerUserID = 7
			if err := srv.Store.DB().Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			ids := make([]uint64, store.MaxMergeSourceIDs)
			for i := range ids {
				ids[i] = rows[i+1].ID
			}
			want := http.StatusForbidden
			if missing {
				ids[len(ids)-1] = 99999
				want = http.StatusNotFound
			}
			w, _ := callAs(t, r, http.MethodPost, "/api/v1/crm/customers/merge", map[string]any{"primary_id": rows[0].ID, "source_ids": ids}, "5")
			if w.Code != want {
				t.Fatalf("tail participant: status=%d want=%d body=%s", w.Code, want, w.Body.String())
			}
			var count int64
			if err := srv.Store.DB().Model(&model.Customer{}).Where("tenant_id = ?", 1).Count(&count).Error; err != nil || count != int64(len(rows)) {
				t.Fatalf("refused merge changed participants: %d %v", count, err)
			}
		})
	}
}
