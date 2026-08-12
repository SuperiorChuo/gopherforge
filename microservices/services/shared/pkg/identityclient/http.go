package identityclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	identityv1 "github.com/go-admin-kit/services/api/gen/identity/v1"
)

var (
	ErrHTTPFallbackDisabled = errors.New("identityclient: http fallback disabled")
	ErrHTTPStatus           = errors.New("identityclient: http non-200 response")
)

// httpGet/httpPost HTTP 回退（Phase 2C 内部端点）。
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	if c.httpBase == "" {
		return ErrHTTPFallbackDisabled
	}
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.httpBase+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", c.internalToken)
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrHTTPStatus, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) httpContacts(ctx context.Context, tenantID uint64, ids []uint64) (map[uint64]*identityv1.Contact, error) {
	var out struct {
		Contacts map[string]struct {
			Email string `json:"email"`
			Phone string `json:"phone"`
		} `json:"contacts"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/internal/users/contacts", map[string]any{"tenant_id": tenantID, "ids": ids}, &out); err != nil {
		return nil, err
	}
	res := map[uint64]*identityv1.Contact{}
	for k, v := range out.Contacts {
		id, _ := strconv.ParseUint(k, 10, 64)
		res[id] = &identityv1.Contact{Email: v.Email, Phone: v.Phone}
	}
	return res, nil
}

func (c *Client) httpDisplayNames(ctx context.Context, ids []uint64) (map[uint64]string, error) {
	var out struct {
		Names map[string]string `json:"names"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/internal/users/display-names", map[string]any{"ids": ids}, &out); err != nil {
		return nil, err
	}
	res := map[uint64]string{}
	for k, v := range out.Names {
		id, _ := strconv.ParseUint(k, 10, 64)
		res[id] = v
	}
	return res, nil
}

func (c *Client) httpAdminIDs(ctx context.Context, tenantID uint64) ([]uint64, error) {
	var out struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/internal/users/admin-ids?tenant_id=%d", tenantID), nil, &out); err != nil {
		return nil, err
	}
	return out.IDs, nil
}

func (c *Client) httpUserDepartment(ctx context.Context, userID, tenantID uint64) (uint64, error) {
	var out struct {
		DepartmentID uint64 `json:"department_id"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/internal/users/department?id=%d&tenant_id=%d", userID, tenantID), nil, &out); err != nil {
		return 0, err
	}
	return out.DepartmentID, nil
}

func (c *Client) httpUsersByRoles(ctx context.Context, tenantID uint64, roleIDs []uint64) ([]uint64, error) {
	var out struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/internal/users/by-roles", map[string]any{"tenant_id": tenantID, "role_ids": roleIDs}, &out); err != nil {
		return nil, err
	}
	return out.IDs, nil
}

func (c *Client) httpUserDataScope(ctx context.Context, userID, tenantID uint64) (string, uint64, error) {
	var out struct {
		DataScope    string `json:"data_scope"`
		DepartmentID uint64 `json:"department_id"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/internal/users/data-scope?id=%d&tenant_id=%d", userID, tenantID), nil, &out); err != nil {
		return "", 0, err
	}
	return out.DataScope, out.DepartmentID, nil
}

func (c *Client) httpDepartmentMembers(ctx context.Context, deptID, tenantID uint64) ([]uint64, error) {
	var out struct {
		IDs []uint64 `json:"ids"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/internal/departments/members?department_id=%d&tenant_id=%d", deptID, tenantID), nil, &out); err != nil {
		return nil, err
	}
	return out.IDs, nil
}

func (c *Client) httpDepartmentInfo(ctx context.Context, id, tenantID uint64) (*identityv1.DepartmentInfoResponse, error) {
	var out struct {
		ID           uint64 `json:"id"`
		Name         string `json:"name"`
		LeaderUserID uint64 `json:"leader_user_id"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/internal/departments/info?id=%d&tenant_id=%d", id, tenantID), nil, &out); err != nil {
		return nil, err
	}
	return &identityv1.DepartmentInfoResponse{Id: out.ID, Name: out.Name, LeaderUserId: out.LeaderUserID}, nil
}

func (c *Client) httpOrg(ctx context.Context, tenantID uint64) (*identityv1.OrgResponse, error) {
	var out struct {
		Depts []*identityv1.OrgDept `json:"depts"`
		Users []*identityv1.OrgUser `json:"users"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v1/internal/org?tenant_id=%d", tenantID), nil, &out); err != nil {
		return nil, err
	}
	return &identityv1.OrgResponse{Depts: out.Depts, Users: out.Users}, nil
}
