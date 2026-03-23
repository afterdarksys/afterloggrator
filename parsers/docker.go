package parsers

import (
	"encoding/json"
	"strings"
)

type DockerEngine struct{}

func NewDockerEngine() *DockerEngine {
	return &DockerEngine{}
}

func (d *DockerEngine) Parse(line string) (string, map[string]string) {
	if !strings.HasPrefix(strings.TrimSpace(line), "{") {
		return "", nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal([]byte(line), &data); err == nil {
		// Native docker json-file driver standard
		logData, hasLog := data["log"].(string)
		stream, hasStream := data["stream"].(string)
		
		if hasLog && hasStream {
			parsed := make(map[string]string)
			parsed["stream"] = stream
			// We expose the raw logged message
			parsed["docker_msg"] = strings.TrimSpace(logData)
			return "docker-container", parsed
		}
	}
	return "", nil
}
