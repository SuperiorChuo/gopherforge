// Package iploc 提供基于 ip2region xdb 的离线 IP 归属地查询。
//
// 数据文件（约 11MB）不随仓库分发：默认从 ./data/ip2region.xdb 加载，可用
// 环境变量 IP2REGION_XDB 覆盖路径；文件缺失或损坏时优雅降级——公网 IP 查询
// 返回空串，调用方可自行回退在线查询或留空。数据文件用仓库根目录的
// scripts/download-ip2region.sh 下载。
//
// xdb 整体加载进内存（进程内一次），单次查询微秒级、无磁盘 IO；同时兼容
// 旧版 IPv4-only（Structure 2.0）与新版含 IPv6（Structure 3.0）的 xdb 文件。
package iploc

import (
	"net"
	"os"
	"strings"
	"sync"

	"github.com/lionsoul2014/ip2region/binding/golang/xdb"
)

// EnvXdbPath 指定 xdb 数据文件路径的环境变量名。
const EnvXdbPath = "IP2REGION_XDB"

// DefaultXdbPath 未设置环境变量时的默认数据文件路径（相对服务工作目录）。
const DefaultXdbPath = "./data/ip2region.xdb"

// IntranetLabel 内网/回环地址的归属地标签。
const IntranetLabel = "内网"

var (
	initOnce sync.Once
	dbBuff   []byte       // xdb 全量内容（只读共享）
	dbVer    *xdb.Version // 从文件头探测出的 IP 版本
	// Searcher 官方实现非并发安全（Search 会写内部 ioCount），
	// 用 sync.Pool 按 goroutine 复用，多个 Searcher 共享同一份 dbBuff。
	searcherPool sync.Pool
)

// loadDB 惰性加载 xdb 到内存，进程内只执行一次；任一步失败则保持降级状态。
func loadDB() {
	path := strings.TrimSpace(os.Getenv(EnvXdbPath))
	if path == "" {
		path = DefaultXdbPath
	}
	buff, err := xdb.LoadContentFromFile(path)
	if err != nil {
		return // 文件缺失/不可读：降级，Lookup 对公网 IP 返回空串
	}
	header, err := xdb.LoadHeaderFromBuff(buff)
	if err != nil {
		return
	}
	ver, err := xdb.VersionFromHeader(header)
	if err != nil {
		return
	}
	dbBuff, dbVer = buff, ver
}

// Enabled 报告离线库是否加载成功（首次调用触发加载）。
func Enabled() bool {
	initOnce.Do(loadDB)
	return dbBuff != nil
}

// Lookup 返回 IP 的「省 市 ISP」精简归属地串（如「广东省 深圳市 电信」）。
// 内网/回环/链路本地地址返回「内网」；空串、无效 IP、离线库未加载或查询
// 失败一律返回空串，绝不报错，调用方可放心串在写入路径上。
func Lookup(ip string) string {
	host := strings.TrimSpace(ip)
	if host == "" {
		return ""
	}
	// 去掉 IPv6 zone（fe80::1%en0）与 IPv4 携带的端口（1.2.3.4:8080）。
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}
	if strings.Count(host, ":") == 1 {
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	parsed := net.ParseIP(host)
	if parsed == nil {
		return ""
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() || parsed.IsUnspecified() {
		return IntranetLabel
	}
	if !Enabled() {
		return ""
	}
	return FormatRegion(search(host))
}

// search 从池里取一个 Searcher 做一次查询；失败返回空串。
func search(host string) string {
	s, _ := searcherPool.Get().(*xdb.Searcher)
	if s == nil {
		var err error
		s, err = xdb.NewWithBuffer(dbVer, dbBuff)
		if err != nil {
			return ""
		}
	}
	region, err := s.Search(host)
	searcherPool.Put(s)
	if err != nil {
		// 典型场景：IPv4 版 xdb 查 IPv6 地址，按未知处理。
		return ""
	}
	return region
}

// FormatRegion 把 xdb 原始串压缩成「省 市 ISP」精简归属地，兼容两代数据格式：
// 旧版「国家|区域|省份|城市|ISP」与新版「国家|省份|城市|ISP|国家码」。
// 规则：剔除「0」占位与空段、末位两位大写国家码（新版 CN/US），国内地址
// 省略「中国」，相邻重复段去重，空格连接。
// 例：「中国|江苏省|南京市|0|CN」→「江苏省 南京市」，
// 「中国|0|广东省|深圳市|电信」→「广东省 深圳市 电信」。
func FormatRegion(region string) string {
	if region == "" {
		return ""
	}
	parts := strings.Split(region, "|")
	if n := len(parts); n >= 2 && isCountryCode(parts[n-1]) {
		parts = parts[:n-1] // 新版末段国家码对展示无用
	}
	keep := make([]string, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "0" {
			continue
		}
		if i == 0 && p == "中国" {
			continue // 国内地址省市已足够定位，省略国名
		}
		if n := len(keep); n > 0 && keep[n-1] == p {
			continue // 相邻重复（如「内网IP|内网IP」）去重
		}
		keep = append(keep, p)
	}
	return strings.Join(keep, " ")
}

// isCountryCode 报告 s 是否形如两位大写 ISO 国家码（CN、US）。
func isCountryCode(s string) bool {
	return len(s) == 2 && s[0] >= 'A' && s[0] <= 'Z' && s[1] >= 'A' && s[1] <= 'Z'
}
