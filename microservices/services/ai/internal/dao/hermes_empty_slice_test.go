package dao

import (
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// 空结果集必须序列化成 []，不能是 null。
//
// `var rows []T` 在无行时保持 nil，encoding/json 把 nil slice 写成 null，前端
// 拿到 null 再调 .length/.filter 就整页崩。这不是理论风险：/hermes/overview 曾因
// usage.devices 为 null 白屏（TypeError: Cannot read properties of null），而
// 「我的 Hermes」在制品目录为空时 downloads 同样是 null——那是新员工的入职必经页。
//
// 所以 DAO 层统一用 make([]T, 0) 兜底，JSON 契约「声明是数组就一定是数组」由本用例钉住。

func assertMarshalsToEmptyArray(t *testing.T, label string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("%s: marshal: %v", label, err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("%s: 空结果集应序列化为 []，实际为 %s", label, encoded)
	}
}

func TestHermesUsageDAOReturnsEmptyArraysNotNull(t *testing.T) {
	db, mock := newDAOTestDB(t)
	dao := NewHermesUsageDAO(db)
	ctx := tenantCtx(1)
	from := time.Now().AddDate(0, 0, -7)
	to := time.Now()

	// 没有任何调用记录时，聚合查询返回零行
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT device_id`)).
		WillReturnRows(sqlmock.NewRows([]string{"device_id"}))
	devices, err := dao.ByDevice(ctx, from, to, 5)
	if err != nil {
		t.Fatalf("ByDevice: %v", err)
	}
	assertMarshalsToEmptyArray(t, "ByDevice", devices)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT DATE(created_at)`)).
		WillReturnRows(sqlmock.NewRows([]string{"day"}))
	daily, err := dao.Daily(ctx, from, to)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	assertMarshalsToEmptyArray(t, "Daily", daily)
}

func TestHermesDAOListsReturnEmptyArraysNotNull(t *testing.T) {
	db, mock := newDAOTestDB(t)
	dao := NewHermesDAO(db)
	ctx := tenantCtx(1)

	// 新设备还没上报过任何事件
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "hermes_device_events"`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	events, err := dao.ListDeviceEvents(ctx, "device-without-events", 10)
	if err != nil {
		t.Fatalf("ListDeviceEvents: %v", err)
	}
	assertMarshalsToEmptyArray(t, "ListDeviceEvents", events)
}
