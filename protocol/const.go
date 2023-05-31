package protocol

const (
	Version                = "v2"
	InitKodoMountCmdName   = "init_kodo_mount"
	InitKodoFsMountCmdName = "init_kodofs_mount"
	KodoUmountCmdName      = "umount_kodo"
	RequestDataCmdName     = "request_data"
	ResponseDataCmdName    = "response_data"
	TerminateCmdName       = "terminate"
)

type contextKey string

const (
	// KodoFS executable name
	KodoFSCmd = "kodofs"
	// Rclone executable name
	RcloneCmd = "rclone"

	ContextKeyConfigFilePath contextKey = "config_file_path"
	ContextKeyUserAgent      contextKey = "user_agent"
	ContextKeyLogFilePath    contextKey = "log_file_path"
	ContextKeyCacheDirPath   contextKey = "cache_dir_path"
)
