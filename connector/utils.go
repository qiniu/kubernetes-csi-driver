package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/Unknwon/goconfig"
	"github.com/qiniu/csi-driver/protocol"
)

const (
	RCLONE_CONFIG_KEY_TYPE                = "type"
	RCLONE_CONFIG_KEY_PROVIDER            = "provider"
	RCLONE_CONFIG_KEY_ACCESS_KEY          = "access_key_id"
	RCLONE_CONFIG_KEY_SECRET_KEY          = "secret_access_key"
	RCLONE_CONFIG_KEY_REGION              = "region"
	RCLONE_CONFIG_KEY_ENDPOINT            = "endpoint"
	RCLONE_CONFIG_KEY_LOCATION_CONSTRAINT = "location_constraint"
	RCLONE_CONFIG_KEY_ACL                 = "acl"
	RCLONE_CONFIG_KEY_STORAGE_CLASS       = "storage_class"
	RCLONE_CONFIG_KEY_NO_CHECK_BUCKET     = "no_check_bucket"
	RCLONE_CONFIG_KEY_UPLOAD_CHUNK_SIZE   = "chunk_size"
	RCLONE_CONFIG_KEY_UPLOAD_CUTOFF       = "upload_cutoff"
	RCLONE_CONFIG_KEY_UPLOAD_CONCURRENCY  = "upload_concurrency"

	RCLONE_CONFIG_S3_TYPE               = "s3"
	RCLONE_CONFIG_QINIU_PROVIDER        = "Qiniu"
	RCLONE_CONFIG_PUBLIC_READ_WRITE_ACL = "public-read-write"
	RCLONE_CONFIG_BOOL_TRUE             = "true"
)

func ensureDirectoryExists(path string) error {
	if fileInfo, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0700)
	} else if !fileInfo.IsDir() {
		return fmt.Errorf("%s exists but not a directory", path)
	}
	return nil
}

func ensureFileNotExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	} else {
		return os.Remove(path)
	}
}

func ensureCommandExists(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("cannot find command %s: %w", name, err)
	} else {
		return nil
	}
}

func writeRcloneConfig(cmd *protocol.InitKodoMountCmd) (string, error) {
	config, _ := goconfig.LoadFromReader(bytes.NewReader([]byte{}))

	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_TYPE, RCLONE_CONFIG_S3_TYPE)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_PROVIDER, RCLONE_CONFIG_QINIU_PROVIDER)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_ACCESS_KEY, cmd.AccessKey)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_SECRET_KEY, cmd.SecretKey)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_REGION, cmd.S3Region)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_ENDPOINT, cmd.S3Endpoint)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_LOCATION_CONSTRAINT, cmd.S3Region)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_ACL, RCLONE_CONFIG_PUBLIC_READ_WRITE_ACL)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_STORAGE_CLASS, cmd.StorageClass)
	config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_NO_CHECK_BUCKET, RCLONE_CONFIG_BOOL_TRUE)
	if cmd.UploadChunkSize != nil {
		config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_UPLOAD_CHUNK_SIZE, formatByteSize(*cmd.UploadChunkSize))
	}
	if cmd.UploadCutoff != nil {
		config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_UPLOAD_CUTOFF, formatByteSize(*cmd.UploadCutoff))
	}
	if cmd.UploadConcurrency != nil {
		config.SetValue(cmd.VolumeId, RCLONE_CONFIG_KEY_UPLOAD_CONCURRENCY, formatUint(*cmd.UploadConcurrency))
	}

	configPath := filepath.Join(rcloneConfigDir, cmd.VolumeId+".conf")
	return configPath, goconfig.SaveConfigFile(config, configPath)
}

var rcloneVersionRegexp, osVersionRegexp, osKernelRegexp *regexp.Regexp

func init() {
	rcloneVersionRegexp = regexp.MustCompile(`rclone\s+([^\s]+)`)
	osVersionRegexp = regexp.MustCompile(`\-\s+os/version:\s+([^\s]+)`)
	osKernelRegexp = regexp.MustCompile(`\-\s+os/kernel:\s+([^\s]+)`)
}

func getRcloneVersion() (rcloneVersion, osVersion, osKernel string, err error) {
	output, err := exec.Command(RcloneCmd, "version").Output()
	if err != nil {
		return
	}
	if matches := rcloneVersionRegexp.FindStringSubmatch(string(output)); len(matches) > 1 {
		rcloneVersion = matches[1]
	}
	if matches := osVersionRegexp.FindStringSubmatch(string(output)); len(matches) > 1 {
		osVersion = matches[1]
	}
	if matches := osKernelRegexp.FindStringSubmatch(string(output)); len(matches) > 1 {
		osKernel = matches[1]
	}
	return
}

func formatUint(i uint64) string {
	return strconv.FormatUint(i, 10)
}

func formatByteSize(i uint64) string {
	return formatUint(i) + "b"
}
