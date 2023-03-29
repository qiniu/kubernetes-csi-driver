package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	log "github.com/sirupsen/logrus"
)

const (
	KodoDriverName   = "kodo"
	KodoFSDriverName = "kodofs"
)

var (
	KubeletRootDir = "/var/lib/kubelet"
)

var (
	// VERSION is CSI Driver Version
	VERSION = ""

	// COMMITID is CSI Driver CommitID
	COMMITID = ""

	// BUILDTIME is CSI Driver Buildtime
	BUILDTIME  = ""
	endpoint   = flag.String("endpoint", "unix://tmp/csi.sock", "CSI endpoint")
	nodeID     = flag.String("nodeid", "", "Node id")
	driverName = flag.String("driver", "", "Driver Name")
	healthPort = flag.Int("health-port", 11260, "Health Port")
)

func init() {
	rootDir := os.Getenv("KUBELET_ROOT_DIR")
	if rootDir != "" {
		KubeletRootDir = rootDir
	}
}

func main() {
	flag.Parse()

	if driverName == nil {
		log.Errorf("-driver must be specified")
		os.Exit(1)
	} else {
		switch *driverName {
		case KodoDriverName, KodoFSDriverName:
			setLogAttribute(*driverName)
		default:
			log.Errorf("-driver must be either kodo or kodofs")
			os.Exit(1)
		}
	}

	if nodeID == nil {
		log.Errorf("-nodeid must be specified")
		os.Exit(1)
	}
	if err := ensureCommandExists("umount"); err != nil {
		log.Errorf("Please make sure umount is installed in PATH: %s", err)
		os.Exit(1)
	}
	if proto, addr, err := csicommon.ParseEndpoint(*endpoint); err != nil {
		log.Errorf("Invalid endpoint: %s", err)
		os.Exit(1)
	} else if proto == "unix" {
		dir := filepath.Dir(addr)
		if err = ensureDirectoryCreated(dir); err != nil {
			log.Errorf("Failed to ensure endpoint directory %s exists: %s", dir, err)
			os.Exit(1)
		}
	}

	log.Infof("CSI Driver Name: %s, nodeID: %s, endPoints: %s", *driverName, *nodeID, *endpoint)
	log.Infof("CSI Driver Version: %s, CommitID: %s, Build time: %s", VERSION, COMMITID, BUILDTIME)

	var wg sync.WaitGroup
	wg.Add(1)

	var driver Runnable = nil
	switch *driverName {
	case KodoDriverName:
		driver = newKodoDriver(*nodeID, *endpoint, VERSION)
	case KodoFSDriverName:
		driver = newKodoFSDriver(*nodeID, *endpoint, VERSION)
	default:
		log.Errorf("-driver must be either kodo or kodofs")
		os.Exit(1)
	}

	go func() {
		defer wg.Done()
		driver.Run()
	}()

	servicePort, err := strconv.Atoi(os.Getenv("SERVICE_PORT"))
	if err != nil || servicePort == 0 {
		servicePort = *healthPort
	}

	log.Info("CSI is running status.")
	log.Infof("CSI will listen on port %d.", servicePort)
	server := &http.Server{Addr: fmt.Sprintf(":%d", servicePort)}
	http.HandleFunc("/health", healthHandler)
	if err = server.ListenAndServe(); err != nil {
		log.Fatalf("Service port listen and serve err: %s", err.Error())
	}
	wg.Wait()
}
