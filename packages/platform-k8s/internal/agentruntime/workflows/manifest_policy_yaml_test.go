package workflows

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/util/yaml"
)

type workflowDocument struct {
	path   string
	object map[string]any
}

func loadDeployWorkflowDocuments() ([]workflowDocument, error) {
	root := filepath.Clean(filepath.Join("..", "..", "..", "deploy", "k8s"))
	var documents []workflowDocument
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
		for {
			var object map[string]any
			if err := decoder.Decode(&object); err != nil {
				if err == io.EOF {
					return nil
				}
				return fmt.Errorf("decode %s: %w", path, err)
			}
			if len(object) != 0 {
				documents = append(documents, workflowDocument{path: path, object: object})
			}
		}
	})
	return documents, err
}

func objectName(object map[string]any) string {
	return stringField(mapField(object, "metadata"), "name")
}

func mapField(object map[string]any, field string) map[string]any {
	value, _ := object[field].(map[string]any)
	return value
}

func sliceField(object map[string]any, field string) []any {
	value, _ := object[field].([]any)
	return value
}

func stringField(object map[string]any, field string) string {
	value, _ := object[field].(string)
	return strings.TrimSpace(value)
}

func numberField(object map[string]any, field string) int64 {
	switch value := object[field].(type) {
	case int:
		return int64(value)
	case int64:
		return value
	case float64:
		return int64(value)
	default:
		return 0
	}
}

func hasField(object map[string]any, field string) bool {
	_, ok := object[field]
	return ok
}
