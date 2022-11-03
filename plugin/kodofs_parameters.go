package main

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	FIELD_GATEWAY_ID            = "gatewayid"
	FIELD_ACCESS_POINT_ID       = "accesspointid"
	FIELD_ACCESS_TOKEN          = "accesstoken"
	FIELD_MOUNT_SERVER_ADDRESS  = "mntsvraddr"
	FIELD_ACCESS_KEY            = "accesskey"
	FIELD_SECRET_KEY            = "secretkey"
	FIELD_MASTER_SERVER_ADDRESS = "mastersvraddr"
	FIELD_REGION                = "region"
	FIELD_FS_TYPE               = "fstype"
	FIELD_BLOCK_SIZE            = "blocksize"
)

type kodofsPvParameter struct {
	kodofsStorageClassParameter
	gatewayID     string
	accessToken   string
	accessPointId string
}

func parseKodoFSPvParameter(functionName string, ctx, secrets map[string]string) (param *kodofsPvParameter, err error) {
	var p kodofsPvParameter

	if scp, _ := parseKodoFSStorageClassParameter(functionName, ctx, make(map[string]string), true); scp != nil {
		p.kodofsStorageClassParameter = *scp
	}

	for key, value := range ctx {
		key = strings.ToLower(key)
		switch key {
		case FIELD_GATEWAY_ID:
			p.gatewayID = strings.TrimSpace(value)
		case FIELD_ACCESS_POINT_ID:
			p.accessPointId = strings.TrimSpace(value)
		case FIELD_ACCESS_TOKEN:
			p.accessToken = strings.TrimSpace(value)
		case FIELD_MOUNT_SERVER_ADDRESS:
			// Don't have to handle it here
		}
	}
	if p.gatewayID == "" {
		if value, ok := secrets[FIELD_GATEWAY_ID]; ok {
			p.gatewayID = strings.TrimSpace(value)
		} else {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_GATEWAY_ID)
			return
		}
	}
	if p.mountServerAddress == nil {
		if value, ok := secrets[FIELD_MOUNT_SERVER_ADDRESS]; ok {
			if p.mountServerAddress, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_MOUNT_SERVER_ADDRESS, value, err)
				return
			}
		} else {
			err = fmt.Errorf("%s: %s is empty", FIELD_MOUNT_SERVER_ADDRESS, functionName)
			return
		}
	}
	if p.accessToken == "" {
		if value, ok := secrets[FIELD_ACCESS_TOKEN]; ok {
			p.accessToken = strings.TrimSpace(value)
		} else {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_ACCESS_TOKEN)
			return
		}
	}
	param = &p
	return
}

type kodofsStorageClassParameter struct {
	accessKey, secretKey                    string
	mountServerAddress, masterServerAddress *url.URL
	region                                  string
	fsType                                  uint8
	blockSize                               uint32
}

func parseKodoFSStorageClassParameter(functionName string, ctx, secrets map[string]string, ignoreSecrets bool) (param *kodofsStorageClassParameter, err error) {
	var (
		p                     kodofsStorageClassParameter
		fsType64, blockSize64 uint64
	)

	for key, value := range ctx {
		key = strings.ToLower(key)
		switch key {
		case FIELD_ACCESS_KEY:
			p.accessKey = strings.TrimSpace(value)
		case FIELD_SECRET_KEY:
			p.secretKey = strings.TrimSpace(value)
		case FIELD_MOUNT_SERVER_ADDRESS:
			if p.mountServerAddress, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_MOUNT_SERVER_ADDRESS, value, err)
				return
			}
		case FIELD_MASTER_SERVER_ADDRESS:
			if p.masterServerAddress, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_MASTER_SERVER_ADDRESS, value, err)
				return
			}
		case FIELD_REGION:
			p.region = strings.TrimSpace(value)
		case FIELD_FS_TYPE:
			if fsType64, err = parseUint(value); err == nil {
				p.fsType = uint8(fsType64)
			} else {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_FS_TYPE, value, err)
				return
			}
		case FIELD_BLOCK_SIZE:
			if blockSize64, err = parseUint(value); err == nil {
				p.blockSize = uint32(blockSize64)
			} else {
				err = fmt.Errorf("%s: invalid %s: %s", functionName, FIELD_BLOCK_SIZE, value)
				return
			}
		}
	}
	if p.accessKey == "" {
		if value, ok := secrets[FIELD_ACCESS_KEY]; ok {
			p.accessKey = strings.TrimSpace(value)
		} else if !ignoreSecrets {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_ACCESS_KEY)
			return
		}
	}
	if p.secretKey == "" {
		if value, ok := secrets[FIELD_SECRET_KEY]; ok {
			p.secretKey = strings.TrimSpace(value)
		} else if !ignoreSecrets {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_SECRET_KEY)
			return
		}
	}
	if p.mountServerAddress == nil {
		if value, ok := secrets[FIELD_MOUNT_SERVER_ADDRESS]; ok {
			if p.mountServerAddress, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_MOUNT_SERVER_ADDRESS, value, err)
				return
			}
		} else if !ignoreSecrets {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_MOUNT_SERVER_ADDRESS)
			return
		}
	}
	if p.masterServerAddress == nil {
		if value, ok := secrets[FIELD_MASTER_SERVER_ADDRESS]; ok {
			if p.masterServerAddress, err = parseUrl(value); err != nil {
				err = fmt.Errorf("%s: invalid %s: %s: %w", functionName, FIELD_MASTER_SERVER_ADDRESS, value, err)
				return
			}
		} else if !ignoreSecrets {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_MASTER_SERVER_ADDRESS)
			return
		}
	}
	if p.region == "" {
		if value, ok := secrets[FIELD_REGION]; ok {
			p.region = strings.TrimSpace(value)
		} else if !ignoreSecrets {
			err = fmt.Errorf("%s: %s is empty", functionName, FIELD_REGION)
			return
		}
	}
	if p.blockSize == 0 {
		p.blockSize = 1 << 22
	}
	param = &p
	return
}

func parseUrl(s string) (*url.URL, error) {
	return url.Parse(strings.TrimSpace(s))
}
