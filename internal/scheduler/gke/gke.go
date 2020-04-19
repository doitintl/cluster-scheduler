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
				NodeCount: np.InitialNodeCount,
			}
			if np.Autoscaling != nil {
				group.Autoscaling = np.Autoscaling.Enabled
				group.MinNodeCount = np.Autoscaling.MinNodeCount
				group.MaxNodeCount = np.Autoscaling.MaxNodeCount
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
		"status":   cluster.Status,
	}).Info("stopping cluster")
	// check cluster status
	if cluster.Status == scheduler.STATUS_DOWN {
		log.Debug("ignore stopped cluster")
		return nil
	}
	// stop cluster
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
	log.WithFields(log.Fields{
		"cluster":  cluster.Name,
		"project":  cluster.Project,
		"location": cluster.Location,
		"status":   cluster.Status,
	}).Info("restarting cluster")
	// check cluster status
	if cluster.Status == scheduler.STATUS_UP {
		log.Debug("ignore already running cluster")
		return nil
	}
	// restart cluster
	clusterName := fmt.Sprintf("projects/%s/locations/%s/clusters/%s",
		cluster.Project, cluster.Location, cluster.Name)
	// update cluster node pools:
	// 1. restore autoscaling
	// 2. update nodepool size to min and max
	for _, np := range cluster.Nodes {
		nodePoolName := fmt.Sprintf("%s/nodePools/%s", clusterName, np.Name)
		upNodePool, err := scheduler.Restore(
			np.Name, cluster.Labels[scheduler.GetBackupLabel(np.Name)])
		if err != nil {
			return errors.Wrap(err, "failed to read backup from label")
		}
		// set node pool autoscaling to false
		log.WithFields(log.Fields{
			"cluster":   cluster.Name,
			"node-pool": np.Name,
		}).Debug("restoring nodepool autoscaling")
		reqAutoscaling := &containerpb.SetNodePoolAutoscalingRequest{
			Name: nodePoolName,
			Autoscaling: &containerpb.NodePoolAutoscaling{
				Enabled:      upNodePool.Autoscaling,
				MinNodeCount: upNodePool.MinNodeCount,
				MaxNodeCount: upNodePool.MaxNodeCount,
			},
		}
		op, err := gke.cm.SetNodePoolAutoscaling(ctx, reqAutoscaling)
		if err != nil {
			return errors.Wrap(err, "failed to restore node pool autoscaling")
		}
		err = gke.waitForOperation(ctx, cluster.Project, cluster.Location, op)
		if err != nil {
			return errors.Wrap(err, "failed to complete 'SetNodePoolAutoscaling' operation")
		}
		// resize node pool size to 0
		log.WithFields(log.Fields{
			"cluster":   cluster.Name,
			"node-pool": np.Name,
		}).Debug("restoring nodepool size")
		reqSize := &containerpb.SetNodePoolSizeRequest{
			Name:      nodePoolName,
			NodeCount: upNodePool.NodeCount,
		}
		op, err = gke.cm.SetNodePoolSize(ctx, reqSize)
		if err != nil {
			return errors.Wrap(err, "failed to restore node pool size")
		}
		err = gke.waitForOperation(ctx, cluster.Project, cluster.Location, op)
		if err != nil {
			return errors.Wrap(err, "failed to complete 'SetNodePoolSize' operation")
		}
	}
	// update cluster scheduler status label
	labels := cluster.Labels
	labels[scheduler.STATUS_LABEL] = scheduler.STATUS_UP
	reqStatusLabel := &containerpb.SetLabelsRequest{
		Name:             clusterName,
		ResourceLabels:   labels,
		LabelFingerprint: cluster.Fingerprint,
	}
	log.Debug("updating cluster scheduler status")
	op, err := gke.cm.SetLabels(ctx, reqStatusLabel)
	if err != nil {
		return errors.Wrap(err, "failed to update cluster scheduler status")
	}
	err = gke.waitForOperation(ctx, cluster.Project, cluster.Location, op)
	if err != nil {
		return errors.Wrap(err, "failed to complete 'SetLabels' operation")
	}
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
		// periodically check operation status (first tick after 10ms)
		ticker := time.NewTicker(default_OPERATION_CHECK)
		for ; true; <-ticker.C {
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
				ticker.Stop()
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
