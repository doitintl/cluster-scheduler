package gke

import (
	"context"
	"fmt"
	"time"

	container "cloud.google.com/go/container/apiv1"
	"github.com/doitintl/cluster-scheduler/internal/scheduler"
	"golang.org/x/oauth2/google"

	log "github.com/sirupsen/logrus"
	containerpb "google.golang.org/genproto/googleapis/container/v1"

	"github.com/pkg/errors"
)

const (
	default_OPERATION_TIMEOUT = time.Minute * 15
	default_OPERATION_CHECK   = time.Second * 15
)

type GkeCluster struct {
	scheduler.Cluster
	// labels fingerprint - required to update GKE cluster labels
	Fingerprint string
}

type GkeScheduler struct {
	project string
	cm      *container.ClusterManagerClient
}

func NewGkeScheduler(ctx context.Context) (scheduler.Runner, error) {
	// handle the 'refresh token' command
	cx, cancel := context.WithCancel(ctx)
	defer cancel()

	creds, err := google.FindDefaultCredentials(cx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find default credentials")
	}

	cm, err := container.NewClusterManagerClient(cx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cluster manager client")
	}
	return &GkeScheduler{creds.ProjectID, cm}, nil
}

func (gke *GkeScheduler) List(ctx context.Context) ([]scheduler.Cluster, error) {
	// handle the 'refresh token' command
	cx, cancel := context.WithCancel(ctx)
	defer cancel()

	req := &containerpb.ListClustersRequest{
		Parent: fmt.Sprintf("projects/%s/locations/-", gke.project), // all regions and zones
	}
	resp, err := gke.cm.ListClusters(cx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to list clusters")
	}

	var clusters []scheduler.Cluster
	for _, r := range resp.Clusters {
		// skip cluster without cluster-scheduler ENABLED label == true
		if r.ResourceLabels[scheduler.ENABLED_LABEL] != "true" {
			continue
		}
		// get cluster details
		cluster := scheduler.Cluster{
			Name:        r.Name,
			Location:    r.Location,
			Project:     gke.project,
			Status:      r.ResourceLabels[scheduler.STATUS_LABEL],
			Labels:      r.ResourceLabels,
			Fingerprint: r.LabelFingerprint,
		}
		// get cluster uptime - time it is supposed to run
		uptime, err := scheduler.ParseUptime(r.ResourceLabels[scheduler.UPTIME_LABEL])
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse cluster uptime range")
		}
		cluster.Uptime = *uptime
		// scan node pools
		for _, np := range r.NodePools {
			group := scheduler.NodeGroup{
				Name:      np.Name,
				NodeCount: int(np.InitialNodeCount),
			}
			if np.Autoscaling != nil {
				group.Autoscaling = np.Autoscaling.Enabled
				group.MinNodeCount = int(np.Autoscaling.MinNodeCount)
				group.MaxNodeCount = int(np.Autoscaling.MaxNodeCount)
			}
			cluster.Nodes = append(cluster.Nodes, group)
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

// Stop node pool: disable autoscaling and resize to 0
func (gke *GkeScheduler) Stop(ctx context.Context, cluster scheduler.Cluster) error {
	log.WithFields(log.Fields{
		"cluster":  cluster.Name,
		"project":  cluster.Project,
		"location": cluster.Location,
	}).Info("stopping cluster")
	clusterName := fmt.Sprintf("projects/%s/locations/%s/clusters/%s",
		cluster.Project, cluster.Location, cluster.Name)
	// prepare node pool backup labels; create/update cluster labels
	labels := cluster.Labels
	for _, np := range cluster.Nodes {
		// backup node pool autoscaling and sizing as label
		npBackup := fmt.Sprintf("%t_%d_%d_%d", np.Autoscaling, np.NodeCount, np.MinNodeCount, np.MaxNodeCount)
		npLabel := fmt.Sprintf("cs-%s-size", np.Name)
		labels[npLabel] = npBackup
	}
	// update cluster scheduler labels
	if len(labels) > 0 {
		labels[scheduler.STATUS_LABEL] = scheduler.STATUS_DOWN
		reqStatusLabel := &containerpb.SetLabelsRequest{
			Name:             clusterName,
			ResourceLabels:   labels,
			LabelFingerprint: cluster.Fingerprint,
		}
		log.Debug("backup nodepools configuration as cluster labels")
		op, err := gke.cm.SetLabels(ctx, reqStatusLabel)
		if err != nil {
			return errors.Wrap(err, "failed to update cluster labels")
		}
		err = gke.waitForOperation(ctx, cluster.Project, cluster.Location, op)
		if err != nil {
			return errors.Wrap(err, "failed to complete 'SetLabels' operation")
		}
	}
	// update cluster node pools:
	// 1. disable autoscaling
	// 2. set size to 0
	for _, np := range cluster.Nodes {
		nodePoolName := fmt.Sprintf("%s/nodePools/%s", clusterName, np.Name)
		// set node pool autoscaling to false
		log.WithFields(log.Fields{
			"cluster":   cluster.Name,
			"node-pool": np.Name,
		}).Debug("disabling nodepool autoscaling")
		reqAutoscaling := &containerpb.SetNodePoolAutoscalingRequest{
			Name:        nodePoolName,
			Autoscaling: &containerpb.NodePoolAutoscaling{Enabled: false},
		}
		op, err := gke.cm.SetNodePoolAutoscaling(ctx, reqAutoscaling)
		if err != nil {
			return errors.Wrap(err, "failed to disable node pool autoscaling")
		}
		err = gke.waitForOperation(ctx, cluster.Project, cluster.Location, op)
		if err != nil {
			return errors.Wrap(err, "failed to complete 'SetNodePoolAutoscaling' operation")
		}
		// resize node pool size to 0
		log.WithFields(log.Fields{
			"cluster":   cluster.Name,
			"node-pool": np.Name,
		}).Debug("resizing nodepool size to 0")
		reqSize := &containerpb.SetNodePoolSizeRequest{
			Name:      nodePoolName,
			NodeCount: 0,
		}
		op, err = gke.cm.SetNodePoolSize(ctx, reqSize)
		if err != nil {
			return errors.Wrap(err, "failed to set node pool size to 0")
		}
		err = gke.waitForOperation(ctx, cluster.Project, cluster.Location, op)
		if err != nil {
			return errors.Wrap(err, "failed to complete 'SetNodePoolSize' operation")
		}
	}
	return nil
}

func (gke *GkeScheduler) Restart(ctx context.Context, cluster scheduler.Cluster) error {
	return nil
}

func (gke *GkeScheduler) DecideOnStatus(cluster scheduler.Cluster) error {
	if cluster.Uptime.IsInRange(time.Now()) {
		fmt.Println("пусть себе бежит...")
		return nil
	}
	fmt.Println("убить нах!")
	return nil
}

func (gke *GkeScheduler) waitForOperation(ctx context.Context, project, location string, op *containerpb.Operation) error {
	// check if operation is completed
	if op == nil || op.Status == containerpb.Operation_DONE {
		return nil
	}
	// wait for operation to be completed (or timeout/error)
	timer := time.NewTimer(default_OPERATION_TIMEOUT)
	done := make(chan int)
	errCh := make(chan error)
	go func(project, location, name string) {
		// periodically check operation status
		for range time.Tick(default_OPERATION_CHECK) {
			opReq := &containerpb.GetOperationRequest{
				Name: fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, location, name),
			}
			log.WithFields(log.Fields{}).Debug("get operation status")
			op, err := gke.cm.GetOperation(ctx, opReq)
			if err != nil {
				errCh <- err
				break
			}
			if op.Status == containerpb.Operation_DONE {
				done <- 1
				break
			}
		}
	}(project, location, op.Name)
	select {
	case <-done:
		// operation successfully completed
		log.WithField("operation", op).Debug("successfully completed operation")
		timer.Stop()
	case err := <-errCh:
		// error from error channel
		return errors.Wrap(err, "failed to get operation")
	case <-timer.C:
		// cancel operation on timeout
		cancelReq := &containerpb.CancelOperationRequest{
			Name: fmt.Sprintf("projects/%s/locations/%s/operations/%s", project, op.Location, op.Name),
		}
		log.WithField("operation", op).Debug("canceling operation request on timeout")
		err := gke.cm.CancelOperation(ctx, cancelReq)
		if err != nil {
			return errors.Wrap(err, "failed to cancel operation request")
		}
	}
	return nil
}
