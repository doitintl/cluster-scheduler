package scheduler

import (
	"fmt"

	"github.com/pkg/errors"
)

const (
	name_FORMAT   = "cs-%s-size"
	backup_FORMAT = "%t_%d_%d_%d"
)

type NodesBackup struct {
	Name  string
	Value string
}

func Backup(ng NodeGroup) NodesBackup {
	name := fmt.Sprintf(name_FORMAT, ng.Name)
	backup := fmt.Sprintf(backup_FORMAT, ng.Autoscaling, ng.NodeCount, ng.MinNodeCount, ng.MaxNodeCount)
	return NodesBackup{name, backup}
}

func GetBackupLabel(name string) string {
	return fmt.Sprintf(name_FORMAT, name)
}

func Restore(name, backup string) (*NodeGroup, error) {
	var autoscaling bool
	var count, min, max int32
	_, err := fmt.Sscanf(backup, backup_FORMAT, &autoscaling, &count, &min, &max)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read configuration node pool backup")
	}
	return &NodeGroup{
		Name:         name,
		NodeCount:    count,
		MinNodeCount: min,
		MaxNodeCount: max,
		Autoscaling:  autoscaling,
	}, nil
}
