package sources

import (
	"afterloggrator/parsers"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/loggingsearch"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func awsLogs(ctx context.Context, s Source, opt Options, emit Emit) error {
	options := []func(*config.LoadOptions) error{config.WithRegion(s.Region)}
	if s.Profile != "" {
		options = append(options, config.WithSharedConfigProfile(s.Profile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return err
	}
	client := cloudwatchlogs.NewFromConfig(cfg, func(o *cloudwatchlogs.Options) {
		o.HTTPClient = &http.Client{Timeout: 60 * time.Second, CheckRedirect: denyRedirect}
		if s.URL != "" {
			o.BaseEndpoint = aws.String(s.URL)
		}
	})
	return queryAWS(ctx, client, s, opt, emit)
}

type awsLogClient interface {
	FilterLogEvents(context.Context, *cloudwatchlogs.FilterLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
}

func queryAWS(ctx context.Context, client awsLogClient, s Source, opt Options, emit Emit) error {
	req := &cloudwatchlogs.FilterLogEventsInput{LogGroupName: aws.String(s.Resource)}
	if s.Query != "" {
		req.FilterPattern = aws.String(s.Query)
	}
	if !opt.Since.IsZero() {
		req.StartTime = aws.Int64(opt.Since.UnixMilli())
	}
	if !opt.Until.IsZero() {
		req.EndTime = aws.Int64(opt.Until.UnixMilli())
	}
	seen := map[string]bool{}
	for {
		resp, err := client.FilterLogEvents(ctx, req)
		if err != nil {
			return err
		}
		for _, event := range resp.Events {
			e := parsers.LogEntry{Line: aws.ToString(event.Message), ParsedFields: map[string]string{"log_stream": aws.ToString(event.LogStreamName), "event_id": aws.ToString(event.EventId)}}
			if event.Timestamp != nil {
				e.Timestamp = time.UnixMilli(*event.Timestamp).UTC()
			}
			if err = emit(e); err != nil {
				return err
			}
		}
		token := aws.ToString(resp.NextToken)
		if token == "" {
			return nil
		}
		if seen[token] {
			return fmt.Errorf("AWS returned a repeated pagination token")
		}
		seen[token] = true
		req.NextToken = resp.NextToken
	}
}
func ociLogs(ctx context.Context, s Source, opt Options, emit Emit) error {
	if opt.Since.IsZero() || opt.Until.IsZero() {
		return fmt.Errorf("OCI requires -since and -until")
	}
	var provider common.ConfigurationProvider = common.DefaultConfigProvider()
	if s.OCIConfig != "" || s.Profile != "" {
		path := s.OCIConfig
		if path == "" {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, ".oci", "config")
		}
		profile := s.Profile
		if profile == "" {
			profile = "DEFAULT"
		}
		var err error
		provider, err = common.ConfigurationProviderFromFileWithProfile(path, profile, "")
		if err != nil {
			return err
		}
	}
	client, err := loggingsearch.NewLogSearchClientWithConfigurationProvider(provider)
	if err != nil {
		return err
	}
	client.SetRegion(s.Region)
	if s.URL != "" {
		client.Host = s.URL
	}
	client.HTTPClient = &http.Client{Timeout: 60 * time.Second, CheckRedirect: denyRedirect}
	return queryOCI(ctx, client, s, opt, emit)
}

type ociLogClient interface {
	SearchLogs(context.Context, loggingsearch.SearchLogsRequest) (loggingsearch.SearchLogsResponse, error)
}

func queryOCI(ctx context.Context, client ociLogClient, s Source, opt Options, emit Emit) error {
	req := loggingsearch.SearchLogsRequest{SearchLogsDetails: loggingsearch.SearchLogsDetails{SearchQuery: &s.Query, TimeStart: &common.SDKTime{Time: opt.Since}, TimeEnd: &common.SDKTime{Time: opt.Until}}, Limit: common.Int(1000)}
	seen := map[string]bool{}
	for {
		resp, err := client.SearchLogs(ctx, req)
		if err != nil {
			return err
		}
		for _, result := range resp.Results {
			if result.Data == nil {
				return fmt.Errorf("OCI result missing data")
			}
			b, err := json.Marshal(*result.Data)
			if err != nil {
				return err
			}
			if err = emit(parsers.LogEntry{Line: string(b)}); err != nil {
				return err
			}
		}
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			return nil
		}
		token := *resp.OpcNextPage
		if seen[token] {
			return fmt.Errorf("OCI returned repeated page token")
		}
		seen[token] = true
		req.Page = resp.OpcNextPage
	}
}
