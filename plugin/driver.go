package main

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
)

const (
	TypePluginKodoFS = "kodofsplugin.storage.qiniu.com"
	TypePluginKodo   = "kodoplugin.storage.qiniu.com"
)

type Runnable interface {
	Run()
}

type KodoFSDriver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string
}

func newKodoFSDriver(nodeID, endpoint, version string) *KodoFSDriver {
	driver := &KodoFSDriver{endpoint: endpoint}

	csiDriver := csicommon.NewCSIDriver(TypePluginKodoFS, version, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})
	csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	})
	driver.csiDriver = csiDriver

	return driver
}

func (driver *KodoFSDriver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(driver.endpoint,
		csicommon.NewDefaultIdentityServer(driver.csiDriver),
		newKodoFSControllerServer(driver.csiDriver),
		newKodoFSNodeServer(driver.csiDriver),
	)
	s.Wait()
}

type KodoDriver struct {
	csiDriver *csicommon.CSIDriver
	endpoint  string
}

func newKodoDriver(nodeID, endpoint, version string) *KodoDriver {
	driver := &KodoDriver{endpoint: endpoint}

	csiDriver := csicommon.NewCSIDriver(TypePluginKodo, version, nodeID)
	csiDriver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER})
	csiDriver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	})
	driver.csiDriver = csiDriver

	return driver
}

func (driver *KodoDriver) Run() {
	s := csicommon.NewNonBlockingGRPCServer()
	s.Start(driver.endpoint,
		csicommon.NewDefaultIdentityServer(driver.csiDriver),
		newKodoControllerServer(driver.csiDriver),
		newKodoNodeServer(driver.csiDriver),
	)
	s.Wait()
}
