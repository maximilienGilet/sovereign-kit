package clientprofile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// OMP 18.1.6 prefers .yml, then .yaml, and migrates legacy .json itself.
// Update the active existing file rather than silently hiding it with a new one.
func ompModelsPath(dir string) string {
	for _, name := range []string{"models.yml", "models.yaml", "models.json"} {
		path := filepath.Join(dir, name)
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			return path
		}
	}
	return filepath.Join(dir, "models.yml")
}

func yamlDocument(raw []byte) (*yaml.Node, error) {
	var doc yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("expected one YAML document")
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected YAML object")
	}
	// Aliases and merge keys can make one provider update affect another provider.
	// Fail closed rather than flattening or changing those relationships.
	var validate func(*yaml.Node) error
	validate = func(n *yaml.Node) error {
		if n.Kind == yaml.AliasNode || n.Tag == "!!merge" {
			return fmt.Errorf("YAML aliases and merge keys require manual provider configuration")
		}
		for _, child := range n.Content {
			if err := validate(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := validate(&doc); err != nil {
		return nil, err
	}
	var v map[string]any
	if err := doc.Decode(&v); err != nil {
		return nil, err
	}
	return &doc, nil
}

func readYAMLObject(raw []byte) (map[string]any, error) {
	doc, err := yamlDocument(raw)
	if err != nil {
		return nil, err
	}
	var value map[string]any
	if err = doc.Decode(&value); err != nil {
		return nil, err
	}
	// Match JSON numbers used by the common provider renderer and comparison.
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(normalized, &value)
	return value, err
}

func renderYAMLProvider(path string, value map[string]any) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		raw = []byte("{}")
	} else if err != nil {
		return nil, err
	}
	doc, err := yamlDocument(raw)
	if err != nil {
		return nil, err
	}
	providers := value["providers"].(map[string]any)
	patch := map[string]any{"providers": map[string]any{provider: providers[provider]}}
	encoded, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}
	var update yaml.Node
	if err = yaml.Unmarshal(encoded, &update); err != nil {
		return nil, err
	}
	mergeYAML(doc.Content[0], update.Content[0])
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err = encoder.Encode(doc); err != nil {
		return nil, err
	}
	if err = encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func mergeYAML(dest, patch *yaml.Node) {
	if dest.Kind == yaml.MappingNode && patch.Kind == yaml.MappingNode {
		for i := 0; i < len(patch.Content); i += 2 {
			key, value := patch.Content[i], patch.Content[i+1]
			found := false
			for j := 0; j < len(dest.Content); j += 2 {
				if dest.Content[j].Value == key.Value {
					mergeYAML(dest.Content[j+1], value)
					found = true
					break
				}
			}
			if !found {
				dest.Content = append(dest.Content, key, value)
			}
		}
		return
	}
	if dest.Kind == yaml.SequenceNode && patch.Kind == yaml.SequenceNode {
		for i, item := range patch.Content {
			if i < len(dest.Content) {
				mergeYAML(dest.Content[i], item)
			} else {
				dest.Content = append(dest.Content, item)
			}
		}
		return
	}
	head, line, foot := dest.HeadComment, dest.LineComment, dest.FootComment
	*dest = *patch
	dest.HeadComment, dest.LineComment, dest.FootComment = head, line, foot
}
