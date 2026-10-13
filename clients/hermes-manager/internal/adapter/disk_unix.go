//go:build !windows

package adapter

import "syscall"

// freeDiskBytes 返回路径所在卷的可用字节数。取不到时返回错误，调用方按
// 「探测失败不阻断」处理——磁盘检查是护栏不是闸门。
func freeDiskBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
