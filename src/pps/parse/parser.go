package parse

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/pachyderm/pachyderm/src/log"
	"github.com/pachyderm/pachyderm/src/pps"
)

var (
	versionToParseFunc = map[string]func(string, string, *config) (*pps.Pipeline, error){
		"v1": parsePipelineV1,
	}
)

type parser struct{}

func newParser() *parser {
	return &parser{}
}

func (p *parser) ParsePipeline(dirPath string, contextDirPath string) (*pps.Pipeline, error) {
	return parsePipeline(dirPath, contextDirPath)
}

type config struct {
	Version string
	Include []string
	Exclude []string
}

func parsePipeline(dirPath string, contextDirPath string) (*pps.Pipeline, error) {
	dirPath = filepath.Clean(dirPath)
	config, err := parseConfig(dirPath, contextDirPath)
	if err != nil {
		return nil, err
	}
	if config.Version == "" {
		return nil, fmt.Errorf("no version specified in pps.yml")
	}
	parseFunc, ok := versionToParseFunc[config.Version]
	if !ok {
		return nil, fmt.Errorf("unknown pps specification version: %s", config.Version)
	}
	return parseFunc(dirPath, contextDirPath, config)
}

func parseConfig(dirPath string, contextDirPath string) (*config, error) {
	configFilePath := filepath.Join(dirPath, contextDirPath, "pps.yml")
	if err := checkFileExists(configFilePath); err != nil {
		return nil, err
	}
	data, err := ioutil.ReadFile(configFilePath)
	if err != nil {
		return nil, err
	}
	config := &config{}
	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, err
	}
	return config, nil
}

func parsePipelineV1(dirPath string, contextDirPath string, config *config) (*pps.Pipeline, error) {
	filePaths, err := getAllFilePaths(dirPath, contextDirPath, config.Include, config.Exclude)
	if err != nil {
		return nil, err
	}
	pipeline := &pps.Pipeline{
		NameToElement: make(map[string]*pps.Element),
	}
	for _, filePath := range filePaths {
		element, err := getElementForPipelineFile(dirPath, filePath)
		if err != nil {
			return nil, err
		}
		if _, ok := pipeline.NameToElement[element.Name]; ok {
			return nil, fmt.Errorf("duplicate element: %s", element.Name)
		}
		pipeline.NameToElement[element.Name] = element
	}
	log.Printf("got pipeline %v\n", pipeline)
	return pipeline, nil
}

func getAllFilePaths(dirPath string, contextDirPath string, includes []string, excludes []string) ([]string, error) {
	var filePaths []string
	if err := filepath.Walk(
		filepath.Join(dirPath, contextDirPath),
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				filePaths = append(filePaths, path)
			}
			return nil
		},
	); err != nil {
		return nil, err
	}
	relFilePaths := make([]string, len(filePaths))
	for i, filePath := range filePaths {
		relFilePath, err := filepath.Rel(dirPath, filePath)
		if err != nil {
			return nil, err
		}
		relFilePaths[i] = relFilePath
	}
	var filteredRelFilePaths []string
	for _, relFilePath := range relFilePaths {
		isPipelineFile, err := isPipelineFile(relFilePath, contextDirPath, includes, excludes)
		if err != nil {
			return nil, err
		}
		if isPipelineFile {
			filteredRelFilePaths = append(filteredRelFilePaths, relFilePath)
		}
	}
	return filteredRelFilePaths, nil
}

func isPipelineFile(filePath string, contextDirPath string, includes []string, excludes []string) (bool, error) {
	var err error
	if contextDirPath != "" {
		filePath, err = filepath.Rel(contextDirPath, filePath)
		if err != nil {
			return false, err
		}
	}
	isPipelineFileIncluded, err := isPipelineFileIncluded(filePath, includes)
	if err != nil {
		return false, err
	}
	isPipelineFileExcluded, err := isPipelineFileExcluded(filePath, excludes)
	if err != nil {
		return false, err
	}
	return isPipelineFileIncluded && !isPipelineFileExcluded, nil
}

func isPipelineFileIncluded(filePath string, includes []string) (bool, error) {
	if !strings.HasSuffix(filePath, ".yml") {
		return false, nil
	}
	if filePath == "pps.yml" {
		return false, nil
	}
	if len(includes) == 0 {
		return true, nil
	}
	for _, include := range includes {
		matched, err := matches(include, filePath)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func isPipelineFileExcluded(filePath string, excludes []string) (bool, error) {
	if !strings.HasSuffix(filePath, ".yml") {
		return true, nil
	}
	for _, exclude := range excludes {
		matched, err := matches(exclude, filePath)
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func matches(match string, filePath string) (bool, error) {
	if strings.HasPrefix(filePath, match) {
		return true, nil
	}
	matched, err := filepath.Match(match, filePath)
	if err != nil {
		return false, err
	}
	return matched, nil
}

func getElementForPipelineFile(dirPath string, relFilePath string) (*pps.Element, error) {
	filePath := filepath.Join(dirPath, relFilePath)
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	m := make(map[interface{}]interface{})
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	ppsMetaObj, ok := m["pps"]
	if !ok {
		return nil, fmt.Errorf("no pps section for %s", relFilePath)
	}
	ppsMeta := ppsMetaObj.(map[interface{}]interface{})
	if ppsMeta["kind"] == "" {
		return nil, fmt.Errorf("no kind specified for %s", relFilePath)
	}
	nameObj, ok := ppsMeta["name"]
	if !ok {
		return nil, fmt.Errorf("no name specified for %s", relFilePath)
	}
	name := strings.TrimSpace(nameObj.(string))
	if name == "" {
		return nil, fmt.Errorf("no name specified for %s", relFilePath)
	}
	element := &pps.Element{
		Name: name,
	}
	kindObj, ok := ppsMeta["kind"]
	if !ok {
		return nil, fmt.Errorf("no kind specified for %s", relFilePath)
	}
	kind := strings.TrimSpace(kindObj.(string))
	switch kind {
	case "node":
		node := &pps.Node{}
		if err := yaml.Unmarshal(data, node); err != nil {
			return nil, err
		}
		element.Node = node
	case "docker_service":
		dockerService := &pps.DockerService{}
		if err := yaml.Unmarshal(data, dockerService); err != nil {
			return nil, err
		}
		if dockerService.Build != "" {
			dockerService.Build = filepath.Clean(filepath.Join(filepath.Dir(relFilePath), dockerService.Build))
		}
		element.DockerService = dockerService
	default:
		return nil, fmt.Errorf("unknown kind %s for %s", kind, relFilePath)
	}
	return element, nil
}

func checkFileExists(path string) error {
	_, err := os.Stat(path)
	return err
}
