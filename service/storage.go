package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/service/s3"
)

const (
	minStorageInterval = time.Second * 120
	maxStorageInterval = time.Second * 240

	expireTimeout = time.Minute * 15
)

type bytesReadCloser struct {
	*bytes.Reader
}

func (brc bytesReadCloser) Read(b []byte) (int, error) {
	return brc.Reader.Read(b)
}

func (bytesReadCloser) Close() (err error) {
	return
}

func randomStorageInterval() time.Duration {
	return randomDuration(minStorageInterval, maxStorageInterval)
}

func initStorage(local *localNode, remotes *remoteNodes, notify <-chan struct{}, reply chan<- []*net.UDPAddr, credData []byte, region, bucket, prefix string, dryRun bool, log *Log) (err error) {
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	localKey := prefix + local.ipAddr

	creds, err := parseCredentials(credData)
	if err != nil {
		return
	}

	var client *s3.S3

	if !dryRun {
		client = s3.New(&aws.Config{
			Credentials: creds,
			Region:      region,
		})
	}

	if err = updateStorage(local, client, bucket, localKey, log); err != nil {
		return
	}

	if err = scanStorage(local, remotes, reply, client, bucket, prefix, log); err != nil {
		return
	}

	go storageLoop(local, remotes, notify, reply, client, bucket, prefix, localKey, log)

	return
}

func storageLoop(local *localNode, remotes *remoteNodes, notify <-chan struct{}, reply chan<- []*net.UDPAddr, client *s3.S3, bucket, prefix, localKey string, log *Log) {
	timer := time.NewTimer(randomStorageInterval())

	for {
		var scan bool

		select {
		case <-notify:
			scan = false

		case <-timer.C:
			timer.Reset(randomStorageInterval())
			scan = true
		}

		if err := updateStorage(local, client, bucket, localKey, log); err != nil {
			log.Error(err)
		}

		if scan {
			if err := scanStorage(local, remotes, reply, client, bucket, prefix, log); err != nil {
				log.Error(err)
			}
		}
	}
}

func updateStorage(local *localNode, client *s3.S3, bucket, key string, log *Log) (err error) {
	log.Debug("updating S3")

	data, err := local.marshalForStorage()
	if err != nil {
		panic(err)
	}

	err = putObject(client, bucket, key, data, "application/json")
	return
}

func scanStorage(local *localNode, remotes *remoteNodes, reply chan<- []*net.UDPAddr, client *s3.S3, bucket, prefix string, log *Log) (err error) {
	log.Debug("scanning S3")

	objects := listObjects(client, bucket, prefix, log)
	if objects == nil {
		return
	}

	var loadKeys []*string
	var deleteKeys []*string

	expireThreshold := time.Now().Add(-expireTimeout)

	for object := range objects {
		ipAddr := (*object.Key)[len(prefix):]
		if ipAddr == "" || ipAddr == local.ipAddr {
			continue
		}

		if ip := net.ParseIP(ipAddr); ip == nil {
			log.Errorf("bad S3 key: %s", *object.Key)
			continue
		} else if !ip.IsGlobalUnicast() {
			log.Errorf("bad IP address in S3: %s", *object.Key)
			continue
		}

		if object.LastModified.After(expireThreshold) {
			if remotes.updatable(ipAddr, *object.LastModified) {
				loadKeys = append(loadKeys, object.Key)
			}
		} else {
			deleteKeys = append(deleteKeys, object.Key)
		}
	}

	for _, key := range deleteKeys {
		ipAddr := (*key)[len(prefix):]

		log.Infof("deleting %s from S3", ipAddr)

		if err := deleteObject(client, bucket, key); err != nil {
			log.Errorf("S3 DeleteObject: %s", err)
		}
	}

	var newAddrs []*net.UDPAddr

	for _, key := range loadKeys {
		ipAddr := (*key)[len(prefix):]

		log.Debugf("loading %s from S3", ipAddr)

		if output, err := getObject(client, bucket, key); err == nil {
			node := new(Node)
			err := json.NewDecoder(output.Body).Decode(node)
			output.Body.Close()
			if err != nil {
				log.Errorf("S3: %s: %s", ipAddr, err)
				continue
			}

			node.IPAddr = ipAddr
			node.TimeNs = output.LastModified.UnixNano()

			if newAddr := remotes.update(node, local, log); newAddr != nil {
				newAddrs = append(newAddrs, newAddr)
			}
		} else {
			log.Errorf("S3 GetObject: %s", err)
		}
	}

	if len(newAddrs) > 0 {
		reply <- newAddrs
	}

	remotes.expire(expireThreshold, local, log)

	return
}

func parseCredentials(data []byte) (creds aws.CredentialsProvider, err error) {
	var accessKey string
	var secretKey string

	if data != nil {
		fields := strings.Fields(strings.TrimSpace(string(data)))
		if len(fields) != 2 {
			err = errors.New("bad AWS credentials file format")
			return
		}

		accessKey = fields[0]
		secretKey = fields[1]
	}

	creds = aws.DetectCreds(accessKey, secretKey, "")
	return
}

func putObject(client *s3.S3, bucket string, key string, body []byte, contentType string) (err error) {
	contentLength := int64(len(body))

	request := &s3.PutObjectInput{
		Body:          bytesReadCloser{bytes.NewReader(body)},
		Bucket:        &bucket,
		ContentLength: &contentLength,
		ContentType:   &contentType,
		Key:           &key,
	}

	if client == nil {
		return
	}

	if _, err = client.PutObject(request); err != nil {
		err = fmt.Errorf("S3 PutObject: %s", err)
	}
	return
}

func getObject(client *s3.S3, bucket string, key *string) (output *s3.GetObjectOutput, err error) {
	request := &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    key,
	}

	return client.GetObject(request)
}

func deleteObject(client *s3.S3, bucket string, key *string) (err error) {
	request := &s3.DeleteObjectInput{
		Bucket: &bucket,
		Key:    key,
	}

	_, err = client.DeleteObject(request)
	return
}

func listObjects(client *s3.S3, bucket, prefix string, log *Log) (channel chan *s3.Object) {
	request := &s3.ListObjectsInput{
		Bucket: &bucket,
		Prefix: &prefix,
	}

	channel = make(chan *s3.Object)

	if client == nil {
		close(channel)
		return
	}

	output, err := client.ListObjects(request)
	if err != nil {
		log.Errorf("S3 ListObjects: %s", err)
		return
	}

	go func() {
		defer close(channel)

		for {
			for i := range output.Contents {
				object := output.Contents[i]
				channel <- object
				request.Marker = object.Key
			}

			if !*output.IsTruncated {
				break
			}

			if output, err = client.ListObjects(request); err != nil {
				log.Errorf("S3 ListObjects: %s", err)
				break
			}
		}
	}()

	return
}
