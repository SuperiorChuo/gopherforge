package content

import (
	"fmt"
	"testing"

	"github.com/go-admin-kit/services/internal/testdb"
	"github.com/go-admin-kit/services/shared/pkg/fullread"
)

func TestCompleteSetPublicationAndAccountHistory(t *testing.T) {
	db := testdb.Open(t, &PostPublication{}, &ChannelAccount{})
	s := NewStore(db)
	const count = fullread.BatchSize*2 + 1
	publications := make([]PostPublication, 0, count)
	accounts := make([]ChannelAccount, 0, count)
	for i := 1; i <= count; i++ {
		publications = append(publications, PostPublication{TenantID: 7, PostID: 42, Channel: ChannelWechat, ExternalID: fmt.Sprint(i)})
		accounts = append(accounts, ChannelAccount{TenantID: 7, Channel: ChannelWechat, Name: fmt.Sprint(i), RefID: uint64(i)})
	}
	for _, rows := range []any{&publications, &accounts} {
		if err := db.CreateInBatches(rows, fullread.BatchSize).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []any{
		&PostPublication{TenantID: 8, PostID: 42}, &PostPublication{TenantID: 7, PostID: 43},
		&ChannelAccount{TenantID: 8, Channel: ChannelWechat, RefID: 10000}, &ChannelAccount{TenantID: 7, Channel: "other", RefID: 10000},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	testdb.Batches(t, db, func() {
		got, err := s.ListPublications(7, 42)
		if err != nil || len(got) != count {
			t.Fatalf("publications=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.ID != uint64(count-i) || row.TenantID != 7 || row.PostID != 42 {
				t.Fatalf("publication %d: %+v", i, row)
			}
		}
	})
	testdb.Batches(t, db, func() {
		got, err := s.ListChannelAccounts(7, ChannelWechat)
		if err != nil || len(got) != count {
			t.Fatalf("accounts=%d %v", len(got), err)
		}
		for i, row := range got {
			if row.ID != uint64(count-i) || row.TenantID != 7 || row.Channel != ChannelWechat {
				t.Fatalf("account %d: %+v", i, row)
			}
		}
	})
}
