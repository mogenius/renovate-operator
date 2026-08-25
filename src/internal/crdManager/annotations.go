package crdmanager

import (
	"context"
	"fmt"

	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// AddAnnotation sets key=value on obj and persists the change with a merge patch.
func AddAnnotation(ctx context.Context, c crclient.Client, obj crclient.Object, key, value string) error {
	patch := crclient.MergeFrom(obj.DeepCopyObject().(crclient.Object))
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	obj.SetAnnotations(annotations)
	if err := c.Patch(ctx, obj, patch); err != nil {
		return fmt.Errorf("adding annotation %s: %w", key, err)
	}
	return nil
}

// RemoveAnnotation deletes one or more annotation keys from obj and persists
// the change with a single merge patch. It is a no-op when none of the keys exist.
func RemoveAnnotation(ctx context.Context, c crclient.Client, obj crclient.Object, keys ...string) error {
	annotations := obj.GetAnnotations()
	present := make([]string, 0, len(keys))
	for _, k := range keys {
		if _, ok := annotations[k]; ok {
			present = append(present, k)
		}
	}
	if len(present) == 0 {
		return nil
	}
	patch := crclient.MergeFrom(obj.DeepCopyObject().(crclient.Object))
	for _, k := range present {
		delete(annotations, k)
	}
	obj.SetAnnotations(annotations)
	if err := c.Patch(ctx, obj, patch); err != nil {
		return fmt.Errorf("removing annotation(s): %w", err)
	}
	return nil
}
