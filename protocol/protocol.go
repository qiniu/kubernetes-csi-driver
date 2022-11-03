package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
)

const (
	Version                = "v2"
	InitKodoMountCmdName   = "init_kodo_mount"
	InitKodoFsMountCmdName = "init_kodofs_mount"
	RequestDataCmdName     = "request_data"
	ResponseDataCmdName    = "response_data"
	TerminateCmdName       = "terminate"
)

type (
	Request struct {
		Version string          `json:"version"`
		Cmd     string          `json:"cmd"`
		Payload json.RawMessage `json:"payload"`
	}

	InitKodoFSMountCmd struct {
		GatewayID string `json:"gateway_id"`
		MountPath string `json:"mount_path"`
		SubDir    string `json:"sub_dir"`
	}

	InitKodoMountCmd struct {
		VolumeId              string  `json:"volume_id"`
		MountPath             string  `json:"mount_path"`
		SubDir                string  `json:"sub_dir"`
		AccessKey             string  `json:"access_key"`
		SecretKey             string  `json:"secret_key"`
		BucketId              string  `json:"bucket_id"`
		S3Region              string  `json:"s3_region"`
		S3Endpoint            string  `json:"s3_endpoint"`
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
	}

	RequestDataCmd struct {
		Data string `json:"data"`
	}

	ResponseDataCmd struct {
		IsError bool   `json:"is_error"`
		Data    string `json:"data"`
	}

	TerminateCmd struct {
		Code int `json:"code"`
	}

	Cmd interface {
		Command()
	}

	ExecutableCmd interface {
		Cmd
		ExecCommand(context.Context) *exec.Cmd
	}
)

func (*InitKodoFSMountCmd) Command() {}
func (*InitKodoMountCmd) Command()   {}
func (*RequestDataCmd) Command()     {}
func (*ResponseDataCmd) Command()    {}
func (*TerminateCmd) Command()       {}

type contextKey string

const (
	// KodoFS executable name
	KodoFSCmd = "kodofs"
	// Rclone executable name
	RcloneCmd = "rclone"

	ContextKeyConfigFilePath contextKey = "config_file_path"
	ContextKeyUserAgent      contextKey = "user_agent"
	ContextKeyLogDirPath     contextKey = "log_dir_path"
	ContextKeyCacheDirPath   contextKey = "cache_dir_path"
)

func (c *InitKodoFSMountCmd) ExecCommand(ctx context.Context) *exec.Cmd {
	var args = []string{"mount", c.GatewayID, c.MountPath, "-s", c.SubDir, "--force_reinit"}
	return exec.CommandContext(ctx, KodoFSCmd, args...)
}

func (c *InitKodoMountCmd) ExecCommand(ctx context.Context) *exec.Cmd {
	rcloneConfigFilePath := ctx.Value(ContextKeyConfigFilePath).(string)
	userAgent := ctx.Value(ContextKeyUserAgent).(string)
	rcloneLogDirPath := ctx.Value(ContextKeyLogDirPath).(string)
	rcloneCacheDirPath := ctx.Value(ContextKeyCacheDirPath).(string)

	var cmdFlags = []string{
		"--auto-confirm",
		"--config", rcloneConfigFilePath,
		"--user-agent", fmt.Sprintf("%s/%s", userAgent, c.VolumeId),
		"--log-file", filepath.Join(rcloneLogDirPath, c.VolumeId+".log")}
	if c.BufferSize != nil {
		cmdFlags = append(cmdFlags, []string{"--buffer-size", formatByteSize(*c.BufferSize)}...)
	}
	if c.Transfers != nil {
		cmdFlags = append(cmdFlags, []string{"--transfers", formatUint(*c.Transfers)}...)
	}
	if c.DebugHttp {
		cmdFlags = append(cmdFlags, []string{"--verbose", "--dump", "headers"}...)
	}
	var mountFlags = []string{"--daemon", "--cache-dir", rcloneCacheDirPath}
	if c.DirCacheDuration != "" {
		mountFlags = append(mountFlags, []string{"--dir-cache-time", c.DirCacheDuration}...)
	}
	if c.VfsCacheMode != "" {
		mountFlags = append(mountFlags, []string{"--vfs-cache-mode", c.VfsCacheMode}...)
	}
	if c.VfsCacheMaxAge != "" {
		mountFlags = append(mountFlags, []string{"--vfs-cache-max-age", c.VfsCacheMaxAge}...)
	}
	if c.VfsCacheMaxSize != nil {
		mountFlags = append(mountFlags, []string{"--vfs-cache-max-size", formatByteSize(*c.VfsCacheMaxSize)}...)
	}
	if c.VfsCachePollInterval != "" {
		mountFlags = append(mountFlags, []string{"--vfs-cache-poll-interval", c.VfsCachePollInterval}...)
	}
	if c.VfsReadAhead != nil {
		mountFlags = append(mountFlags, []string{"--vfs-read-ahead", formatByteSize(*c.VfsReadAhead)}...)
	}
	if c.VfsFastFingerPrint {
		mountFlags = append(mountFlags, []string{"--vfs-fast-fingerprint"}...)
	}
	if c.VfsWriteBack != "" {
		mountFlags = append(mountFlags, []string{"--vfs-write-back", c.VfsWriteBack}...)
	}
	if c.VfsReadChunkSize != nil {
		mountFlags = append(mountFlags, []string{"--vfs-read-chunk-size", formatByteSize(*c.VfsReadChunkSize)}...)
	}
	if c.VfsReadChunkSizeLimit != nil {
		mountFlags = append(mountFlags, []string{"--vfs-read-chunk-size-limit", formatByteSize(*c.VfsReadChunkSizeLimit)}...)
	}
	if c.NoCheckSum {
		mountFlags = append(mountFlags, []string{"--no-checksum"}...)
	}
	if c.NoModTime {
		mountFlags = append(mountFlags, []string{"--no-modtime"}...)
	}
	if c.NoSeek {
		mountFlags = append(mountFlags, []string{"--no-seek"}...)
	}
	if c.ReadOnly {
		mountFlags = append(mountFlags, []string{"--read-only"}...)
	}
	if c.VfsReadWait != "" {
		mountFlags = append(mountFlags, []string{"--vfs-read-wait", c.VfsReadWait}...)
	}
	if c.VfsWriteWait != "" {
		mountFlags = append(mountFlags, []string{"--vfs-write-wait", c.VfsWriteWait}...)
	}
	if c.VfsDiskSpaceTotalSize != nil {
		mountFlags = append(mountFlags, []string{"--vfs-disk-space-total-size", formatByteSize(*c.VfsDiskSpaceTotalSize)}...)
	}
	if c.WriteBackCache {
		mountFlags = append(mountFlags, []string{"--write-back-cache"}...)
	}
	if c.DebugFuse {
		mountFlags = append(mountFlags, []string{"--debug-fuse"}...)
	}
	var args = append(
		append(
			append(cmdFlags, "mount"), mountFlags...),
		[]string{fmt.Sprintf("%s:%s/%s", c.VolumeId, c.BucketId, c.SubDir), c.MountPath}...)
	return exec.CommandContext(ctx, RcloneCmd, args...)
}

func formatUint(i uint64) string {
	return strconv.FormatUint(i, 10)
}

func formatByteSize(i uint64) string {
	return formatUint(i) + "b"
}
