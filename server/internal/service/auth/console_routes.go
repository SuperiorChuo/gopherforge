package auth

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	authDAO "github.com/go-admin-kit/server/internal/dao/auth"
	"github.com/go-admin-kit/server/internal/model"
	"gorm.io/gorm"
)

const DefaultConsoleRouteSeedVersion = 6

var ErrConsoleRouteNotFound = errors.New("console route not found")

type ConsoleRouteValidationError struct {
	Message string
}

func (e ConsoleRouteValidationError) Error() string {
	return e.Message
}

type ConsoleRouteService struct{}

type ConsoleRouteView struct {
	RouteKey     string         `json:"route_key"`
	Path         string         `json:"path"`
	Name         string         `json:"name"`
	ComponentKey string         `json:"component_key"`
	Redirect     string         `json:"redirect,omitempty"`
	ParentKey    string         `json:"parent_key,omitempty"`
	SortOrder    int            `json:"sort_order"`
	Hidden       bool           `json:"hidden"`
	Public       bool           `json:"public"`
	Enabled      bool           `json:"enabled"`
	Permissions  []string       `json:"permissions"`
	Roles        []string       `json:"roles"`
	Meta         map[string]any `json:"meta"`
}

type ConsoleRouteCreateRequest struct {
	RouteKey     string         `json:"route_key"`
	Path         string         `json:"path"`
	Name         string         `json:"name"`
	ComponentKey string         `json:"component_key"`
	Redirect     string         `json:"redirect"`
	ParentKey    string         `json:"parent_key"`
	SortOrder    *int           `json:"sort_order"`
	Hidden       *bool          `json:"hidden"`
	Public       *bool          `json:"public"`
	Enabled      *bool          `json:"enabled"`
	Permissions  []string       `json:"permissions"`
	Roles        []string       `json:"roles"`
	Meta         map[string]any `json:"meta"`
}

type ConsoleRouteUpdateRequest struct {
	Path         *string         `json:"path"`
	Name         *string         `json:"name"`
	ComponentKey *string         `json:"component_key"`
	Redirect     *string         `json:"redirect"`
	ParentKey    *string         `json:"parent_key"`
	SortOrder    *int            `json:"sort_order"`
	Hidden       *bool           `json:"hidden"`
	Public       *bool           `json:"public"`
	Enabled      *bool           `json:"enabled"`
	Permissions  *[]string       `json:"permissions"`
	Roles        *[]string       `json:"roles"`
	Meta         *map[string]any `json:"meta"`
}

type ConsoleRouteBootstrapResult struct {
	Routes  int `json:"routes"`
	Updated int `json:"updated"`
}

func (s ConsoleRouteService) routeDAO() authDAO.ConsoleRouteDAO {
	return authDAO.NewConsoleRouteDAO()
}

func (s ConsoleRouteService) BootstrapDefaults() (ConsoleRouteBootstrapResult, error) {
	return s.BootstrapDefaultsContext(context.Background())
}

func (s ConsoleRouteService) BootstrapDefaultsContext(ctx context.Context) (ConsoleRouteBootstrapResult, error) {
	return s.bootstrapDefaultsContext(ctx, s.routeDAO())
}

func (s ConsoleRouteService) ListRoutes() ([]ConsoleRouteView, error) {
	return s.ListRoutesContext(context.Background())
}

func (s ConsoleRouteService) ListRoutesContext(ctx context.Context) ([]ConsoleRouteView, error) {
	if _, err := s.BootstrapDefaultsContext(ctx); err != nil {
		return nil, err
	}

	rows, err := s.routeDAO().ListAllContext(ctx)
	if err != nil {
		return nil, err
	}
	return serializeConsoleRoutes(rows), nil
}

func (s ConsoleRouteService) ListAccessibleRoutes(permissions, roles []string) ([]ConsoleRouteView, error) {
	return s.ListAccessibleRoutesContext(context.Background(), permissions, roles)
}

func (s ConsoleRouteService) ListAccessibleRoutesContext(ctx context.Context, permissions, roles []string) ([]ConsoleRouteView, error) {
	if _, err := s.BootstrapDefaultsContext(ctx); err != nil {
		return nil, err
	}

	rows, err := s.routeDAO().ListEnabledContext(ctx)
	if err != nil {
		return nil, err
	}
	return FilterConsoleRoutes(serializeConsoleRoutes(rows), permissions, roles), nil
}

func (s ConsoleRouteService) AllRoutePermissions() ([]string, error) {
	return s.AllRoutePermissionsContext(context.Background())
}

func (s ConsoleRouteService) AllRoutePermissionsContext(ctx context.Context) ([]string, error) {
	if _, err := s.BootstrapDefaultsContext(ctx); err != nil {
		return nil, err
	}
	rows, err := s.routeDAO().ListPermissionRowsContext(ctx)
	if err != nil {
		return nil, err
	}
	values := []string{}
	for _, route := range rows {
		values = append(values, route.PermissionsJSON...)
	}
	return UniqueSortedConsoleStrings(values), nil
}

func (s ConsoleRouteService) GetRoute(routeKey string) (ConsoleRouteView, error) {
	return s.GetRouteContext(context.Background(), routeKey)
}

func (s ConsoleRouteService) GetRouteContext(ctx context.Context, routeKey string) (ConsoleRouteView, error) {
	route, err := s.getRouteModelContext(ctx, s.routeDAO(), routeKey)
	if err != nil {
		return ConsoleRouteView{}, err
	}
	return serializeConsoleRoute(*route), nil
}

func (s ConsoleRouteService) CreateRoute(req ConsoleRouteCreateRequest) (ConsoleRouteView, error) {
	return s.CreateRouteContext(context.Background(), req)
}

func (s ConsoleRouteService) CreateRouteContext(ctx context.Context, req ConsoleRouteCreateRequest) (ConsoleRouteView, error) {
	routeKey, err := normalizeConsoleRouteKey(req.RouteKey)
	if err != nil {
		return ConsoleRouteView{}, err
	}
	path, err := normalizeConsoleRoutePath(req.Path)
	if err != nil {
		return ConsoleRouteView{}, err
	}
	name, err := normalizeConsoleRouteName(req.Name)
	if err != nil {
		return ConsoleRouteView{}, err
	}
	componentKey, err := normalizeConsoleRouteComponent(req.ComponentKey)
	if err != nil {
		return ConsoleRouteView{}, err
	}
	redirect, err := normalizeOptionalConsoleRoutePath(req.Redirect)
	if err != nil {
		return ConsoleRouteView{}, err
	}
	parentKey, err := normalizeConsoleRouteOptional(req.ParentKey, "parent_key", 64)
	if err != nil {
		return ConsoleRouteView{}, err
	}

	sortOrder := 1000
	if req.SortOrder != nil {
		sortOrder = *req.SortOrder
	}
	hidden := false
	if req.Hidden != nil {
		hidden = *req.Hidden
	}
	public := false
	if req.Public != nil {
		public = *req.Public
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	routeDAO := s.routeDAO()
	if err := ensureUniqueConsoleRouteContext(ctx, routeDAO, routeKey, path, name, routeKey); err != nil {
		return ConsoleRouteView{}, err
	}

	route := model.ConsoleRoute{
		RouteKey:        routeKey,
		Path:            path,
		Name:            name,
		ComponentKey:    componentKey,
		Redirect:        redirect,
		ParentKey:       parentKey,
		SortOrder:       sortOrder,
		Hidden:          hidden,
		Public:          public,
		Enabled:         enabled,
		PermissionsJSON: normalizeConsoleList(req.Permissions),
		RolesJSON:       normalizeConsoleList(req.Roles),
		MetaJSON:        normalizeConsoleMeta(req.Meta),
	}
	if err := routeDAO.CreateContext(ctx, &route); err != nil {
		return ConsoleRouteView{}, err
	}
	return serializeConsoleRoute(route), nil
}

func (s ConsoleRouteService) UpdateRoute(routeKey string, req ConsoleRouteUpdateRequest) (ConsoleRouteView, error) {
	return s.UpdateRouteContext(context.Background(), routeKey, req)
}

func (s ConsoleRouteService) UpdateRouteContext(ctx context.Context, routeKey string, req ConsoleRouteUpdateRequest) (ConsoleRouteView, error) {
	normalizedKey, err := normalizeConsoleRouteKey(routeKey)
	if err != nil {
		return ConsoleRouteView{}, err
	}

	routeDAO := s.routeDAO()
	route, err := s.getRouteModelContext(ctx, routeDAO, normalizedKey)
	if err != nil {
		return ConsoleRouteView{}, err
	}

	nextPath := route.Path
	if req.Path != nil {
		nextPath, err = normalizeConsoleRoutePath(*req.Path)
		if err != nil {
			return ConsoleRouteView{}, err
		}
	}
	nextName := route.Name
	if req.Name != nil {
		nextName, err = normalizeConsoleRouteName(*req.Name)
		if err != nil {
			return ConsoleRouteView{}, err
		}
	}
	if err := ensureUniqueConsoleRouteContext(ctx, routeDAO, normalizedKey, nextPath, nextName, normalizedKey); err != nil {
		return ConsoleRouteView{}, err
	}

	if req.Path != nil {
		route.Path = nextPath
	}
	if req.Name != nil {
		route.Name = nextName
	}
	if req.ComponentKey != nil {
		componentKey, err := normalizeConsoleRouteComponent(*req.ComponentKey)
		if err != nil {
			return ConsoleRouteView{}, err
		}
		route.ComponentKey = componentKey
	}
	if req.Redirect != nil {
		redirect, err := normalizeOptionalConsoleRoutePath(*req.Redirect)
		if err != nil {
			return ConsoleRouteView{}, err
		}
		route.Redirect = redirect
	}
	if req.ParentKey != nil {
		parentKey, err := normalizeConsoleRouteOptional(*req.ParentKey, "parent_key", 64)
		if err != nil {
			return ConsoleRouteView{}, err
		}
		route.ParentKey = parentKey
	}
	if req.SortOrder != nil {
		route.SortOrder = *req.SortOrder
	}
	if req.Hidden != nil {
		route.Hidden = *req.Hidden
	}
	if req.Public != nil {
		route.Public = *req.Public
	}
	if req.Enabled != nil {
		route.Enabled = *req.Enabled
	}
	if req.Permissions != nil {
		route.PermissionsJSON = normalizeConsoleList(*req.Permissions)
	}
	if req.Roles != nil {
		route.RolesJSON = normalizeConsoleList(*req.Roles)
	}
	if req.Meta != nil {
		route.MetaJSON = normalizeConsoleMeta(*req.Meta)
	}

	if err := routeDAO.SaveContext(ctx, route); err != nil {
		return ConsoleRouteView{}, err
	}
	return serializeConsoleRoute(*route), nil
}

func (s ConsoleRouteService) DeleteRoute(routeKey string) (ConsoleRouteView, error) {
	return s.DeleteRouteContext(context.Background(), routeKey)
}

func (s ConsoleRouteService) DeleteRouteContext(ctx context.Context, routeKey string) (ConsoleRouteView, error) {
	routeDAO := s.routeDAO()
	route, err := s.getRouteModelContext(ctx, routeDAO, routeKey)
	if err != nil {
		return ConsoleRouteView{}, err
	}
	before := serializeConsoleRoute(*route)
	if err := routeDAO.DeleteContext(ctx, route); err != nil {
		return ConsoleRouteView{}, err
	}
	return before, nil
}

func (s ConsoleRouteService) bootstrapDefaultsContext(ctx context.Context, routeDAO authDAO.ConsoleRouteDAO) (ConsoleRouteBootstrapResult, error) {
	var result ConsoleRouteBootstrapResult
	if !routeDAO.Ready() {
		return result, authDAO.ErrConsoleRouteDatabaseNotInitialized
	}

	err := routeDAO.TransactionContext(ctx, func(tx authDAO.ConsoleRouteDAO) error {
		for _, item := range DefaultConsoleRoutes() {
			route, err := tx.GetByRouteKeyContext(ctx, item.RouteKey)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				modelRoute := consoleRouteViewToModel(item)
				if err := tx.CreateContext(ctx, &modelRoute); err != nil {
					return err
				}
				result.Routes++
				continue
			}
			if err != nil {
				return err
			}
			if syncExistingDefaultConsoleRoute(route, item) {
				if err := tx.SaveContext(ctx, route); err != nil {
					return err
				}
				result.Updated++
			}
		}
		return nil
	})
	return result, err
}

func (s ConsoleRouteService) getRouteModelContext(ctx context.Context, routeDAO authDAO.ConsoleRouteDAO, routeKey string) (*model.ConsoleRoute, error) {
	normalizedKey, err := normalizeConsoleRouteKey(routeKey)
	if err != nil {
		return nil, err
	}
	if !routeDAO.Ready() {
		return nil, authDAO.ErrConsoleRouteDatabaseNotInitialized
	}
	route, err := routeDAO.GetByRouteKeyContext(ctx, normalizedKey)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrConsoleRouteNotFound
		}
		return nil, err
	}
	return route, nil
}

func ensureUniqueConsoleRouteContext(ctx context.Context, routeDAO authDAO.ConsoleRouteDAO, routeKey, path, name, currentKey string) error {
	currentKey, _ = normalizeConsoleRouteKey(currentKey)
	if routeKey != currentKey {
		count, err := routeDAO.CountByRouteKeyContext(ctx, routeKey)
		if err != nil {
			return err
		}
		if count > 0 {
			return ConsoleRouteValidationError{Message: fmt.Sprintf("Console route key already exists: %s", routeKey)}
		}
	}

	owner, err := routeDAO.FindRouteKeyByPathContext(ctx, path)
	if err != nil {
		return err
	}
	if owner != "" && owner != currentKey {
		return ConsoleRouteValidationError{Message: fmt.Sprintf("Console route path already exists: %s", path)}
	}

	owner, err = routeDAO.FindRouteKeyByNameContext(ctx, name)
	if err != nil {
		return err
	}
	if owner != "" && owner != currentKey {
		return ConsoleRouteValidationError{Message: fmt.Sprintf("Console route name already exists: %s", name)}
	}
	return nil
}

func serializeConsoleRoutes(rows []model.ConsoleRoute) []ConsoleRouteView {
	result := make([]ConsoleRouteView, 0, len(rows))
	for _, row := range rows {
		result = append(result, serializeConsoleRoute(row))
	}
	return result
}

func serializeConsoleRoute(route model.ConsoleRoute) ConsoleRouteView {
	permissions := normalizeConsoleList(route.PermissionsJSON)
	roles := normalizeConsoleList(route.RolesJSON)
	meta := normalizeConsoleMeta(route.MetaJSON)
	meta["hidden"] = route.Hidden
	meta["public"] = route.Public
	meta["permissions"] = permissions
	meta["roles"] = roles
	meta["routeKey"] = route.RouteKey
	meta["sortOrder"] = route.SortOrder

	return ConsoleRouteView{
		RouteKey:     route.RouteKey,
		Path:         route.Path,
		Name:         route.Name,
		ComponentKey: route.ComponentKey,
		Redirect:     route.Redirect,
		ParentKey:    route.ParentKey,
		SortOrder:    route.SortOrder,
		Hidden:       route.Hidden,
		Public:       route.Public,
		Enabled:      route.Enabled,
		Permissions:  permissions,
		Roles:        roles,
		Meta:         meta,
	}
}

func ConsoleRouteSnapshot(route ConsoleRouteView) map[string]any {
	return map[string]any{
		"route_key":     route.RouteKey,
		"path":          route.Path,
		"name":          route.Name,
		"component_key": route.ComponentKey,
		"redirect":      route.Redirect,
		"parent_key":    route.ParentKey,
		"sort_order":    route.SortOrder,
		"hidden":        route.Hidden,
		"public":        route.Public,
		"enabled":       route.Enabled,
		"permissions":   append([]string{}, route.Permissions...),
		"roles":         append([]string{}, route.Roles...),
		"meta":          normalizeConsoleMeta(route.Meta),
	}
}

func FilterConsoleRoutes(routes []ConsoleRouteView, permissions, roles []string) []ConsoleRouteView {
	result := []ConsoleRouteView{}
	permissionSet := consoleStringSet(permissions)
	roleSet := consoleStringSet(roles)
	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		if !consoleSetHasAll(permissionSet, route.Permissions) {
			continue
		}
		if len(route.Roles) > 0 && !consoleSetHasAny(roleSet, route.Roles) {
			continue
		}
		result = append(result, route)
	}
	return result
}

func AllConsoleRoutePermissions() []string {
	values := []string{}
	for _, route := range defaultConsoleRouteSeed {
		values = append(values, route.Permissions...)
	}
	return UniqueSortedConsoleStrings(values)
}

func ConsoleAuditTarget(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}

func ConsoleAuthAuditSummary(action, targetID string) string {
	switch action {
	case "auth.login.success":
		return fmt.Sprintf("Console login succeeded for %s", ConsoleAuditTarget(targetID, "unknown"))
	case "auth.login.failed":
		return fmt.Sprintf("Console login failed for %s", ConsoleAuditTarget(targetID, "unknown"))
	case "auth.logout":
		return fmt.Sprintf("Console logout for %s", ConsoleAuditTarget(targetID, "unknown"))
	default:
		return fmt.Sprintf("Console auth event for %s", ConsoleAuditTarget(targetID, "unknown"))
	}
}

func ConsoleRoleCodes(roles []model.Role) []string {
	values := make([]string, 0, len(roles))
	for _, role := range roles {
		code := strings.TrimSpace(role.Code)
		if code != "" {
			values = append(values, code)
		}
	}
	return UniqueSortedConsoleStrings(values)
}

func ConsolePermissionsForUser(ctx context.Context, user *model.User, base []string) []string {
	if ctx == nil {
		ctx = context.Background()
	}
	values := append([]string{}, base...)
	values = append(values, consolePermissionAliases(base)...)
	if ConsoleHasRole(user, "super_admin") {
		routePermissions, err := ConsoleRouteService{}.AllRoutePermissionsContext(ctx)
		if err != nil {
			routePermissions = AllConsoleRoutePermissions()
		}
		values = append(values, routePermissions...)
		values = append(values,
			"dashboard.view",
			"logs.read",
			"settings.read",
			"settings.write",
			"rbac.read",
			"rbac.write",
		)
	}
	return UniqueSortedConsoleStrings(values)
}

func ConsoleHasRole(user *model.User, roleCode string) bool {
	if user == nil {
		return false
	}
	for _, role := range user.Roles {
		if strings.TrimSpace(role.Code) == roleCode {
			return true
		}
	}
	return false
}

func consolePermissionAliases(base []string) []string {
	aliasMap := map[string][]string{
		"system:log:audit":         {"logs.read"},
		"system:log:operation":     {"logs.read"},
		"system:user:list":         {"rbac.read"},
		"system:role:list":         {"rbac.read"},
		"system:permission:list":   {"rbac.read"},
		"system:department:list":   {"rbac.read"},
		"system:user:update":       {"rbac.write"},
		"system:role:update":       {"rbac.write"},
		"system:permission:update": {"rbac.write"},
		"system:department:update": {"rbac.write"},
		"system:monitor":           {"dashboard.view"},
	}
	values := []string{}
	for _, permission := range base {
		values = append(values, aliasMap[permission]...)
	}
	return values
}

func DefaultConsoleRoutes() []ConsoleRouteView {
	result := make([]ConsoleRouteView, 0, len(defaultConsoleRouteSeed))
	for _, route := range defaultConsoleRouteSeed {
		copied := route
		copied.Permissions = append([]string{}, route.Permissions...)
		copied.Roles = append([]string{}, route.Roles...)
		copied.Meta = normalizeConsoleMeta(route.Meta)
		result = append(result, copied)
	}
	return result
}

func consoleRouteViewToModel(route ConsoleRouteView) model.ConsoleRoute {
	return model.ConsoleRoute{
		RouteKey:        route.RouteKey,
		Path:            route.Path,
		Name:            route.Name,
		ComponentKey:    route.ComponentKey,
		Redirect:        route.Redirect,
		ParentKey:       route.ParentKey,
		SortOrder:       route.SortOrder,
		Hidden:          route.Hidden,
		Public:          route.Public,
		Enabled:         route.Enabled,
		PermissionsJSON: append([]string{}, route.Permissions...),
		RolesJSON:       append([]string{}, route.Roles...),
		MetaJSON:        normalizeConsoleMeta(route.Meta),
	}
}

func syncExistingDefaultConsoleRoute(route *model.ConsoleRoute, item ConsoleRouteView) bool {
	updated := false
	if strings.TrimSpace(route.ComponentKey) == "" {
		route.ComponentKey = item.ComponentKey
		updated = true
	}
	if strings.TrimSpace(route.Path) == "" {
		route.Path = item.Path
		updated = true
	}
	if strings.TrimSpace(route.Name) == "" {
		route.Name = item.Name
		updated = true
	}
	if len(normalizeConsoleList(route.PermissionsJSON)) == 0 {
		route.PermissionsJSON = append([]string{}, item.Permissions...)
		updated = true
	}

	seedVersion := intFromMeta(route.MetaJSON["seedVersion"])
	if seedVersion != DefaultConsoleRouteSeedVersion {
		route.Enabled = item.Enabled
		route.PermissionsJSON = append([]string{}, item.Permissions...)
		route.RolesJSON = append([]string{}, item.Roles...)
		route.MetaJSON = normalizeConsoleMeta(item.Meta)
		updated = true
	}
	return updated
}

func normalizeConsoleRouteKey(value string) (string, error) {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", "-"))
	if normalized == "" {
		return "", ConsoleRouteValidationError{Message: "Route key is required"}
	}
	if len(normalized) > 64 {
		return "", ConsoleRouteValidationError{Message: "route_key must be no more than 64 characters"}
	}
	return normalized, nil
}

func normalizeConsoleRoutePath(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", ConsoleRouteValidationError{Message: "Route path is required"}
	}
	if !strings.HasPrefix(normalized, "/") {
		normalized = "/" + normalized
	}
	if len(normalized) > 255 {
		return "", ConsoleRouteValidationError{Message: "path must be no more than 255 characters"}
	}
	return normalized, nil
}

func normalizeOptionalConsoleRoutePath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return normalizeConsoleRoutePath(value)
}

func normalizeConsoleRouteName(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", ConsoleRouteValidationError{Message: "Route name is required"}
	}
	if len(normalized) > 128 {
		return "", ConsoleRouteValidationError{Message: "name must be no more than 128 characters"}
	}
	return normalized, nil
}

func normalizeConsoleRouteComponent(value string) (string, error) {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return "", ConsoleRouteValidationError{Message: "component_key is required"}
	}
	if len(normalized) > 128 {
		return "", ConsoleRouteValidationError{Message: "component_key must be no more than 128 characters"}
	}
	return normalized, nil
}

func normalizeConsoleRouteOptional(value, field string, maxLen int) (string, error) {
	normalized := strings.TrimSpace(value)
	if len(normalized) > maxLen {
		return "", ConsoleRouteValidationError{Message: fmt.Sprintf("%s must be no more than %d characters", field, maxLen)}
	}
	return normalized, nil
}

func normalizeConsoleList(value []string) []string {
	if value == nil {
		return []string{}
	}
	result := make([]string, 0, len(value))
	for _, item := range value {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func normalizeConsoleMeta(value map[string]any) map[string]any {
	result := map[string]any{}
	for key, item := range value {
		result[key] = item
	}
	return result
}

func consoleStringSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			set[trimmed] = true
		}
	}
	return set
}

func consoleSetHasAll(set map[string]bool, required []string) bool {
	for _, item := range normalizeConsoleList(required) {
		if !set[item] {
			return false
		}
	}
	return true
}

func consoleSetHasAny(set map[string]bool, required []string) bool {
	for _, item := range normalizeConsoleList(required) {
		if set[item] {
			return true
		}
	}
	return false
}

func UniqueSortedConsoleStrings(values []string) []string {
	set := consoleStringSet(values)
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func intFromMeta(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case uint:
		return int(typed)
	case uint64:
		return int(typed)
	default:
		return 0
	}
}

func defaultRoute(routeKey, path, name, componentKey string, sortOrder int, permissions []string, groupID, navTitle string, enabled bool) ConsoleRouteView {
	metaPermissions := append([]string{}, permissions...)
	return ConsoleRouteView{
		RouteKey:     routeKey,
		Path:         path,
		Name:         name,
		ComponentKey: componentKey,
		SortOrder:    sortOrder,
		Enabled:      enabled,
		Permissions:  append([]string{}, permissions...),
		Roles:        []string{},
		Meta: map[string]any{
			"title":       navTitle,
			"navTitle":    navTitle,
			"groupId":     groupID,
			"icon":        "SettingIcon",
			"permissions": metaPermissions,
			"seedVersion": DefaultConsoleRouteSeedVersion,
		},
	}
}

var defaultConsoleRouteSeed = []ConsoleRouteView{
	defaultRoute("dashboard", "/dashboard", "Dashboard", "DashboardPage", 100, []string{"dashboard.view"}, "monitor", "仪表盘", true),
	defaultRoute("rbac-users", "/rbac/users", "RbacUsers", "RbacGovernancePage", 210, []string{"rbac.read", "logs.read"}, "rbac", "用户管理", true),
	defaultRoute("rbac-roles", "/rbac/roles", "RbacRoles", "RbacGovernancePage", 220, []string{"rbac.read", "logs.read"}, "rbac", "角色管理", true),
	defaultRoute("rbac-policies", "/rbac/policies", "RbacPolicies", "RbacGovernancePage", 230, []string{"rbac.read", "logs.read"}, "rbac", "权限管理", true),
	defaultRoute("rbac-departments", "/rbac/departments", "RbacDepartments", "RbacGovernancePage", 240, []string{"rbac.read", "logs.read"}, "rbac", "部门管理", true),
	defaultRoute("audit", "/audit", "Audit", "AuditPage", 270, []string{"logs.read"}, "security", "审计日志", true),
	defaultRoute("security-logins", "/security/logins", "SecurityLogins", "SecurityLoginsPage", 280, []string{"logs.read"}, "security", "登录日志", true),
	defaultRoute("settings-routes", "/settings/routes", "ConsoleRoutes", "ConsoleRoutesPage", 315, []string{"settings.write"}, "settings", "路由设置", true),
}
