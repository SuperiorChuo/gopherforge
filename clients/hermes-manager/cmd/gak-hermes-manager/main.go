package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-admin-kit/clients/hermes-manager/internal/adapter"
	"github.com/go-admin-kit/clients/hermes-manager/internal/manager"
	"github.com/go-admin-kit/clients/hermes-manager/internal/protocol"
	"github.com/go-admin-kit/clients/hermes-manager/internal/state"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		_, _ = fmt.Fprintln(os.Stderr, "gak-hermes-manager:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		printUsage()
		return errors.New("command is required")
	}
	store, err := state.NewStore("")
	if err != nil {
		return err
	}
	localManager := manager.New(store, nil)

	// 平台执行层。不支持的平台上 NewAdapter 失败，manager 退化为「只同步不
	// 执行」并上报 PROCESS_ADAPTER_NOT_CONFIGURED——不是致命错误，同步与
	// 诊断命令照常可用。
	localAdapter, adapterErr := adapter.NewAdapter(adapter.Options{
		Home:              store.Dir(),
		AllowInsecureHTTP: os.Getenv("GAK_HERMES_ALLOW_INSECURE") == "1",
	})
	if adapterErr == nil {
		localManager.SetAdapter(localAdapter)
	}

	switch os.Args[1] {
	case "enroll":
		return runEnroll(localManager, os.Args[2:])
	case "sync":
		return runSync(localManager, os.Stdout)
	case "heartbeat":
		return localManager.Heartbeat(context.Background())
	case "run":
		return runLoop(localManager, os.Args[2:])
	case "status":
		return printStatus(localManager, localAdapter, store.Dir())
	case "start", "stop", "restart", "rollback":
		return runProcessCommand(localAdapter, adapterErr, os.Args[1])
	case "version":
		fmt.Println(manager.Version)
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", os.Args[1])
	}
}

// runProcessCommand 是本机排障入口（安装/更新仍只由控制面策略驱动，
// 刻意不提供 `install` 子命令——员工手动装任意版本会绕过制品审批）。
func runProcessCommand(localAdapter adapter.Adapter, adapterErr error, command string) error {
	if adapterErr != nil {
		return adapterErr
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch command {
	case "start":
		return localAdapter.Start(ctx)
	case "stop":
		return localAdapter.Stop(ctx)
	case "restart":
		return localAdapter.Restart(ctx)
	case "rollback":
		return localAdapter.Rollback(ctx)
	}
	return fmt.Errorf("unknown process command %q", command)
}

func runEnroll(localManager *manager.Manager, args []string) error {
	flags := flag.NewFlagSet("enroll", flag.ContinueOnError)
	server := flags.String("server", "", "go-admin-kit control-plane URL")
	deviceName := flags.String("name", "Hermes device", "employee-visible device name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*server) == "" {
		return errors.New("--server is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return localManager.Enroll(ctx, *server, *deviceName, func(authorization *protocol.DeviceAuthorization) {
		verificationURI := resolveVerificationURI(*server, authorization.VerificationURI)
		fmt.Println("请在已登录的 go-admin-kit 中确认设备：")
		fmt.Println("地址:", verificationURI)
		fmt.Println("代码:", authorization.UserCode)
		fmt.Println("等待确认中...")
	})
}

type syncManager interface {
	Sync(context.Context) (*protocol.DesiredState, error)
	Heartbeat(context.Context) error
}

func runSync(localManager syncManager, output io.Writer) error {
	ctx := context.Background()
	desired, err := localManager.Sync(ctx)
	if err != nil {
		// 已经拉到期望状态说明失败发生在本机收敛阶段。先把失败快照送回
		// 控制面，再保留原错误让 CLI 非零退出；拉取失败则没有新状态可报。
		if desired != nil {
			if heartbeatErr := localManager.Heartbeat(ctx); heartbeatErr != nil {
				return fmt.Errorf("%w; 失败状态心跳上报也失败: %v", err, heartbeatErr)
			}
		}
		return err
	}
	if err := localManager.Heartbeat(ctx); err != nil {
		return fmt.Errorf("同步已收敛，但立即心跳上报失败: %w", err)
	}
	fmt.Fprintf(output, "已验证期望状态：revision=%d state=%s\n", desired.Revision, desired.DesiredState)
	return nil
}

func runLoop(localManager *manager.Manager, args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	interval := flags.Duration("interval", time.Minute, "desired-state and heartbeat interval")
	if err := flags.Parse(args); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return localManager.Run(ctx, *interval, func(err error) {
		_, _ = fmt.Fprintln(os.Stderr, time.Now().Format(time.RFC3339), err)
	})
}

func printStatus(localManager *manager.Manager, localAdapter adapter.Adapter, stateDir string) error {
	config, desired, err := localManager.Status()
	if err != nil {
		return err
	}
	output := struct {
		StateDir                string    `json:"state_dir"`
		ServerURL               string    `json:"server_url"`
		DeviceID                string    `json:"device_id"`
		ManagerVersion          string    `json:"manager_version"`
		DesiredPolicyRevision   uint64    `json:"desired_policy_revision"`
		AppliedPolicyRevision   uint64    `json:"applied_policy_revision"`
		DesiredState            string    `json:"desired_state,omitempty"`
		ActualState             string    `json:"actual_state"`
		HealthStatus            string    `json:"health_status"`
		HermesVersion           string    `json:"hermes_version"`
		LastErrorCode           string    `json:"last_error_code"`
		DesiredStateLeaseExpiry time.Time `json:"desired_state_lease_expiry,omitempty"`
		// 三期：API server 密钥指纹与最近一次成功下发任务 run id（诊断用）。
		APIServerFingerprint string `json:"api_server_fingerprint,omitempty"`
		LastPushedRunID      uint64 `json:"last_pushed_run_id,omitempty"`
		// Kanban 里程碑：最近一次经 kanban 完成回传的 run 与 gateway kanban 摘要。
		KanbanReportedRunID uint64 `json:"kanban_reported_run_id,omitempty"`
		KanbanConfigDigest  string `json:"kanban_config_digest,omitempty"`
		// 本机实况：与上面来自状态文件的字段分开，便于诊断「状态文件说在跑但
		// 进程其实死了」这类不一致。
		InstalledVersion string `json:"installed_version"`
		ProcessRunning   *bool  `json:"process_running,omitempty"`
		AdapterAvailable bool   `json:"adapter_available"`
	}{
		StateDir:              stateDir,
		ServerURL:             config.ServerURL,
		DeviceID:              config.DeviceID,
		ManagerVersion:        config.ManagerVersion,
		DesiredPolicyRevision: config.DesiredPolicyRevision,
		AppliedPolicyRevision: config.AppliedPolicyRevision,
		ActualState:           config.ActualState,
		HealthStatus:          config.HealthStatus,
		HermesVersion:         config.HermesVersion,
		LastErrorCode:         config.LastErrorCode,
		APIServerFingerprint:  config.APIServerFingerprint,
		LastPushedRunID:       config.LastPushedRunID,
		KanbanReportedRunID:   config.KanbanReportedRunID,
		KanbanConfigDigest:    config.KanbanConfigDigest,
		AdapterAvailable:      localAdapter != nil,
	}
	if desired != nil {
		output.DesiredState = desired.DesiredState
		output.DesiredStateLeaseExpiry = desired.LeaseExpiresAt
	}
	if localAdapter != nil {
		if version, versionErr := localAdapter.InstalledVersion(); versionErr == nil {
			output.InstalledVersion = version
		}
		if running, runErr := localAdapter.Running(context.Background()); runErr == nil {
			output.ProcessRunning = &running
		}
	}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func resolveVerificationURI(serverURL, verificationURI string) string {
	parsed, err := url.Parse(verificationURI)
	if err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	server, err := url.Parse(serverURL)
	if err != nil {
		return verificationURI
	}
	reference, err := url.Parse(verificationURI)
	if err != nil {
		return verificationURI
	}
	return server.ResolveReference(reference).String()
}

func printUsage() {
	fmt.Println(`gak-hermes-manager commands:
  enroll --server <url> [--name <display-name>]
  sync                受管收敛一次（拉期望状态、按需安装/启停、上报）
  heartbeat
  run [--interval 1m] 常驻循环
  status              本机状态（含已装版本与进程实况）
  start | stop | restart   手工进程操作（诊断用；常态由 sync 收敛）
  rollback            指针切回上一版本并重启
  version

环境变量:
  GAK_HERMES_HOME              覆盖状态与安装目录
  GAK_HERMES_ALLOW_INSECURE=1  允许 http 制品源（仅内网自托管）`)
}
