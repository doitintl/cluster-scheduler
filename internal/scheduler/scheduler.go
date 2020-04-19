package scheduler

import "context"

const (
	// cluster scheduler labels
	ENABLED_LABEL = "cs-enabled"
	UPTIME_LABEL  = "cs-uptime"
	STATUS_LABEL  = "cs-status"
	// cluster scheduler status values
	STATUS_DOWN = "down"
	STATUS_UP   = "up"
)

type NodeGroup struct {
	Name         string
	NodeCount    int32
	MinNodeCount int32
	MaxNodeCount int32
	Autoscaling  bool
}

type Cluster struct {
	Name     string
	Location string //region or zone
	Project  string
	Status   string
	Uptime   UptimeRange
	Nodes    []NodeGroup
	// GKE specific
	Labels      map[string]string
	Fingerprint string
}

type Runner interface {
	List(context.Context) ([]Cluster, error)
	Stop(context.Context, Cluster) error
	Restart(context.Context, Cluster) error
	DecideOnStatus(Cluster) error
}
