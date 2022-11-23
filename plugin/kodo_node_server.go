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

type kodoNodeServer struct {
	k8smounter k8smount.Interface
	*csicommon.DefaultNodeServer
}

func newKodoNodeServer(d *csicommon.CSIDriver) csi.NodeServer {
	return &kodoNodeServer{
		k8smounter:        k8smount.New(""),
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
	}
}

func (server *kodoNodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	mountPath := req.GetTargetPath()
	if mountPath == "" {
		return nil, errors.New("NodePublishVolume: mountPath is empty")
	}
	log.Infof("NodePublishVolume: starting mount kodo volume %s to path: %s", req.GetVolumeId(), mountPath)

	parameter, err := parseKodoPvParameter("NodePublishVolume", req.GetVolumeContext(), req.GetSecrets())
	if err != nil {
		return nil, err
	}

	if err = ensureDirectoryCreated(mountPath); err != nil {
		return nil, fmt.Errorf("NodePublishVolume: create mount path %s error: %w", mountPath, err)
	}
	if err = mountKodo(req.GetVolumeId(), mountPath, "", parameter.accessKey, parameter.secretKey,
		parameter.bucketID, parameter.s3Region, parameter.s3Endpoint.String(), parameter.storageClass,
		parameter.vfsCacheMode, parameter.dirCacheDuration, parameter.bufferSize,
		parameter.vfsCacheMaxAge, parameter.vfsCachePollInterval, parameter.vfsWriteBack, parameter.vfsCacheMaxSize,
		parameter.vfsReadAhead, parameter.vfsFastFingerprint, parameter.vfsReadChunkSize, parameter.vfsReadChunkSizeLimit,
		parameter.noCheckSum, parameter.noModTime, parameter.noSeek, parameter.readOnly,
		parameter.vfsReadWait, parameter.vfsWriteWait, parameter.transfers, parameter.vfsDiskSpaceTotalSize, parameter.writeBackCache,
		parameter.uploadCutoff, parameter.uploadChunkSize, parameter.uploadConcurrency, parameter.debugHttp, parameter.debugFuse); err != nil {
		return nil, fmt.Errorf("NodePublishVolume: failed to to mount kodo to %s: %w", mountPath, err)
	}
	log.Infof("NodePublishVolume: kodo volume %s is mounted on %s", req.GetVolumeId(), mountPath)
	return &csi.NodePublishVolumeResponse{}, nil
}

func (server *kodoNodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	mountPath := req.GetTargetPath()
	if mountPath == "" {
		return nil, errors.New("NodeUnpublishVolume: mountPath is empty")
	}
	log.Infof("NodeUnpublishVolume: starting umount kodo volume from path: %s", mountPath)
	mounted, err := isKodoMounted(mountPath)
	if err != nil {
		log.Warnf("NodeUnpublishVolume: failed to detect mount point: %s", err)
	} else if !mounted {
		log.Warnf("NodeUnpublishVolume: mountPath is not mounted by kodo")
	} else if err = umount(mountPath); err != nil {
		return nil, fmt.Errorf("NodeUnpublishVolume: failed to unmount kodo: %w", err)
	} else {
		log.Infof("NodeUnpublishVolume: umounted kodo volume from path: %s", mountPath)
	}
	if err = cleanAfterKodoUmount(req.VolumeId, mountPath); err != nil {
		log.Warnf("NodeUnpublishVolume: failed to clean kodo volume cache and log files: %s", err)
	} else {
		log.Infof("NodeUnpublishVolume: kodo volume cache and log files are cleaned")
	}
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (server *kodoNodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (server *kodoNodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (server *kodoNodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
