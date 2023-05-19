package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/qiniu/kubernetes-csi-driver/qiniu"
)

const (
	FIELD_BUCKET_ID                 = "bucketid"
	FIELD_BUCKET_NAME               = "bucketname"
	FIELD_SUB_DIR                   = "subdir"
	FIELD_S3_REGION                 = "s3region"
	FIELD_S3_ENDPOINT               = "s3endpoint"
	FIELD_S3_FORCE_PATH_STYLE       = "s3forcepathstyle"
	FIELD_UC_ENDPOINT               = "ucendpoint"
	FIELD_STORAGE_CLASS             = "storageclass"
	FIELD_VFS_CACHE_MODE            = "vfscachemode"
	FIELD_DIR_CACHE_DURATION        = "dircacheduration"
	FIELD_BUFFER_SIZE               = "buffersize"
	FIELD_VFS_CACHE_MAX_AGE         = "vfscachemaxage"
	FIELD_VFS_CACHE_POLL_INTERVAL   = "vfscachepollinterval"
	FIELD_VFS_WRITE_BACK            = "vfswriteback"
	FIELD_VFS_CACHE_MAX_SIZE        = "vfscachemaxsize"
	FIELD_VFS_READ_AHEAD            = "vfsreadahead"
	FIELD_VFS_FAST_FINGER_PRINT     = "vfsfastfingerprint"
	FIELD_VFS_READ_CHUNK_SIZE       = "vfsreadchunksize"
	FIELD_VFS_READ_CHUNK_SIZE_LIMIT = "vfsreadchunksizelimit"
	FIELD_NO_CHECKSUM               = "nochecksum"
	FIELD_NO_MOD_TIME               = "nomodtime"
	FIELD_NO_SEEK                   = "noseek"
	FIELD_READ_ONLY                 = "readonly"
	FIELD_VFS_READ_WAIT             = "vfsreadwait"
	FIELD_VFS_WRITE_WAIT            = "vfswritewait"
	FIELD_TRANSFERS                 = "transfers"
	FIELD_VFS_DISK_SPACE_TOTAL_SIZE = "vfsdiskspacetotalsize"
	FIELD_WRITE_BACK_CACHE          = "writebackcache"
	FIELD_UPLOAD_CUTOFF             = "uploadcutoff"
	FIELD_UPLOAD_CHUNK_SIZE         = "uploadchunksize"
	FIELD_UPLOAD_CONCURRENCY        = "uploadconcurrency"
	FIELD_DEBUG_HTTP                = "debughttp"
	FIELD_DEBUG_FUSE                = "debugfuse"
	FIELD_ORIGINAL_ACCESS_KEY       = "originalaccesskey"
	FIELD_ORIGINAL_SECRET_KEY       = "originalsecretkey"
)

type VfsCacheMode string

const (
	VFS_CACHE_MODE_OFF     VfsCacheMode = "off"
	VFS_CACHE_MODE_MINIMAL VfsCacheMode = "minimal"
	VFS_CACHE_MODE_WRITES  VfsCacheMode = "writes"
	VFS_CACHE_MODE_FULL    VfsCacheMode = "full"
)

func (mode VfsCacheMode) String() string {
	return string(mode)
}

type kodoPvParameter struct {
	kodoStorageClassParameter
	bucketID, bucketName                 string
	originalAccessKey, originalSecretKey string
	s3Endpoint                           *url.URL
	s3Region                             string
}

func parseKodoPvParameter(functionName string, ctx, secrets map[string]string) (param *kodoPvParameter, err error) {
	var p kodoPvParameter

	// 先解析 storage class 参数
	if scp, err := parseKodoStorageClassParameter(functionName, ctx, secrets); err != nil {
		return nil, err
	} else {
		p.kodoStorageClassParameter = *scp
	}

	// 再从 ctx 里解析 pv 参数
	for key, value := range ctx {
		key = strings.ToLower(key)
		switch key {
		case FIELD_ORIGINAL_ACCESS_KEY:
			p.originalAccessKey = strings.TrimSpace(value)
		case FIELD_ORIGINAL_SECRET_KEY:
			p.originalSecretKey = strings.TrimSpace(value)
		case FIELD_BUCKET_ID:
			p.bucketID = strings.TrimSpace(value)
		case FIELD_BUCKET_NAME:
			p.bucketName = strings.TrimSpace(value)
		case FIELD_S3_ENDPOINT:
			if p.s3Endpoint, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_S3_ENDPOINT, value, err)
				return
			}
		case FIELD_S3_REGION:
			p.s3Region = strings.TrimSpace(value)
		}
	}

	// ctx中未解析到的参数，尝试从secrets中解析
	if p.s3Endpoint == nil {
		if value, ok := secrets[FIELD_S3_ENDPOINT]; ok {
			if p.s3Endpoint, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_S3_ENDPOINT, value, err)
				return
			}
		}
	}
	if p.s3Region == "" {
		if value, ok := secrets[FIELD_S3_REGION]; ok {
			p.s3Region = strings.TrimSpace(value)
		}
	}
	if p.bucketID == "" {
		if value, ok := secrets[FIELD_BUCKET_ID]; ok {
			p.bucketID = strings.TrimSpace(value)
		}
	}
	if p.bucketName == "" {
		if value, ok := secrets[FIELD_BUCKET_NAME]; ok {
			p.bucketName = strings.TrimSpace(value)
		}
	}

	client := qiniu.NewKodoClient(p.accessKey, p.secretKey, p.ucEndpoint, VERSION, COMMITID)

	if p.bucketID == "" {
		if p.bucketName != "" {
			if bucket, findError := client.FindBucketByName(context.Background(), p.bucketName, true); findError != nil {
				err = fmt.Errorf("%s: failed to find bucket by %s: %w", functionName, p.bucketName, findError)
				return
			} else if bucket != nil {
				p.bucketID = bucket.ID
				p.region = bucket.KodoRegionID
			} else {
				err = fmt.Errorf("%s: cannot find bucket by %s", functionName, p.bucketName)
				return
			}
		} else {
			err = fmt.Errorf("%s: both %s and %s are empty", functionName, FIELD_BUCKET_ID, FIELD_BUCKET_NAME)
			return
		}
	}
	if p.s3Endpoint == nil {
		if s3Endpoint, getError := client.GetS3Endpoint(context.Background(), p.region); getError != nil {
			err = fmt.Errorf("%s: failed to get s3 endpoint by kodo region %s: %w", functionName, p.region, getError)
			return
		} else if s3Endpoint != nil {
			p.s3Endpoint = s3Endpoint
		} else {
			err = fmt.Errorf("%s: cannot get s3 endpoint by kodo region %s", functionName, p.region)
			return
		}
	}
	if p.s3Region == "" {
		if s3Region, getError := client.FromKodoRegionIDToS3RegionID(context.Background(), p.region); getError != nil {
			err = fmt.Errorf("%s: failed to get s3 region by kodo region %s: %w", functionName, p.region, getError)
			return
		} else if s3Region != nil {
			p.s3Region = *s3Region
		} else {
			err = fmt.Errorf("%s: cannot get s3 region by kodo region %s", functionName, p.region)
			return
		}
	}

	return &p, nil
}

type kodoStorageClassParameter struct {
	accessKey, secretKey, region                       string
	ucEndpoint                                         *url.URL
	storageClass                                       string
	subDir                                             string
	s3ForcePathStyle                                   bool
	dirCacheDuration                                   *time.Duration
	bufferSize                                         *uint64
	vfsCacheMode                                       VfsCacheMode
	vfsCacheMaxAge, vfsCachePollInterval, vfsWriteBack *time.Duration
	vfsCacheMaxSize, vfsReadAhead                      *uint64
	vfsFastFingerprint                                 bool
	vfsReadChunkSize, vfsReadChunkSizeLimit            *uint64
	noCheckSum, noModTime, noSeek, readOnly            bool
	vfsReadWait, vfsWriteWait                          *time.Duration
	transfers                                          *uint64
	vfsDiskSpaceTotalSize                              *uint64
	uploadCutoff, uploadChunkSize, uploadConcurrency   *uint64
	writeBackCache                                     bool
	debugHttp, debugFuse                               bool
}

func parseKodoStorageClassParameter(functionName string, ctx, secrets map[string]string) (param *kodoStorageClassParameter, err error) {
	p := kodoStorageClassParameter{
		s3ForcePathStyle: true,
	}

	for key, value := range ctx {
		key = strings.ToLower(key)
		switch key {
		case FIELD_ACCESS_KEY:
			p.accessKey = strings.TrimSpace(value)
		case FIELD_SECRET_KEY:
			p.secretKey = strings.TrimSpace(value)
		case FIELD_UC_ENDPOINT:
			if p.ucEndpoint, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_UC_ENDPOINT, value, err)
				return
			}
		case FIELD_REGION:
			p.region = strings.TrimSpace(value)
		case FIELD_STORAGE_CLASS:
			p.storageClass = strings.TrimSpace(value)
		case FIELD_SUB_DIR:
			p.subDir = strings.TrimSpace(value)
		case FIELD_S3_FORCE_PATH_STYLE:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_S3_FORCE_PATH_STYLE, value)
				return
			} else {
				p.s3ForcePathStyle = b
			}
		case FIELD_VFS_CACHE_MODE:
			switch toLower(value) {
			case "off", "":
				p.vfsCacheMode = VFS_CACHE_MODE_OFF
			case "min", "minimal":
				p.vfsCacheMode = VFS_CACHE_MODE_MINIMAL
			case "writes":
				p.vfsCacheMode = VFS_CACHE_MODE_WRITES
			case "full":
				p.vfsCacheMode = VFS_CACHE_MODE_FULL
			default:
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_VFS_CACHE_MODE, value)
				return
			}
		case FIELD_DIR_CACHE_DURATION:
			if d, parseError := parseDuration(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_DIR_CACHE_DURATION, parseError)
				return
			} else {
				p.dirCacheDuration = &d
			}
		case FIELD_BUFFER_SIZE:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_BUFFER_SIZE, parseError)
				return
			} else {
				p.bufferSize = &s
			}
		case FIELD_VFS_CACHE_MAX_AGE:
			if d, parseError := parseDuration(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_CACHE_MAX_AGE, parseError)
				return
			} else {
				p.vfsCacheMaxAge = &d
			}
		case FIELD_VFS_CACHE_POLL_INTERVAL:
			if d, parseError := parseDuration(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_CACHE_POLL_INTERVAL, parseError)
				return
			} else {
				p.vfsCachePollInterval = &d
			}
		case FIELD_VFS_WRITE_BACK:
			if d, parseError := parseDuration(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_WRITE_BACK, parseError)
				return
			} else {
				p.vfsWriteBack = &d
			}
		case FIELD_VFS_CACHE_MAX_SIZE:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_CACHE_MAX_SIZE, parseError)
				return
			} else {
				p.vfsCacheMaxSize = &s
			}
		case FIELD_VFS_READ_AHEAD:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_READ_AHEAD, parseError)
				return
			} else {
				p.vfsReadAhead = &s
			}
		case FIELD_VFS_FAST_FINGER_PRINT:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_VFS_FAST_FINGER_PRINT, value)
				return
			} else {
				p.vfsFastFingerprint = b
			}
		case FIELD_VFS_READ_CHUNK_SIZE:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_READ_CHUNK_SIZE, parseError)
				return
			} else {
				p.vfsReadChunkSize = &s
			}
		case FIELD_VFS_READ_CHUNK_SIZE_LIMIT:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_READ_CHUNK_SIZE_LIMIT, parseError)
				return
			} else {
				p.vfsReadChunkSizeLimit = &s
			}
		case FIELD_NO_CHECKSUM:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_NO_CHECKSUM, value)
				return
			} else {
				p.noCheckSum = b
			}
		case FIELD_NO_MOD_TIME:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_NO_MOD_TIME, value)
				return
			} else {
				p.noModTime = b
			}
		case FIELD_NO_SEEK:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_NO_SEEK, value)
				return
			} else {
				p.noSeek = b
			}
		case FIELD_READ_ONLY:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_READ_ONLY, value)
				return
			} else {
				p.readOnly = b
			}
		case FIELD_VFS_READ_WAIT:
			if d, parseError := parseDuration(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_READ_WAIT, parseError)
				return
			} else {
				p.vfsReadWait = &d
			}
		case FIELD_VFS_WRITE_WAIT:
			if d, parseError := parseDuration(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_WRITE_WAIT, parseError)
				return
			} else {
				p.vfsWriteWait = &d
			}
		case FIELD_TRANSFERS:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_TRANSFERS, parseError)
				return
			} else {
				p.transfers = &s
			}
		case FIELD_VFS_DISK_SPACE_TOTAL_SIZE:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_VFS_DISK_SPACE_TOTAL_SIZE, parseError)
				return
			} else {
				p.vfsDiskSpaceTotalSize = &s
			}
		case FIELD_WRITE_BACK_CACHE:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_WRITE_BACK_CACHE, value)
				return
			} else {
				p.writeBackCache = b
			}
		case FIELD_UPLOAD_CHUNK_SIZE:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_UPLOAD_CHUNK_SIZE, parseError)
				return
			} else {
				p.uploadChunkSize = &s
			}
		case FIELD_UPLOAD_CUTOFF:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_UPLOAD_CUTOFF, parseError)
				return
			} else {
				p.uploadCutoff = &s
			}
		case FIELD_UPLOAD_CONCURRENCY:
			if s, parseError := parseUint(value); parseError != nil {
				err = fmt.Errorf("%s: failed to parse %s: %w", functionName, FIELD_UPLOAD_CONCURRENCY, parseError)
				return
			} else {
				p.uploadConcurrency = &s
			}
		case FIELD_DEBUG_HTTP:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_DEBUG_HTTP, value)
				return
			} else {
				p.debugHttp = b
			}
		case FIELD_DEBUG_FUSE:
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_DEBUG_FUSE, value)
				return
			} else {
				p.debugFuse = b
			}
		}
	}
	if p.accessKey == "" {
		if value, ok := secrets[FIELD_ACCESS_KEY]; ok {
			p.accessKey = strings.TrimSpace(value)
		} else {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_ACCESS_KEY)
			return
		}
	}
	if p.secretKey == "" {
		if value, ok := secrets[FIELD_SECRET_KEY]; ok {
			p.secretKey = strings.TrimSpace(value)
		} else {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_SECRET_KEY)
			return
		}
	}
	if p.ucEndpoint == nil {
		if value, ok := secrets[FIELD_UC_ENDPOINT]; ok {
			if p.ucEndpoint, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_UC_ENDPOINT, value, err)
				return
			}
		} else {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_UC_ENDPOINT)
			return
		}
	}
	if p.region == "" {
		if value, ok := secrets[FIELD_REGION]; ok {
			p.region = strings.TrimSpace(value)
		} else {
			p.region = "z0"
		}
	}
	if p.storageClass == "" {
		if value, ok := secrets[FIELD_STORAGE_CLASS]; ok {
			p.storageClass = strings.TrimSpace(value)
		} else {
			p.storageClass = "STANDARD"
		}
	}
	if p.subDir == "" {
		if value, ok := secrets[FIELD_SUB_DIR]; ok {
			p.subDir = strings.TrimSpace(value)
		}
	}
	// 默认值就是true，如果是默认值，再检查一下secrets中是否设置了
	if p.s3ForcePathStyle {
		if value, ok := secrets[FIELD_S3_FORCE_PATH_STYLE]; ok {
			if b, ok := parseBool(value); !ok {
				err = fmt.Errorf("%s: unrecognized %s: %s", functionName, FIELD_S3_FORCE_PATH_STYLE, value)
				return
			} else {
				p.s3ForcePathStyle = b
			}
		}
	}
	param = &p
	return
}

func toLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func parseBool(s string) (b, ok bool) {
	switch toLower(s) {
	case "yes", "true", "y", "on":
		b = true
		ok = true
	case "no", "false", "n", "off":
		b = false
		ok = true
	default:
		ok = false
	}
	return
}

func parseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(strings.TrimSpace(s))
}

func parseUint(s string) (uint64, error) {
	return strconv.ParseUint(strings.TrimSpace(s), 10, 64)
}

func formatUint(i uint64) string {
	return strconv.FormatUint(i, 10)
}

func formatBool(b bool) string {
	return strconv.FormatBool(b)
}
