package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/qiniu/kubernetes-csi-driver/protocol"
	daemon "github.com/sevlyar/go-daemon"
	log "github.com/sirupsen/logrus"
)

const (
	// LogFilename name of log file
	LogFilename = "/var/log/qiniu/storage/csi-plugin/connector.log"
	// PIDFilename name of pid file
	PIDFilename = "/var/lib/qiniu/storage/csi-plugin/connector.pid"
	// SocketPath socket path
	SocketPath = "/var/lib/qiniu/storage/csi-plugin/connector.sock"
	// Connector name
	ConnectorName = "connector.csi-plugin.storage.qiniu.com"
	// Fusermount executable name
	FusermountCmd = "fusermount3"
	// KodoFS executable name
	KodoFSCmd = protocol.KodoFSCmd
	// Rclone executable name
	RcloneCmd = protocol.RcloneCmd
)

var (
	// 这些变量由编译期通过编译命令动态传入

	// VERSION is CSI Driver Version
	VERSION = ""

	// COMMITID is CSI Driver CommitID
	COMMITID = ""

	// BUILDTIME is CSI Driver Buildtime
	BUILDTIME = ""
)

var (
	isTest                                        = flag.Bool("test", false, "To test whether the connect could start or not")
	rcloneConfigDir, rcloneCacheDir, rcloneLogDir string
	rcloneVersion, osVersion, osKernel            string
	userAgent                                     string
)

func main() {
	// 开启日志文件名，行号，函数名
	log.SetReportCaller(true)
	// 设置日志级别
	log.SetLevel(log.DebugLevel)
	// 设置日志格式
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: time.RFC3339,
		CallerPrettyfier: func(frame *runtime.Frame) (function string, file string) {
			function = frame.Function
			file = fmt.Sprintf("%s:%d", filepath.Base(frame.File), frame.Line)
			return
		},
	})
	flag.Parse()

	log.Infof("CSI Connector Version: %s, CommitID: %s, Build time: %s\n", VERSION, COMMITID, BUILDTIME)

	var err error

	logDir := filepath.Dir(LogFilename)
	if err = ensureDirectoryExists(logDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure directory %s exists: %s", logDir, err)
		os.Exit(1)
	}

	pidDir := filepath.Dir(PIDFilename)
	if err = ensureDirectoryExists(pidDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure directory %s exists: %s", pidDir, err)
		os.Exit(1)
	}

	sockDir := filepath.Dir(SocketPath)
	if err = ensureDirectoryExists(sockDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure directory %s exists: %s", sockDir, err)
		os.Exit(1)
	}

	if userConfigDir, err := os.UserConfigDir(); err != nil {
		rcloneConfigDir = filepath.Join(os.TempDir(), ".rclone", "config")
	} else {
		rcloneConfigDir = filepath.Join(userConfigDir, "rclone")
	}

	if userCacheDir, err := os.UserCacheDir(); err != nil {
		rcloneCacheDir = filepath.Join(os.TempDir(), ".rclone", "cache")
	} else {
		rcloneCacheDir = filepath.Join(userCacheDir, "rclone")
	}

	if userLogDir, err := userLogDir(); err != nil {
		rcloneLogDir = filepath.Join(os.TempDir(), ".rclone", "log")
	} else {
		rcloneLogDir = filepath.Join(userLogDir, "rclone")
	}

	if err = ensureDirectoryExists(rcloneConfigDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure directory %s exists: %s", rcloneConfigDir, err)
		os.Exit(1)
	}
	if err = ensureDirectoryExists(rcloneCacheDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure directory %s exists: %s", rcloneCacheDir, err)
		os.Exit(1)
	}
	if err = ensureDirectoryExists(rcloneLogDir); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to ensure directory %s exists: %s", rcloneLogDir, err)
		os.Exit(1)
	}

	if err := ensureCommandExists(KodoFSCmd); err != nil {
		log.Errorf("Please make sure kodofs is installed in PATH: %s", err)
		os.Exit(1)
	}
	if err := ensureCommandExists(RcloneCmd); err != nil {
		log.Errorf("Please make sure rclone is installed in PATH: %s", err)
		os.Exit(1)
	}
	if err := ensureCommandExists(FusermountCmd); err != nil {
		log.Errorf("Please make sure fusermount3 is installed in PATH: %s", err)
		os.Exit(1)
	}

	if rcloneVersion, osVersion, osKernel, err = getRcloneVersion(); err != nil {
		log.Errorf("Failed to get rclone version", err)
		os.Exit(1)
	}

	if *isTest {
		os.Exit(0)
	}

	userAgent = fmt.Sprintf("QiniuCSIDriver/%s/%s/rclone/%s/%s/%s", VERSION, COMMITID, rcloneVersion, osVersion, osKernel)

	daemonCtx := &daemon.Context{
		PidFileName: PIDFilename,
		PidFilePerm: 0644,
		LogFileName: LogFilename,
		LogFilePerm: 0640,
		WorkDir:     "./",
		Umask:       077,
		Args:        []string{ConnectorName},
	}
	child, err := daemonCtx.Reborn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start connector as daemon: %s", err)
		os.Exit(1)
	}
	if child != nil {
		// Now we're in the parent process, exit
		return
	}
	defer daemonCtx.Release()
	// Now we're in the child process, continue
	log.Infoln("Starting connector as daemon ...")

	if err = ensureDirectoryExists(sockDir); err != nil {
		log.Errorf("Failed to ensure directory %s exists: %s", sockDir, err)
		os.Exit(1)
	}
	if err = ensureFileNotExists(SocketPath); err != nil {
		log.Errorf("Failed to ensure file %s not exists: %s", SocketPath, err)
		os.Exit(1)
	}
	socket, err := net.Listen("unix", SocketPath)
	if err != nil {
		log.Errorf("Failed to listen on socket file %s: %s", SocketPath, err)
		os.Exit(1)
	}
	defer socket.Close()
	log.Infoln("Connector daemon is started ...")

	for {
		conn, err := socket.Accept()
		if err != nil {
			log.Infof("Failed to accept connection: %s", err)
			continue
		}
		conn.SetDeadline(time.Now().Add(30 * time.Second))

		cmdIn := make(chan protocol.Cmd)
		cmdOut := make(chan protocol.Cmd)
		go handleConn(conn, cmdIn, cmdOut)
		go handleCmd(cmdIn, cmdOut)
	}
}

// 处理一个连接请求，
func handleConn(conn net.Conn, cmdIn <-chan protocol.Cmd, cmdOut chan<- protocol.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		defer conn.Close()

		marshalToConn := func(conn net.Conn, cmdName string, cmd protocol.Cmd) {
			bytes, err := json.Marshal(cmd)
			if err != nil {
				log.Errorf("Protocol marshal error: %s", err)
				return
			}
			bytes, err = json.Marshal(protocol.Request{
				Version: protocol.Version,
				Cmd:     cmdName,
				Payload: json.RawMessage(bytes),
			})
			if err != nil {
				log.Errorf("Protocol marshal error: %s", err)
				return
			}
			if _, err = conn.Write(bytes); err != nil {
				log.Errorf("Write into conn error: %s", err)
				return
			}
			if _, err = conn.Write([]byte("\n")); err != nil {
				log.Errorf("Write into conn error: %s", err)
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case cmd := <-cmdIn:
				switch cmd.(type) {
				case *protocol.ResponseDataCmd:
					marshalToConn(conn, protocol.ResponseDataCmdName, cmd)
				case *protocol.TerminateCmd:
					marshalToConn(conn, protocol.TerminateCmdName, cmd)
				}
			}
		}
	}()

	defer wg.Wait()
	defer cancel()
	defer close(cmdOut)

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		// 接收到一行数据并反序列化到 request
		var request protocol.Request
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			log.Warnf("Protocol parse error: %s", err)
			return
		}
		if request.Version != protocol.Version {
			log.Warnf("Unrecognized protocol version: %s", request.Version)
			return
		}

		// 根据 request.Cmd 的值，反序列化其 payload 到对应的实现了 Cmd 接口的结构体
		switch request.Cmd {
		case protocol.InitKodoFsMountCmdName:
			payload := new(protocol.InitKodoFSMountCmd)
			if err := json.Unmarshal([]byte(request.Payload), payload); err != nil {
				log.Warnf("Protocol %s payload parse error: %s", request.Cmd, err)
				return
			} else {
				log.Infof("Received initKodoFsMountCmd: %#v", payload)
				cmdOut <- payload
			}
		case protocol.InitKodoMountCmdName:
			payload := new(protocol.InitKodoMountCmd)
			if err := json.Unmarshal([]byte(request.Payload), payload); err != nil {
				log.Warnf("Protocol %s payload parse error: %s", request.Cmd, err)
				return
			} else {
				log.Infof("Received initKodoMountCmd: %#v", payload)
				cmdOut <- payload
			}
		case protocol.RequestDataCmdName:
			payload := new(protocol.RequestDataCmd)
			if err := json.Unmarshal([]byte(request.Payload), payload); err != nil {
				log.Warnf("Protocol %s payload parse error: %s", request.Cmd, err)
				return
			} else {
				log.Infof("Received requestDataCmd: %#v", payload)
				cmdOut <- payload
			}
		case protocol.KodoUmountCmdName:
			payload := new(protocol.KodoUmountCmd)
			if err := json.Unmarshal([]byte(request.Payload), payload); err != nil {
				log.Warnf("Protocol %s payload parse error: %s", request.Cmd, err)
				return
			} else {
				log.Infof("Received kodoUmountCmd: %#v", payload)
				cmdOut <- payload
			}
		default:
			log.Warnf("Unrecognized request cmd: %s", request.Cmd)
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Warnf("Read from conn error: %s", err)
		return
	}
}

func handleCmd(cmdOut chan<- protocol.Cmd, cmdIn <-chan protocol.Cmd) {
	defer close(cmdOut)

	var (
		isClosed         uint32         = 0
		execCmd          *exec.Cmd      = nil
		rcloneConfigPath string         = ""
		stdin            io.WriteCloser = nil
		stdout           io.ReadCloser  = nil
		stderr           io.ReadCloser  = nil
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		atomic.StoreUint32(&isClosed, 1)

		if stdin != nil {
			stdin.Close()
		}
		if stdout != nil {
			stdout.Close()
		}
		if stderr != nil {
			stderr.Close()
		}
	}()

	outputReader := func(name string, output io.Reader, isError bool) {
		for {
			buf := make([]byte, 4096)
			n, err := stdout.Read(buf)
			if err != nil {
				if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
					return
				}
				log.Errorf("Failed to read from %s: %s", name, err)
				return
			}
			if atomic.LoadUint32(&isClosed) > 0 {
				return
			}
			cmdOut <- &protocol.ResponseDataCmd{Data: string(buf[:n]), IsError: isError}
		}
	}

	execCommand := func(ec *exec.Cmd, afterRun func()) bool {
		var err error
		if execCmd != nil {
			log.Warnf("Received duplicated init cmd, which is unacceptable")
			return false
		}
		execCmd = ec
		stdin, err = execCmd.StdinPipe()
		if err != nil {
			log.Errorf("Failed to create stdin pipe: %s", err)
			return false
		}
		stdout, err = execCmd.StdoutPipe()
		if err != nil {
			log.Errorf("Failed to create stdout pipe: %s", err)
			return false
		}
		go outputReader("stdout", stdout, false)
		stderr, err = execCmd.StderrPipe()
		if err != nil {
			log.Errorf("Failed to create stderr pipe: %s", err)
			return false
		}
		go outputReader("stderr", stderr, true)
		go func() {
			defer cancel()
			err := execCmd.Run()
			if afterRun != nil {
				afterRun()
			}
			if atomic.LoadUint32(&isClosed) > 0 {
				return
			}
			cmdOut <- &protocol.TerminateCmd{Code: execCmd.ProcessState.ExitCode()}
			if err != nil {
				log.Warnf("Failed to run command (%s): %s", execCmd, err)
			} else {
				log.Infof("Run command (%s) successfully", execCmd)
			}
		}()
		return true
	}

	for {
		var err error
		select {
		case cmd, ok := <-cmdIn:
			if !ok {
				return
			}
			log.Infof("Execute cmd: %#v", cmd)
			switch c := cmd.(type) {
			case *protocol.InitKodoFSMountCmd:
				if ok := execCommand(c.ExecCommand(ctx), nil); !ok {
					return
				}
			case *protocol.InitKodoMountCmd:
				if rcloneConfigPath, err = writeRcloneConfig(c); err != nil {
					log.Warnf("Failed to write rclone config: %s", err)
					return
				}
				uuid := rcloneCacheId(c.MountPath)
				volumeCacheDir := filepath.Join(rcloneCacheDir, c.VolumeId, uuid)
				if err = ensureDirectoryExists(volumeCacheDir); err != nil {
					log.Errorf("Failed to ensure directory %s exists: %s", volumeCacheDir, err)
					return
				}
				rcloneLogFile := filepath.Join(rcloneLogDir, c.VolumeId, uuid+".log")
				if err = ensureDirectoryExists(filepath.Dir(rcloneLogFile)); err != nil {
					log.Errorf("Failed to ensure directory %s exists: %s", filepath.Dir(rcloneLogFile), err)
					return
				}
				ctx = context.WithValue(ctx, protocol.ContextKeyConfigFilePath, rcloneConfigPath)
				ctx = context.WithValue(ctx, protocol.ContextKeyUserAgent, userAgent)
				ctx = context.WithValue(ctx, protocol.ContextKeyLogFilePath, rcloneLogFile)
				ctx = context.WithValue(ctx, protocol.ContextKeyCacheDirPath, volumeCacheDir)
				if ok := execCommand(c.ExecCommand(ctx), func() { os.Remove(rcloneConfigPath) }); !ok {
					return
				}
			case *protocol.KodoUmountCmd:
				uuid := rcloneCacheId(c.MountPath)
				volumeCacheDir := filepath.Join(rcloneCacheDir, c.VolumeId, uuid)
				rcloneLogFile := filepath.Join(rcloneLogDir, c.VolumeId, uuid+".log")
				os.RemoveAll(volumeCacheDir)
				os.Remove(rcloneLogFile)
				os.Remove(filepath.Dir(rcloneLogFile))
				os.Remove(filepath.Dir(volumeCacheDir))
			case *protocol.RequestDataCmd:
				if stdin == nil {
					log.Warnf("Received RequestDataCmd when process is not started")
					return
				}
				if _, err = stdin.Write([]byte(c.Data)); err != nil {
					log.Warnf("Failed to write data into stdin: %s", err)
					return
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
