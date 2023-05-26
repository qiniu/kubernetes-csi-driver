package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/sys/mountinfo"
	"github.com/qiniu/kubernetes-csi-driver/protocol"
	log "github.com/sirupsen/logrus"
)

const LOG_DIR_PATH = "/var/log/qiniu/storage/csi-plugin/"

// rotate log file by 2M bytes
// default print log to stdout and file both.
func setLogAttribute(driverName string) {
	logType := os.Getenv("LOG_TYPE")
	logType = strings.ToLower(logType)
	if logType != "stdout" && logType != "host" {
		logType = "both"
	}
	if logType == "stdout" {
		return
	}

	os.MkdirAll(LOG_DIR_PATH, os.FileMode(0755))
	logFile := filepath.Join(LOG_DIR_PATH, driverName+"plugin.log")
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		os.Exit(1)
	}

	// rotate the log file if too large
	if fi, err := f.Stat(); err == nil && fi.Size() > 2*1024*1024 {
		f.Close()
		timeStr := time.Now().Format("-2006-01-02-15:04:05")
		timedLogfile := filepath.Join(LOG_DIR_PATH, driverName+"plugin"+timeStr+".log")
		os.Rename(logFile, timedLogfile)
		f, err = os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if logType == "both" {
		mw := io.MultiWriter(os.Stdout, f)
		log.SetOutput(mw)
	} else {
		log.SetOutput(f)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	time := time.Now()
	message := "Liveness probe is OK, time:" + time.String()
	w.Write([]byte(message))
}

func ensureCommandExists(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("cannot find command %s: %w", name, err)
	} else {
		return nil
	}
}

func ensureDirectoryCreated(path string) error {
	fi, err := os.Lstat(path)

	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0755)
	} else if err != nil {
		return err
	}

	if !fi.IsDir() {
		return fmt.Errorf("%s already exist and it's not directory", path)
	}
	return nil
}

const (
	SocketPath = "/var/lib/qiniu/storage/csi-plugin/connector.sock"
)

func redirectToLog(logPrefix string, reader io.Reader) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		log.Warnf("%s: %s", logPrefix, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Warnf("Read from %s error: %s", logPrefix, err)
	}
}

func redirectToChan(logPrefix string, reader io.Reader, c chan<- string) {
	defer func() { recover() }()
	for {
		buf := make([]byte, 4096)
		n, err := reader.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				return
			}
			log.Errorf("Failed to read from %s: %s", logPrefix, err)
			return
		}
		c <- string(buf[:n])
	}
}

func mountKodoFSLocally(ctx context.Context, volumeId, gatewayID, mountPath string, mountServerAddress *url.URL, accessToken, subDir string) error {
	outputChan := make(chan string)
	defer close(outputChan)

	cmd := protocol.InitKodoFSMountCmd{
		VolumeId:     volumeId,
		GatewayID:    gatewayID,
		MountPath:    mountPath,
		SubDir:       subDir,
		RunOnSystemd: false,
	}
	execCmd := cmd.ExecCommand(ctx)
	stdin, err := execCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %s", err)
	}
	defer stdin.Close()
	stdout, err := execCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %s", err)
	}
	defer stdout.Close()
	go redirectToChan(protocol.KodoFSCmd+" mount stdout", stdout, outputChan)
	stderr, err := execCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %s", err)
	}
	defer stderr.Close()
	go redirectToLog(protocol.KodoFSCmd+" mount stderr", stderr)

	go func(ctx context.Context, input io.Writer, output <-chan string) {
		for text := range output {
			if strings.Contains(text, "please enter the master address(separate multiple addresses with commas):") {
				io.WriteString(input, mountServerAddress.String()+"\n")
			} else if strings.Contains(text, "please enter the AccessToken:") {
				io.WriteString(input, accessToken+"\n")
			} else {
				log.Infof(protocol.KodoFSCmd+" mount stdout: %s", text)
			}
		}
	}(ctx, stdin, outputChan)

	return execCmd.Run()
}

func mountKodoFS(volumeId, gatewayID, mountPath string, mountServerAddress *url.URL, accessToken, subDir string) error {
	conn, err := net.Dial("unix", SocketPath)
	if err != nil {
		return fmt.Errorf("failed to dial unix socket %s: %w", SocketPath, err)
	}
	defer conn.Close()

	if subDir == "" {
		subDir = "/"
	} else if !strings.HasPrefix(subDir, "/") {
		subDir = filepath.Join("/", subDir)
	}

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	writeCmdToConn := func(encoder *json.Encoder, cmd protocol.Cmd) error {
		buf, err := json.Marshal(cmd)
		if err != nil {
			return fmt.Errorf("failed to marshal json payload: %w", err)
		}
		switch cmd.(type) {
		case *protocol.InitKodoFSMountCmd:
			if err = encoder.Encode(makeRequest(protocol.InitKodoFsMountCmdName, buf)); err != nil {
				return fmt.Errorf("failed to write command to unix socket %s: %w", SocketPath, err)
			}
		case *protocol.RequestDataCmd:
			if err = encoder.Encode(makeRequest(protocol.RequestDataCmdName, buf)); err != nil {
				return fmt.Errorf("failed to write command to unix socket %s: %w", SocketPath, err)
			}
		}
		return nil
	}

	if err = writeCmdToConn(encoder, &protocol.InitKodoFSMountCmd{
		VolumeId:     volumeId,
		GatewayID:    gatewayID,
		MountPath:    mountPath,
		SubDir:       subDir,
		RunOnSystemd: true,
	}); err != nil {
		return err
	}

	for decoder.More() {
		var request protocol.Request
		if err = decoder.Decode(&request); err != nil {
			return fmt.Errorf("failed to decode json request: %w", err)
		}
		if request.Version != protocol.Version {
			return fmt.Errorf("unrecognized protocol version: %s", request.Version)
		}
		switch request.Cmd {
		case protocol.ResponseDataCmdName:
			var cmd protocol.ResponseDataCmd
			if err = json.Unmarshal([]byte(request.Payload), &cmd); err != nil {
				return fmt.Errorf("failed to marshal json payload: %w", err)
			}
			if cmd.IsError {
				log.Warnf("kodofs mount stderr prompt: %s", cmd.Data)
			} else if strings.Contains(cmd.Data, "please enter the master address(separate multiple addresses with commas):") {
				if err = writeCmdToConn(encoder, &protocol.RequestDataCmd{
					Data: mountServerAddress.String() + "\n",
				}); err != nil {
					return fmt.Errorf("failed to enter the master address: %w", err)
				}
			} else if strings.Contains(cmd.Data, "please enter the AccessToken:") {
				if err = writeCmdToConn(encoder, &protocol.RequestDataCmd{
					Data: accessToken + "\n",
				}); err != nil {
					return fmt.Errorf("failed to enter the AccessToken: %w", err)
				}
			} else {
				log.Infof("kodofs mount stdout prompt: %s", cmd.Data)
			}
		case protocol.TerminateCmdName:
			var cmd protocol.TerminateCmd
			if err = json.Unmarshal([]byte(request.Payload), &cmd); err != nil {
				return fmt.Errorf("failed to marshal json payload: %w", err)
			}
			if cmd.Code == 0 {
				return nil
			} else {
				return fmt.Errorf("unexpected command returns code: %d", cmd.Code)
			}
		}
	}

	return nil
}

func mountKodo(volumeId, mountPath, subDir, accessKey, secretKey, bucketId string,
	s3Region, s3Endpoint, storageClass string, s3ForcePathStyle bool,
	vfsCacheMode VfsCacheMode, dirCacheDuration *time.Duration, bufferSize *uint64,
	vfsCacheMaxAge, vfsCachePollInterval, vfsWriteBack *time.Duration, vfsCacheMaxSize, vfsReadAhead *uint64,
	vfsFastFingerPrint bool, vfsReadChunkSize, vfsReadChunkSizeLimit *uint64,
	noCheckSum, noModTime, noSeek, readOnly bool, vfsReadWait, vfsWriteWait *time.Duration,
	transfers, vfsDiskSpaceTotalSize *uint64, writeBackCache bool,
	uploadCutoff, uploadChunkSize, uploadConcurrency *uint64, debugHttp, debugFuse bool) error {

	conn, err := net.Dial("unix", SocketPath)
	if err != nil {
		return fmt.Errorf("failed to dial unix socket %s: %w", SocketPath, err)
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	decoder := json.NewDecoder(conn)

	writeCmdToConn := func(encoder *json.Encoder, cmd protocol.Cmd) error {
		buf, err := json.Marshal(cmd)
		if err != nil {
			return fmt.Errorf("failed to marshal json payload: %w", err)
		}
		switch cmd.(type) {
		case *protocol.InitKodoMountCmd:
			if err = encoder.Encode(makeRequest(protocol.InitKodoMountCmdName, buf)); err != nil {
				return fmt.Errorf("failed to write command to unix socket %s: %w", SocketPath, err)
			}
		}
		return nil
	}

	cmd := protocol.InitKodoMountCmd{
		VolumeId:           volumeId,
		MountPath:          mountPath,
		SubDir:             subDir,
		AccessKey:          accessKey,
		SecretKey:          secretKey,
		BucketId:           bucketId,
		S3Region:           s3Region,
		S3Endpoint:         s3Endpoint,
		S3ForcePathStyle:   s3ForcePathStyle,
		StorageClass:       storageClass,
		VfsCacheMode:       vfsCacheMode.String(),
		VfsFastFingerPrint: vfsFastFingerPrint,
		NoCheckSum:         noCheckSum,
		NoModTime:          noModTime,
		NoSeek:             noSeek,
		ReadOnly:           readOnly,
		WriteBackCache:     writeBackCache,
		DebugHttp:          debugHttp,
		DebugFuse:          debugFuse,
	}
	if dirCacheDuration != nil {
		cmd.DirCacheDuration = dirCacheDuration.String()
	}
	if bufferSize != nil {
		cmd.BufferSize = bufferSize
	}
	if vfsCacheMaxAge != nil {
		cmd.VfsCacheMaxAge = vfsCacheMaxAge.String()
	}
	if vfsCachePollInterval != nil {
		cmd.VfsCachePollInterval = vfsCachePollInterval.String()
	}
	if vfsWriteBack != nil {
		cmd.VfsWriteBack = vfsWriteBack.String()
	}
	if vfsCacheMaxSize != nil {
		cmd.VfsCacheMaxSize = vfsCacheMaxSize
	}
	if vfsReadAhead != nil {
		cmd.VfsReadAhead = vfsReadAhead
	}
	if vfsReadChunkSize != nil {
		cmd.VfsReadChunkSize = vfsReadChunkSize
	}
	if vfsReadChunkSizeLimit != nil {
		cmd.VfsReadChunkSizeLimit = vfsReadChunkSizeLimit
	}
	if vfsReadWait != nil {
		cmd.VfsReadWait = vfsReadWait.String()
	}
	if vfsWriteWait != nil {
		cmd.VfsWriteWait = vfsWriteWait.String()
	}
	if transfers != nil {
		cmd.Transfers = transfers
	}
	if vfsDiskSpaceTotalSize != nil {
		cmd.VfsDiskSpaceTotalSize = vfsDiskSpaceTotalSize
	}
	if uploadCutoff != nil {
		cmd.UploadCutoff = uploadCutoff
	}
	if uploadChunkSize != nil {
		cmd.UploadChunkSize = uploadChunkSize
	}
	if uploadConcurrency != nil {
		cmd.UploadConcurrency = uploadConcurrency
	}

	if err = writeCmdToConn(encoder, &cmd); err != nil {
		return err
	}

	for decoder.More() {
		var request protocol.Request
		if err = decoder.Decode(&request); err != nil {
			return fmt.Errorf("failed to decode json request: %w", err)
		}
		if request.Version != protocol.Version {
			return fmt.Errorf("unrecognized protocol version: %s", request.Version)
		}
		switch request.Cmd {
		case protocol.ResponseDataCmdName:
			var cmd protocol.ResponseDataCmd
			if err = json.Unmarshal([]byte(request.Payload), &cmd); err != nil {
				return fmt.Errorf("failed to marshal json payload: %w", err)
			}
			if cmd.IsError {
				log.Warnf("kodo mount stderr prompt: %s", cmd.Data)
			} else {
				log.Infof("kodo mount stdout prompt: %s", cmd.Data)
			}
		case protocol.TerminateCmdName:
			var cmd protocol.TerminateCmd
			if err = json.Unmarshal([]byte(request.Payload), &cmd); err != nil {
				return fmt.Errorf("failed to marshal json payload: %w", err)
			}
			if cmd.Code == 0 {
				return nil
			} else {
				return fmt.Errorf("unexpected command returns code: %d", cmd.Code)
			}
		}
	}

	return nil
}

func umount(mountPath string) error {
	_, err := exec.Command("umount", "-f", mountPath).Output()
	return err
}

func cleanAfterKodoUmount(volumeId, mountPath string) error {
	conn, err := net.Dial("unix", SocketPath)
	if err != nil {
		return fmt.Errorf("failed to dial unix socket %s: %w", SocketPath, err)
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)

	writeCmdToConn := func(encoder *json.Encoder, cmd protocol.Cmd) error {
		buf, err := json.Marshal(cmd)
		if err != nil {
			return fmt.Errorf("failed to marshal json payload: %w", err)
		}
		switch cmd.(type) {
		case *protocol.KodoUmountCmd:
			if err = encoder.Encode(makeRequest(protocol.KodoUmountCmdName, buf)); err != nil {
				return fmt.Errorf("failed to write command to unix socket %s: %w", SocketPath, err)
			}
		}
		return nil
	}

	cmd := protocol.KodoUmountCmd{
		VolumeId:  volumeId,
		MountPath: mountPath,
	}
	return writeCmdToConn(encoder, &cmd)
}

func makeRequest(cmdName string, buf []byte) *protocol.Request {
	return &protocol.Request{
		Version: protocol.Version,
		Cmd:     cmdName,
		Payload: json.RawMessage(buf),
	}
}

const (
	FuseTypeKodoFS = "fuse.KodoFS"
	FuseTypeKodo   = "fuse.rclone"
)

func isKodoFSMounted(mountPath string) (bool, error) {
	return isMounted(mountPath, FuseTypeKodoFS)
}

func isKodoMounted(mountPath string) (bool, error) {
	return isMounted(mountPath, FuseTypeKodo)
}

func isMounted(mountPath, fsType string) (bool, error) {
	info, err := mountinfo.GetMounts(func(i *mountinfo.Info) (skip bool, stop bool) {
		// 全都不跳过
		skip = false
		// 找到了就直接停止
		stop = i.Mountpoint == mountPath && i.FSType == fsType
		return
	})

	if err != nil {
		return false, fmt.Errorf("failed to find the mount point: %w", err)
	}

	return len(info) > 0, nil
}

func randomPassword(n int) string {
	const choices = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789~`!@#$%^&*()-=_+[];'<,>.?/\\\""
	return randomChoices(choices, n)
}

func randomBucketName(n int) string {
	const choices = "abcdefghijklmnopqrstuvwxyz0123456789"
	return randomChoices(choices, n)
}

func randomChoices(choices string, n int) string {
	choicesCount := len(choices)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, n)
	for i := range b {
		b[i] = choices[r.Intn(choicesCount)]
	}
	return string(b)
}

func normalizePolicyName(s string) string {
	return strings.ReplaceAll(s, "-", "")
}
