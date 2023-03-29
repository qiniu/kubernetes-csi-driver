package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/qiniu/kubernetes-csi-driver/qiniu"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kodofsControllerServer struct {
	volumes     map[string]*csi.Volume
	volumesLock sync.Mutex
	client      kubernetes.Interface
	*csicommon.DefaultControllerServer
}

func newKodoFSControllerServer(d *csicommon.CSIDriver) csi.ControllerServer {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("newKodoFSControllerServer: failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("newKodoFSControllerServer: failed to create client: %v", err)
	}

	c := &kodofsControllerServer{
		volumes:                 make(map[string]*csi.Volume),
		client:                  clientset,
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
	return c
}

func (cs *kodofsControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	pvName := req.GetName()
	log.Infof("CreateVolume: starting creating KodoFS volume %s", pvName)

	cs.volumesLock.Lock()
	defer cs.volumesLock.Unlock()

	if volume, exists := cs.volumes[pvName]; exists {
		log.Warnf("CreateVolume: volume %s already exists", pvName)
		return &csi.CreateVolumeResponse{Volume: volume}, nil
	}

	parameter, err := parseKodoFSStorageClassParameter("CreateVolume", req.GetParameters(), req.GetSecrets(), false)
	if err != nil {
		return nil, err
	}
	client := qiniu.NewKodoFSClient(parameter.accessKey, parameter.secretKey, parameter.masterServerAddress, VERSION, COMMITID)
	gatewayId, err := client.CreateVolume(ctx, pvName, pvName, parameter.region, parameter.fsType, parameter.blockSize)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume: create mount %s error: %w", pvName, err)
	}
	accessPointId, err := client.CreateAccessPoint(ctx, pvName, pvName)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume: create access point %s error: %w", pvName, err)
	}
	accessToken, err := client.GetAccessToken(ctx, accessPointId)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume: get access token %s error: %w", accessPointId, err)
	}
	log.Infof("CreateVolume: KodoFS volume %s is created", pvName)

	volumeContext := map[string]string{
		FIELD_GATEWAY_ID:            gatewayId,
		FIELD_ACCESS_POINT_ID:       accessPointId,
		FIELD_ACCESS_TOKEN:          accessToken,
		FIELD_ACCESS_KEY:            parameter.accessKey,
		FIELD_SECRET_KEY:            parameter.secretKey,
		FIELD_MOUNT_SERVER_ADDRESS:  parameter.mountServerAddress.String(),
		FIELD_MASTER_SERVER_ADDRESS: parameter.masterServerAddress.String(),
		FIELD_REGION:                parameter.region,
		FIELD_FS_TYPE:               strconv.FormatUint(uint64(parameter.fsType), 10),
		FIELD_BLOCK_SIZE:            strconv.FormatUint(uint64(parameter.blockSize), 10),
	}
	volume := &csi.Volume{
		CapacityBytes: int64(req.GetCapacityRange().GetRequiredBytes()),
		VolumeId:      pvName,
		VolumeContext: volumeContext,
	}
	cs.volumes[pvName] = volume
	return &csi.CreateVolumeResponse{Volume: volume}, nil
}

func (cs *kodofsControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeId := req.GetVolumeId()

	pvInfo, err := cs.client.CoreV1().PersistentVolumes().Get(ctx, volumeId, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("DeleteVolume: get volume %s info from Kubernetes error: %w", volumeId, err)
	}
	parameter, err := parseKodoFSPvParameter("DeleteVolume", pvInfo.Spec.CSI.VolumeAttributes, req.GetSecrets())
	if err != nil {
		return nil, err
	}

	persistentVolumeReclaimPolicy := pvInfo.Spec.PersistentVolumeReclaimPolicy
	if persistentVolumeReclaimPolicy != "" {
		log.Infof("DeleteVolume: starting deleting KodoFS volume %s (%s)", volumeId, persistentVolumeReclaimPolicy)
	} else {
		log.Infof("DeleteVolume: starting deleting KodoFS volume %s", volumeId)
	}

	cs.volumesLock.Lock()
	defer cs.volumesLock.Unlock()

	delete(cs.volumes, volumeId)

	client := qiniu.NewKodoFSClient(parameter.accessKey, parameter.secretKey, parameter.masterServerAddress, VERSION, COMMITID)

	if persistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
		if tempMountPath, err := ioutil.TempDir("", "temp-mnt-point-*"); err != nil {
			return nil, fmt.Errorf("DeleteVolume: failed to create temporary mount point: %w", err)
		} else {
			defer os.Remove(tempMountPath)
			if err = mountKodoFSLocally(ctx, parameter.gatewayID, tempMountPath, parameter.mountServerAddress, parameter.accessToken, "/"); err != nil {
				return nil, fmt.Errorf("DeleteVolume: failed to to mount kodofs to %s: %w", tempMountPath, err)
			}
			defer umount(tempMountPath)
			if entries, err := os.ReadDir(tempMountPath); err != nil {
				return nil, fmt.Errorf("DeleteVolume: failed to list all directory entries of %s: %w", tempMountPath, err)
			} else {
				for _, entry := range entries {
					toDeletePath := filepath.Join(tempMountPath, entry.Name())
					if err = os.RemoveAll(toDeletePath); err != nil {
						return nil, fmt.Errorf("DeleteVolume: failed to clean volume file %s: %w", toDeletePath, err)
					}
				}
			}
			if err = umount(tempMountPath); err != nil {
				return nil, fmt.Errorf("DeleteVolume: failed to umount %s: %w", tempMountPath, err)
			} else if err = client.RemoveAccessPoint(ctx, parameter.accessPointId); err != nil {
				return nil, fmt.Errorf("DeleteVolume: remove access point %s error: %w", parameter.accessPointId, err)
			} else if err = client.RemoveVolume(ctx, volumeId); err != nil {
				return nil, fmt.Errorf("DeleteVolume: delete volume %s error: %w", volumeId, err)
			}
			log.Infof("DeleteVolume: KodoFS volume %s is deleted", volumeId)
		}
	} else {
		if err = client.RemoveAccessPoint(ctx, parameter.accessPointId); err != nil {
			return nil, fmt.Errorf("DeleteVolume: remove access point %s error: %w", parameter.accessPointId, err)
		}
		for i := 0; ; i++ {
			newVolumeId := fmt.Sprintf("deleted-%d-%s", i, volumeId)
			if exists, err := client.IsVolumeExists(ctx, newVolumeId); err != nil {
				return nil, fmt.Errorf("DeleteVolume: get volume %s error: %w", newVolumeId, err)
			} else if !exists {
				if err = client.RenameVolume(ctx, volumeId, newVolumeId); err != nil {
					return nil, fmt.Errorf("DeleteVolume: rename volume %s => %s error: %w", volumeId, newVolumeId, err)
				}
				log.Infof("DeleteVolume: KodoFS volume %s is deleted, archive to %s", volumeId, newVolumeId)
			}
		}
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *kodofsControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *kodofsControllerServer) ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
