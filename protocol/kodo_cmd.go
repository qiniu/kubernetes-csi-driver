package protocol

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"
)

type InitKodoMountCmd struct {
	VolumeId              string  `json:"volume_id"`
	MountPath             string  `json:"mount_path"`
	SubDir                string  `json:"sub_dir"`
	AccessKey             string  `json:"access_key"`
	SecretKey             string  `json:"secret_key"`
	BucketId              string  `json:"bucket_id"`
	S3Region              string  `json:"s3_region"`
	S3Endpoint            string  `json:"s3_endpoint"`
	S3ForcePathStyle      bool    `json:"s3_force_path_style"`
	StorageClass          string  `json:"storage_class"`
	VfsCacheMode          string  `json:"vfs_cache_mode,omitempty"`
	DirCacheDuration      string  `json:"dir_cache_duration,omitempty"`
	BufferSize            *uint64 `json:"buffer_size,omitempty"`
	VfsCacheMaxAge        string  `json:"vfs_cache_max_age,omitempty"`
	VfsCachePollInterval  string  `json:"vfs_cache_poll_interval,omitempty"`
	VfsWriteBack          string  `json:"vfs_write_back,omitempty"`
	VfsCacheMaxSize       *uint64 `json:"vfs_cache_max_size,omitempty"`
	VfsReadAhead          *uint64 `json:"vfs_read_ahead,omitempty"`
	VfsFastFingerPrint    bool    `json:"vfs_fast_finger_print,omitempty"`
	VfsReadChunkSize      *uint64 `json:"vfs_read_chunk_size,omitempty"`
	VfsReadChunkSizeLimit *uint64 `json:"vfs_read_chunk_size_limit,omitempty"`
	NoCheckSum            bool    `json:"no_cache_sum,omitempty"`
	NoModTime             bool    `json:"no_mod_time,omitempty"`
	NoSeek                bool    `json:"no_seek,omitempty"`
	ReadOnly              bool    `json:"read_only,omitempty"`
	VfsReadWait           string  `json:"vfs_read_wait,omitempty"`
	VfsWriteWait          string  `json:"vfs_write_wait,omitempty"`
	Transfers             *uint64 `json:"transfers,omitempty"`
	VfsDiskSpaceTotalSize *uint64 `json:"vfs_disk_space_total_size,omitempty"`
	UploadCutoff          *uint64 `json:"upload_cutoff,omitempty"`
	UploadChunkSize       *uint64 `json:"upload_chunk_size,omitempty"`
	UploadConcurrency     *uint64 `json:"upload_concurrency,omitempty"`
	WriteBackCache        bool    `json:"write_back_cache,omitempty"`
	DebugHttp             bool    `json:"debug_http,omitempty"`
	DebugFuse             bool    `json:"debug_fuse,omitempty"`
	MayRunOnSystemd       bool    `json:"may_run_on_systemd"`
}

func (*InitKodoMountCmd) Command() {}

// ExecCommand 实际上Kodo的挂载是基于rclone的，这里的ExecCommand是将配置转换为rclone的命令行参数并执行
func (c *InitKodoMountCmd) ExecCommand(ctx context.Context) *exec.Cmd {
	rcloneConfigFilePath := ctx.Value(ContextKeyConfigFilePath).(string)
	userAgent := ctx.Value(ContextKeyUserAgent).(string)
	rcloneLogFilePath := ctx.Value(ContextKeyLogFilePath).(string)
	rcloneCacheDirPath := ctx.Value(ContextKeyCacheDirPath).(string)

	var cmdFlags = []string{
		"--auto-confirm",
		"--config", rcloneConfigFilePath,
		"--user-agent", fmt.Sprintf("%s/%s", userAgent, c.VolumeId),
		"--log-file", rcloneLogFilePath,
	}
	if c.BufferSize != nil {
		cmdFlags = append(cmdFlags, "--buffer-size", formatByteSize(*c.BufferSize))
	}
	if c.Transfers != nil {
		cmdFlags = append(cmdFlags, "--transfers", formatUint(*c.Transfers))
	}
	if c.DebugHttp {
		cmdFlags = append(cmdFlags, "--verbose", "--dump", "headers")
	}

	var mountFlags = []string{
		"--daemon",
		"--cache-dir", rcloneCacheDirPath,
	}
	if c.DirCacheDuration != "" {
		mountFlags = append(mountFlags, "--dir-cache-time", c.DirCacheDuration)
	}
	if c.VfsCacheMode != "" {
		mountFlags = append(mountFlags, "--vfs-cache-mode", c.VfsCacheMode)
	}
	if c.VfsCacheMaxAge != "" {
		mountFlags = append(mountFlags, "--vfs-cache-max-age", c.VfsCacheMaxAge)
	}
	if c.VfsCacheMaxSize != nil {
		mountFlags = append(mountFlags, "--vfs-cache-max-size", formatByteSize(*c.VfsCacheMaxSize))
	}
	if c.VfsCachePollInterval != "" {
		mountFlags = append(mountFlags, "--vfs-cache-poll-interval", c.VfsCachePollInterval)
	}
	if c.VfsReadAhead != nil {
		mountFlags = append(mountFlags, "--vfs-read-ahead", formatByteSize(*c.VfsReadAhead))
	}
	if c.VfsFastFingerPrint {
		mountFlags = append(mountFlags, "--vfs-fast-fingerprint")
	}
	if c.VfsWriteBack != "" {
		mountFlags = append(mountFlags, "--vfs-write-back", c.VfsWriteBack)
	}
	if c.VfsReadChunkSize != nil {
		mountFlags = append(mountFlags, "--vfs-read-chunk-size", formatByteSize(*c.VfsReadChunkSize))
	}
	if c.VfsReadChunkSizeLimit != nil {
		mountFlags = append(mountFlags, "--vfs-read-chunk-size-limit", formatByteSize(*c.VfsReadChunkSizeLimit))
	}
	if c.NoCheckSum {
		mountFlags = append(mountFlags, "--no-checksum")
	}
	if c.NoModTime {
		mountFlags = append(mountFlags, "--no-modtime")
	}
	if c.NoSeek {
		mountFlags = append(mountFlags, "--no-seek")
	}
	if c.ReadOnly {
		mountFlags = append(mountFlags, "--read-only")
	}
	if c.VfsReadWait != "" {
		mountFlags = append(mountFlags, "--vfs-read-wait", c.VfsReadWait)
	}
	if c.VfsWriteWait != "" {
		mountFlags = append(mountFlags, "--vfs-write-wait", c.VfsWriteWait)
	}
	if c.VfsDiskSpaceTotalSize != nil {
		mountFlags = append(mountFlags, "--vfs-disk-space-total-size", formatByteSize(*c.VfsDiskSpaceTotalSize))
	}
	if c.WriteBackCache {
		mountFlags = append(mountFlags, "--write-back-cache")
	}
	if c.DebugFuse {
		mountFlags = append(mountFlags, "--debug-fuse")
	}

	// 拼接命令行参数
	args := cmdFlags

	// 拼接rclone挂载命令与相关参数
	args = append(args, "mount")
	args = append(args, mountFlags...)

	// 拼接要挂载的目标bucket与其bucket内部的路径
	args = append(args, fmt.Sprintf("%s:%s/%s", c.VolumeId, c.BucketId, normalizeDirKey(c.SubDir)))

	// 拼接本地挂载点
	args = append(args, c.MountPath)

	log.Infof("rclone mount command: %s %s", RcloneCmd, strings.Join(args, " "))

	// 执行挂载命令
	if c.MayRunOnSystemd {
		return execOnSystemd(ctx, fmt.Sprintf("run-kodo-rclone-%s-%s.service", c.VolumeId, randomName(8)), RcloneCmd, args...)
	} else {
		return exec.CommandContext(ctx, RcloneCmd, args...)
	}
}

type KodoUmountCmd struct {
	VolumeId  string `json:"volume_id"`
	MountPath string `json:"mount_path"`
}

func (*KodoUmountCmd) Command() {}
