package utils

import (
	"encoding/json"
	"fmt"

	wranglecorev1 "github.com/rancher/wrangler/v3/pkg/generated/controllers/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetNADInterfaceMapping retrieves and parses the NAD→interface mapping from the
// harvester-nad-mapping ConfigMap in kube-system.
//
// Return values:
//   - (nil, nil):              the ConfigMap does not exist; callers should treat this as
//                              "no mapping available" and pass through.
//   - (map[string]string{}, nil): the ConfigMap exists but the data key is absent, empty,
//                              or the JSON object has no entries; callers decide whether
//                              this is an error.
//   - (map[string]string{…}, nil): the successfully parsed mapping.
//   - (nil, err):              a Get or JSON-unmarshal error occurred.
func GetNADInterfaceMapping(cache wranglecorev1.ConfigMapCache) (map[string]string, error) {
	cm, err := cache.Get(metav1.NamespaceSystem, ConfigMapNADMapping)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get NAD mapping ConfigMap: %w", err)
	}

	mappingStr := cm.Data[ConfigMapKeyNADMapping]
	if mappingStr == "" {
		return map[string]string{}, nil
	}

	var mapping map[string]string
	if err := json.Unmarshal([]byte(mappingStr), &mapping); err != nil {
		return nil, fmt.Errorf("invalid %s in ConfigMap %s/%s: %w", ConfigMapKeyNADMapping, cm.Namespace, cm.Name, err)
	}

	return mapping, nil
}
