package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	k8smount "k8s.io/utils/mount"
)

type kodofsNodeServer struct {
	k8smounter k8smount.Interface
	*csicommon.DefaultNodeServer
}

func newKodoFSNodeServer(d *csicommon.CSIDriver) csi.NodeServer {
	return &kodofsNodeServer{
		k8smounter:        k8smount.New(""),
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
	}
}

func (server *kodofsNodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	mountPath := req.GetTargetPath()
	if mountPath == "" {
		return nil, errors.New("NodePublishVolume: mountPath is empty")
	}
	log.Infof("NodePublishVolume: starting mount kodofs volume %s to path: %s", req.GetVolumeId(), mountPath)

	parameter, err := parseKodoFSPvParameter("NodePublishVolume", req.GetVolumeContext(), req.GetSecrets())
	if err != nil {
		return nil, err
	}
	if err = ensureDirectoryCreated(mountPath); err != nil {
		return nil, fmt.Errorf("NodePublishVolume: create mount path %s error: %w", mountPath, err)
	}
	if err = mountKodoFS(parameter.gatewayID, mountPath, parameter.mountServerAddress, parameter.accessToken, "/"); err != nil {
		return nil, fmt.Errorf("NodePublishVolume: failed to to mount kodofs to %s: %w", mountPath, err)
	}
	log.Infof("NodePublishVolume: kodofs volume %s is mounted on %s", req.GetVolumeId(), mountPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (server *kodofsNodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	mountPath := req.GetTargetPath()
	if mountPath == "" {
		return nil, errors.New("NodeUnpublishVolume: mountPath is empty")
	}
	log.Infof("NodeUnpublishVolume: starting umount kodofs volume from path: %s", mountPath)
	mounted, err := isKodoFSMounted(mountPath)
	if err != nil {
		log.Warnf("NodeUnpublishVolume: failed to detect mount point: %s", err)
	} else if !mounted {
		log.Warnf("NodeUnpublishVolume: mountPath is not mounted by kodofs")
	} else if err = umount(mountPath); err != nil {
		return nil, fmt.Errorf("NodeUnpublishVolume: failed to unmount kodofs: %w", err)
	} else {
		log.Infof("NodeUnpublishVolume: umounted kodofs volume from path: %s", mountPath)
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (server *kodofsNodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (server *kodofsNodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (server *kodofsNodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
