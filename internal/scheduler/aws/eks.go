package aws

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/doitintl/cluster-scheduler/internal/scheduler"
	"github.com/pkg/errors"
)

type EksScheduler struct {
	eks *eks.Client
}

func NewEksScheduler(ctx context.Context) (scheduler.Runner, error) {
	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		return nil, errors.Wrap(err, "unable to load aws SDK config")
	}
	// Using the Config value, create the EKS client
	return &EksScheduler{eks.New(cfg)}, nil
}

func (e EksScheduler) List(ctx context.Context) ([]scheduler.Cluster, error) {
	// handle the 'refresh token' command
	cx, cancel := context.WithCancel(ctx)
	defer cancel()

	// get location (region)
	location := e.eks.Config.Region

	var clusters []scheduler.Cluster
	req := e.eks.ListClustersRequest(&eks.ListClustersInput{})
	p := eks.NewListClustersPaginator(req)
	for p.Next(cx) {
		page := p.CurrentPage()
		// get cluster details
		for _, name := range page.Clusters {
			info, err := e.eks.DescribeClusterRequest(&eks.DescribeClusterInput{Name: &name}).Send(cx)
			if err != nil {
				return nil, errors.Wrap(err, "failed to describe cluster")
			}
			// get EKS cluster tags
			tags := info.Cluster.Tags
			// skip cluster
			if tags[scheduler.ENABLED_LABEL] != "true" {
				continue
			}
			// prepare cluster record
			cluster := scheduler.Cluster{
				Name:     name,
				Location: location,
				Status:   tags[scheduler.STATUS_LABEL],
				Labels:   tags,
			}
			// get cluster uptime - time it is supposed to run
			uptime, err := scheduler.ParseUptime(tags[scheduler.UPTIME_LABEL])
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse cluster uptime range")
			}
			cluster.Uptime = *uptime
			// scan node groups

			// append cluster
			log.WithField("cluster", cluster).Debug("listing cluster")
			clusters = append(clusters, cluster)
		}
	}
	if err := p.Err(); err != nil {
		return nil, errors.Wrap(err, "failed to list EKS clusters")
	}

	return clusters, nil
}

func (e EksScheduler) Stop(ctx context.Context, cluster scheduler.Cluster) error {
	panic("implement me")
}

func (e EksScheduler) Restart(ctx context.Context, cluster scheduler.Cluster) error {
	panic("implement me")
}

func (e EksScheduler) DecideOnStatus(cluster scheduler.Cluster) error {
	if cluster.Uptime.IsInRange(time.Now()) {
		fmt.Println("пусть себе бежит...")
		return nil
	}
	fmt.Println("убить нах!")
	return nil
}
