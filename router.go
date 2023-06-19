package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/akerl/go-lambda/apigw/events"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type metric struct {
	Name  string            `json:"name"`
	Type  string            `json:"type"`
	Tags  map[string]string `json:"tags"`
	Value string            `json:"value"`
}

type metricFile struct {
	FileName string   `json:"name"`
	Metrics  []metric `json:"metrics"`
}

var textRegex = regexp.MustCompile(`^[\w\-/]+$`)
var valueRegex = regexp.MustCompile(`^\d+(.\+)?$`)

func (m *metric) String() string {
	return fmt.Sprintf(
		"# TYPE %s %s\n%s%s %s\n\n",
		m.Name,
		m.Type,
		m.Name,
		m.TagString(),
		m.Value,
	)
}

func (m *metric) TagString() string {
	if len(m.Tags) == 0 {
		return ""
	}
	tagStrings := []string{}
	for k, v := range m.Tags {
		tagStrings = append(tagStrings, fmt.Sprintf("%s=\"%s\"", k, v))
	}
	return fmt.Sprintf("{%s}", strings.Join(tagStrings, ","))
}

func (m *metric) Validate() bool {
	if !textRegex.MatchString(m.Name) {
		return false
	}
	if !textRegex.MatchString(m.Type) {
		return false
	}
	if !valueRegex.MatchString(m.Value) {
		return false
	}
	for k, v := range m.Tags {
		if !textRegex.MatchString(k) {
			return false
		}
		if !textRegex.MatchString(v) {
			return false
		}
	}
	return true
}

func (mf *metricFile) String() string {
	var sb strings.Builder
	for _, x := range mf.Metrics {
		sb.WriteString(x.String())
	}
	return sb.String()
}

func (mf *metricFile) Validate() bool {
	if mf.FileName == "" {
		return false
	}
	for _, x := range mf.Metrics {
		if !x.Validate() {
			return false
		}
	}
	return true
}

func metricAuth(req events.Request) (events.Response, error) {
	auth := req.Headers["Authorization"]

	if !strings.HasPrefix(auth, "Bearer ") {
		return events.Reject("no auth token")
	}

	token := auth[7:]
	if subtle.ConstantTimeCompare([]byte(token), []byte(c.AuthToken)) != 1 {
		return events.Reject("bad auth token")
	}
	return events.Response{}, nil
}

func metricHandler(req events.Request) (events.Response, error) {
	body, err := req.DecodedBody()
	if err != nil {
		return events.Fail(fmt.Sprintf("failed to decode: %s", err))
	}

	var mf metricFile
	err = json.Unmarshal([]byte(body), &mf)
	if err != nil {
		return events.Fail(fmt.Sprintf("failed to unmarshal: %s", err))
	}

	if !mf.Validate() {
		return events.Fail("failed validation")
	}

	content, err := json.Marshal(mf)
	if err != nil {
		return events.Fail(fmt.Sprintf("failed to marshal: %s", err))
	}

	client, err := getClient()
	if err != nil {
		return events.Fail(fmt.Sprintf("failed to load client: %s", err))
	}

	_, err = client.PutObject(context.TODO(), &s3.PutObjectInput{
		Bucket: &c.MetricBucket,
		Key:    &mf.FileName,
		Body:   bytes.NewReader(content),
	})
	if err != nil {
		return events.Fail(fmt.Sprintf("failed to write: %s", err))
	}
	return events.Succeed("")
}

func indexHandler(_ events.Request) (events.Response, error) {
	client, err := getClient()
	if err != nil {
		return events.Fail(fmt.Sprintf("failed to load client: %s", err))
	}

	allMetrics, err := readMetrics(client)
	if err != nil {
		return events.Fail(fmt.Sprintf("failed to read metrics: %s", err))
	}

	return events.Succeed(allMetrics.String())
}

func getClient() (*s3.Client, error) {
	cfg, err := awsConfig.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}

	return s3.NewFromConfig(cfg), nil
}

func readMetrics(client *s3.Client) (metricFile, error) {
	files, err := listMetricFiles(client)
	if err != nil {
		return metricFile{}, err
	}

	allMetrics := metricFile{FileName: "__all__"}
	for _, f := range files {
		mf, err := readMetricFile(client, f)
		if err != nil {
			return metricFile{}, err
		}
		allMetrics.Metrics = append(allMetrics.Metrics, mf.Metrics...)
	}
	return allMetrics, nil
}

func readMetricFile(client *s3.Client, f string) (metricFile, error) {
	input := &s3.GetObjectInput{
		Bucket: &c.MetricBucket,
		Key:    &f,
	}

	result, err := client.GetObject(context.TODO(), input)
	if err != nil {
		return metricFile{}, err
	}

	body, err := io.ReadAll(result.Body)
	if err != nil {
		return metricFile{}, err
	}

	var mf metricFile
	err = json.Unmarshal(body, &mf)
	if err != nil {
		return metricFile{}, err
	}

	if !mf.Validate() {
		return metricFile{}, fmt.Errorf("failed validation for %s", f)
	}
	return mf, nil
}

func listMetricFiles(client *s3.Client) ([]string, error) {
	paginator := s3.NewListObjectsV2Paginator(
		client,
		&s3.ListObjectsV2Input{Bucket: &c.MetricBucket},
	)
	metricFiles := []string{}

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(context.TODO())
		if err != nil {
			return []string{}, err
		}
		for _, obj := range page.Contents {
			metricFiles = append(metricFiles, *obj.Key)
		}
	}
	return metricFiles, nil
}
