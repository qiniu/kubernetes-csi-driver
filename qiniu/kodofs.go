package qiniu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
)

type KodoFSClient struct {
	httpClient *http.Client
	masterUrl  *url.URL
}

func NewKodoFSClient(accessKey, secretKey string, masterUrl *url.URL, version, commitId string) *KodoFSClient {
	httpClient := new(http.Client)
	transport := NewUserAgentTransport(fmt.Sprintf("QiniuCSIDriver/%s/%s/kodofs", version, commitId), httpClient.Transport)
	transport = NewQiniuAuthTransport(accessKey, secretKey, transport, true)
	httpClient.Transport = transport
	return &KodoFSClient{httpClient: httpClient, masterUrl: masterUrl}
}

func (client *KodoFSClient) CreateVolume(ctx context.Context, volumeName, description, region string, fsType uint8, blockSize uint32) (string, error) {
	type Request struct {
		Description string `json:"description"`
		Region      string `json:"region"`
		FsType      uint8  `json:"fsType"`
		BlockSize   uint32 `json:"blockSize"`
		VolumeName  string `json:"volumeName"`
	}
	type Response struct {
		GatewayId string `json:"volume"`
	}
	body, err := json.Marshal(&Request{
		Description: description,
		Region:      region,
		FsType:      fsType,
		BlockSize:   blockSize,
		VolumeName:  volumeName,
	})
	if err != nil {
		return "", fmt.Errorf("KodoFSClient.CreateVolume: marshal json request body err: %w", err)
	}
	url := client.masterUrl.String() + "/v1/kodofs-master/volume/create"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("KodoFSClient.CreateVolume: create request err: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	var response Response
	if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return "", fmt.Errorf("KodoFSClient.CreateVolume: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bytes, err := ioutil.ReadAll(resp.Body); err != nil {
			return "", fmt.Errorf("KodoFSClient.CreateVolume: read response err: %w", err)
		} else if errBody, err := parseKodoFSErrorFromResponseBody(bytes); err != nil {
			return "", err
		} else if errBody != nil {
			return "", errBody
		} else if err = json.Unmarshal(bytes, &response); err != nil {
			return "", fmt.Errorf("KodoFSClient.CreateVolume: parse response body err: %w", err)
		} else {
			return response.GatewayId, nil
		}
	}
}

func (client *KodoFSClient) CreateAccessPoint(ctx context.Context, volumeName, description string) (string, error) {
	type Request struct {
		Description string   `json:"description"`
		Volume      string   `json:"volume"`
		Path        string   `json:"path"`
		Mode        []string `json:"mode"`
	}
	type Response struct {
		AccessId string `json:"accessId"`
	}
	body, err := json.Marshal(&Request{
		Description: description,
		Volume:      volumeName,
		Path:        "/",
		Mode:        []string{"read", "write", "delete"},
	})
	if err != nil {
		return "", fmt.Errorf("KodoFSClient.CreateAccessPoint: marshal json request body err: %w", err)
	}
	url := client.masterUrl.String() + "/v1/kodofs-master/accessPoint/create"
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("KodoFSClient.CreateAccessPoint: create request err: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	var response Response
	if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return "", fmt.Errorf("KodoFSClient.CreateAccessPoint: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bytes, err := ioutil.ReadAll(resp.Body); err != nil {
			return "", fmt.Errorf("KodoFSClient.CreateAccessPoint: read response err: %w", err)
		} else if errBody, err := parseKodoFSErrorFromResponseBody(bytes); err != nil {
			return "", err
		} else if errBody != nil {
			return "", errBody
		} else if err = json.Unmarshal(bytes, &response); err != nil {
			return "", fmt.Errorf("KodoFSClient.CreateAccessPoint: parse response body err: %w", err)
		} else {
			return response.AccessId, nil
		}
	}
}

func (client *KodoFSClient) GetAccessToken(ctx context.Context, accessPointId string) (string, error) {
	type Response struct {
		AccessToken string `json:"accessToken"`
	}
	var response Response
	queryPairs := make(url.Values)
	queryPairs.Add("accessId", accessPointId)
	url := client.masterUrl.String() + "/v1/kodofs-master/accessPoint/info?" + queryPairs.Encode()
	if request, err := http.NewRequest(http.MethodGet, url, http.NoBody); err != nil {
		return "", fmt.Errorf("KodoFSClient.GetAccessToken: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return "", fmt.Errorf("KodoFSClient.GetAccessToken: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bytes, err := ioutil.ReadAll(resp.Body); err != nil {
			return "", fmt.Errorf("KodoFSClient.GetAccessToken: read response err: %w", err)
		} else if errBody, err := parseKodoFSErrorFromResponseBody(bytes); err != nil {
			return "", err
		} else if errBody != nil {
			return "", errBody
		} else if err = json.Unmarshal(bytes, &response); err != nil {
			return "", fmt.Errorf("KodoFSClient.GetAccessToken: parse response body err: %w", err)
		} else {
			return response.AccessToken, nil
		}
	}
}

func (client *KodoFSClient) RemoveAccessPoint(ctx context.Context, accessPointId string) error {
	type Request struct {
		AccessId string `json:"accessId"`
	}
	body, err := json.Marshal(&Request{
		AccessId: accessPointId,
	})
	if err != nil {
		return fmt.Errorf("KodoFSClient.RemoveAccessPoint: marshal json request body err: %w", err)
	}
	url := client.masterUrl.String() + "/v1/kodofs-master/accessPoint/remove"
	if request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body)); err != nil {
		return fmt.Errorf("KodoFSClient.RemoveAccessPoint: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return fmt.Errorf("KodoFSClient.RemoveAccessPoint: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bytes, err := ioutil.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("KodoFSClient.RemoveAccessPoint: read response err: %w", err)
		} else if errBody, err := parseKodoFSErrorFromResponseBody(bytes); err != nil {
			return err
		} else if errBody != nil {
			return errBody
		} else {
			return nil
		}
	}
}

func (client *KodoFSClient) IsVolumeExists(ctx context.Context, volumeName string) (bool, error) {
	queryPairs := make(url.Values)
	queryPairs.Add("volume", volumeName)
	url := client.masterUrl.String() + "/v1/kodofs-master/volume/info?" + queryPairs.Encode()
	if request, err := http.NewRequest(http.MethodGet, url, http.NoBody); err != nil {
		return false, fmt.Errorf("KodoFSClient.IsVolumeExists: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return false, fmt.Errorf("KodoFSClient.IsVolumeExists: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bytes, err := ioutil.ReadAll(resp.Body); err != nil {
			return false, fmt.Errorf("KodoFSClient.IsVolumeExists: read response err: %w", err)
		} else if errBody, err := parseKodoFSErrorFromResponseBody(bytes); err != nil {
			return false, err
		} else if errBody != nil {
			if errBody.Code == -2000 {
				return false, nil
			} else {
				return false, errBody
			}
		} else {
			return true, nil
		}
	}
}

func (client *KodoFSClient) RenameVolume(ctx context.Context, oldVolumeName, newVolumeName string) error {
	type Request struct {
		OldVolumeName string `json:"oldVolumeName"`
		NewVolumeName string `json:"newVolumeName"`
	}
	body, err := json.Marshal(&Request{
		OldVolumeName: oldVolumeName,
		NewVolumeName: newVolumeName,
	})
	if err != nil {
		return fmt.Errorf("KodoFSClient.RenameVolume: marshal json request body err: %w", err)
	}
	url := client.masterUrl.String() + "/v1/kodofs-master/volume/rename"
	if request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body)); err != nil {
		return fmt.Errorf("KodoFSClient.RenameVolume: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return fmt.Errorf("KodoFSClient.RenameVolume: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bytes, err := ioutil.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("KodoFSClient.RenameVolume: read response err: %w", err)
		} else if errBody, err := parseKodoFSErrorFromResponseBody(bytes); err != nil {
			return err
		} else if errBody != nil {
			return errBody
		} else {
			return nil
		}
	}
}

func (client *KodoFSClient) RemoveVolume(ctx context.Context, volumeName string) error {
	type Request struct {
		Volume string `json:"volume"`
	}
	body, err := json.Marshal(&Request{
		Volume: volumeName,
	})
	if err != nil {
		return fmt.Errorf("KodoFSClient.RemoveVolume: marshal json request body err: %w", err)
	}
	url := client.masterUrl.String() + "/v1/kodofs-master/volume/remove"
	if request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body)); err != nil {
		return fmt.Errorf("KodoFSClient.RemoveVolume: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return fmt.Errorf("KodoFSClient.RemoveVolume: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bytes, err := ioutil.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("KodoFSClient.RemoveVolume: read response err: %w", err)
		} else if errBody, err := parseKodoFSErrorFromResponseBody(bytes); err != nil {
			return err
		} else if errBody != nil {
			return errBody
		} else {
			return nil
		}
	}
}

type KodoFSErrorResponseBody struct {
	Code      int32  `json:"code"`
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
}

func (err *KodoFSErrorResponseBody) Error() string {
	return fmt.Sprintf("[%d][%s] %s", err.Code, err.ErrorCode, err.Message)
}

func parseKodoFSErrorFromResponseBody(respBody []byte) (*KodoFSErrorResponseBody, error) {
	var body KodoFSErrorResponseBody
	if err := json.Unmarshal(respBody, &body); err != nil {
		return nil, fmt.Errorf("parseKodoFSErrorFromResponseBody: failed to parse response body as json: %w, body: %s", err, respBody)
	} else if body.Code != 0 {
		return &body, nil
	} else {
		return nil, nil
	}
}
