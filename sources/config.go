// Package sources reads logs from explicitly configured local and remote sources.
package sources

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
)

type Source struct {
	Name          string            `json:"name"`
	Type          string            `json:"type"`
	Path          string            `json:"path,omitempty"`
	Host          string            `json:"host,omitempty"`
	Service       string            `json:"service,omitempty"`
	Container     string            `json:"container,omitempty"`
	Cluster       string            `json:"cluster,omitempty"`
	Labels        map[string]string `json:"labels,omitempty"`
	URL           string            `json:"url,omitempty"`
	Method        string            `json:"method,omitempty"`
	Body          map[string]any    `json:"body,omitempty"`
	HeadersEnv    map[string]string `json:"headers_env,omitempty"`
	TokenEnv      string            `json:"token_env,omitempty"`
	APIKeyEnv     string            `json:"api_key_env,omitempty"`
	APIKeyHeader  string            `json:"api_key_header,omitempty"`
	RecordsPath   string            `json:"records_path,omitempty"`
	NextTokenPath string            `json:"next_token_path,omitempty"`
	PageParam     string            `json:"page_param,omitempty"`
	Format        string            `json:"format,omitempty"`
	Region        string            `json:"region,omitempty"`
	Profile       string            `json:"profile,omitempty"`
	Resource      string            `json:"resource,omitempty"`
	Query         string            `json:"query,omitempty"`
	User          string            `json:"user,omitempty"`
	KeyFile       string            `json:"key_file,omitempty"`
	KnownHosts    string            `json:"known_hosts,omitempty"`
	Context       string            `json:"context,omitempty"`
	Namespace     string            `json:"namespace,omitempty"`
	Pod           string            `json:"pod,omitempty"`
	OCIConfig     string            `json:"oci_config,omitempty"`
}

func Load(path string) ([]Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cfg struct {
		Sources []Source `json:"sources"`
	}
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(&cfg); err != nil {
		return nil, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("sources config must contain exactly one JSON document")
	}
	return cfg.Sources, nil
}
func Validate(list []Source) error {
	names := map[string]bool{}
	for _, s := range list {
		if s.Name == "" || names[s.Name] {
			return fmt.Errorf("source names must be nonempty and unique: %q", s.Name)
		}
		names[s.Name] = true
		required := func(ok bool, what string) error {
			if !ok {
				return fmt.Errorf("source %s: %s required", s.Name, what)
			}
			return nil
		}
		var err error
		switch s.Type {
		case "file":
			err = required(s.Path != "", "path")
		case "ssh":
			err = required(s.Path != "" && s.Host != "" && s.User != "" && s.KeyFile != "" && s.KnownHosts != "", "path, host, user, key_file, known_hosts")
		case "docker":
			err = required(s.Container != "", "container")
		case "kubernetes":
			err = required(s.Context != "" && s.Namespace != "" && s.Pod != "" && s.Container != "", "context, namespace, pod, container")
		case "aws":
			err = required(s.Region != "" && s.Resource != "", "region and resource (log group)")
		case "gcp":
			err = required(s.Resource != "" && s.TokenEnv != "", "resource (projects/ID) and token_env")
		case "azure":
			err = required(s.Resource != "" && s.Query != "" && s.TokenEnv != "", "resource (workspace ID), query and token_env")
		case "oci":
			err = required(s.Region != "" && s.Query != "", "region and query")
		case "https", "darkapi":
			err = required(s.URL != "", "url")
		default:
			return fmt.Errorf("source %s: unknown type %q", s.Name, s.Type)
		}
		if err != nil {
			return err
		}
		if s.URL != "" {
			u, e := url.Parse(s.URL)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
				return fmt.Errorf("source %s: URL must be HTTPS without userinfo or fragment", s.Name)
			}
		}
		if s.Method != "" && s.Method != "GET" && s.Method != "POST" {
			return fmt.Errorf("source %s: method must be GET or POST", s.Name)
		}
		if s.Format != "" && s.Format != "json" && s.Format != "ndjson" && s.Format != "text" {
			return fmt.Errorf("source %s: invalid format", s.Name)
		}
		if (s.NextTokenPath != "") != (s.PageParam != "") {
			return fmt.Errorf("source %s: next_token_path and page_param must be used together", s.Name)
		}
		for _, env := range []string{s.TokenEnv, s.APIKeyEnv} {
			if env != "" && os.Getenv(env) == "" {
				return fmt.Errorf("source %s: environment variable %s is empty", s.Name, env)
			}
		}
		for _, env := range s.HeadersEnv {
			if env == "" || os.Getenv(env) == "" {
				return fmt.Errorf("source %s: header environment variable %s is empty", s.Name, env)
			}
		}
	}
	return nil
}
