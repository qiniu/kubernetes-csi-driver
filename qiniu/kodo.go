package qiniu

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

var (
	cacheMap          sync.Map
	singleflightGroup singleflight.Group
)

type KodoClient struct {
	httpClient           *http.Client
	ucUrl                *url.URL
	accessKey, secretKey string
}

type Region struct {
	KodoRegionID string   `json:"id"`
	S3           *Service `json:"s3"`
	Rs           *Service `json:"rs"`
	Rsf          *Service `json:"rsf"`
	Api          *Service `json:"api"`
}

type Service struct {
	S3RegionID string   `json:"region_alias"`
	Domains    []string `json:"domains"`
}

type Bucket struct {
	ID           string `json:"id"`
	Name         string `json:"tbl"`
	KodoRegionID string `json:"region"`
}

// NewKodoClient 根据指定的 accessKey 和 secretKey 创建一个 KodoClient
// ucUrl 在公有云上是 https://uc.qbox.me
// version 和 commitId 用于标识当前的版本，用于 UserAgent 传递给服务端
func NewKodoClient(accessKey, secretKey string, ucUrl *url.URL, version, commitId string) *KodoClient {
	httpClient := new(http.Client)
	transport := NewUserAgentTransport(fmt.Sprintf("QiniuCSIDriver/%s/%s/kodo", version, commitId), httpClient.Transport)
	transport = NewQiniuAuthTransport(accessKey, secretKey, transport, false)
	httpClient.Transport = transport
	return &KodoClient{httpClient: httpClient, ucUrl: ucUrl, accessKey: accessKey, secretKey: secretKey}
}

// CreateBucket 根据指定的 bucketName 和 regionID 创建一个 bucket
func (client *KodoClient) CreateBucket(ctx context.Context, bucketName, regionID string) error {
	requestUrl := client.ucUrl.String() + "/mkbucketv3/" + bucketName + "/region/" + regionID + "/private/true/nodomain/true"
	if request, err := http.NewRequest(http.MethodPost, requestUrl, http.NoBody); err != nil {
		return fmt.Errorf("KodoClient.CreateBucket: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return fmt.Errorf("KodoClient.CreateBucket: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("KodoClient.CreateBucket: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			return nil
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return err
		} else if errBody != nil {
			return errBody
		} else {
			return fmt.Errorf("KodoClient.CreateBucket: invalid status code: %s", resp.Status)
		}
	}
}

func (client *KodoClient) CleanObjects(ctx context.Context, bucketName string) error {
	listedObjectResults, err := client.listObjects(ctx, bucketName)
	if err != nil {
		return err
	}
	var (
		listedObjectNamesChan = make(chan string, 10)
		errorChan             = make(chan error, 1)
		wg                    sync.WaitGroup
	)
	go func() {
		defer wg.Done()
		defer close(errorChan)
		defer close(listedObjectNamesChan)

		for listedObjectResult := range listedObjectResults {
			if listedObjectResult.Error != nil {
				errorChan <- listedObjectResult.Error
				return
			}
			listedObjectNamesChan <- listedObjectResult.ObjectName
		}
	}()
	wg.Add(1)

	if err = client.deleteObjects(ctx, bucketName, listedObjectNamesChan); err != nil {
		return err
	}
	wg.Wait()

	err, ok := <-errorChan
	if !ok {
		return nil
	} else {
		return err
	}
}

func (client *KodoClient) CreateIAMUser(ctx context.Context, userName, password string) error {
	type RequestBody struct {
		UserName string `json:"alias"`
		Password string `json:"password"`
	}

	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return err
	} else if apiEndpoint == nil {
		return fmt.Errorf("KodoClient.CreateIAMUser: cannot get api endpoint of central region")
	}
	requestBodyBytes, err := json.Marshal(RequestBody{UserName: userName, Password: password})
	if err != nil {
		return fmt.Errorf("KodoClient.CreateIAMUser: failed to marshal request body")
	}
	requestUrl := apiEndpoint.String() + "/iam/v1/users"
	if request, err := http.NewRequest(http.MethodPost, requestUrl, bytes.NewReader(requestBodyBytes)); err != nil {
		return fmt.Errorf("KodoClient.CreateIAMUser: create request err: %w", err)
	} else {
		request.Header.Set("Content-Type", "application/json")
		if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
			return fmt.Errorf("KodoClient.CreateIAMUser: send request err: %w", err)
		} else {
			defer resp.Body.Close()
			if bs, err := io.ReadAll(resp.Body); err != nil {
				return fmt.Errorf("KodoClient.CreateIAMUser: read response err: %w", err)
			} else if resp.StatusCode == http.StatusOK {
				return nil
			} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
				return err
			} else if errBody != nil {
				return errBody
			} else {
				return fmt.Errorf("KodoClient.CreateIAMUser: invalid status code: %s", resp.Status)
			}
		}
	}
}

func (client *KodoClient) GetIAMUserKeyPair(ctx context.Context, userName string) (string, string, error) {
	keyPairs, err := client.getFirstIAMUserKeyPair(ctx, userName)
	if err == nil && keyPairs != nil {
		return keyPairs[0], keyPairs[1], nil
	}
	keyPairs, err = client.createIAMUserKeyPair(ctx, userName)
	if err != nil {
		return "", "", err
	} else {
		return keyPairs[0], keyPairs[1], nil
	}
}

func (client *KodoClient) getFirstIAMUserKeyPair(ctx context.Context, userName string) (*[2]string, error) {
	type ResponseBody struct {
		Data struct {
			List []struct {
				AccessKey string `json:"access_key"`
				SecretKey string `json:"secret_key"`
			} `json:"list"`
		} `json:"data"`
	}

	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return nil, err
	} else if apiEndpoint == nil {
		return nil, fmt.Errorf("KodoClient.getFirstIAMUserKeyPair: cannot get api endpoint of central region")
	}
	requestUrl := apiEndpoint.String() + "/iam/v1/users/" + userName + "/keypairs"
	if request, err := http.NewRequest(http.MethodGet, requestUrl, http.NoBody); err != nil {
		return nil, fmt.Errorf("KodoClient.getFirstIAMUserKeyPair: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return nil, fmt.Errorf("KodoClient.getFirstIAMUserKeyPair: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return nil, fmt.Errorf("KodoClient.getFirstIAMUserKeyPair: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			var responseBody ResponseBody
			if err = json.Unmarshal(bs, &responseBody); err != nil {
				return nil, fmt.Errorf("KodoClient.getFirstIAMUserKeyPair: parse response body err: %w", err)
			}
			if len(responseBody.Data.List) > 0 {
				keyPair := [2]string{responseBody.Data.List[0].AccessKey, responseBody.Data.List[0].SecretKey}
				return &keyPair, nil
			} else {
				return nil, nil
			}
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return nil, err
		} else if errBody != nil {
			return nil, errBody
		} else {
			return nil, fmt.Errorf("KodoClient.getFirstIAMUserKeyPair: invalid status code: %s", resp.Status)
		}
	}
}

func (client *KodoClient) createIAMUserKeyPair(ctx context.Context, userName string) (*[2]string, error) {
	type ResponseBody struct {
		Data struct {
			AccessKey string `json:"access_key"`
			SecretKey string `json:"secret_key"`
		} `json:"data"`
	}
	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return nil, err
	} else if apiEndpoint == nil {
		return nil, fmt.Errorf("KodoClient.createIAMUserKeyPair: cannot get api endpoint of central region")
	}
	requestUrl := apiEndpoint.String() + "/iam/v1/users/" + userName + "/keypairs"
	if request, err := http.NewRequest(http.MethodPost, requestUrl, http.NoBody); err != nil {
		return nil, fmt.Errorf("KodoClient.createIAMUserKeyPair: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return nil, fmt.Errorf("KodoClient.createIAMUserKeyPair: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return nil, fmt.Errorf("KodoClient.createIAMUserKeyPair: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			var responseBody ResponseBody
			if err = json.Unmarshal(bs, &responseBody); err != nil {
				return nil, fmt.Errorf("KodoClient.createIAMUserKeyPair: parse response body err: %w", err)
			}
			keyPair := [2]string{responseBody.Data.AccessKey, responseBody.Data.SecretKey}
			return &keyPair, nil
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return nil, err
		} else if errBody != nil {
			return nil, errBody
		} else {
			return nil, fmt.Errorf("KodoClient.createIAMUserKeyPair: invalid status code: %s", resp.Status)
		}
	}
}

func (client *KodoClient) DeleteIAMUser(ctx context.Context, userName string) error {
	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return err
	} else if apiEndpoint == nil {
		return fmt.Errorf("KodoClient.DeleteIAMUser: cannot get api endpoint of central region")
	}
	requestUrl := apiEndpoint.String() + "/iam/v1/users/" + userName
	if request, err := http.NewRequest(http.MethodDelete, requestUrl, http.NoBody); err != nil {
		return fmt.Errorf("KodoClient.DeleteIAMUser: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return fmt.Errorf("KodoClient.DeleteIAMUser: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("KodoClient.DeleteIAMUser: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			return nil
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return err
		} else if errBody != nil {
			return errBody
		} else {
			return fmt.Errorf("KodoClient.DeleteIAMUser: invalid status code: %s", resp.Status)
		}
	}
}

func (client *KodoClient) CreateIAMPolicy(ctx context.Context, name, bucketName string) error {
	type Statement struct {
		Action   []string `json:"action"`
		Resource []string `json:"resource"`
		Effect   string   `json:"effect"`
	}
	type RequestBody struct {
		PolicyName string      `json:"alias"`
		EditType   int         `json:"edit_type"`
		Statement  []Statement `json:"statement"`
	}

	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return err
	} else if apiEndpoint == nil {
		return fmt.Errorf("KodoClient.CreateIAMPolicy: cannot get api endpoint of central region")
	}

	actions := []string{
		"kodo/get", "kodo/upload", "kodo/mkfile", "kodo/stat", "kodo/chgm", "kodo/delete", "kodo/list",
		"kodo/listParts", "kodo/abortMultipartUpload"}
	requestBodyBytes, err := json.Marshal(RequestBody{
		PolicyName: name,
		EditType:   1,
		Statement: []Statement{
			{Action: actions, Resource: []string{fmt.Sprintf("qrn:kodo:::bucket/%s", bucketName)}, Effect: "Allow"},
		}})
	if err != nil {
		return fmt.Errorf("KodoClient.CreateIAMPolicy: failed to marshal request body")
	}

	requestUrl := apiEndpoint.String() + "/iam/v1/policies"
	if request, err := http.NewRequest(http.MethodPost, requestUrl, bytes.NewReader(requestBodyBytes)); err != nil {
		return fmt.Errorf("KodoClient.CreateIAMPolicy: create request err: %w", err)
	} else {
		request.Header.Set("Content-Type", "application/json")
		if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
			return fmt.Errorf("KodoClient.CreateIAMPolicy: send request err: %w", err)
		} else {
			defer resp.Body.Close()
			if bs, err := io.ReadAll(resp.Body); err != nil {
				return fmt.Errorf("KodoClient.CreateIAMPolicy: read response err: %w", err)
			} else if resp.StatusCode == http.StatusOK {
				return nil
			} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
				return err
			} else if errBody != nil {
				return errBody
			} else {
				return fmt.Errorf("KodoClient.CreateIAMPolicy: invalid status code: %s", resp.Status)
			}
		}
	}
}

func (client *KodoClient) DeleteIAMPolicy(ctx context.Context, name string) error {
	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return err
	} else if apiEndpoint == nil {
		return fmt.Errorf("KodoClient.DeleteIAMPolicy: cannot get api endpoint of central region")
	}
	requestUrl := apiEndpoint.String() + "/iam/v1/policies/" + name
	if request, err := http.NewRequest(http.MethodDelete, requestUrl, http.NoBody); err != nil {
		return fmt.Errorf("KodoClient.DeleteIAMPolicy: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return fmt.Errorf("KodoClient.DeleteIAMPolicy: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("KodoClient.DeleteIAMPolicy: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			return nil
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return err
		} else if errBody != nil {
			return errBody
		} else {
			return fmt.Errorf("KodoClient.DeleteIAMPolicy: invalid status code: %s", resp.Status)
		}
	}
}

func (client *KodoClient) GrantIAMPolicyToUser(ctx context.Context, userName string, policyNames []string) error {
	type RequestBody struct {
		PolicyNames []string `json:"policy_aliases"`
	}
	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return err
	} else if apiEndpoint == nil {
		return fmt.Errorf("KodoClient.GrantIAMPolicyToUser: cannot get api endpoint of central region")
	}

	requestBodyBytes, err := json.Marshal(RequestBody{PolicyNames: policyNames})
	if err != nil {
		return fmt.Errorf("KodoClient.GrantIAMPolicyToUser: failed to marshal request body")
	}

	requestUrl := apiEndpoint.String() + "/iam/v1/users/" + userName + "/policies"
	if request, err := http.NewRequest(http.MethodPatch, requestUrl, bytes.NewReader(requestBodyBytes)); err != nil {
		return fmt.Errorf("KodoClient.GrantIAMPolicyToUser: create request err: %w", err)
	} else {
		request.Header.Set("Content-Type", "application/json")
		if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
			return fmt.Errorf("KodoClient.GrantIAMPolicyToUser: send request err: %w", err)
		} else {
			defer resp.Body.Close()
			if bs, err := io.ReadAll(resp.Body); err != nil {
				return fmt.Errorf("KodoClient.GrantIAMPolicyToUser: read response err: %w", err)
			} else if resp.StatusCode == http.StatusOK {
				return nil
			} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
				return err
			} else if errBody != nil {
				return errBody
			} else {
				return fmt.Errorf("KodoClient.GrantIAMPolicyToUser: invalid status code: %s", resp.Status)
			}
		}
	}
}

func (client *KodoClient) RevokeIAMPolicyFromUser(ctx context.Context, userName string, policyNames []string) error {
	type RequestBody struct {
		PolicyNames []string `json:"policy_aliases"`
	}
	apiEndpoint, err := client.GetCentralApiEndpoint(ctx)
	if err != nil {
		return err
	} else if apiEndpoint == nil {
		return fmt.Errorf("KodoClient.RevokeIAMPolicyFromUser: cannot get api endpoint of central region")
	}

	requestBodyBytes, err := json.Marshal(RequestBody{PolicyNames: policyNames})
	if err != nil {
		return fmt.Errorf("KodoClient.RevokeIAMPolicyFromUser: failed to marshal request body")
	}

	requestUrl := apiEndpoint.String() + "/iam/v1/users/" + userName + "/policies"
	if request, err := http.NewRequest(http.MethodDelete, requestUrl, bytes.NewReader(requestBodyBytes)); err != nil {
		return fmt.Errorf("KodoClient.RevokeIAMPolicyFromUser: create request err: %w", err)
	} else {
		request.Header.Set("Content-Type", "application/json")
		if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
			return fmt.Errorf("KodoClient.RevokeIAMPolicyFromUser: send request err: %w", err)
		} else {
			defer resp.Body.Close()
			if bs, err := io.ReadAll(resp.Body); err != nil {
				return fmt.Errorf("KodoClient.RevokeIAMPolicyFromUser: read response err: %w", err)
			} else if resp.StatusCode == http.StatusOK {
				return nil
			} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
				return err
			} else if errBody != nil {
				return errBody
			} else {
				return fmt.Errorf("KodoClient.RevokeIAMPolicyFromUser: invalid status code: %s", resp.Status)
			}
		}
	}
}

type ListedObjectResult struct {
	ObjectName string
	Error      error
}

func (client *KodoClient) listObjects(ctx context.Context, bucketName string) (<-chan ListedObjectResult, error) {
	type (
		ListedObjectItem struct {
			ObjectName string `json:"key"`
		}
		ListedObject struct {
			Marker string           `json:"marker"`
			Item   ListedObjectItem `json:"item"`
		}
	)

	bucket, err := client.FindBucketByName(ctx, bucketName, true)
	if err != nil {
		return nil, err
	} else if bucket == nil {
		return nil, fmt.Errorf("KodoClient.listObjects: cannot find bucket %s", bucketName)
	}

	rsfEndpoint, err := client.GetRsfEndpoint(ctx, bucket.KodoRegionID)
	if err != nil {
		return nil, err
	} else if rsfEndpoint == nil {
		return nil, fmt.Errorf("KodoClient.listObjects: cannot get rsf endpoint of %s", bucketName)
	}

	sendListObjectsRequest := func(ctx context.Context, marker string) (<-chan ListedObject, error) {
		values := make(url.Values, 2)
		values.Set("bucket", bucketName)
		if marker != "" {
			values.Set("marker", marker)
		}
		listUrl := rsfEndpoint.String() + "/v2/list?" + values.Encode()
		if request, err := http.NewRequest(http.MethodPost, listUrl, http.NoBody); err != nil {
			return nil, fmt.Errorf("KodoClient.listObjects: create request err: %w", err)
		} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
			return nil, fmt.Errorf("KodoClient.listObjects: send request err: %w", err)
		} else {
			if resp.StatusCode == http.StatusOK {
				listedObjectsChan := make(chan ListedObject, 1024)
				go func() {
					defer resp.Body.Close()
					defer close(listedObjectsChan)
					jsonDecoder := json.NewDecoder(resp.Body)
					var listedObject ListedObject

					for {
						if err = jsonDecoder.Decode(&listedObject); err != nil {
							if err != io.EOF {
								log.Warnf("KodoClient.listObjects: decode object line: %s", err)
							}
							return
						}
						select {
						case <-ctx.Done():
							return
						case listedObjectsChan <- listedObject:
						}
					}
				}()
				return listedObjectsChan, nil
			} else {
				defer resp.Body.Close()
				if bs, err := io.ReadAll(resp.Body); err != nil {
					return nil, fmt.Errorf("KodoClient.listObjects: read response err: %w", err)
				} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
					return nil, err
				} else if errBody != nil {
					return nil, errBody
				} else {
					return nil, fmt.Errorf("KodoClient.listObjects: invalid status code: %s", resp.Status)
				}
			}
		}
	}

	listedObjectNamesChan := make(chan ListedObjectResult, 1024)
	go func() {
		defer close(listedObjectNamesChan)
		var lastMarker string
		for {
			listedObjectsChan, err := sendListObjectsRequest(ctx, lastMarker)
			if err != nil {
				listedObjectNamesChan <- ListedObjectResult{Error: err}
				return
			}
			for listedObject := range listedObjectsChan {
				lastMarker = listedObject.Marker
				select {
				case <-ctx.Done():
					listedObjectNamesChan <- ListedObjectResult{Error: ctx.Err()}
					return
				case listedObjectNamesChan <- ListedObjectResult{ObjectName: listedObject.Item.ObjectName}:
				}
			}
			if lastMarker == "" {
				return
			}
		}
	}()
	return listedObjectNamesChan, nil
}

func (client *KodoClient) deleteObjects(ctx context.Context, bucketName string, objectNamesChan <-chan string) error {
	bucket, err := client.FindBucketByName(ctx, bucketName, true)
	if err != nil {
		return err
	} else if bucket == nil {
		return fmt.Errorf("KodoClient.deleteObjects: cannot find bucket %s", bucketName)
	}

	rsfEndpoint, err := client.GetRsEndpoint(ctx, bucket.KodoRegionID)
	if err != nil {
		return err
	} else if rsfEndpoint == nil {
		return fmt.Errorf("KodoClient.deleteObjects: cannot get rs endpoint of %s", bucketName)
	}

	encodeEntry := func(bucket, objectName string) string {
		entry := fmt.Sprintf("%s:%s", bucket, objectName)
		return base64.URLEncoding.EncodeToString([]byte(entry))
	}

	sendDeleteObjectsRequest := func(ctx context.Context, objectNames []string) error {
		values := make(url.Values, 1)
		for _, objectName := range objectNames {
			values.Add("op", "/delete/"+encodeEntry(bucketName, objectName))
		}
		requestUrl := rsfEndpoint.String() + "/batch"
		if request, err := http.NewRequest(http.MethodPost, requestUrl, strings.NewReader(values.Encode())); err != nil {
			return fmt.Errorf("KodoClient.deleteObjects: create request err: %w", err)
		} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
			return fmt.Errorf("KodoClient.deleteObjects: send request err: %w", err)
		} else {
			defer resp.Body.Close()
			if bs, err := io.ReadAll(resp.Body); err != nil {
				return fmt.Errorf("KodoClient.deleteObjects: read response err: %w", err)
			} else if resp.StatusCode == http.StatusOK || resp.StatusCode == 298 {
				return nil
			} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
				return err
			} else if errBody != nil {
				return errBody
			} else {
				return fmt.Errorf("KodoClient.deleteObjects: invalid status code: %s", resp.Status)
			}
		}
	}

	const (
		WORKER_COUNT   = 10
		MAX_BATCH_SIZE = 100
	)
	var (
		batchDeleteObjectsChan = make(chan []string, WORKER_COUNT)
		errorsChan             = make(chan error, WORKER_COUNT+1)
		wg                     sync.WaitGroup
	)
	defer close(errorsChan)

	for i := 0; i < WORKER_COUNT; i++ {
		go func(workerId int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					errorsChan <- ctx.Err()
					return
				case batchDeleteObjects, ok := <-batchDeleteObjectsChan:
					if !ok {
						return
					}
					if err := sendDeleteObjectsRequest(ctx, batchDeleteObjects); err != nil {
						errorsChan <- err
						return
					}
				}
			}
		}(i)
		wg.Add(1)
	}
	go func() {
		defer wg.Done()
		defer close(batchDeleteObjectsChan)
		objectNames := make([]string, 0, MAX_BATCH_SIZE)
	loop:
		for {
			select {
			case <-ctx.Done():
				errorsChan <- ctx.Err()
				return
			case objectName, ok := <-objectNamesChan:
				if !ok {
					break loop
				}
				objectNames = append(objectNames, objectName)
				if len(objectNames) >= MAX_BATCH_SIZE {
					batchDeleteObjectsChan <- objectNames
					objectNames = make([]string, 0, MAX_BATCH_SIZE)
				}
			}
		}
		if len(objectNames) > 0 {
			batchDeleteObjectsChan <- objectNames
		}
	}()
	wg.Add(1)

	wg.Wait()
	select {
	case err := <-errorsChan:
		return err
	default:
		return nil
	}
}

func (client *KodoClient) DeleteBucket(ctx context.Context, bucketName string) error {
	requestUrl := client.ucUrl.String() + "/drop/" + bucketName
	if request, err := http.NewRequest(http.MethodPost, requestUrl, http.NoBody); err != nil {
		return fmt.Errorf("KodoClient.DeleteBucket: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return fmt.Errorf("KodoClient.DeleteBucket: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return fmt.Errorf("KodoClient.DeleteBucket: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			return nil
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return err
		} else if errBody != nil {
			return errBody
		} else {
			return fmt.Errorf("KodoClient.DeleteBucket: invalid status code: %s", resp.Status)
		}
	}
}

func (client *KodoClient) GetS3Endpoint(ctx context.Context, regionID string) (*url.URL, error) {
	cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-s3Endpoint-%s", client.accessKey, client.secretKey, client.ucUrl, regionID)
	if value, err := getCacheValueByKey(cacheKey, 24*time.Hour, func() (interface{}, error) {
		return client.getS3Endpoint(ctx, regionID)
	}); err != nil {
		return nil, err
	} else {
		return value.(*url.URL), nil
	}
}

func (client *KodoClient) getS3Endpoint(ctx context.Context, regionID string) (*url.URL, error) {
	if regions, err := client.GetRegions(ctx); err != nil {
		return nil, err
	} else {
		for _, region := range regions {
			if region.KodoRegionID == regionID {
				if region.S3 != nil && len(region.S3.Domains) > 0 {
					s3Url := region.S3.Domains[0]
					if !strings.Contains(s3Url, "://") {
						s3Url = fmt.Sprintf("%s://%s", client.ucUrl.Scheme, s3Url)
					}
					if s3Endpoint, err := url.Parse(s3Url); err != nil {
						return nil, fmt.Errorf("KodoClient.getS3Endpoint: invalid s3 url %s: %w", s3Url, err)
					} else {
						return s3Endpoint, nil
					}
				}
				return nil, fmt.Errorf("KodoClient.getS3Endpoint: s3 is not configured for region %s", regionID)
			}
		}
		return nil, nil
	}
}

func (client *KodoClient) GetRsEndpoint(ctx context.Context, regionID string) (*url.URL, error) {
	cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-rsEndpoint-%s", client.accessKey, client.secretKey, client.ucUrl, regionID)
	if value, err := getCacheValueByKey(cacheKey, 24*time.Hour, func() (interface{}, error) {
		return client.getRsEndpoint(ctx, regionID)
	}); err != nil {
		return nil, err
	} else {
		return value.(*url.URL), nil
	}
}

func (client *KodoClient) getRsEndpoint(ctx context.Context, regionID string) (*url.URL, error) {
	if regions, err := client.GetRegions(ctx); err != nil {
		return nil, err
	} else {
		for _, region := range regions {
			if region.KodoRegionID == regionID {
				if region.Rs != nil && len(region.Rs.Domains) > 0 {
					rsUrl := region.Rs.Domains[0]
					if !strings.Contains(rsUrl, "://") {
						rsUrl = fmt.Sprintf("%s://%s", client.ucUrl.Scheme, rsUrl)
					}
					if rsEndpoint, err := url.Parse(rsUrl); err != nil {
						return nil, fmt.Errorf("KodoClient.getRsEndpoint: invalid rs url %s: %w", rsUrl, err)
					} else {
						return rsEndpoint, nil
					}
				}
				return nil, fmt.Errorf("KodoClient.getRsEndpoint: rs is not configured for region %s", regionID)
			}
		}
		return nil, nil
	}
}

func (client *KodoClient) GetRsfEndpoint(ctx context.Context, regionID string) (*url.URL, error) {
	cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-rsfEndpoint-%s", client.accessKey, client.secretKey, client.ucUrl, regionID)
	if value, err := getCacheValueByKey(cacheKey, 24*time.Hour, func() (interface{}, error) {
		return client.getRsfEndpoint(ctx, regionID)
	}); err != nil {
		return nil, err
	} else {
		return value.(*url.URL), nil
	}
}

func (client *KodoClient) getRsfEndpoint(ctx context.Context, regionID string) (*url.URL, error) {
	if regions, err := client.GetRegions(ctx); err != nil {
		return nil, err
	} else {
		for _, region := range regions {
			if region.KodoRegionID == regionID {
				if region.Rsf != nil && len(region.Rsf.Domains) > 0 {
					rsfUrl := region.Rsf.Domains[0]
					if !strings.Contains(rsfUrl, "://") {
						rsfUrl = fmt.Sprintf("%s://%s", client.ucUrl.Scheme, rsfUrl)
					}
					if rsfEndpoint, err := url.Parse(rsfUrl); err != nil {
						return nil, fmt.Errorf("KodoClient.getRsfEndpoint: invalid rsf url %s: %w", rsfUrl, err)
					} else {
						return rsfEndpoint, nil
					}
				}
				return nil, fmt.Errorf("KodoClient.getRsfEndpoint: rsf is not configured for region %s", regionID)
			}
		}
		return nil, nil
	}
}

func (client *KodoClient) GetCentralApiEndpoint(ctx context.Context) (*url.URL, error) {
	cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-centralApiEndpoint", client.accessKey, client.secretKey, client.ucUrl)
	if value, err := getCacheValueByKey(cacheKey, 24*time.Hour, func() (interface{}, error) {
		return client.getCentralApiEndpoint(ctx)
	}); err != nil {
		return nil, err
	} else {
		return value.(*url.URL), nil
	}
}

func (client *KodoClient) getCentralApiEndpoint(ctx context.Context) (*url.URL, error) {
	if regions, err := client.GetRegions(ctx); err != nil {
		return nil, err
	} else {
		if len(regions) == 0 {
			return nil, nil
		}
		region := regions[0]
		if region.Api != nil && len(region.Api.Domains) > 0 {
			apiUrl := region.Api.Domains[0]
			if !strings.Contains(apiUrl, "://") {
				apiUrl = fmt.Sprintf("%s://%s", client.ucUrl.Scheme, apiUrl)
			}
			if apiEndpoint, err := url.Parse(apiUrl); err != nil {
				return nil, fmt.Errorf("KodoClient.getApiEndpoint: invalid api url %s: %w", apiUrl, err)
			} else {
				return apiEndpoint, nil
			}
		}
		return nil, fmt.Errorf("KodoClient.getApiEndpoint: api is not configured for first region")
	}
}

func (client *KodoClient) FromKodoRegionIDToS3RegionID(ctx context.Context, regionID string) (*string, error) {
	cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-s3RegionId-%s", client.accessKey, client.secretKey, client.ucUrl, regionID)
	if value, err := getCacheValueByKey(cacheKey, 24*time.Hour, func() (interface{}, error) {
		return client.fromKodoRegionIDToS3RegionID(ctx, regionID)
	}); err != nil {
		return nil, err
	} else {
		return value.(*string), nil
	}
}

func (client *KodoClient) fromKodoRegionIDToS3RegionID(ctx context.Context, regionID string) (*string, error) {
	if regions, err := client.GetRegions(ctx); err != nil {
		return nil, err
	} else {
		for _, region := range regions {
			if region.KodoRegionID == regionID {
				if region.S3 != nil {
					return &region.S3.S3RegionID, nil
				} else {
					return nil, nil
				}
			}
		}
		return nil, nil
	}
}

// GetRegions 通过UC域名获取所有Region的域名信息
func (client *KodoClient) GetRegions(ctx context.Context) ([]*Region, error) {
	cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-regions", client.accessKey, client.secretKey, client.ucUrl)
	if value, err := getCacheValueByKey(cacheKey, 24*time.Hour, func() (interface{}, error) {
		return client.getRegions(ctx)
	}); err != nil {
		return nil, err
	} else {
		return value.([]*Region), nil
	}
}

func (client *KodoClient) getRegions(ctx context.Context) ([]*Region, error) {
	var response struct {
		Regions []*Region `json:"regions"`
	}
	requestUrl := client.ucUrl.String() + "/regions"
	if request, err := http.NewRequest(http.MethodGet, requestUrl, http.NoBody); err != nil {
		return nil, fmt.Errorf("KodoClient.getRegions: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return nil, fmt.Errorf("KodoClient.getRegions: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return nil, fmt.Errorf("KodoClient.getRegions: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			if err = json.Unmarshal(bs, &response); err != nil {
				return nil, fmt.Errorf("KodoClient.getRegions: parse response body err: %w", err)
			} else {
				return response.Regions, nil
			}
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return nil, err
		} else if errBody != nil {
			return nil, errBody
		} else {
			return nil, fmt.Errorf("KodoClient.getRegions: invalid status code: %s", resp.Status)
		}
	}
}

func (client *KodoClient) FindBucketByName(ctx context.Context, bucketName string, useCache bool) (*Bucket, error) {
	if useCache {
		cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-bucketName-%s", client.accessKey, client.secretKey, client.ucUrl, bucketName)
		if value, err := getCacheValueByKey(cacheKey, 24*time.Hour, func() (interface{}, error) {
			return client.findBucketByName(ctx, bucketName, true)
		}); err != nil {
			return nil, err
		} else {
			return value.(*Bucket), nil
		}
	} else if value, err := client.findBucketByName(ctx, bucketName, false); err != nil {
		return nil, err
	} else {
		return value, nil
	}
}

func (client *KodoClient) findBucketByName(ctx context.Context, bucketName string, useCache bool) (*Bucket, error) {
	var (
		buckets []*Bucket
		err     error
	)

	if useCache {
		buckets, err = client.GetBuckets(ctx)
	} else {
		buckets, err = client.getBuckets(ctx)
	}

	if err != nil {
		return nil, err
	} else {
		for _, bucket := range buckets {
			if bucket.Name == bucketName {
				return bucket, nil
			}
		}
		return nil, nil
	}
}

func (client *KodoClient) GetBuckets(ctx context.Context) ([]*Bucket, error) {
	cacheKey := fmt.Sprintf("cacheKey-%s-%s-%s-buckets", client.accessKey, client.secretKey, client.ucUrl)
	if value, err := getCacheValueByKey(cacheKey, 1*time.Second, func() (interface{}, error) {
		return client.getBuckets(ctx)
	}); err != nil {
		return nil, err
	} else {
		return value.([]*Bucket), nil
	}
}

func (client *KodoClient) getBuckets(ctx context.Context) ([]*Bucket, error) {
	var response []*Bucket
	requestUrl := client.ucUrl.String() + "/v2/buckets?shared=rd"
	if request, err := http.NewRequest(http.MethodGet, requestUrl, http.NoBody); err != nil {
		return nil, fmt.Errorf("KodoClient.getBuckets: create request err: %w", err)
	} else if resp, err := client.httpClient.Do(request.WithContext(ctx)); err != nil {
		return nil, fmt.Errorf("KodoClient.getBuckets: send request err: %w", err)
	} else {
		defer resp.Body.Close()
		if bs, err := io.ReadAll(resp.Body); err != nil {
			return nil, fmt.Errorf("KodoClient.getBuckets: read response err: %w", err)
		} else if resp.StatusCode == http.StatusOK {
			if err = json.Unmarshal(bs, &response); err != nil {
				return nil, fmt.Errorf("KodoClient.getBuckets: parse response body err: %w", err)
			} else {
				return response, nil
			}
		} else if errBody, err := parseKodoErrorFromResponseBody(bs); err != nil {
			return nil, err
		} else if errBody != nil {
			return nil, errBody
		} else {
			return nil, fmt.Errorf("KodoClient.getBuckets: invalid status code: %s", resp.Status)
		}
	}
}

func getCacheValueByKey(cacheKey string, cacheTtl time.Duration, fn func() (interface{}, error)) (interface{}, error) {
	type CacheValue struct {
		value    interface{}
		deadline time.Time
	}

	if cacheValue, ok := cacheMap.Load(cacheKey); ok && time.Now().Before(cacheValue.(CacheValue).deadline) {
		return cacheValue.(CacheValue).value, nil
	} else if value, err, _ := singleflightGroup.Do(cacheKey, func() (interface{}, error) {
		if cacheValue, ok := cacheMap.Load(cacheKey); ok && time.Now().Before(cacheValue.(CacheValue).deadline) {
			return cacheValue.(CacheValue).value, nil
		} else {
			if value, err := fn(); err != nil {
				return nil, err
			} else {
				cacheMap.Store(cacheKey, CacheValue{value: value, deadline: time.Now().Add(cacheTtl)})
				return value, nil
			}
		}
	}); err != nil {
		return nil, err
	} else {
		return value, nil
	}
}

type KodoErrorResponseBody struct {
	Message string `json:"error"`
}

func (err *KodoErrorResponseBody) Error() string {
	return err.Message
}

func parseKodoErrorFromResponseBody(respBody []byte) (*KodoErrorResponseBody, error) {
	var body KodoErrorResponseBody
	if err := json.Unmarshal(respBody, &body); err != nil {
		return nil, fmt.Errorf("parseKodoErrorFromResponseBody: failed to parse response body as json: %w, body: %s", err, respBody)
	} else if body.Message != "" {
		return &body, nil
	} else {
		return nil, nil
	}
}
