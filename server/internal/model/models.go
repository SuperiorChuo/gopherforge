package model

import (
	"time"
)

// User 用户模型
type User struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	Username           string    `gorm:"size:50;not null;uniqueIndex" json:"username"`
	Password           string    `gorm:"size:255;not null" json:"-"`
	Nickname           string    `gorm:"size:50" json:"nickname"`
	Email              string    `gorm:"size:100;uniqueIndex" json:"email"`
	Phone              string    `gorm:"size:20;uniqueIndex" json:"phone"`
	Avatar             string    `gorm:"size:255" json:"avatar"`
	DepartmentID       uint      `gorm:"default:0;index" json:"department_id"` // 部门ID
	MustChangePassword bool      `gorm:"default:false" json:"must_change_password"`
	Status             int8      `gorm:"default:1" json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	Roles              []Role    `gorm:"many2many:user_roles;" json:"roles,omitempty"`
}

// Department 部门模型
type Department struct {
	ID        uint         `gorm:"primaryKey" json:"id"`
	Name      string       `gorm:"size:100;not null" json:"name"`    // 部门名称
	Code      string       `gorm:"size:50;uniqueIndex" json:"code"`  // 部门编码
	ParentID  uint         `gorm:"default:0;index" json:"parent_id"` // 父部门ID
	Leader    string       `gorm:"size:50" json:"leader"`            // 负责人
	Phone     string       `gorm:"size:20" json:"phone"`             // 联系电话
	Email     string       `gorm:"size:100" json:"email"`            // 邮箱
	Sort      int          `gorm:"default:0" json:"sort"`            // 排序
	Status    int8         `gorm:"default:1" json:"status"`          // 状态 1启用 0禁用
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	Children  []Department `gorm:"-" json:"children,omitempty"` // 子部门（不存数据库）
}

// Role 角色模型
type Role struct {
	ID                     uint                      `gorm:"primaryKey" json:"id"`
	Name                   string                    `gorm:"size:50;not null" json:"name"`
	Code                   string                    `gorm:"size:50;not null;uniqueIndex" json:"code"`
	Description            string                    `gorm:"size:255" json:"description"`
	DataScope              string                    `gorm:"size:32;not null;default:self;index" json:"data_scope"`
	DataScopeDepartmentIDs []uint                    `gorm:"-" json:"data_scope_department_ids,omitempty"`
	DataScopeDepartments   []RoleDataScopeDepartment `gorm:"foreignKey:RoleID" json:"-"`
	CreatedAt              time.Time                 `json:"created_at"`
	UpdatedAt              time.Time                 `json:"updated_at"`
	Users                  []User                    `gorm:"many2many:user_roles;" json:"users,omitempty"`
	Permissions            []Permission              `gorm:"many2many:role_permissions;" json:"permissions,omitempty"`
}

// Permission 权限模型
type Permission struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:50;not null" json:"name"`
	Code        string       `gorm:"size:100;not null;uniqueIndex" json:"code"`
	Description string       `gorm:"size:255;default:''" json:"description"`
	Type        int8         `gorm:"not null" json:"type"` // 1菜单，2按钮
	Path        string       `gorm:"size:255" json:"path"`
	Method      string       `gorm:"size:10" json:"method"`
	ParentID    uint         `gorm:"default:0" json:"parent_id"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Roles       []Role       `gorm:"many2many:role_permissions;" json:"roles,omitempty"`
	Children    []Permission `gorm:"-" json:"children,omitempty"` // 子权限（不存数据库）
}

// Menu 菜单模型
type Menu struct {
	ID          uint         `gorm:"primaryKey" json:"id"`
	Name        string       `gorm:"size:50;not null" json:"name"`
	Title       string       `gorm:"size:50;not null" json:"title"`
	Icon        string       `gorm:"size:100" json:"icon"`
	Path        string       `gorm:"size:255" json:"path"`
	Component   string       `gorm:"size:255" json:"component"`
	ParentID    uint         `gorm:"default:0;index" json:"parent_id"`
	Sort        int          `gorm:"default:0" json:"sort"`
	Status      int8         `gorm:"default:1" json:"status"`    // 1启用，0禁用
	Hidden      int8         `gorm:"default:0" json:"hidden"`    // 0显示，1隐藏
	Permission  string       `gorm:"size:100" json:"permission"` // 关联的权限代码（可选，用于权限控制）
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Children    []Menu       `gorm:"-" json:"children,omitempty"`                              // 子菜单（不存数据库）
	Permissions []Permission `gorm:"many2many:menu_permissions;" json:"permissions,omitempty"` // 菜单关联的权限
}

// UserRole 用户角色关联表
type UserRole struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `gorm:"not null;index" json:"user_id"`
	RoleID uint `gorm:"not null;index" json:"role_id"`
}

// RolePermission 角色权限关联表
type RolePermission struct {
	ID           uint `gorm:"primaryKey" json:"id"`
	RoleID       uint `gorm:"not null;index" json:"role_id"`
	PermissionID uint `gorm:"not null;index" json:"permission_id"`
}

// RoleDataScopeDepartment 角色自定义数据范围部门关联表
type RoleDataScopeDepartment struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	RoleID       uint      `gorm:"not null;uniqueIndex:uk_role_data_scope_department;index" json:"role_id"`
	DepartmentID uint      `gorm:"not null;uniqueIndex:uk_role_data_scope_department;index" json:"department_id"`
	CreatedAt    time.Time `json:"created_at"`
}

// MenuPermission 菜单权限关联表
type MenuPermission struct {
	ID           uint `gorm:"primaryKey" json:"id"`
	MenuID       uint `gorm:"not null;index" json:"menu_id"`
	PermissionID uint `gorm:"not null;index" json:"permission_id"`
}

// OperationLog 操作日志模型
type OperationLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	UserID       uint      `gorm:"index" json:"user_id"`
	Username     string    `gorm:"size:50" json:"username"`
	ActorType    string    `gorm:"size:64;default:operator;index" json:"actor_type"`
	ActorID      string    `gorm:"size:128;default:web-console;index" json:"actor_id"`
	RequestID    string    `gorm:"size:64;index" json:"request_id"`
	Module       string    `gorm:"size:50" json:"module"`          // 操作模块
	Action       string    `gorm:"size:50" json:"action"`          // 操作类型
	Method       string    `gorm:"size:10" json:"method"`          // HTTP 方法
	Path         string    `gorm:"size:255" json:"path"`           // 请求路径
	Query        string    `gorm:"size:1024" json:"query"`         // 查询参数
	RequestBody  string    `gorm:"type:text" json:"request_body"`  // 请求体
	ResponseBody string    `gorm:"type:text" json:"response_body"` // 响应体（可选）
	Status       int       `json:"status"`                         // HTTP 状态码
	IP           string    `gorm:"size:45" json:"ip"`              // IPv6 max length 45
	UserAgent    string    `gorm:"size:500" json:"user_agent"`     // 用户代理
	Latency      int64     `json:"latency"`                        // 响应时间（毫秒）
	ErrorMsg     string    `gorm:"size:1024" json:"error_msg"`     // 错误信息
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// AuditLog 业务审计日志，迁移自 Python wm_audit_log。
type AuditLog struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	ActorType  string         `gorm:"size:64;default:operator;index" json:"actor_type"`
	ActorID    string         `gorm:"size:128;default:web-console;index" json:"actor_id"`
	Action     string         `gorm:"size:128;not null;index" json:"action"`
	TargetType string         `gorm:"size:64;not null;index" json:"target_type"`
	TargetID   string         `gorm:"size:128;not null;index" json:"target_id"`
	BeforeJSON map[string]any `gorm:"column:before_json;type:json;serializer:json" json:"before"`
	AfterJSON  map[string]any `gorm:"column:after_json;type:json;serializer:json" json:"after"`
	Summary    string         `gorm:"type:text" json:"summary"`
	CreatedAt  time.Time      `gorm:"index" json:"created_at"`
}

func (AuditLog) TableName() string {
	return "wm_audit_log"
}

// File 文件模型
type File struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`                        // 上传用户ID
	FileName    string    `gorm:"size:255;not null" json:"file_name"`          // 原始文件名
	FilePath    string    `gorm:"size:500;not null" json:"file_path"`          // 存储路径
	FileSize    int64     `json:"file_size"`                                   // 文件大小（字节）
	FileType    string    `gorm:"size:50" json:"file_type"`                    // 文件类型（image/video/document/other）
	MimeType    string    `gorm:"size:100" json:"mime_type"`                   // MIME 类型
	Extension   string    `gorm:"size:20" json:"extension"`                    // 文件扩展名
	StorageType string    `gorm:"size:20;default:'local'" json:"storage_type"` // 存储类型（local/oss/s3）
	URL         string    `gorm:"size:500" json:"url"`                         // 访问URL
	Hash        string    `gorm:"size:64;index" json:"hash"`                   // 文件哈希（用于去重）
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// LoginLog 登录日志模型
type LoginLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`        // 用户ID
	Username  string    `gorm:"size:50" json:"username"`     // 用户名
	LoginType int8      `gorm:"default:1" json:"login_type"` // 登录类型：1账号密码，2GitHub，3微信
	Status    int8      `gorm:"default:1" json:"status"`     // 状态：1成功，0失败
	IP        string    `gorm:"size:45" json:"ip"`           // IP地址
	Location  string    `gorm:"size:100" json:"location"`    // 登录地点
	Device    string    `gorm:"size:100" json:"device"`      // 设备类型
	OS        string    `gorm:"size:50" json:"os"`           // 操作系统
	Browser   string    `gorm:"size:100" json:"browser"`     // 浏览器
	UserAgent string    `gorm:"size:500" json:"user_agent"`  // 用户代理
	Message   string    `gorm:"size:255" json:"message"`     // 登录消息
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// DictType 字典类型
type DictType struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Name        string     `gorm:"size:100;not null" json:"name"`             // 字典名称
	Code        string     `gorm:"size:100;not null;uniqueIndex" json:"code"` // 字典编码
	Description string     `gorm:"size:255" json:"description"`               // 描述
	Status      int8       `gorm:"default:1" json:"status"`                   // 状态：1启用，0禁用
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	Items       []DictItem `gorm:"-" json:"items,omitempty"` // 字典项（不存数据库）
}

// DictItem 字典数据项
type DictItem struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	DictTypeID uint      `gorm:"not null;index" json:"dict_type_id"` // 字典类型ID
	Label      string    `gorm:"size:100;not null" json:"label"`     // 显示标签
	Value      string    `gorm:"size:100;not null" json:"value"`     // 数据值
	Sort       int       `gorm:"default:0" json:"sort"`              // 排序
	Status     int8      `gorm:"default:1" json:"status"`            // 状态：1启用，0禁用
	Remark     string    `gorm:"size:255" json:"remark"`             // 备注
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// OAuthBinding OAuth绑定表
type OAuthBinding struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	UserID         uint      `gorm:"not null;index" json:"user_id"`
	Provider       string    `gorm:"size:50;not null" json:"provider"`
	ProviderUserID string    `gorm:"size:100;not null" json:"provider_user_id"`
	AccessToken    string    `gorm:"size:255" json:"access_token"`
	RefreshToken   string    `gorm:"size:255" json:"refresh_token"`
	ExpiresAt      time.Time `json:"expires_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ScheduledJob 定时任务模型
type ScheduledJob struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	Name           string     `gorm:"size:100;not null;uniqueIndex" json:"name"` // 任务名称
	GroupName      string     `gorm:"size:50;default:default" json:"group_name"` // 任务组
	CronExpression string     `gorm:"size:50;not null" json:"cron_expression"`   // Cron表达式
	InvokeTarget   string     `gorm:"size:255;not null" json:"invoke_target"`    // 调用目标
	Description    string     `gorm:"size:500" json:"description"`               // 任务描述
	Status         int8       `gorm:"default:1" json:"status"`                   // 状态 1:运行 0:暂停
	Concurrent     int8       `gorm:"default:0" json:"concurrent"`               // 是否允许并发
	LastRunTime    *time.Time `json:"last_run_time"`                             // 上次执行时间
	NextRunTime    *time.Time `json:"next_run_time"`                             // 下次执行时间
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 定时任务表名
func (ScheduledJob) TableName() string {
	return "scheduled_jobs"
}

// ScheduledJobLog 定时任务执行日志
type ScheduledJobLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	JobID     uint      `gorm:"not null;index" json:"job_id"`      // 任务ID
	JobName   string    `gorm:"size:100;not null" json:"job_name"` // 任务名称
	Status    int8      `gorm:"default:1" json:"status"`           // 状态 1:成功 0:失败
	Message   string    `gorm:"type:text" json:"message"`          // 执行结果
	Duration  int       `json:"duration"`                          // 执行时长(毫秒)
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName 定时任务日志表名
func (ScheduledJobLog) TableName() string {
	return "scheduled_job_logs"
}

// Notice 通知公告模型
type Notice struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Title     string     `gorm:"size:200;not null" json:"title"`    // 公告标题
	Content   string     `gorm:"type:text;not null" json:"content"` // 公告内容
	Type      int8       `gorm:"default:1" json:"type"`             // 类型 1:通知 2:公告
	Status    int8       `gorm:"default:1" json:"status"`           // 状态 1:正常 0:关闭
	CreatorID uint       `gorm:"index" json:"creator_id"`           // 创建者ID
	Creator   string     `gorm:"size:50" json:"creator"`            // 创建者名称
	StartTime *time.Time `json:"start_time"`                        // 生效开始时间
	EndTime   *time.Time `json:"end_time"`                          // 生效结束时间
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TableName 通知公告表名
func (Notice) TableName() string {
	return "notices"
}
