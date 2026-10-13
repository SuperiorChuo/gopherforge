package wxclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// TestGetUserInfo 验证拉单个粉丝：GET user/info、openid+lang query 正确、字段解析全。
func TestGetUserInfo(t *testing.T) {
	var gotMethod, gotOpenID, gotLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/user/info":
			gotMethod = r.Method
			gotOpenID = r.URL.Query().Get("openid")
			gotLang = r.URL.Query().Get("lang")
			_, _ = w.Write([]byte(`{
				"subscribe":1,"openid":"OPENID-x","nickname":"小明","sex":1,
				"city":"广州","province":"广东","country":"中国",
				"headimgurl":"https://wx.qlogo.cn/x","subscribe_time":1600000000,
				"unionid":"UNION-x","remark":"vip","tagid_list":[101,102]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	info, err := c.GetUserInfo(context.Background(), testAccount(), "OPENID-x")
	if err != nil {
		t.Fatalf("GetUserInfo: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if gotOpenID != "OPENID-x" {
		t.Fatalf("openid query = %q", gotOpenID)
	}
	if gotLang != "zh_CN" {
		t.Fatalf("lang query = %q, want zh_CN", gotLang)
	}
	want := WxUserInfo{
		OpenID: "OPENID-x", Nickname: "小明", Sex: 1, City: "广州",
		Province: "广东", Country: "中国", HeadImgURL: "https://wx.qlogo.cn/x",
		Subscribe: 1, SubscribeTime: 1600000000, UnionID: "UNION-x",
		Remark: "vip", TagIDList: []int64{101, 102},
	}
	if !reflect.DeepEqual(info, want) {
		t.Fatalf("info = %+v\nwant %+v", info, want)
	}
}

// TestGetUserInfoWxError 验证 errcode!=0（如 40003 非法 openid）转 WxError。
func TestGetUserInfoWxError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/user/info":
			_, _ = w.Write([]byte(`{"errcode":40003,"errmsg":"invalid openid"}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	_, err := c.GetUserInfo(context.Background(), testAccount(), "bad")
	we, ok := err.(*WxError)
	if !ok || we.Code != 40003 {
		t.Fatalf("expected WxError 40003, got %T %v", err, err)
	}
}

// TestListFanOpenIDs 验证粉丝列表：GET user/get、无游标时不带 next_openid、
// openid 数组与 next 游标解析正确。
func TestListFanOpenIDs(t *testing.T) {
	var gotMethod string
	var gotNext string
	var nextPresent bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/user/get":
			gotMethod = r.Method
			gotNext = r.URL.Query().Get("next_openid")
			_, nextPresent = r.URL.Query()["next_openid"]
			_, _ = w.Write([]byte(`{"total":2,"count":2,"data":{"openid":["o1","o2"]},"next_openid":"o2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)

	// 首页：nextOpenID 为空，不应带 next_openid query。
	ids, next, err := c.ListFanOpenIDs(context.Background(), testAccount(), "")
	if err != nil {
		t.Fatalf("ListFanOpenIDs: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if nextPresent {
		t.Fatalf("empty cursor should omit next_openid query, but it was present (=%q)", gotNext)
	}
	if !reflect.DeepEqual(ids, []string{"o1", "o2"}) {
		t.Fatalf("ids = %v, want [o1 o2]", ids)
	}
	if next != "o2" {
		t.Fatalf("next = %q, want o2", next)
	}
}

// TestListFanOpenIDsWithCursor 验证带游标时 next_openid query 正确拼接。
func TestListFanOpenIDsWithCursor(t *testing.T) {
	var gotNext string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/user/get":
			gotNext = r.URL.Query().Get("next_openid")
			// 末页：next_openid 为空表示到底。
			_, _ = w.Write([]byte(`{"total":3,"count":1,"data":{"openid":["o3"]},"next_openid":""}`))
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv.URL)
	ids, next, err := c.ListFanOpenIDs(context.Background(), testAccount(), "o2")
	if err != nil {
		t.Fatalf("ListFanOpenIDs: %v", err)
	}
	if gotNext != "o2" {
		t.Fatalf("next_openid query = %q, want o2", gotNext)
	}
	if !reflect.DeepEqual(ids, []string{"o3"}) {
		t.Fatalf("ids = %v, want [o3]", ids)
	}
	if next != "" {
		t.Fatalf("next = %q, want empty (end of list)", next)
	}
}

func TestListFanPageReturnsProgressMetadata(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/user/get":
			_, _ = w.Write([]byte(`{"total":321,"count":2,"data":{"openid":["o1","o2"]},"next_openid":"cursor-2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	page, err := newTestClient(t, srv.URL).ListFanPage(context.Background(), testAccount(), "")
	if err != nil {
		t.Fatalf("ListFanPage: %v", err)
	}
	if page.Total != 321 || page.Count != 2 || page.NextOpenID != "cursor-2" || !reflect.DeepEqual(page.OpenIDs, []string{"o1", "o2"}) {
		t.Fatalf("page = %+v", page)
	}
}

func TestBatchGetUserInfoUsesOneHundredUserContract(t *testing.T) {
	var gotMethod string
	var gotBody batchUserInfoRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cgi-bin/token":
			_, _ = w.Write([]byte(`{"access_token":"TOK","expires_in":7200}`))
		case "/cgi-bin/user/info/batchget":
			gotMethod = r.Method
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decode request: %v", err)
			}
			_, _ = w.Write([]byte(`{"user_info_list":[{"subscribe":1,"openid":"o1","nickname":"甲","tagid_list":[7]},{"subscribe":1,"openid":"o2","nickname":"乙"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	profiles, err := newTestClient(t, srv.URL).BatchGetUserInfo(context.Background(), testAccount(), []string{"o1", "o2"})
	if err != nil {
		t.Fatalf("BatchGetUserInfo: %v", err)
	}
	if gotMethod != http.MethodPost || len(gotBody.UserList) != 2 {
		t.Fatalf("method=%s body=%+v", gotMethod, gotBody)
	}
	if gotBody.UserList[0].OpenID != "o1" || gotBody.UserList[0].Lang != "zh_CN" {
		t.Fatalf("first request item=%+v", gotBody.UserList[0])
	}
	if len(profiles) != 2 || profiles[0].Nickname != "甲" || !reflect.DeepEqual(profiles[0].TagIDList, []int64{7}) {
		t.Fatalf("profiles=%+v", profiles)
	}

	tooMany := make([]string, MaxBatchUserInfo+1)
	for i := range tooMany {
		tooMany[i] = "open-id"
	}
	if _, err := newTestClient(t, srv.URL).BatchGetUserInfo(context.Background(), testAccount(), tooMany); err == nil {
		t.Fatal("expected oversized batch to fail")
	}
}
