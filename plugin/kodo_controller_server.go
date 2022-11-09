package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/qiniu/csi-driver/qiniu"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type kodoControllerServer struct {
	volumes     map[string]*csi.Volume
	volumesLock sync.Mutex
	client      kubernetes.Interface
	*csicommon.DefaultControllerServer
}

func newKodoControllerServer(d *csicommon.CSIDriver) csi.ControllerServer {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("newKodoControllerServer: failed to create config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("newKodoControllerServer: failed to create client: %v", err)
	}

	c := &kodoControllerServer{
		volumes:                 make(map[string]*csi.Volume),
		client:                  clientset,
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
	}
	return c
}

func (cs *kodoControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	pvName := req.GetName()
	log.Infof("CreateVolume: starting creating Kodo bucket %s", pvName)

	cs.volumesLock.Lock()
	defer cs.volumesLock.Unlock()

	if volume, exists := cs.volumes[pvName]; exists {
		log.Warnf("CreateVolume: bucket %s already exists", pvName)
		return &csi.CreateVolumeResponse{Volume: volume}, nil
	}

	parameter, err := parseKodoStorageClassParameter("CreateVolume", req.GetParameters(), req.GetSecrets())
	if err != nil {
		return nil, err
	}
	client := qiniu.NewKodoClient(parameter.accessKey, parameter.secretKey, parameter.ucEndpoint, VERSION, COMMITID)

	bucketName := pvName + "-" + randomBucketName(16)
	bucket, err := client.FindBucketByName(ctx, bucketName, false)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume: find bucket %s error: %w", bucketName, err)
	} else if bucket == nil {
		if err = client.CreateBucket(ctx, bucketName, parameter.region); err != nil {
			return nil, fmt.Errorf("CreateVolume: create bucket %s error: %w", bucketName, err)
		}
		log.Infof("CreateVolume: Kodo bucket %s is created", bucketName)
		if bucket, err = client.FindBucketByName(ctx, bucketName, false); err != nil {
			return nil, fmt.Errorf("CreateVolume: find bucket %s error: %w", bucketName, err)
		} else if bucket == nil {
			return nil, fmt.Errorf("CreateVolume: cannot find new bucket %s", bucketName)
		}
	} else {
		parameter.region = bucket.KodoRegionID
		log.Infof("CreateVolume: Kodo bucket %s has been created, reuse it", bucketName)
	}

	s3Endpoint, err := client.GetS3Endpoint(ctx, parameter.region)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume: get s3 endpoint of %s error: %w", parameter.region, err)
	} else if s3Endpoint == nil {
		return nil, fmt.Errorf("CreateVolume: cannot get s3 endpoint of %s", parameter.region)
	}

	s3RegionId, err := client.FromKodoRegionIDToS3RegionID(ctx, parameter.region)
	if err != nil {
		return nil, fmt.Errorf("CreateVolume: get s3 region id %s error: %w", parameter.region, err)
	} else if s3RegionId == nil {
		return nil, fmt.Errorf("CreateVolume: cannot get s3 region id %s", parameter.region)
	}

	iamUserName := pvName
	iamPolicyName := normalizePolicyName(pvName)
	originalAccessKey, originalSecretKey := parameter.accessKey, parameter.secretKey
	if err = client.CreateIAMUser(context.Background(), iamUserName, randomPassword(128)); err != nil {
		return nil, fmt.Errorf("CreateVolume: create IAM user %s error: %w", iamUserName, err)
	} else if parameter.accessKey, parameter.secretKey, err = client.GetIAMUserKeyPair(context.Background(), iamUserName); err != nil {
		return nil, fmt.Errorf("CreateVolume: create key pair for IAM user %s error: %w", iamUserName, err)
	} else if err = client.CreateIAMPolicy(ctx, iamPolicyName, bucket.Name); err != nil {
		return nil, fmt.Errorf("CreateVolume: create IAM policy %s error: %w", iamPolicyName, err)
	} else if err = client.GrantIAMPolicyToUser(ctx, iamUserName, []string{iamPolicyName}); err != nil {
		return nil, fmt.Errorf("CreateVolume: grant IAM policy %s to %s error: %w", iamPolicyName, iamUserName, err)
	} else {
		log.Infof("CreateVolume: Kodo bucket %s is granted", bucket.Name)
	}

	volumeContext := map[string]string{
		FIELD_BUCKET_ID:           bucket.ID,
		FIELD_BUCKET_NAME:         bucket.Name,
		FIELD_S3_ENDPOINT:         s3Endpoint.String(),
		FIELD_S3_REGION:           *s3RegionId,
		FIELD_ACCESS_KEY:          parameter.accessKey,
		FIELD_SECRET_KEY:          parameter.secretKey,
		FIELD_ORIGINAL_ACCESS_KEY: originalAccessKey,
		FIELD_ORIGINAL_SECRET_KEY: originalSecretKey,
		FIELD_UC_ENDPOINT:         parameter.ucEndpoint.String(),
		FIELD_REGION:              parameter.region,
		FIELD_STORAGE_CLASS:       parameter.storageClass,
		FIELD_VFS_CACHE_MODE:      parameter.vfsCacheMode.String(),
	}
	if parameter.dirCacheDuration != nil {
		volumeContext[FIELD_DIR_CACHE_DURATION] = parameter.dirCacheDuration.String()
	}
	if parameter.bufferSize != nil {
		volumeContext[FIELD_BUFFER_SIZE] = formatUint(*parameter.bufferSize)
	}
	if parameter.vfsCacheMaxAge != nil {
		volumeContext[FIELD_VFS_CACHE_MAX_AGE] = parameter.vfsCacheMaxAge.String()
	}
	if parameter.vfsCachePollInterval != nil {
		volumeContext[FIELD_VFS_CACHE_POLL_INTERVAL] = parameter.vfsCachePollInterval.String()
	}
	if parameter.vfsWriteBack != nil {
		volumeContext[FIELD_VFS_WRITE_BACK] = parameter.vfsWriteBack.String()
	}
	if parameter.vfsCacheMaxSize != nil {
		volumeContext[FIELD_VFS_CACHE_MAX_SIZE] = formatUint(*parameter.vfsCacheMaxSize)
	}
	if parameter.vfsReadAhead != nil {
		volumeContext[FIELD_VFS_READ_AHEAD] = formatUint(*parameter.vfsReadAhead)
	}
	if parameter.vfsFastFingerprint {
		volumeContext[FIELD_VFS_FAST_FINGER_PRINT] = formatBool(parameter.vfsFastFingerprint)
	}
	if parameter.vfsReadChunkSize != nil {
		volumeContext[FIELD_VFS_READ_CHUNK_SIZE] = formatUint(*parameter.vfsReadChunkSize)
	}
	if parameter.vfsReadChunkSizeLimit != nil {
		volumeContext[FIELD_VFS_READ_CHUNK_SIZE_LIMIT] = formatUint(*parameter.vfsReadChunkSizeLimit)
	}
	if parameter.noCheckSum {
		volumeContext[FIELD_NO_CHECKSUM] = formatBool(parameter.noCheckSum)
	}
	if parameter.noModTime {
		volumeContext[FIELD_NO_MOD_TIME] = formatBool(parameter.noModTime)
	}
	if parameter.noSeek {
		volumeContext[FIELD_NO_SEEK] = formatBool(parameter.noSeek)
	}
	if parameter.readOnly {
		volumeContext[FIELD_READ_ONLY] = formatBool(parameter.readOnly)
	}
	if parameter.vfsReadWait != nil {
		volumeContext[FIELD_VFS_READ_WAIT] = parameter.vfsReadWait.String()
	}
	if parameter.vfsWriteWait != nil {
		volumeContext[FIELD_VFS_WRITE_WAIT] = parameter.vfsWriteWait.String()
	}
	if parameter.transfers != nil {
		volumeContext[FIELD_TRANSFERS] = formatUint(*parameter.transfers)
	}
	if parameter.vfsDiskSpaceTotalSize != nil {
		volumeContext[FIELD_VFS_DISK_SPACE_TOTAL_SIZE] = formatUint(*parameter.vfsDiskSpaceTotalSize)
	}
	if parameter.writeBackCache {
		volumeContext[FIELD_WRITE_BACK_CACHE] = formatBool(parameter.writeBackCache)
	}
	if parameter.uploadCutoff != nil {
		volumeContext[FIELD_UPLOAD_CUTOFF] = formatUint(*parameter.uploadCutoff)
	}
	if parameter.uploadChunkSize != nil {
		volumeContext[FIELD_UPLOAD_CHUNK_SIZE] = formatUint(*parameter.uploadChunkSize)
	}
	if parameter.uploadConcurrency != nil {
		volumeContext[FIELD_UPLOAD_CONCURRENCY] = formatUint(*parameter.uploadConcurrency)
	}
	if parameter.debugHttp {
		volumeContext[FIELD_DEBUG_HTTP] = formatBool(parameter.debugHttp)
	}
	if parameter.debugFuse {
		volumeContext[FIELD_DEBUG_FUSE] = formatBool(parameter.debugFuse)
	}
	volume := &csi.Volume{
		CapacityBytes: int64(req.GetCapacityRange().GetRequiredBytes()),
		VolumeId:      pvName,
		VolumeContext: volumeContext,
	}
	cs.volumes[pvName] = volume
	return &csi.CreateVolumeResponse{Volume: volume}, nil
}

func (cs *kodoControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	volumeId := req.GetVolumeId()

	pvInfo, err := cs.client.CoreV1().PersistentVolumes().Get(ctx, volumeId, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("DeleteVolume: get volume %s info from Kubernetes error: %w", volumeId, err)
	}
	parameter, err := parseKodoPvParameter("DeleteVolume", pvInfo.Spec.CSI.VolumeAttributes, req.GetSecrets())
	if err != nil {
		return nil, err
	}

	persistentVolumeReclaimPolicy := pvInfo.Spec.PersistentVolumeReclaimPolicy
	if persistentVolumeReclaimPolicy != "" {
		log.Infof("DeleteVolume: starting deleting Kodo volume %s (%s)", volumeId, persistentVolumeReclaimPolicy)
	} else {
		log.Infof("DeleteVolume: starting deleting Kodo volume %s", volumeId)
	}

	cs.volumesLock.Lock()
	defer cs.volumesLock.Unlock()

	delete(cs.volumes, volumeId)

	client := qiniu.NewKodoClient(parameter.originalAccessKey, parameter.originalSecretKey, parameter.ucEndpoint, VERSION, COMMITID)
	iamUserName := volumeId
	iamPolicyName := normalizePolicyName(volumeId)

	if err = client.RevokeIAMPolicyFromUser(ctx, iamUserName, []string{iamPolicyName}); err != nil {
		return nil, fmt.Errorf("DeleteVolume: revoke IAM policy %s from %s error: %w", iamPolicyName, iamUserName, err)
	} else if err = client.DeleteIAMPolicy(ctx, iamPolicyName); err != nil {
		return nil, fmt.Errorf("DeleteVolume: delete IAM policy %s error: %w", iamPolicyName, err)
	} else if err = client.DeleteIAMUser(ctx, iamUserName); err != nil {
		return nil, fmt.Errorf("DeleteVolume: delete IAM user %s error: %w", iamUserName, err)
	} else {
		log.Infof("DeleteVolume: Kodo bucket %s is revoked", parameter.bucketName)
	}

	if persistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
		if err = client.CleanObjects(ctx, parameter.bucketName); err != nil {
			return nil, fmt.Errorf("DeleteVolume: failed to clean all objects from %s", parameter.bucketName)
		} else if err = client.DeleteBucket(ctx, parameter.bucketName); err != nil {
			return nil, fmt.Errorf("DeleteVolume: failed to delete bucket %s", parameter.bucketName)
		} else {
			log.Infof("DeleteVolume: Kodo bucket %s is deleted", parameter.bucketName)
		}
	}

	return &csi.DeleteVolumeResponse{}, nil
}

func (cs *kodoControllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest,
) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (cs *kodoControllerServer) ControllerGetVolume(context.Context, *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
